package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

func TestAntigravityReasoningReplayCache_RecordAndRetrieve(t *testing.T) {
	cache := NewAntigravityReasoningReplayCache(AntigravityReasoningReplayCacheConfig{
		TTL:        time.Hour,
		MaxEntries: 100,
	})

	sessionKey := "session-test-1"

	// 记录空或 dummy 签名应被忽略
	cache.RecordSignature(sessionKey, "call-1", "tool_a", "")
	cache.RecordSignature(sessionKey, "call-1", "tool_a", antigravity.DummyThoughtSignature)
	_, found := cache.GetFunctionCallSignature(sessionKey, "call-1", "tool_a", 0)
	require.False(t, found, "dummy or empty signature should not be cached")

	// 记录真实签名
	realSig := "EikIAxgCIAIoATIAOAE="
	cache.RecordSignature(sessionKey, "call-1", "tool_a", realSig)

	// 按 callID 查找
	sig, found := cache.GetFunctionCallSignature(sessionKey, "call-1", "", 0)
	require.True(t, found)
	require.Equal(t, realSig, sig)

	// 按 funcName 查找
	sigByName, foundByName := cache.GetFunctionCallSignature(sessionKey, "", "tool_a", 0)
	require.True(t, foundByName)
	require.Equal(t, realSig, sigByName)

	// 获取最新签名
	latest, foundLatest := cache.GetLatestSignature(sessionKey)
	require.True(t, foundLatest)
	require.Equal(t, realSig, latest)
}

func TestAntigravityReasoningReplayCache_MultipleOccurrences(t *testing.T) {
	cache := NewAntigravityReasoningReplayCache()
	sessionKey := "session-multi-tool"

	sig1 := "sig-tool-occurrence-1"
	sig2 := "sig-tool-occurrence-2"

	cache.RecordSignature(sessionKey, "call-1", "execute_command", sig1)
	cache.RecordSignature(sessionKey, "call-2", "execute_command", sig2)

	// 第 0 次出现
	s0, ok0 := cache.GetFunctionCallSignature(sessionKey, "", "execute_command", 0)
	require.True(t, ok0)
	require.Equal(t, sig1, s0)

	// 第 1 次出现
	s1, ok1 := cache.GetFunctionCallSignature(sessionKey, "", "execute_command", 1)
	require.True(t, ok1)
	require.Equal(t, sig2, s1)

	// 超过次数回退至最后一次
	s2, ok2 := cache.GetFunctionCallSignature(sessionKey, "", "execute_command", 5)
	require.True(t, ok2)
	require.Equal(t, sig2, s2)
}

func TestAntigravityReasoningReplayCache_RecordSignaturesFromChunk(t *testing.T) {
	cache := NewAntigravityReasoningReplayCache()
	sessionKey := "session-chunk-test"

	// 1. 普通文本 chunk，不含签名
	cache.RecordSignaturesFromChunk(sessionKey, []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]}}]}`))
	require.Equal(t, 0, cache.Size())

	// 2. 包含思考签名的 chunk
	thoughtChunk := []byte(`{
		"candidates": [{
			"content": {
				"role": "model",
				"parts": [{
					"thought": true,
					"text": "Planning steps...",
					"thoughtSignature": "valid-thought-signature-12345"
				}]
			}
		}]
	}`)
	cache.RecordSignaturesFromChunk(sessionKey, thoughtChunk)
	require.Equal(t, 1, cache.Size())

	latest, found := cache.GetLatestSignature(sessionKey)
	require.True(t, found)
	require.Equal(t, "valid-thought-signature-12345", latest)

	// 3. 包含 functionCall 的 chunk
	fcChunk := []byte(`{
		"candidates": [{
			"content": {
				"role": "model",
				"parts": [{
					"functionCall": {
						"name": "read_file",
						"id": "call-read-01",
						"args": {"path": "main.go"}
					},
					"thoughtSignature": "valid-function-call-signature-67890"
				}]
			}
		}]
	}`)
	cache.RecordSignaturesFromChunk(sessionKey, fcChunk)

	fcSig, foundFC := cache.GetFunctionCallSignature(sessionKey, "call-read-01", "read_file", 0)
	require.True(t, foundFC)
	require.Equal(t, "valid-function-call-signature-67890", fcSig)

	// 4. v1internal 包装的 chunk
	v1WrappedChunk := []byte(`{
		"response": {
			"candidates": [{
				"content": {
					"role": "model",
					"parts": [{
						"functionCall": {
							"name": "write_file",
							"args": {"path": "out.txt"}
						},
						"thoughtSignature": "wrapped-sig-abcde"
					}]
				}
			}]
		}
	}`)
	cache.RecordSignaturesFromChunk(sessionKey, v1WrappedChunk)
	wfSig, foundWF := cache.GetFunctionCallSignature(sessionKey, "", "write_file", 0)
	require.True(t, foundWF)
	require.Equal(t, "wrapped-sig-abcde", wfSig)
}

func TestAntigravityReasoningReplayCache_RestoreSignatures(t *testing.T) {
	cache := NewAntigravityReasoningReplayCache()
	sessionKey := "session-restore-test"

	cachedFCSig := "cached-func-sig-999"
	cachedThoughtSig := "cached-thought-sig-888"

	cache.RecordSignature(sessionKey, "call-1", "tool_weather", cachedFCSig)
	cache.RecordSignature(sessionKey, "", "", cachedThoughtSig)

	// 1. 缺失签名的多轮请求，成功从缓存恢复
	inboundRequest := []byte(`{
		"contents": [
			{"role": "user", "parts": [{"text": "What is the weather?"}]},
			{"role": "model", "parts": [
				{"thought": true, "text": "Checking forecast..."},
				{"functionCall": {"name": "tool_weather", "id": "call-1", "args": {"city": "Tokyo"}}}
			]},
			{"role": "user", "parts": [
				{"functionResponse": {"name": "tool_weather", "response": {"result": "sunny"}}}
			]}
		]
	}`)

	restored := cache.RestoreSignatures(sessionKey, inboundRequest)
	require.NotEqual(t, inboundRequest, restored)

	var parsed map[string]any
	err := json.Unmarshal(restored, &parsed)
	require.NoError(t, err)

	contents := parsed["contents"].([]any)
	modelParts := contents[1].(map[string]any)["parts"].([]any)

	// 验证思考部分签名恢复
	thoughtPart := modelParts[0].(map[string]any)
	require.Equal(t, cachedThoughtSig, thoughtPart["thoughtSignature"])

	// 验证工具调用签名恢复
	fcPart := modelParts[1].(map[string]any)
	require.Equal(t, cachedFCSig, fcPart["thoughtSignature"])

	// 2. 未缓存的全新会话，应 fallback 为 skip_thought_signature_validator
	unknownSession := "unknown-session"
	unboundRequest := []byte(`{
		"contents": [
			{"role": "user", "parts": [{"text": "Run tool"}]},
			{"role": "model", "parts": [
				{"functionCall": {"name": "execute", "args": {"cmd": "ls"}}}
			]}
		]
	}`)

	fallbackRestored := cache.RestoreSignatures(unknownSession, unboundRequest)
	require.Contains(t, string(fallbackRestored), antigravity.DummyThoughtSignature)

	// 3. 已有合法原生签名的请求不被覆盖，并被学习入缓存
	nativeSig := "native-client-existing-sig"
	nativeRequest := []byte(`{
		"contents": [
			{"role": "user", "parts": [{"text": "Run tool"}]},
			{"role": "model", "parts": [
				{"functionCall": {"name": "native_tool", "id": "native-1", "args": {}}, "thoughtSignature": "` + nativeSig + `"}
			]}
		]
	}`)

	restoredNative := cache.RestoreSignatures("new-learned-session", nativeRequest)
	require.Contains(t, string(restoredNative), nativeSig)
	require.NotContains(t, string(restoredNative), antigravity.DummyThoughtSignature)

	// 验证已被学习
	learned, foundLearned := cache.GetFunctionCallSignature("new-learned-session", "native-1", "native_tool", 0)
	require.True(t, foundLearned)
	require.Equal(t, nativeSig, learned)

	// 4. v1internal 包装的 request.contents 格式恢复
	wrappedRequest := []byte(`{
		"request": {
			"contents": [
				{"role": "model", "parts": [
					{"functionCall": {"name": "tool_weather", "id": "call-1", "args": {}}}
				]}
			]
		}
	}`)
	restoredWrapped := cache.RestoreSignatures(sessionKey, wrappedRequest)
	require.Contains(t, string(restoredWrapped), cachedFCSig)
}

func TestAntigravityReasoningReplayCache_TTLExpirationAndCapacity(t *testing.T) {
	// TTL 测试
	shortTTL := 20 * time.Millisecond
	cache := NewAntigravityReasoningReplayCache(AntigravityReasoningReplayCacheConfig{
		TTL:        shortTTL,
		MaxEntries: 2,
	})

	cache.RecordSignature("s1", "c1", "f1", "sig-1")
	time.Sleep(30 * time.Millisecond)

	_, found := cache.GetFunctionCallSignature("s1", "c1", "f1", 0)
	require.False(t, found, "expired session should not be found")

	// 容量淘汰测试
	cacheCap := NewAntigravityReasoningReplayCache(AntigravityReasoningReplayCacheConfig{
		TTL:        time.Hour,
		MaxEntries: 2,
	})
	cacheCap.RecordSignature("s1", "c1", "f1", "sig-1")
	cacheCap.RecordSignature("s2", "c2", "f2", "sig-2")
	require.Equal(t, 2, cacheCap.Size())

	// 插入第 3 个触发淘汰
	cacheCap.RecordSignature("s3", "c3", "f3", "sig-3")
	require.LessOrEqual(t, cacheCap.Size(), 2)

	// Clear 测试
	cacheCap.Clear("s3")
	_, foundS3 := cacheCap.GetLatestSignature("s3")
	require.False(t, foundS3)
}

func TestDeriveGeminiReplayKeyFromBody(t *testing.T) {
	// 1. 来自 sessionId
	key1 := deriveGeminiReplayKeyFromBody([]byte(`{"sessionId": "sess-abc"}`))
	require.Equal(t, "session:sess-abc", key1)

	// 2. 来自 session_id
	key2 := deriveGeminiReplayKeyFromBody([]byte(`{"session_id": "sess-xyz"}`))
	require.Equal(t, "session:sess-xyz", key2)

	// 3. 来自首个 user turn 文本计算稳定的 prompt hash
	body := []byte(`{
		"contents": [
			{"role": "user", "parts": [{"text": "Hello world assistant"}]}
		]
	}`)
	key3 := deriveGeminiReplayKeyFromBody(body)
	require.True(t, len(key3) > len("prompt:"))

	// 相同输入产生相同 prompt hash
	key4 := deriveGeminiReplayKeyFromBody(body)
	require.Equal(t, key3, key4)
}
