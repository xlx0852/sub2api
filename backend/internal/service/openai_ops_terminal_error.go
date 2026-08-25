package service

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// markOpenAIStreamTerminalError records a client-visible terminal error carried
// inside an otherwise successful HTTP/WS transport. OpenAI Responses streams
// commonly open with HTTP 200 (or WS 101) and only later emit response.failed
// or error, so status-based middleware cannot discover the failure by itself.
func (s *OpenAIGatewayService) markOpenAIStreamTerminalError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	payload []byte,
	message string,
) {
	if c == nil || len(payload) == 0 {
		return
	}
	// cyber_policy has a dedicated per-request/per-turn audit sink with billing
	// metadata. Do not create a second generic ops row for the same rejection.
	if hit, _, _ := detectOpenAICyberPolicy(payload); hit {
		return
	}

	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI upstream response failed"
	}
	semanticStatus := openAIStreamFailedEventSemanticStatus(payload, message)
	errType := openAIStreamTerminalErrorType(payload, semanticStatus)

	detail := ""
	if s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		detail = truncateString(openAIStreamTerminalErrorDetail(payload, message, semanticStatus), maxBytes)
	}

	MarkOpsStreamError(c, errType, message, semanticStatus)
	setOpsUpstreamError(c, semanticStatus, message, detail)
	event := OpsUpstreamErrorEvent{
		Platform:           PlatformOpenAI,
		UpstreamStatusCode: semanticStatus,
		UpstreamRequestID:  strings.TrimSpace(upstreamRequestID),
		Passthrough:        passthrough,
		Kind:               "terminal_error",
		Message:            message,
		Detail:             detail,
	}
	if account != nil {
		event.Platform = account.Platform
		event.AccountID = account.ID
		event.AccountName = account.Name
	}
	appendOpsUpstreamError(c, event)
}

// openAIStreamTerminalErrorDetail projects a terminal SSE frame onto the small
// set of fields needed for operations diagnostics. In particular, it must not
// persist response.instructions, output content, tools, or other customer data
// carried by verbose response.failed frames.
func openAIStreamTerminalErrorDetail(payload []byte, message string, semanticStatus int) string {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return ""
	}
	firstString := func(paths ...string) string {
		for _, path := range paths {
			if value := strings.TrimSpace(gjson.GetBytes(payload, path).String()); value != "" {
				return value
			}
		}
		return ""
	}
	detail := struct {
		EventType      string `json:"event_type,omitempty"`
		ResponseID     string `json:"response_id,omitempty"`
		ResponseStatus string `json:"response_status,omitempty"`
		ErrorCode      string `json:"error_code,omitempty"`
		ErrorType      string `json:"error_type,omitempty"`
		Message        string `json:"message,omitempty"`
		SemanticStatus int    `json:"semantic_status,omitempty"`
		SequenceNumber int64  `json:"sequence_number,omitempty"`
		InputTokens    int64  `json:"input_tokens,omitempty"`
		OutputTokens   int64  `json:"output_tokens,omitempty"`
		TotalTokens    int64  `json:"total_tokens,omitempty"`
	}{
		EventType:      firstString("type"),
		ResponseID:     firstString("response.id", "id"),
		ResponseStatus: firstString("response.status", "status"),
		ErrorCode:      firstString("response.error.code", "error.code", "code"),
		ErrorType:      firstString("response.error.type", "error.type"),
		Message:        sanitizeUpstreamErrorMessage(strings.TrimSpace(message)),
		SemanticStatus: semanticStatus,
		SequenceNumber: gjson.GetBytes(payload, "sequence_number").Int(),
		InputTokens:    gjson.GetBytes(payload, "response.usage.input_tokens").Int(),
		OutputTokens:   gjson.GetBytes(payload, "response.usage.output_tokens").Int(),
		TotalTokens:    gjson.GetBytes(payload, "response.usage.total_tokens").Int(),
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func openAIStreamTerminalErrorType(payload []byte, semanticStatus int) string {
	for _, path := range []string{"response.error.type", "error.type"} {
		if value := strings.TrimSpace(gjson.GetBytes(payload, path).String()); value != "" {
			return value
		}
	}
	switch semanticStatus {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "upstream_error"
	}
}
