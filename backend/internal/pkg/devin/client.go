package devin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	devinpb "github.com/Wei-Shaw/sub2api/internal/pkg/devin/proto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/proto/devinpbconnect"
	"github.com/google/uuid"
)

const (
	DefaultBaseURL = "https://server.codeium.com"
	ClientName     = "chisel"
	ClientVersion  = "3000.2.17"
)

type Client struct {
	baseURL string
	token   string
	model   string
	api     devinpbconnect.ApiServerServiceClient
}

func NewClient(baseURL, token, model string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("devin token is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, errors.New("devin model is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	transport := &authTransport{base: httpClient.Transport, token: token}
	client := *httpClient
	client.Transport = transport
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, model: model,
		api: devinpbconnect.NewApiServerServiceClient(&client, strings.TrimRight(baseURL, "/"))}, nil
}

type authTransport struct {
	base  http.RoundTripper
	token string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Basic "+t.token+"-"+t.token)
	return base.RoundTrip(clone)
}

type Stream struct {
	upstream *connect.ServerStreamForClient[devinpb.GetChatMessageResponse]
}

func (s *Stream) Recv() (*devinpb.GetChatMessageResponse, error) {
	if s == nil || s.upstream == nil {
		return nil, io.EOF
	}
	if !s.upstream.Receive() {
		if err := s.upstream.Err(); err != nil {
			return nil, normalizeError(err)
		}
		return nil, io.EOF
	}
	return s.upstream.Msg(), nil
}

func (c *Client) Stream(ctx context.Context, req *devinpb.GetChatMessageRequest) (*Stream, error) {
	response, err := c.api.GetChatMessage(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, normalizeError(err)
	}
	return &Stream{upstream: response}, nil
}

func (c *Client) BuildRequest(req *apicompat.ChatCompletionsRequest) (*devinpb.GetChatMessageRequest, error) {
	if req == nil || strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("devin request model is required")
	}
	result := &devinpb.GetChatMessageRequest{
		Metadata: &devinpb.Metadata{ApiKey: c.token, IdeName: ClientName, IdeVersion: ClientVersion, ExtensionName: ClientName, ExtensionVersion: ClientVersion, Locale: "en", Os: "linux", DeviceFingerprint: uuid.NewString()},
		Prompt:   "", ChatModelUid: req.Model, RequestType: devinpb.ChatMessageRequestType_CHAT_MESSAGE_REQUEST_TYPE_CASCADE,
		Configuration:       &devinpb.CompletionConfiguration{NumCompletions: 1, MaxTokens: 128000, MaxNewlines: 400, Temperature: 1, TopK: 40, TopP: 0.95},
		TrajectoryReference: &devinpb.TrajectoryReference{TrajectoryId: uuid.NewString(), TrajectoryType: devinpb.CortexTrajectoryType_CORTEX_TRAJECTORY_TYPE_CASCADE, StepType: devinpb.CortexStepType_CORTEX_STEP_TYPE_USER_INPUT},
		CascadeId:           uuid.NewString(), PlannerMode: devinpb.CortexPlannerMode_CONVERSATIONAL_PLANNER_MODE_DEFAULT, ExecutionId: uuid.NewString(),
	}
	var prompt strings.Builder
	for _, m := range req.Messages {
		converted, err := convertMessage(m)
		if err != nil {
			return nil, err
		}
		result.ChatMessagePrompts = append(result.ChatMessagePrompts, converted)
	}
	if len(req.Tools) > 0 {
		prompt.WriteString("# tools descriptions")
	}
	for _, tool := range req.Tools {
		if tool.Function == nil || tool.Function.Name == "" {
			continue
		}
		result.Tools = append(result.Tools, &devinpb.ChatToolDefinition{Name: tool.Function.Name, Description: tool.Function.Name, JsonSchemaString: string(tool.Function.Parameters)})
		if tool.Function.Description != "" {
			prompt.WriteString("\n<tool name=\"")
			prompt.WriteString(tool.Function.Name)
			prompt.WriteString("\">\n")
			prompt.WriteString(tool.Function.Description)
			prompt.WriteString("\n</tool>")
		}
	}
	if prompt.Len() > 0 {
		result.Prompt = prompt.String()
	}
	return result, nil
}

func convertMessage(message apicompat.ChatMessage) (*devinpb.ChatMessagePrompt, error) {
	source := devinpb.ChatMessageSource_CHAT_MESSAGE_SOURCE_USER
	switch strings.ToLower(message.Role) {
	case "system", "developer":
		source = devinpb.ChatMessageSource_CHAT_MESSAGE_SOURCE_SYSTEM
	case "assistant":
		source = devinpb.ChatMessageSource_CHAT_MESSAGE_SOURCE_SYSTEM
	case "tool":
		source = devinpb.ChatMessageSource_CHAT_MESSAGE_SOURCE_TOOL
	}
	prompt := &devinpb.ChatMessagePrompt{MessageId: uuid.NewString(), Source: source, ToolCallId: message.ToolCallID, ToolResultIsError: false}
	if message.Content != nil {
		var text string
		if json.Unmarshal(message.Content, &text) == nil {
			prompt.Prompt = text
		} else {
			var parts []apicompat.ChatContentPart
			if err := json.Unmarshal(message.Content, &parts); err != nil {
				return nil, fmt.Errorf("decode Devin message content: %w", err)
			}
			for _, part := range parts {
				if part.Text != "" {
					if prompt.Prompt != "" {
						prompt.Prompt += "\n"
					}
					prompt.Prompt += part.Text
				}
				if part.ImageURL != nil {
					if header, data, ok := strings.Cut(part.ImageURL.URL, ","); ok && strings.HasPrefix(header, "data:") {
						mimeType := strings.TrimPrefix(strings.SplitN(header, ";", 2)[0], "data:")
						if mimeType == "" {
							mimeType = "image/png"
						}
						prompt.Images = append(prompt.Images, &devinpb.ImageData{Base64Data: data, MimeType: mimeType})
					}
				}
			}
		}
	}
	if message.ReasoningContent != "" {
		prompt.Thinking = message.ReasoningContent
	} else if message.Reasoning != "" {
		prompt.Thinking = message.Reasoning
	}
	for _, call := range message.ToolCalls {
		prompt.ToolCalls = append(prompt.ToolCalls, &devinpb.ChatToolCall{Id: call.ID, Name: call.Function.Name, ArgumentsJson: call.Function.Arguments})
	}
	return prompt, nil
}

type Delta struct {
	Text, Reasoning, ToolCallID, ToolName, ToolArgs string
	ToolIndex                                       int
	FinishReason                                    string
	Done                                            bool
	Usage                                           *Usage
}
type Usage struct{ InputTokens, OutputTokens, CacheReadTokens, CacheWriteTokens int }

func DecodeResponse(resp *devinpb.GetChatMessageResponse, toolIndex int) Delta {
	if resp == nil {
		return Delta{}
	}
	d := Delta{Text: resp.GetDeltaText(), Reasoning: resp.GetDeltaThinking(), ToolIndex: toolIndex}
	if len(resp.GetDeltaToolCalls()) > 0 {
		call := resp.GetDeltaToolCalls()[0]
		d.ToolCallID, d.ToolName, d.ToolArgs = call.GetId(), call.GetName(), call.GetArgumentsJson()
	}
	if usage := resp.GetUsage(); usage != nil {
		d.Usage = &Usage{InputTokens: int(usage.GetInputTokens()), OutputTokens: int(usage.GetOutputTokens()), CacheReadTokens: int(usage.GetCacheReadTokens()), CacheWriteTokens: int(usage.GetCacheWriteTokens())}
	}
	switch resp.GetStopReason() {
	case devinpb.StopReason_STOP_REASON_FUNCTION_CALL:
		d.FinishReason = "tool_calls"
	case devinpb.StopReason_STOP_REASON_MAX_TOKENS, devinpb.StopReason_STOP_REASON_INCOMPLETE, devinpb.StopReason_STOP_REASON_PARTIAL:
		d.FinishReason = "length"
	case devinpb.StopReason_STOP_REASON_ERROR:
		d.FinishReason = "error"
	case devinpb.StopReason_STOP_REASON_STOP_PATTERN:
		d.FinishReason = "stop"
	}
	return d
}

func normalizeError(err error) error {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return fmt.Errorf("%s: %s", ce.Code(), ce.Message())
	}
	return err
}
