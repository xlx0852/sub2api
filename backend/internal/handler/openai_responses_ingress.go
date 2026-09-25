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

// openAIResponsesIngressFullValidationMaxBytes 是执行整包 JSON 校验的体积上限。
//
// gjson.ValidBytes 要完整走一遍 JSON：实测 42MiB 的 Responses 包里它单独占约
// 850ms（见 openspec/OBSERVE-2026-09-25-perf-wave.md）。超过该阈值时改为惰性校验：
// 只确认能取到非空字符串 model（下面的逻辑），格式非法会在后续解析或上游被拒绝。
const openAIResponsesIngressFullValidationMaxBytes = 4 << 20

func parseOpenAIResponsesIngress(body []byte) (openAIResponsesIngress, string, bool) {
	if len(body) <= openAIResponsesIngressFullValidationMaxBytes && !gjson.ValidBytes(body) {
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
