package service

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
)

func applyKimiCodingHeaders(header http.Header, account *Account) {
	if header == nil || account == nil {
		return
	}
	header.Set("User-Agent", kimi.ClientUserAgent)
	header.Set("X-Msh-Platform", kimi.ClientPlatform)
	header.Set("X-Msh-Version", kimi.ClientVersion)
	header.Set("X-Msh-Device-Name", account.GetCredential("device_name"))
	header.Set("X-Msh-Device-Model", account.GetCredential("device_model"))
	header.Set("X-Msh-Device-Id", account.GetCredential("device_id"))
}

func normalizeKimiToolMessageLinks(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	messages, ok := payload["messages"].([]any)
	if !ok {
		return body, nil
	}

	pending := make([]string, 0)
	previousReasoning := ""
	normalized := make([]any, 0, len(messages))
	changed := false
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			normalized = append(normalized, raw)
			continue
		}
		role, _ := message["role"].(string)
		switch role {
		case "assistant":
			if reasoning, _ := message["reasoning_content"].(string); strings.TrimSpace(reasoning) != "" {
				previousReasoning = reasoning
			}
			toolCalls, _ := message["tool_calls"].([]any)
			if len(toolCalls) == 0 && kimiMessageContentEmpty(message["content"]) {
				changed = true
				continue
			}
			pending = pending[:0]
			for _, rawCall := range toolCalls {
				if call, ok := rawCall.(map[string]any); ok {
					if id, _ := call["id"].(string); strings.TrimSpace(id) != "" {
						pending = append(pending, id)
					}
				}
			}
			if len(toolCalls) > 0 {
				if reasoning, _ := message["reasoning_content"].(string); strings.TrimSpace(reasoning) == "" {
					if strings.TrimSpace(previousReasoning) == "" {
						previousReasoning = "Tool calls prepared."
					}
					message["reasoning_content"] = previousReasoning
					changed = true
				}
			}
		case "tool":
			toolCallID, _ := message["tool_call_id"].(string)
			if strings.TrimSpace(toolCallID) == "" {
				if callID, _ := message["call_id"].(string); strings.TrimSpace(callID) != "" {
					message["tool_call_id"] = callID
					changed = true
				} else if len(pending) == 1 {
					message["tool_call_id"] = pending[0]
					changed = true
				}
			}
		}
		normalized = append(normalized, message)
	}
	if !changed {
		return body, nil
	}
	payload["messages"] = normalized
	return json.Marshal(payload)
}

func kimiMessageContentEmpty(content any) bool {
	switch value := content.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(value) == ""
	case []any:
		return len(value) == 0
	default:
		return false
	}
}
