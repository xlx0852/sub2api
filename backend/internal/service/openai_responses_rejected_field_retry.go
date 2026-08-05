package service

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const maxOpenAIResponsesRejectedFieldRetries = 6

var (
	openAIResponsesRejectedIndexedParamPattern = regexp.MustCompile(`(?i)^input\[(\d+)\]\.(namespace|arguments|id)$`)
	openAIResponsesRejectedMessageParamPattern = regexp.MustCompile(`(?i)(?:(?:unknown|unsupported)[ _-]+parameter|missing required parameter|invalid(?:\s+(?:parameter|value\s+for))?)\s*(?::|=|is)?\s*["']?(max_output_tokens|input\[\d+\]\.(?:namespace|arguments|id))(?:["']|\b)`)
	openAIResponsesMissingToolCallPattern      = regexp.MustCompile(`(?i)no tool call found for (?:custom tool call|function call|tool call) output with call_id\s+["']?([A-Za-z0-9_:-]+)`)
)

type openAIResponsesRejectedFieldRetryState struct {
	attempts          int
	replayRepairTried bool
	seenBodyHashes    map[[sha256.Size]byte]struct{}
}

func newOpenAIResponsesRejectedFieldRetryState(initialBody []byte) *openAIResponsesRejectedFieldRetryState {
	state := &openAIResponsesRejectedFieldRetryState{
		seenBodyHashes: make(map[[sha256.Size]byte]struct{}, maxOpenAIResponsesRejectedFieldRetries+1),
	}
	state.remember(initialBody)
	return state
}

func (s *openAIResponsesRejectedFieldRetryState) Allow(nextBody []byte) bool {
	return s.AllowForReason(nextBody, "")
}

func (s *openAIResponsesRejectedFieldRetryState) AllowForReason(nextBody []byte, reason string) bool {
	if s == nil || len(nextBody) == 0 || s.attempts >= maxOpenAIResponsesRejectedFieldRetries {
		return false
	}
	normalizedReason := strings.TrimSpace(reason)
	isReplayRepair := strings.HasPrefix(normalizedReason, "indexed arguments ") ||
		strings.HasPrefix(normalizedReason, "indexed id ") ||
		strings.HasPrefix(normalizedReason, "tool call pairing ")
	if isReplayRepair && s.replayRepairTried {
		return false
	}
	bodyHash := sha256.Sum256(nextBody)
	if _, seen := s.seenBodyHashes[bodyHash]; seen {
		return false
	}
	s.seenBodyHashes[bodyHash] = struct{}{}
	s.attempts++
	if isReplayRepair {
		s.replayRepairTried = true
	}
	return true
}

func (s *openAIResponsesRejectedFieldRetryState) remember(body []byte) {
	if s == nil || len(body) == 0 {
		return
	}
	if s.seenBodyHashes == nil {
		s.seenBodyHashes = make(map[[sha256.Size]byte]struct{}, maxOpenAIResponsesRejectedFieldRetries+1)
	}
	s.seenBodyHashes[sha256.Sum256(body)] = struct{}{}
}

func normalizeOpenAIResponsesRejectedFieldRetryBody(statusCode int, body, responseBody []byte) ([]byte, string, bool, error) {
	if statusCode != http.StatusBadRequest || len(body) == 0 || len(responseBody) == 0 {
		return nil, "", false, nil
	}

	code := strings.ToLower(strings.TrimSpace(extractUpstreamErrorCode(responseBody)))
	rawMessage := strings.TrimSpace(extractUpstreamErrorMessage(responseBody))
	message := strings.ToLower(rawMessage)
	if callID, ok := openAIResponsesMissingToolCallID(rawMessage); ok {
		retryBody, changed, err := normalizeOpenAIRejectedReplayPairByCallID(body, callID)
		if err != nil || !changed {
			return nil, "", false, err
		}
		return retryBody, "tool call pairing compatibility rejection", true, nil
	}
	if !isExplicitOpenAIResponsesFieldRejection(code, message) {
		return nil, "", false, nil
	}

	param := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "error.param").String()))
	if param == "" {
		param = openAIResponsesRejectedParamFromMessage(message)
	}
	if index, field, ok := openAIResponsesRejectedIndexedField(param); ok {
		switch field {
		case "namespace":
			return removeOpenAIResponsesRejectedNamespaceAtIndex(body, index)
		case "arguments", "id":
			retryBody, changed, err := normalizeOpenAIRejectedReplayPairAtIndex(body, index)
			if err != nil || !changed {
				return nil, "", false, err
			}
			return retryBody, fmt.Sprintf("indexed %s compatibility rejection", field), true, nil
		}
	}
	if param == "max_output_tokens" && gjson.GetBytes(body, "max_output_tokens").Exists() {
		retryBody, err := sjson.DeleteBytes(body, "max_output_tokens")
		if err != nil {
			return nil, "", false, fmt.Errorf("delete rejected max_output_tokens: %w", err)
		}
		return retryBody, "max_output_tokens parameter rejection", true, nil
	}
	return nil, "", false, nil
}

func openAIResponsesMissingToolCallID(message string) (string, bool) {
	match := openAIResponsesMissingToolCallPattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(match) != 2 {
		return "", false
	}
	callID := strings.TrimSpace(match[1])
	return callID, callID != ""
}

func isExplicitOpenAIResponsesFieldRejection(code, message string) bool {
	switch strings.TrimSpace(code) {
	case "unknown_parameter", "unsupported_parameter", "missing_required_parameter", "invalid_parameter", "invalid_value":
		return true
	}
	return strings.Contains(message, "unknown parameter") ||
		strings.Contains(message, "unsupported parameter") ||
		strings.Contains(message, "missing required parameter") ||
		strings.Contains(message, "invalid parameter") ||
		(strings.Contains(message, "invalid '") && strings.Contains(message, "input["))
}

func openAIResponsesRejectedParamFromMessage(message string) string {
	match := openAIResponsesRejectedMessageParamPattern.FindStringSubmatch(strings.TrimSpace(message))
	if len(match) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(match[1]))
}

func openAIResponsesRejectedIndexedField(param string) (int, string, bool) {
	match := openAIResponsesRejectedIndexedParamPattern.FindStringSubmatch(strings.TrimSpace(param))
	if len(match) != 3 {
		return 0, "", false
	}
	index, err := strconv.Atoi(match[1])
	if err == nil && index >= 0 {
		return index, strings.ToLower(match[2]), true
	}
	return 0, "", false
}

func removeOpenAIResponsesRejectedNamespaceAtIndex(body []byte, index int) ([]byte, string, bool, error) {
	itemPath := fmt.Sprintf("input.%d", index)
	itemType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, itemPath+".type").String()))
	switch itemType {
	case "function_call", "tool_call", "custom_tool_call", "mcp_tool_call":
	default:
		return nil, "", false, nil
	}

	namespacePath := itemPath + ".namespace"
	if !gjson.GetBytes(body, namespacePath).Exists() {
		return nil, "", false, nil
	}
	retryBody, err := sjson.DeleteBytes(body, namespacePath)
	if err != nil {
		return nil, "", false, fmt.Errorf("delete rejected namespace at input[%d]: %w", index, err)
	}
	return retryBody, "indexed namespace parameter rejection", true, nil
}
