package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

const grokToolOutputTruncationMarker = "\n...[truncated by sub2api]...\n"
const grokRemoveEmptyImageContainerKey = "__sub2api_remove_empty_image_container"

type grokPayloadKind int

const (
	grokPayloadResponses grokPayloadKind = iota
	grokPayloadChat
)

type grokPayloadTooLargeError struct {
	Size  int
	Limit int64
}

func (e *grokPayloadTooLargeError) Error() string {
	return fmt.Sprintf("Grok request body is %d bytes after safe optimization; maximum is %d bytes", e.Size, e.Limit)
}

func (s *OpenAIGatewayService) optimizeGrokPayload(account *Account, body []byte, kind grokPayloadKind) ([]byte, error) {
	if s == nil || s.cfg == nil || account == nil || account.Platform != PlatformGrok {
		return body, nil
	}
	return optimizeGrokPayload(body, kind, s.cfg.Gateway.GrokPayload)
}

func (s *OpenAIGatewayService) optimizeGrokPayloadBeforeImageBridge(account *Account, body []byte) ([]byte, error) {
	if s == nil || s.cfg == nil || account == nil || account.Platform != PlatformGrok {
		return body, nil
	}
	cfg := s.cfg.Gateway.GrokPayload
	cfg.HardLimitBytes = 0
	return optimizeGrokPayload(body, grokPayloadChat, cfg)
}

func optimizeGrokPayload(body []byte, kind grokPayloadKind, cfg config.GrokPayloadConfig) ([]byte, error) {
	var payload map[string]any
	if err := decodeGrokJSON(body, &payload); err != nil {
		return nil, fmt.Errorf("parse Grok payload for optimization: %w", err)
	}

	changed := false
	containers := grokPayloadContainers(payload, kind)
	hasImages := grokPayloadHasImages(containers)
	if hasImages && cfg.DisableStoreOnImages {
		if store, exists := payload["store"].(bool); !exists || store {
			payload["store"] = false
			changed = true
		}
	}
	if hasImages && cfg.DeduplicateImages {
		if deduplicateGrokHistoricalImages(containers) {
			removeEmptyGrokImageContainers(payload, kind)
			containers = grokPayloadContainers(payload, kind)
			changed = true
		}
	}

	optimized := body
	if changed {
		var err error
		optimized, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("serialize optimized Grok payload: %w", err)
		}
	}

	if cfg.SoftLimitBytes > 0 && int64(len(optimized)) > cfg.SoftLimitBytes && cfg.ToolOutputMaxBytes > 0 {
		if truncateGrokHistoricalToolOutputs(containers, kind, cfg.ToolOutputMaxBytes) {
			var err error
			optimized, err = json.Marshal(payload)
			if err != nil {
				return nil, fmt.Errorf("serialize truncated Grok payload: %w", err)
			}
		}
	}
	if cfg.HardLimitBytes > 0 && int64(len(optimized)) > cfg.HardLimitBytes {
		return nil, &grokPayloadTooLargeError{Size: len(optimized), Limit: cfg.HardLimitBytes}
	}
	return optimized, nil
}

func grokPayloadContainers(payload map[string]any, kind grokPayloadKind) []any {
	key := "input"
	if kind == grokPayloadChat {
		key = "messages"
	}
	switch value := payload[key].(type) {
	case []any:
		return value
	case map[string]any:
		return []any{value}
	default:
		return nil
	}
}

func grokPayloadHasImages(containers []any) bool {
	for _, raw := range containers {
		container, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for _, part := range grokPayloadContentParts(container["content"]) {
			if grokImageURL(part) != "" {
				return true
			}
		}
	}
	return false
}

func deduplicateGrokHistoricalImages(containers []any) bool {
	if len(containers) < 2 {
		return false
	}
	seen := make(map[string]struct{})
	changed := false
	current := len(containers) - 1
	for i := len(containers) - 1; i >= 0; i-- {
		container, ok := containers[i].(map[string]any)
		if !ok {
			continue
		}
		parts, ok := container["content"].([]any)
		if !ok {
			continue
		}
		filtered := make([]any, 0, len(parts))
		for partIndex := len(parts) - 1; partIndex >= 0; partIndex-- {
			part := parts[partIndex]
			imageURL := grokImageURL(part)
			if !strings.HasPrefix(strings.ToLower(imageURL), "data:image/") {
				filtered = append(filtered, part)
				continue
			}
			_, duplicate := seen[imageURL]
			if duplicate && i != current {
				changed = true
				continue
			}
			seen[imageURL] = struct{}{}
			filtered = append(filtered, part)
		}
		reverseAny(filtered)
		if len(filtered) != len(parts) {
			container["content"] = filtered
			if len(filtered) == 0 {
				container[grokRemoveEmptyImageContainerKey] = true
			}
		}
	}
	return changed
}

func removeEmptyGrokImageContainers(payload map[string]any, kind grokPayloadKind) {
	key := "input"
	if kind == grokPayloadChat {
		key = "messages"
	}
	containers, ok := payload[key].([]any)
	if !ok {
		return
	}
	filtered := make([]any, 0, len(containers))
	for _, raw := range containers {
		container, ok := raw.(map[string]any)
		if ok {
			if remove, _ := container[grokRemoveEmptyImageContainerKey].(bool); remove {
				delete(container, grokRemoveEmptyImageContainerKey)
				continue
			}
			delete(container, grokRemoveEmptyImageContainerKey)
		}
		filtered = append(filtered, raw)
	}
	payload[key] = filtered
}

func grokPayloadContentParts(content any) []any {
	parts, _ := content.([]any)
	return parts
}

func grokImageURL(raw any) string {
	part, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(part["type"]))) {
	case "input_image":
		switch imageURL := part["image_url"].(type) {
		case string:
			return imageURL
		case map[string]any:
			url, _ := imageURL["url"].(string)
			return url
		}
	case "image_url":
		switch imageURL := part["image_url"].(type) {
		case string:
			return imageURL
		case map[string]any:
			url, _ := imageURL["url"].(string)
			return url
		}
	}
	return ""
}

func truncateGrokHistoricalToolOutputs(containers []any, kind grokPayloadKind, maxBytes int) bool {
	latest := -1
	for i, raw := range containers {
		if isGrokToolOutput(raw, kind) {
			latest = i
		}
	}
	changed := false
	for i, raw := range containers {
		if i == latest || i == len(containers)-1 {
			continue
		}
		container, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		output := grokToolOutput(raw, kind)
		if len(output) <= maxBytes {
			continue
		}
		truncated, ok := truncateGrokUTF8(output, maxBytes)
		if !ok {
			continue
		}
		if kind == grokPayloadChat {
			container["content"] = truncated
		} else {
			container["output"] = truncated
		}
		changed = true
	}
	return changed
}

func grokToolOutput(raw any, kind grokPayloadKind) string {
	container, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	if kind == grokPayloadChat {
		if !isGrokToolOutput(raw, kind) {
			return ""
		}
		output, _ := container["content"].(string)
		return output
	}
	if !isGrokToolOutput(raw, kind) {
		return ""
	}
	output, _ := container["output"].(string)
	return output
}

func isGrokToolOutput(raw any, kind grokPayloadKind) bool {
	container, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	if kind == grokPayloadChat {
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(container["role"])), "tool")
	}
	return strings.TrimSpace(fmt.Sprint(container["type"])) == "function_call_output"
}

func truncateGrokUTF8(value string, maxBytes int) (string, bool) {
	if len(value) <= maxBytes || maxBytes <= len(grokToolOutputTruncationMarker) {
		return value, false
	}
	remaining := maxBytes - len(grokToolOutputTruncationMarker)
	headBytes := remaining / 2
	tailBytes := remaining - headBytes
	headBytes = previousUTF8Boundary(value, headBytes)
	tailStart := nextUTF8Boundary(value, len(value)-tailBytes)
	if headBytes >= tailStart {
		return value, false
	}
	return value[:headBytes] + grokToolOutputTruncationMarker + value[tailStart:], true
}

func previousUTF8Boundary(value string, index int) int {
	if index >= len(value) {
		return len(value)
	}
	for index > 0 && !utf8.RuneStart(value[index]) {
		index--
	}
	return index
}

func nextUTF8Boundary(value string, index int) int {
	if index <= 0 {
		return 0
	}
	for index < len(value) && !utf8.RuneStart(value[index]) {
		index++
	}
	return index
}

func reverseAny(values []any) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func writeGrokPayloadTooLarge(c *gin.Context, kind grokPayloadKind, err error) {
	if kind == grokPayloadChat {
		writeChatCompletionsError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", err.Error())
		return
	}
	writeResponsesError(c, http.StatusRequestEntityTooLarge, "invalid_request_error", err.Error())
}

func handleGrokPayloadOptimizationError(c *gin.Context, kind grokPayloadKind, err error) bool {
	var tooLarge *grokPayloadTooLargeError
	if !errors.As(err, &tooLarge) {
		return false
	}
	writeGrokPayloadTooLarge(c, kind, tooLarge)
	return true
}
