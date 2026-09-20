package service

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// Gemini tool calling now requires `thoughtSignature` in parts that include `functionCall`.
// Many clients don't send it; we inject a known dummy signature to satisfy the validator.
// Ref: https://ai.google.dev/gemini-api/docs/thought-signatures
const geminiDummyThoughtSignature = "skip_thought_signature_validator"

var (
	sensitiveQueryParamRegex = regexp.MustCompile(`(?i)([?&](?:key|client_secret|access_token|refresh_token)=)[^&"\s]+`)
	retryInRegex             = regexp.MustCompile(`Please retry in ([0-9.]+)s`)
)

func sanitizeUpstreamErrorMessage(msg string) string {
	if msg == "" {
		return msg
	}
	// Never keep raw Cloudflare/proxy HTML pages in client or ops messages.
	if summarized := summarizeNonJSONUpstreamErrorBody([]byte(msg)); summarized != "" {
		return summarized
	}
	return sensitiveQueryParamRegex.ReplaceAllString(msg, `$1***`)
}

func pickGeminiCollectResult(last map[string]any, lastWithParts map[string]any) map[string]any {
	if lastWithParts != nil {
		return lastWithParts
	}
	if last != nil {
		return last
	}
	return map[string]any{}
}

func extractGeminiUsage(data []byte) *ClaudeUsage {
	usage := gjson.GetBytes(data, "usageMetadata")
	if !usage.Exists() {
		return nil
	}
	prompt := int(usage.Get("promptTokenCount").Int())
	cand := int(usage.Get("candidatesTokenCount").Int())
	cached := int(usage.Get("cachedContentTokenCount").Int())
	thoughts := int(usage.Get("thoughtsTokenCount").Int())

	imageTokens := 0
	candidateDetails := usage.Get("candidatesTokensDetails")
	if candidateDetails.Exists() {
		candidateDetails.ForEach(func(_, detail gjson.Result) bool {
			if detail.Get("modality").String() == "IMAGE" {
				imageTokens = int(detail.Get("tokenCount").Int())
				return false
			}
			return true
		})
	}

	return &ClaudeUsage{
		InputTokens:          prompt - cached,
		OutputTokens:         cand + thoughts,
		CacheReadInputTokens: cached,
		ImageOutputTokens:    imageTokens,
	}
}

// ParseGeminiRateLimitResetTime 解析 Gemini 格式的 429 响应，返回重置时间的 Unix 时间戳
func ParseGeminiRateLimitResetTime(body []byte) *int64 {
	errMsg := gjson.GetBytes(body, "error.message").String()
	if looksLikeGeminiDailyQuota(errMsg) {
		if ts := nextGeminiDailyResetUnix(); ts != nil {
			return ts
		}
	}

	var found *int64
	gjson.GetBytes(body, "error.details").ForEach(func(_, detail gjson.Result) bool {
		v := detail.Get("metadata.quotaResetDelay").String()
		if v == "" {
			return true
		}
		if dur, err := time.ParseDuration(v); err == nil {
			ts := time.Now().Unix() + int64(math.Ceil(dur.Seconds()))
			found = &ts
			return false
		}
		return true
	})
	if found != nil {
		return found
	}

	matches := retryInRegex.FindStringSubmatch(string(body))
	if len(matches) == 2 {
		if dur, err := time.ParseDuration(matches[1] + "s"); err == nil {
			ts := time.Now().Unix() + int64(math.Ceil(dur.Seconds()))
			return &ts
		}
	}

	return nil
}

func looksLikeGeminiDailyQuota(message string) bool {
	m := strings.ToLower(message)
	return strings.Contains(m, "per day") || strings.Contains(m, "requests per day") || (strings.Contains(m, "quota") && strings.Contains(m, "per day"))
}

func nextGeminiDailyResetUnix() *int64 {
	reset := geminiDailyResetTime(time.Now())
	ts := reset.Unix()
	return &ts
}

func ensureGeminiFunctionCallThoughtSignatures(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"functionCall"`)) {
		return body
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	contentsAny, ok := payload["contents"].([]any)
	if !ok || len(contentsAny) == 0 {
		return body
	}

	modified := false
	for _, c := range contentsAny {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		partsAny, ok := cm["parts"].([]any)
		if !ok || len(partsAny) == 0 {
			continue
		}
		for _, p := range partsAny {
			pm, ok := p.(map[string]any)
			if !ok || pm == nil {
				continue
			}
			if fc, ok := pm["functionCall"].(map[string]any); !ok || fc == nil {
				continue
			}
			ts, _ := pm["thoughtSignature"].(string)
			if strings.TrimSpace(ts) == "" {
				pm["thoughtSignature"] = geminiDummyThoughtSignature
				modified = true
			}
		}
	}

	if !modified {
		return body
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return b
}

func extractGeminiParts(geminiResp map[string]any) []map[string]any {
	if candidates, ok := geminiResp["candidates"].([]any); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]any); ok {
			if content, ok := cand["content"].(map[string]any); ok {
				if partsAny, ok := content["parts"].([]any); ok && len(partsAny) > 0 {
					out := make([]map[string]any, 0, len(partsAny))
					for _, p := range partsAny {
						pm, ok := p.(map[string]any)
						if !ok {
							continue
						}
						out = append(out, pm)
					}
					return out
				}
			}
		}
	}
	return nil
}
