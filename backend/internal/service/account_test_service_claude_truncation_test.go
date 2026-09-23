package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 自包含上下文：newTestContext 位于 //go:build unit 的测试文件里，而该带标签的套件
// 在共享分支上无法整体编译，因此本文件不依赖它。
func newClaudeTruncationTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/139/test", nil)
	return c, rec
}

// 回归背景：推理模型在输出预算不足时会稳定返回「只有 thinking、没有答案正文」的截断流
// （实测 deepseek-v4.1-flash + max_tokens=1024 → 3760 字符 thinking、0 字符答案、
// stop_reason=max_tokens）。旧实现无条件报 test_complete success=true，
// 操作者看到的是「测试完成 + 空白」，与「正常通过」完全同形。
// 本测试锁住「必须把截断显式说出来」这一行为。
func TestProcessClaudeStream_ThinkingOnlyTruncationIsReported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newClaudeTruncationTestContext()

	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"m1","usage":{"input_tokens":10,"output_tokens":0}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"先分析取法..."}}`,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":1024}}`,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	svc := &AccountTestService{}
	require.NoError(t, svc.processClaudeStream(ctx, strings.NewReader(stream)))

	out := recorder.Body.String()
	require.Contains(t, out, "模型正在推理", "thinking 期间必须播报进度，否则静默会被读成挂死")
	require.Contains(t, out, "max_tokens=1024")
	require.Contains(t, out, "截断")
	require.Contains(t, out, "非协议故障")
	require.NotContains(t, out, `"type":"content"`, "截断样本不应产出答案正文")
	require.Contains(t, out, `"success":true`, "截断不是账号故障，不应报 error")
	require.NotContains(t, out, `"type":"error"`)
}

// 正常产出答案的流不应被误报为截断，且正文照常转发。
func TestProcessClaudeStream_AnsweredStreamIsNotFlaggedAsTruncated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newClaudeTruncationTestContext()

	stream := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"m2","usage":{"input_tokens":10,"output_tokens":0}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"29"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" 个。"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}`,
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")

	svc := &AccountTestService{}
	require.NoError(t, svc.processClaudeStream(ctx, strings.NewReader(stream)))

	out := recorder.Body.String()
	require.Contains(t, out, "29")
	require.NotContains(t, out, "截断")
	require.NotContains(t, out, "模型正在推理")
	require.Contains(t, out, `"success":true`)
}

// 回归：空答案必须在判决行里显式说明「本样本未判定」。
// 旧实现在 answer == "" 时静默 return，界面上与「判定为正确」同形。
func TestEmitReasoningProbeVerdict_NoAnswerIsReportedNotSilentlySkipped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newClaudeTruncationTestContext()

	svc := &AccountTestService{}
	svc.emitReasoningProbeVerdict(ctx, `data: {"type":"status","text":"模型正在推理…"}`+"\n")

	out := recorder.Body.String()
	require.Contains(t, out, "未收到答案正文")
	require.Contains(t, out, "未判定")
	require.Contains(t, out, "thinking")
}

// 空答案说明只针对「没有 content 且没有 error」的情况；真错误仍保持静默（由 error 事件表达）。
func TestEmitReasoningProbeVerdict_ErrorStaysSilent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newClaudeTruncationTestContext()

	svc := &AccountTestService{}
	svc.emitReasoningProbeVerdict(ctx, `data: {"type":"error","error":"upstream 500"}`+"\n")

	require.Empty(t, recorder.Body.String())
}
