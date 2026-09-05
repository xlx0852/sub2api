package handler

import (
	"strings"

	"github.com/tidwall/gjson"
)

// openAIResponsesIngress is the immutable request data shared by admission and
// forwarding. Keeping parsing here prevents transport code from re-reading the
// wire payload and makes the lifecycle boundary explicit.
type openAIResponsesIngress struct {
	model              string
	stream             bool
	previousResponseID string
}

func parseOpenAIResponsesIngress(body []byte) (openAIResponsesIngress, string, bool) {
	if !gjson.ValidBytes(body) {
		return openAIResponsesIngress{}, "Failed to parse request body", false
	}
	model := gjson.GetBytes(body, "model")
	if !model.Exists() || model.Type != gjson.String || model.String() == "" {
		return openAIResponsesIngress{}, "model is required", false
	}
	stream, ok := parseOpenAICompatibleStream(body)
	if !ok {
		return openAIResponsesIngress{}, invalidStreamFieldTypeMessage, false
	}
	return openAIResponsesIngress{
		model:              model.String(),
		stream:             stream,
		previousResponseID: strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()),
	}, "", true
}
