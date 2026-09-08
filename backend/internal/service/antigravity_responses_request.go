package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
)

// convertAntigravityResponsesRequest reuses the existing Responses custom-tool
// normalization, then maps the normalized messages directly to Gemini.
func convertAntigravityResponsesRequest(req *apicompat.ResponsesRequest, previous []antigravity.GeminiContent) ([]byte, map[string]bool, error) {
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens <= 0 {
		return nil, nil, fmt.Errorf("max_output_tokens must be positive")
	}
	for _, item := range gjson.ParseBytes(req.Input).Array() {
		if item.Type == gjson.String {
			continue
		}
		switch item.Get("type").String() {
		case "", "message", "input_text", "text", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output", "additional_tools":
		case "reasoning":
			if item.Get("encrypted_content").String() != "" {
				return nil, nil, fmt.Errorf("opaque reasoning from another provider cannot be replayed to Antigravity")
			}
		default:
			return nil, nil, fmt.Errorf("unsupported Responses input item %q", item.Get("type").String())
		}
	}
	tools, err := apicompat.EffectiveResponsesTools(req)
	if err != nil {
		return nil, nil, err
	}
	for _, tool := range tools {
		if tool.Type == "namespace" {
			children := tool.Tools
			if len(children) == 0 {
				children = tool.Children
			}
			for _, child := range children {
				if child.Type != "function" {
					return nil, nil, fmt.Errorf("only function children are supported in namespaces")
				}
			}
			continue
		}
		if tool.Type != "function" && tool.Type != "custom" {
			return nil, nil, fmt.Errorf("unsupported Antigravity tool type %q", tool.Type)
		}
	}
	chat, err := apicompat.ResponsesToChatCompletionsRequest(req)
	if err != nil {
		return nil, nil, err
	}
	chat.Messages, err = apicompat.ResponsesInputMessages(req.Instructions, req.Input)
	if err != nil {
		return nil, nil, err
	}
	contents := append([]antigravity.GeminiContent(nil), previous...)
	names := map[string]string{}
	for _, content := range contents {
		for _, p := range content.Parts {
			if p.FunctionCall != nil {
				names[p.FunctionCall.ID] = p.FunctionCall.Name
			}
		}
	}
	var system []antigravity.GeminiPart
	for _, msg := range chat.Messages {
		var parts []antigravity.GeminiPart
		if len(msg.Content) > 0 && string(msg.Content) != "null" {
			var str string
			if json.Unmarshal(msg.Content, &str) == nil {
				if str != "" {
					parts = append(parts, antigravity.GeminiPart{Text: str})
				}
			} else {
				var cp []apicompat.ChatContentPart
				if err := json.Unmarshal(msg.Content, &cp); err != nil {
					return nil, nil, fmt.Errorf("invalid message content: %w", err)
				}
				for _, p := range cp {
					if p.Type != "text" {
						return nil, nil, fmt.Errorf("Antigravity Responses supports text input only")
					}
					parts = append(parts, antigravity.GeminiPart{Text: p.Text})
				}
			}
		}
		if msg.Role == "system" || msg.Role == "developer" {
			system = append(system, parts...)
			continue
		}
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
			if msg.ReasoningContent != "" {
				parts = append([]antigravity.GeminiPart{{Text: msg.ReasoningContent}}, parts...)
			}
		}
		if msg.Role == "tool" {
			name := names[msg.ToolCallID]
			if name == "" {
				return nil, nil, fmt.Errorf("function_call_output has no matching call: %s", msg.ToolCallID)
			}
			var output any
			if json.Unmarshal(msg.Content, &output) != nil {
				return nil, nil, fmt.Errorf("invalid tool output")
			}
			parts = []antigravity.GeminiPart{{FunctionResponse: &antigravity.GeminiFunctionResponse{Name: name, ID: msg.ToolCallID, Response: map[string]any{"output": output}}}}
		}
		for _, call := range msg.ToolCalls {
			var args map[string]any
			if json.Unmarshal([]byte(call.Function.Arguments), &args) != nil || args == nil {
				return nil, nil, fmt.Errorf("invalid arguments for %s", call.Function.Name)
			}
			names[call.ID] = call.Function.Name
			parts = append(parts, antigravity.GeminiPart{FunctionCall: &antigravity.GeminiFunctionCall{ID: call.ID, Name: call.Function.Name, Args: args}})
		}
		if len(parts) > 0 {
			contents = append(contents, antigravity.GeminiContent{Role: role, Parts: parts})
		}
	}
	var cleanedContents []antigravity.GeminiContent
	for _, content := range contents {
		var validParts []antigravity.GeminiPart
		for _, p := range content.Parts {
			if p.FunctionCall == nil && p.FunctionResponse == nil && p.InlineData == nil && strings.TrimSpace(p.Text) == "" {
				if p.ThoughtSignature != "" {
					for j := len(validParts) - 1; j >= 0; j-- {
						if validParts[j].FunctionCall != nil {
							if validParts[j].ThoughtSignature == "" {
								validParts[j].ThoughtSignature = p.ThoughtSignature
							}
							break
						}
					}
				}
				continue
			}
			if p.FunctionCall != nil && p.ThoughtSignature == "" {
				p.ThoughtSignature = antigravity.DummyThoughtSignature
			}
			validParts = append(validParts, p)
		}
		if len(validParts) > 0 {
			content.Parts = validParts
			cleanedContents = append(cleanedContents, content)
		}
	}
	contents = cleanedContents
	if len(contents) == 0 {
		return nil, nil, fmt.Errorf("input must contain text or tool results")
	}
	out := map[string]any{"contents": contents}
	if len(system) > 0 {
		out["systemInstruction"] = map[string]any{"parts": system}
	}
	var declarations []antigravity.GeminiFunctionDecl
	for _, tool := range chat.Tools {
		if tool.Function != nil {
			params := map[string]any{"type": "object", "properties": map[string]any{}}
			if len(tool.Function.Parameters) > 0 && json.Unmarshal(tool.Function.Parameters, &params) != nil {
				return nil, nil, fmt.Errorf("invalid tool schema")
			}
			antigravity.DeepCleanUndefined(params)
			cleaned := antigravity.CleanJSONSchema(params)
			declarations = append(declarations, antigravity.GeminiFunctionDecl{Name: tool.Function.Name, Description: tool.Function.Description, Parameters: cleaned})
		}
	}
	if len(declarations) > 0 {
		out["tools"] = []antigravity.GeminiToolDeclaration{{FunctionDeclarations: declarations}}
	}
	config := map[string]any{}
	if req.MaxOutputTokens != nil {
		maxTokens := *req.MaxOutputTokens
		if maxTokens > 65536 {
			maxTokens = 65536
		}
		config["maxOutputTokens"] = maxTokens
	}
	if req.Temperature != nil {
		temp := *req.Temperature
		if temp > 2.0 {
			temp = 2.0
		} else if temp < 0.0 {
			temp = 0.0
		}
		config["temperature"] = temp
	}
	if req.TopP != nil {
		topP := *req.TopP
		if topP > 1.0 {
			topP = 1.0
		} else if topP < 0.0 {
			topP = 0.0
		}
		config["topP"] = topP
	}
	if req.Reasoning != nil {
		effort := strings.ToLower(req.Reasoning.Effort)
		switch effort {
		case "none":
			config["thinkingConfig"] = map[string]any{"thinkingBudget": 0}
		case "low", "medium", "high", "xhigh", "max", "ultra", "minimal", "":
			level := effort
			if level == "minimal" {
				level = "low"
			}
			if level == "xhigh" || level == "max" || level == "ultra" || level == "" {
				level = "high"
			}
			config["thinkingConfig"] = map[string]any{"thinkingLevel": level, "includeThoughts": req.Reasoning.Summary != "" && req.Reasoning.Summary != "none"}
		default:
			return nil, nil, fmt.Errorf("unsupported reasoning effort %q", effort)
		}
	}
	if req.Text != nil && len(req.Text.Format) > 0 {
		var format struct {
			Type   string         `json:"type"`
			Schema map[string]any `json:"schema"`
		}
		if json.Unmarshal(req.Text.Format, &format) != nil {
			return nil, nil, fmt.Errorf("invalid text.format")
		}
		switch format.Type {
		case "json_object":
			config["responseMimeType"] = "application/json"
		case "json_schema":
			config["responseMimeType"] = "application/json"
			config["responseJsonSchema"] = format.Schema
		case "text", "":
		default:
			return nil, nil, fmt.Errorf("unsupported text.format")
		}
	}
	if len(req.ToolChoice) > 0 && string(req.ToolChoice) != "null" {
		var choice string
		names := []string(nil)
		if json.Unmarshal(req.ToolChoice, &choice) != nil {
			var selected struct{ Type, Name string }
			if json.Unmarshal(req.ToolChoice, &selected) != nil || (selected.Type != "function" && selected.Type != "custom") || selected.Name == "" {
				return nil, nil, fmt.Errorf("unsupported tool_choice")
			}
			choice = "required"
			names = []string{selected.Name}
		}
		mode := "AUTO"
		switch choice {
		case "none":
			mode = "NONE"
		case "required":
			mode = "ANY"
		case "auto":
		default:
			return nil, nil, fmt.Errorf("unsupported tool_choice")
		}
		fc := map[string]any{"mode": mode}
		if len(names) > 0 {
			fc["allowedFunctionNames"] = names
		}
		out["toolConfig"] = map[string]any{"functionCallingConfig": fc}
	}
	out["generationConfig"] = config
	body, err := json.Marshal(out)
	return body, apicompat.CustomToolNames(tools), err
}
