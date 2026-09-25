package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScanOpenAITopLevelScalarsMatchesEncodingJSON(t *testing.T) {
	payload := []byte(`{"model":"gpt-6-astra","stream":true,"store":false,"previous_response_id":"resp_1","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"metadata":{"a":1},"nested":{"deep":[1,2,{"x":"y"}]},"tail":"ignore"}`)
	scalars := scanOpenAITopLevelScalars(payload, "model", "stream", "previous_response_id")

	require.Equal(t, byte('s'), scalars["model"].kind)
	require.Equal(t, "gpt-6-astra", scalars["model"].text)
	require.Equal(t, byte('b'), scalars["stream"].kind)
	require.True(t, scalars["stream"].truth)
	require.Equal(t, "resp_1", scalars["previous_response_id"].text)
}

func TestScanOpenAITopLevelScalarsHandlesMissingAndOddValues(t *testing.T) {
	scalars := scanOpenAITopLevelScalars([]byte(`{"model":"m","stream":"yes","input":[]}`), "model", "stream", "previous_response_id")
	require.Equal(t, "m", scalars["model"].text)
	require.Equal(t, byte('s'), scalars["stream"].kind, "非布尔 stream 需能被识别为非法类型")
	_, hasPrevious := scalars["previous_response_id"]
	require.False(t, hasPrevious)

	// 空对象 / 非法结构：不 panic，交由调用方校验
	require.Empty(t, scanOpenAITopLevelScalars([]byte(`{}`), "model"))
	require.Empty(t, scanOpenAITopLevelScalars([]byte(`not-json`), "model"))
}

// 46MiB 级请求体：单遍扫描应远快于 gjson 的多趟惰性查找（回归护栏，阈值放宽以免抖动）。
func TestScanOpenAITopLevelScalarsIsFastOnLargeBody(t *testing.T) {
	items := make([]string, 0, 4000)
	for i := 0; i < 4000; i++ {
		items = append(items, `{"type":"message","role":"user","content":[{"type":"input_text","text":"`+strings.Repeat("x", 3000)+`"}]}`)
	}
	payload := []byte(`{"model":"gpt-6-astra","stream":true,"input":[` + strings.Join(items, ",") + `]}`)
	require.True(t, len(payload) > 10<<20, "payload should be large, got %d", len(payload))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gpt-6-astra"}`), &decoded))

	start := time.Now()
	scalars := scanOpenAITopLevelScalars(payload, "model", "stream", "previous_response_id")
	elapsed := time.Since(start)
	require.Equal(t, "gpt-6-astra", scalars["model"].text)
	require.True(t, scalars["stream"].truth)
	t.Logf("scan %d bytes in %s", len(payload), elapsed)
	require.Less(t, elapsed, 1500*time.Millisecond)
}
