package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAICapacityABDumpEnv            = "SUB2API_OPENAI_CAPACITY_AB_DUMP"
	openAICapacityABDumpDefaultOKEvery = 20
	openAICapacityABDumpBodyMaxBytes   = 2048
	openAICapacityABDumpGinKey         = "openai_capacity_ab_dumped"
	openAICapacityABDumpEntryKey       = "openai_capacity_ab_entry"
)

var openAICapacityABDumpOKSeq atomic.Uint64

// openAICapacityABDumpRecord is the redacted A/B sample written for capacity vs ok.
type openAICapacityABDumpRecord struct {
	TS             string            `json:"ts"`
	Label          string            `json:"label"`
	Entry          string            `json:"entry"`
	HTTPStatus     int               `json:"http_status"`
	SemanticStatus int               `json:"semantic_status,omitempty"`
	Model          string            `json:"model,omitempty"`
	AccountID      int64             `json:"account_id,omitempty"`
	AccountType    string            `json:"account_type,omitempty"`
	Originator     string            `json:"originator,omitempty"`
	UserAgent      string            `json:"user_agent,omitempty"`
	EventType      string            `json:"event_type,omitempty"`
	ErrorType      string            `json:"error_type,omitempty"`
	ErrorCode      string            `json:"error_code,omitempty"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	CapacityShed   bool              `json:"capacity_shed"`
	Headers        map[string]string `json:"headers,omitempty"`
	CodexSnapshot  map[string]any    `json:"codex_snapshot,omitempty"`
	AccountQuota   map[string]any    `json:"account_quota,omitempty"`
	TicketState    map[string]any    `json:"ticket_state,omitempty"`
	BodyExcerpt    string            `json:"body_excerpt,omitempty"`
}

type openAICapacityABDumpInput struct {
	Label          string
	Entry          string
	HTTPStatus     int
	SemanticStatus int
	Model          string
	EventType      string
	Message        string
	Body           []byte
	Headers        http.Header
	Account        *Account
	Client         *gin.Context
	Force          bool
}

func classifyOpenAICapacityABLabel(httpStatus int, message string, body []byte) string {
	if isOpenAIUpstreamCapacityShedEvent(body) {
		return "capacity"
	}
	if isOpenAITransientProcessingError(httpStatus, message, body) {
		return "capacity"
	}
	errMsg := strings.TrimSpace(message)
	if errMsg == "" {
		errMsg = firstNonEmptyJSONString(body, "response.error.message", "error.message", "message")
	}
	errCode := firstNonEmptyJSONString(body, "response.error.code", "error.code", "code")
	eventType := strings.TrimSpace(gjson.GetBytes(body, "type").String())
	combined := strings.ToLower(strings.TrimSpace(errMsg + " " + errCode + " " + eventType))
	if strings.Contains(combined, "selected model is at capacity") ||
		strings.Contains(combined, "server_is_overloaded") ||
		strings.Contains(combined, "slow_down") {
		return "capacity"
	}
	// HTTP 200 + 流内 error/response.failed 不是成功样本。
	if eventType == "error" || eventType == "response.failed" ||
		gjson.GetBytes(body, "error").Exists() ||
		gjson.GetBytes(body, "response.error").Exists() ||
		strings.TrimSpace(errMsg) != "" {
		return ""
	}
	if httpStatus > 0 && httpStatus < 400 {
		return "ok"
	}
	return ""
}

func openAICapacityABDumpDir(cfg *config.Config) string {
	if dir := strings.TrimSpace(os.Getenv(openAICapacityABDumpEnv)); dir != "" {
		return dir
	}
	if cfg != nil {
		return strings.TrimSpace(cfg.Gateway.OpenAICapacityABDumpDir)
	}
	return ""
}

func openAICapacityABDumpOKEvery(cfg *config.Config) int {
	if cfg == nil {
		return openAICapacityABDumpDefaultOKEvery
	}
	n := cfg.Gateway.OpenAICapacityABDumpOKEvery
	if n == 0 {
		return openAICapacityABDumpDefaultOKEvery
	}
	return n
}

func (s *OpenAIGatewayService) maybeDumpOpenAICapacityAB(in openAICapacityABDumpInput) {
	if s == nil {
		return
	}
	dir := openAICapacityABDumpDir(s.cfg)
	if dir == "" {
		return
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = classifyOpenAICapacityABLabel(in.HTTPStatus, in.Message, in.Body)
	}
	if label == "" {
		return
	}
	if in.Client != nil {
		if dumped, ok := in.Client.Get(openAICapacityABDumpGinKey); ok {
			if prev, _ := dumped.(string); prev != "" {
				return
			}
		}
		if override, ok := in.Client.Get(openAICapacityABDumpEntryKey); ok {
			if entry, _ := override.(string); strings.TrimSpace(entry) != "" {
				in.Entry = strings.TrimSpace(entry)
			}
		}
	}
	if label == "ok" && !in.Force {
		every := openAICapacityABDumpOKEvery(s.cfg)
		if every < 0 {
			return
		}
		n := openAICapacityABDumpOKSeq.Add(1)
		if n%uint64(every) != 0 {
			return
		}
	}

	msg := strings.TrimSpace(in.Message)
	if msg == "" {
		msg = extractOpenAISSEErrorMessage(in.Body)
	}
	if msg == "" {
		msg = sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(in.Body)))
	}
	eventType := strings.TrimSpace(in.EventType)
	if eventType == "" {
		eventType = strings.TrimSpace(gjson.GetBytes(in.Body, "type").String())
	}
	errCode := firstNonEmptyJSONString(in.Body, "response.error.code", "error.code", "code")
	errType := firstNonEmptyJSONString(in.Body, "response.error.type", "error.type", "type")
	if errType == eventType && (eventType == "response.failed" || eventType == "error") {
		errType = firstNonEmptyJSONString(in.Body, "response.error.type", "error.type")
	}
	semantic := in.SemanticStatus
	if semantic == 0 && label == "capacity" {
		semantic = openAIStreamFailedEventSemanticStatus(in.Body, msg)
	}

	record := openAICapacityABDumpRecord{
		TS:             time.Now().UTC().Format(time.RFC3339Nano),
		Label:          label,
		Entry:          strings.TrimSpace(in.Entry),
		HTTPStatus:     in.HTTPStatus,
		SemanticStatus: semantic,
		Model:          strings.TrimSpace(in.Model),
		EventType:      eventType,
		ErrorType:      errType,
		ErrorCode:      errCode,
		ErrorMessage:   truncateString(msg, 512),
		CapacityShed:   isOpenAIUpstreamCapacityShedEvent(in.Body) || label == "capacity",
		Headers:        pickOpenAICapacityABHeaders(in.Headers),
		CodexSnapshot:  openAICapacityABCodexSnapshot(in.Headers),
		AccountQuota:   openAICapacityABAccountQuota(in.Account),
		TicketState:    openAICapacityABTicketState(in.Account, in.Model),
		BodyExcerpt:    truncateString(openAICapacityABBodyExcerpt(in.Body, msg), openAICapacityABDumpBodyMaxBytes),
	}
	if in.Account != nil {
		record.AccountID = in.Account.ID
		record.AccountType = in.Account.Type
	}
	if in.Client != nil {
		if record.Originator == "" {
			record.Originator = strings.TrimSpace(in.Client.GetHeader("originator"))
		}
		if record.UserAgent == "" {
			record.UserAgent = truncateString(strings.TrimSpace(in.Client.GetHeader("User-Agent")), 160)
		}
		if record.Model == "" {
			record.Model = strings.TrimSpace(in.Client.GetHeader("x-sub2api-model"))
		}
	}
	if record.Originator == "" && in.Headers != nil {
		record.Originator = strings.TrimSpace(in.Headers.Get("originator"))
	}
	if record.UserAgent == "" && in.Headers != nil {
		record.UserAgent = truncateString(strings.TrimSpace(in.Headers.Get("User-Agent")), 160)
	}

	if err := writeOpenAICapacityABDump(dir, record); err != nil {
		return
	}
	if in.Client != nil {
		in.Client.Set(openAICapacityABDumpGinKey, label)
	}
}

func writeOpenAICapacityABDump(dir string, record openAICapacityABDumpRecord) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%s_%s_%d.json", record.Label, time.Now().UTC().Format("150405.000000"), time.Now().UnixNano()%1e6)
	return os.WriteFile(filepath.Join(dir, name), body, 0o600)
}

func pickOpenAICapacityABHeaders(headers http.Header) map[string]string {
	if headers == nil {
		return nil
	}
	keys := []string{
		"x-request-id",
		"cf-ray",
		"openai-processing-ms",
		"openai-version",
		"x-codex-primary-used-percent",
		"x-codex-primary-reset-after-seconds",
		"x-codex-primary-window-minutes",
		"x-codex-secondary-used-percent",
		"x-codex-secondary-reset-after-seconds",
		"x-codex-secondary-window-minutes",
		"x-codex-primary-over-secondary-limit-percent",
		"retry-after",
		"originator",
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if v := strings.TrimSpace(headers.Get(key)); v != "" {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func openAICapacityABCodexSnapshot(headers http.Header) map[string]any {
	snapshot := ParseCodexRateLimitHeaders(headers)
	if snapshot == nil {
		return nil
	}
	out := map[string]any{}
	if snapshot.PrimaryUsedPercent != nil {
		out["primary_used_percent"] = *snapshot.PrimaryUsedPercent
	}
	if snapshot.PrimaryResetAfterSeconds != nil {
		out["primary_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
	}
	if snapshot.PrimaryWindowMinutes != nil {
		out["primary_window_minutes"] = *snapshot.PrimaryWindowMinutes
	}
	if snapshot.SecondaryUsedPercent != nil {
		out["secondary_used_percent"] = *snapshot.SecondaryUsedPercent
	}
	if snapshot.SecondaryResetAfterSeconds != nil {
		out["secondary_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
	}
	if snapshot.SecondaryWindowMinutes != nil {
		out["secondary_window_minutes"] = *snapshot.SecondaryWindowMinutes
	}
	if snapshot.PrimaryOverSecondaryPercent != nil {
		out["primary_over_secondary_percent"] = *snapshot.PrimaryOverSecondaryPercent
	}
	if normalized := snapshot.Normalize(); normalized != nil {
		if normalized.Used5hPercent != nil {
			out["used_5h_percent"] = *normalized.Used5hPercent
		}
		if normalized.Used7dPercent != nil {
			out["used_7d_percent"] = *normalized.Used7dPercent
		}
		if normalized.Window5hMinutes != nil {
			out["window_5h_minutes"] = *normalized.Window5hMinutes
		}
		if normalized.Window7dMinutes != nil {
			out["window_7d_minutes"] = *normalized.Window7dMinutes
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func openAICapacityABAccountQuota(account *Account) map[string]any {
	if account == nil || account.Extra == nil {
		return nil
	}
	keys := []string{
		"codex_5h_used_percent",
		"codex_7d_used_percent",
		"codex_5h_window_minutes",
		"codex_7d_window_minutes",
		"codex_5h_reset_at",
		"codex_7d_reset_at",
		"codex_primary_used_percent",
		"codex_secondary_used_percent",
		"codex_usage_updated_at",
	}
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		if v, ok := account.Extra[key]; ok && v != nil {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// openAICapacityABTicketState 记录 overload 发生时的 turn-state 缓存状态：
// 有没有票 / 票龄 / 票签发的 model 是否与本次请求一致（桶不匹配时粘性无效）。
// 回答「带票还 overloaded 是哪种情况」。
func openAICapacityABTicketState(account *Account, reqModel string) map[string]any {
	if account == nil || account.Extra == nil {
		return nil
	}
	token, _ := account.Extra[openAICodexTurnStateExtraToken].(string)
	token = strings.TrimSpace(token)
	model, _ := account.Extra[openAICodexTurnStateExtraModel].(string)
	model = strings.TrimSpace(model)
	out := map[string]any{
		"has_ticket": token != "",
	}
	if token != "" {
		out["ticket_len"] = len(token)
		if m := strings.TrimSpace(model); m != "" {
			out["ticket_model"] = m
			if reqModel != "" {
				out["ticket_model_match"] = m == reqModel
			}
		}
		if captured := parseOpenAICodexTurnStateCapturedAt(account); !captured.IsZero() {
			out["ticket_age_seconds"] = int64(time.Since(captured).Seconds())
		}
	}
	return out
}

func openAICapacityABBodyExcerpt(body []byte, message string) string {
	if len(body) == 0 {
		return message
	}
	if !gjson.ValidBytes(body) {
		return truncateString(string(body), openAICapacityABDumpBodyMaxBytes)
	}
	excerpt := map[string]any{}
	if t := strings.TrimSpace(gjson.GetBytes(body, "type").String()); t != "" {
		excerpt["type"] = t
	}
	if errObj := gjson.GetBytes(body, "response.error"); errObj.Exists() {
		excerpt["response.error"] = json.RawMessage(errObj.Raw)
	} else if errObj := gjson.GetBytes(body, "error"); errObj.Exists() {
		excerpt["error"] = json.RawMessage(errObj.Raw)
	}
	if message != "" {
		excerpt["message"] = message
	}
	if len(excerpt) == 0 {
		return truncateString(string(body), openAICapacityABDumpBodyMaxBytes)
	}
	b, err := json.Marshal(excerpt)
	if err != nil {
		return truncateString(string(body), openAICapacityABDumpBodyMaxBytes)
	}
	return string(b)
}

func firstNonEmptyJSONString(body []byte, paths ...string) string {
	for _, path := range paths {
		if v := strings.TrimSpace(gjson.GetBytes(body, path).String()); v != "" {
			return v
		}
	}
	return ""
}

func (s *OpenAIGatewayService) dumpOpenAICapacityABHTTP(
	c *gin.Context,
	account *Account,
	resp *http.Response,
	entry string,
	model string,
	message string,
	body []byte,
	okSample bool,
) {
	if resp == nil {
		return
	}
	in := openAICapacityABDumpInput{
		Entry:      entry,
		HTTPStatus: resp.StatusCode,
		Model:      model,
		Message:    message,
		Body:       body,
		Headers:    resp.Header,
		Account:    account,
		Client:     c,
	}
	if okSample {
		in.Label = "ok"
		s.captureOpenAICodexTurnState(account, resp.Header, model, true)
	} else if classifyOpenAICapacityABLabel(resp.StatusCode, message, body) == "capacity" {
		s.scheduleOpenAICodexTurnStateRefresh(account)
	}
	s.maybeDumpOpenAICapacityAB(in)
}

func (s *OpenAIGatewayService) dumpOpenAICapacityABStream(
	c *gin.Context,
	account *Account,
	entry string,
	httpStatus int,
	headers http.Header,
	payload []byte,
	message string,
	model string,
) {
	if classifyOpenAICapacityABLabel(httpStatus, message, payload) == "capacity" {
		s.scheduleOpenAICodexTurnStateRefresh(account)
	}
	s.maybeDumpOpenAICapacityAB(openAICapacityABDumpInput{
		Entry:      entry,
		HTTPStatus: httpStatus,
		Model:      model,
		Message:    message,
		Body:       payload,
		Headers:    headers,
		Account:    account,
		Client:     c,
	})
}
