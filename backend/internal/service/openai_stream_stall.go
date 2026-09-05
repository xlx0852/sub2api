package service

import (
	"encoding/json"
	"strconv"
)

const (
	OpenAIStreamStallReason = "upstream_stall_timeout"
)

// buildOpenAIResponsesIncompleteEvent builds a Responses-compatible incomplete terminal event.
func buildOpenAIResponsesIncompleteEvent(responseID, reason string) string {
	type incompleteDetails struct {
		Reason string `json:"reason"`
	}
	type responseBody struct {
		ID                string            `json:"id,omitempty"`
		Object            string            `json:"object"`
		Status            string            `json:"status"`
		IncompleteDetails incompleteDetails `json:"incomplete_details"`
	}
	type eventBody struct {
		Type           string       `json:"type"`
		SequenceNumber int          `json:"sequence_number"`
		Response       responseBody `json:"response"`
	}
	event := eventBody{
		Type:           "response.incomplete",
		SequenceNumber: 0,
		Response: responseBody{
			ID:     responseID,
			Object: "response",
			Status: "incomplete",
			IncompleteDetails: incompleteDetails{
				Reason: reason,
			},
		},
	}
	raw, err := json.Marshal(event)
	if err != nil {
		// Fallback minimal payload; reason is already constrained.
		return `data: {"type":"response.incomplete","sequence_number":0,"response":{"object":"response","status":"incomplete","incomplete_details":{"reason":` + strconv.Quote(reason) + `}}}` + "\n\n"
	}
	return "data: " + string(raw) + "\n\n"
}
