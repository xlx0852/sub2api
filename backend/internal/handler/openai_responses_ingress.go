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
	// 单遍扫描顶层标量：gjson 对「不存在或靠后」的键会走完整包，实测 46MiB 请求上
	// 三次查找合计约 900ms（见 openspec/OBSERVE-2026-09-25-perf-wave.md）。
	scalars := scanOpenAITopLevelScalars(body, "model", "stream", "previous_response_id")
	model, ok := scalars["model"]
	if !ok || model.kind != 's' || model.text == "" {
		return openAIResponsesIngress{}, "model is required", false
	}
	stream := false
	if raw, exists := scalars["stream"]; exists {
		if raw.kind != 'b' {
			return openAIResponsesIngress{}, invalidStreamFieldTypeMessage, false
		}
		stream = raw.truth
	}
	previous := ""
	if raw, exists := scalars["previous_response_id"]; exists && raw.kind == 's' {
		previous = strings.TrimSpace(raw.text)
	}
	return openAIResponsesIngress{
		model:              model.text,
		stream:             stream,
		previousResponseID: previous,
	}, "", true
}
