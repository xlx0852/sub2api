package service

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func testGrokEncryptedContentForSeed(seed byte) string {
	buf := make([]byte, 0, 256)
	for i := 0; len(buf) < 256; i++ {
		sum := sha256.Sum256([]byte{seed, byte(i), byte(i >> 8), byte(i >> 16)})
		buf = append(buf, sum[:]...)
	}
	return base64.RawStdEncoding.EncodeToString(buf[:256])
}

func TestSanitizeGrokInputEncryptedContent_DropsInvalidReasoningBlob(t *testing.T) {
	body := []byte(`{"model":"grok-build-0.1","input":[{"type":"reasoning","summary":[],"encrypted_content":"bad"},{"type":"reasoning","summary":[],"encrypted_content":"gAAAAABinvalid-gpt-shape"},{"role":"user","content":"hi"}]}`)
	got := sanitizeGrokInputEncryptedContent(body)
	require.False(t, gjson.GetBytes(got, "input.0.encrypted_content").Exists())
	require.False(t, gjson.GetBytes(got, "input.1.encrypted_content").Exists())
	require.Equal(t, "hi", gjson.GetBytes(got, "input.2.content").String())
}

func TestSanitizeGrokInputEncryptedContent_PreservesValidBlob(t *testing.T) {
	sample := testGrokEncryptedContentForSeed(7)
	body := []byte(`{"model":"grok-build-0.1","input":[{"type":"reasoning","summary":[],"encrypted_content":""}]}`)
	body, _ = sjson.SetBytes(body, "input.0.encrypted_content", sample)
	got := sanitizeGrokInputEncryptedContent(body)
	require.Equal(t, sample, gjson.GetBytes(got, "input.0.encrypted_content").String())
}

func TestApplyGrokReasoningReplayCache_InjectsCachedReasoning(t *testing.T) {
	grokReasoningReplayMu.Lock()
	grokReasoningReplayEntries = make(map[string]grokReasoningReplayEntry)
	grokReasoningReplayMu.Unlock()

	encrypted := testGrokEncryptedContentForSeed(9)
	cachedItem, _ := sjson.SetBytes([]byte(`{"type":"reasoning","summary":[]}`), "encrypted_content", encrypted)
	require.True(t, cacheGrokReasoningReplayItems("grok-build-0.1", "prompt-cache:session-1", [][]byte{cachedItem}))

	body := []byte(`{"model":"grok-build-0.1","prompt_cache_key":"session-1","input":[{"type":"reasoning","summary":[],"encrypted_content":"gAAAAABcodex-blob"},{"role":"user","content":"continue"}]}`)
	got := applyGrokReasoningReplayCache(body, "grok-build-0.1", nil)
	got = sanitizeGrokInputEncryptedContent(got)
	require.Equal(t, encrypted, gjson.GetBytes(got, "input.0.encrypted_content").String())
	require.Equal(t, "reasoning", gjson.GetBytes(got, "input.0.type").String())
	require.False(t, gjson.GetBytes(got, "input.1.encrypted_content").Exists())
	require.Equal(t, "continue", gjson.GetBytes(got, "input.2.content").String())
}

func TestNormalizeGrokReasoningReplayReasoningItem_SummaryFallback(t *testing.T) {
	item := []byte(`{"type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"The user wants exactly REASONING_LIVE_1."}]}`)
	normalized, ok := normalizeGrokReasoningReplayReasoningItem(gjson.ParseBytes(item))
	require.True(t, ok)
	require.False(t, gjson.GetBytes(normalized, "encrypted_content").Exists())
	require.Equal(t, "summary_text", gjson.GetBytes(normalized, "summary.0.type").String())
	require.Contains(t, gjson.GetBytes(normalized, "summary.0.text").String(), "REASONING_LIVE_1")
}

func TestCacheGrokReasoningReplayFromCompleted_StoresReasoningTextFallback(t *testing.T) {
	grokReasoningReplayMu.Lock()
	grokReasoningReplayEntries = make(map[string]grokReasoningReplayEntry)
	grokReasoningReplayMu.Unlock()

	requestBody := []byte(`{"model":"grok-build-0.1","prompt_cache_key":"session-3","input":[{"role":"user","content":"hi"}]}`)
	completed := []byte(`{"type":"response.completed","response":{"output":[{"type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"remember 42"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"42"}]}]}}`)
	cacheGrokReasoningReplayFromCompleted("grok-build-0.1", requestBody, completed)

	items, ok := getGrokReasoningReplayItems("grok-build-0.1", "prompt-cache:session-3")
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Equal(t, "remember 42", gjson.GetBytes(items[0], "summary.0.text").String())
}

func TestCacheGrokReasoningReplayFromCompleted_StoresOutputItems(t *testing.T) {
	grokReasoningReplayMu.Lock()
	grokReasoningReplayEntries = make(map[string]grokReasoningReplayEntry)
	grokReasoningReplayMu.Unlock()

	encrypted := testGrokEncryptedContentForSeed(11)
	requestBody := []byte(`{"model":"grok-build-0.1","prompt_cache_key":"session-2","input":[{"role":"user","content":"hi"}]}`)
	completed := []byte(`{"type":"response.completed","response":{"output":[{"type":"reasoning","summary":[],"encrypted_content":""},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}}`)
	completed, _ = sjson.SetBytes(completed, "response.output.0.encrypted_content", encrypted)
	cacheGrokReasoningReplayFromCompleted("grok-build-0.1", requestBody, completed)

	items, ok := getGrokReasoningReplayItems("grok-build-0.1", "prompt-cache:session-2")
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Equal(t, encrypted, gjson.GetBytes(items[0], "encrypted_content").String())
}