package cursor

import (
	"encoding/json"
	"strings"

	cursorpb "github.com/Wei-Shaw/sub2api/internal/pkg/cursor/proto"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
)

// OpenAIContentPart represents a multi-modal content part in OpenAI schema.
type OpenAIContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *OpenAIImageURL `json:"image_url,omitempty"`
}

type OpenAIImageURL struct {
	URL string `json:"url"`
}

// OpenAIMessage represents an OpenAI chat message.
type OpenAIMessage struct {
	Role       string            `json:"role"`
	Content    json.RawMessage   `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	Name       string            `json:"name,omitempty"`
}

type OpenAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function OpenAIFunctionCallDesc `json:"function"`
}

type OpenAIFunctionCallDesc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAITool describes a function tool in OpenAI schema.
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

type OpenAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// OpenAIChatRequest represents the incoming chat completions parameters for Cursor conversion.
type OpenAIChatRequest struct {
	Model               string          `json:"model"`
	Messages            []OpenAIMessage `json:"messages"`
	Tools               []OpenAITool    `json:"tools,omitempty"`
	ToolChoice          any             `json:"tool_choice,omitempty"`
	MaxTokens           *int32          `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int32          `json:"max_completion_tokens,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	Stop                json.RawMessage `json:"stop,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
}

// BuildToolCallMap builds a mapping from tool_call_id to tool name from all assistant messages.
func BuildToolCallMap(messages []OpenAIMessage) map[string]string {
	m := make(map[string]string)
	for _, msg := range messages {
		if msg.Role == "assistant" {
			for _, call := range msg.ToolCalls {
				if call.ID != "" && call.Function.Name != "" {
					m[call.ID] = call.Function.Name
				}
			}
		}
	}
	return m
}

// ResolveRequestedModel determines the Cursor model ID, max mode flag, and parameters.
func ResolveRequestedModel(model string) (string, bool, []*cursorpb.InferenceModelParameterValue) {
	if model == "grok-4.5" || model == "cursor-grok-4.5" {
		return "grok-4.5", true, []*cursorpb.InferenceModelParameterValue{
			{Id: "effort", Value: "high"},
			{Id: "fast", Value: "true"},
		}
	}
	if strings.HasSuffix(model, "+max") {
		return strings.TrimSuffix(model, "+max"), true, nil
	}
	if strings.HasSuffix(model, "-max") {
		return strings.TrimSuffix(model, "-max"), true, nil
	}
	return model, false, nil
}

// MapOpenAIRequestToProto converts an OpenAI Chat Completions request to a Cursor InferenceStreamRequest.
func MapOpenAIRequestToProto(req *OpenAIChatRequest) (*cursorpb.InferenceStreamRequest, error) {
	toolCallMap := BuildToolCallMap(req.Messages)
	protoMessages := make([]*cursorpb.InferenceCoreMessage, 0, len(req.Messages))

	for _, msg := range req.Messages {
		coreMsg := &cursorpb.InferenceCoreMessage{}
		role := strings.ToLower(strings.TrimSpace(msg.Role))

		switch role {
		case "system", "developer":
			coreMsg.Role = cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_SYSTEM
			setMessageContent(coreMsg, msg.Content)

		case "user":
			coreMsg.Role = cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER
			setMessageContent(coreMsg, msg.Content)

		case "assistant":
			coreMsg.Role = cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT
			if len(msg.Content) > 0 {
				setMessageContent(coreMsg, msg.Content)
			} else {
				coreMsg.Content = &cursorpb.InferenceCoreMessage_Text{Text: ""}
			}
			if len(msg.ToolCalls) > 0 {
				coreMsg.ToolCalls = make([]*cursorpb.InferenceToolCall, 0, len(msg.ToolCalls))
				for _, call := range msg.ToolCalls {
					var argsStruct *structpb.Struct
					if call.Function.Arguments != "" {
						var argsMap map[string]any
						if err := json.Unmarshal([]byte(call.Function.Arguments), &argsMap); err == nil {
							argsStruct, _ = structpb.NewStruct(argsMap)
						}
					}
					if argsStruct == nil {
						argsStruct, _ = structpb.NewStruct(map[string]any{})
					}
					coreMsg.ToolCalls = append(coreMsg.ToolCalls, &cursorpb.InferenceToolCall{
						ToolCallId: call.ID,
						ToolName:   call.Function.Name,
						Args:       argsStruct,
					})
				}
			}

		case "tool":
			coreMsg.Role = cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL
			toolName := toolCallMap[msg.ToolCallID]
			contentStr := extractContentString(msg.Content)
			var val *structpb.Value
			var valInterface any
			if err := json.Unmarshal([]byte(contentStr), &valInterface); err == nil {
				val, _ = structpb.NewValue(valInterface)
			}
			if val == nil {
				val = structpb.NewStringValue(contentStr)
			}
			coreMsg.Content = &cursorpb.InferenceCoreMessage_ToolContent{
				ToolContent: &cursorpb.InferenceToolResultContent{
					Parts: []*cursorpb.InferenceToolResultPart{
						{
							ToolCallId: msg.ToolCallID,
							ToolName:   toolName,
							Result:     val,
						},
					},
				},
			}

		default:
			coreMsg.Role = cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_UNSPECIFIED
			coreMsg.Content = &cursorpb.InferenceCoreMessage_Text{Text: string(msg.Content)}
		}

		protoMessages = append(protoMessages, coreMsg)
	}

	// Tools
	var protoTools []*cursorpb.InferenceAgentTool
	var acceptedUnadvertised []string
	if len(req.Tools) > 0 {
		protoTools = make([]*cursorpb.InferenceAgentTool, 0, len(req.Tools))
		for _, tool := range req.Tools {
			if tool.Type != "function" || tool.Function.Name == "" {
				continue
			}
			var paramsStruct *structpb.Struct
			if len(tool.Function.Parameters) > 0 {
				var paramsMap map[string]any
				if err := json.Unmarshal(tool.Function.Parameters, &paramsMap); err == nil {
					paramsStruct, _ = structpb.NewStruct(paramsMap)
				}
			}
			if paramsStruct == nil {
				paramsStruct, _ = structpb.NewStruct(map[string]any{})
			}
			protoTools = append(protoTools, &cursorpb.InferenceAgentTool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  paramsStruct,
			})
		}
	}

	// Model config
	var modelConfig *cursorpb.InferenceModelConfig
	maxTokens := req.MaxTokens
	if maxTokens == nil {
		maxTokens = req.MaxCompletionTokens
	}
	var stopSeqs []string
	if len(req.Stop) > 0 {
		var singleStop string
		if json.Unmarshal(req.Stop, &singleStop) == nil && singleStop != "" {
			stopSeqs = []string{singleStop}
		} else {
			var multiStop []string
			if json.Unmarshal(req.Stop, &multiStop) == nil {
				stopSeqs = multiStop
			}
		}
	}
	if maxTokens != nil || req.Temperature != nil || req.TopP != nil || len(stopSeqs) > 0 {
		modelConfig = &cursorpb.InferenceModelConfig{
			MaxTokens:     maxTokens,
			Temperature:   req.Temperature,
			TopP:          req.TopP,
			StopSequences: stopSeqs,
		}
	}

	modelID, maxMode, params := ResolveRequestedModel(req.Model)
	convID := uuid.NewString()
	invID := uuid.NewString()

	protoReq := &cursorpb.InferenceStreamRequest{
		Messages:                      protoMessages,
		Tools:                         protoTools,
		AcceptedUnadvertisedToolNames: acceptedUnadvertised,
		ModelConfig:                   modelConfig,
		ModelId:                       &modelID,
		RequestedModel: &cursorpb.InferenceRequestedModel{
			ModelId:                      modelID,
			MaxMode:                      maxMode,
			Parameters:                   params,
			BuiltInModel:                 false,
			IsVariantStringRepresentation: false,
		},
		ConversationId: &convID,
		InvocationId:   &invID,
	}

	return protoReq, nil
}

func extractContentString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

func setMessageContent(coreMsg *cursorpb.InferenceCoreMessage, raw json.RawMessage) {
	if len(raw) == 0 {
		coreMsg.Content = &cursorpb.InferenceCoreMessage_Text{Text: ""}
		return
	}
	var textContent string
	if err := json.Unmarshal(raw, &textContent); err == nil {
		coreMsg.Content = &cursorpb.InferenceCoreMessage_Text{Text: textContent}
		return
	}

	var parts []OpenAIContentPart
	if err := json.Unmarshal(raw, &parts); err == nil && len(parts) > 0 {
		protoParts := make([]*cursorpb.InferenceContentPart, 0, len(parts))
		for _, p := range parts {
			switch p.Type {
			case "text":
				protoParts = append(protoParts, &cursorpb.InferenceContentPart{
					Part: &cursorpb.InferenceContentPart_Text{
						Text: &cursorpb.InferenceTextPart{Text: p.Text},
					},
				})
			case "image_url":
				if p.ImageURL != nil {
					mimeType := "image/png"
					if strings.HasPrefix(p.ImageURL.URL, "data:image/jpeg") {
						mimeType = "image/jpeg"
					} else if strings.HasPrefix(p.ImageURL.URL, "data:image/webp") {
						mimeType = "image/webp"
					}
					protoParts = append(protoParts, &cursorpb.InferenceContentPart{
						Part: &cursorpb.InferenceContentPart_Image{
							Image: &cursorpb.InferenceImagePart{
								Data:     p.ImageURL.URL,
								MimeType: mimeType,
							},
						},
					})
				}
			default:
				b, _ := json.Marshal(p)
				protoParts = append(protoParts, &cursorpb.InferenceContentPart{
					Part: &cursorpb.InferenceContentPart_Text{
						Text: &cursorpb.InferenceTextPart{Text: string(b)},
					},
				})
			}
		}
		coreMsg.Content = &cursorpb.InferenceCoreMessage_Parts{
			Parts: &cursorpb.InferenceContentParts{Parts: protoParts},
		}
		return
	}

	coreMsg.Content = &cursorpb.InferenceCoreMessage_Text{Text: string(raw)}
}
