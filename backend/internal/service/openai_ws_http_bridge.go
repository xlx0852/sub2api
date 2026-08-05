package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIWSClientReadLimitBytesDefault     int64 = 64 * 1024 * 1024
	openAIWSHTTPBridgeThresholdBytesDefault int64 = 15 * 1024 * 1024
	openAIWSHTTPBridgeErrorBodyLimitBytes         = 64 * 1024
	// Hard caps for WS→HTTP bridge streams. Idle timeout alone is not enough
	// when an upstream keeps emitting token events without ever completing.
	openAIWSHTTPBridgeMaxTokenEventsDefault = 80_000
	openAIWSHTTPBridgeMaxDurationDefault    = 8 * time.Minute
)

// ResolveOpenAIWSClientFirstMessageTimeout returns the effective client ingress deadline.
func ResolveOpenAIWSClientFirstMessageTimeout(cfg *config.Config) time.Duration {
	seconds := config.DefaultOpenAIWSClientFirstMessageTimeoutSeconds
	if cfg != nil && cfg.Gateway.OpenAIWS.ClientFirstMessageTimeoutSeconds > 0 {
		seconds = cfg.Gateway.OpenAIWS.ClientFirstMessageTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func ResolveOpenAIWSClientReadLimitBytes(cfg *config.Config) int64 {
	if cfg == nil || cfg.Gateway.OpenAIWS.ClientReadLimitBytes <= 0 {
		return openAIWSClientReadLimitBytesDefault
	}
	return cfg.Gateway.OpenAIWS.ClientReadLimitBytes
}

func (s *OpenAIGatewayService) openAIWSHTTPBridgeEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled
}

func (s *OpenAIGatewayService) openAIWSHTTPBridgeThresholdBytes() int64 {
	if s == nil || s.cfg == nil || s.cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes <= 0 {
		return openAIWSHTTPBridgeThresholdBytesDefault
	}
	return s.cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes
}

func (s *OpenAIGatewayService) shouldBridgeOpenAIWSHTTP(account *Account, payloadBytes int, previousResponseID string) bool {
	if !s.openAIWSHTTPBridgeEnabled() {
		return false
	}
	if strings.TrimSpace(previousResponseID) != "" {
		return false
	}
	threshold := s.openAIWSHTTPBridgeThresholdBytes()
	return threshold > 0 && int64(payloadBytes) >= threshold
}

func prepareOpenAIWSHTTPBridgeBody(payload []byte) ([]byte, error) {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("response.create payload must be a JSON object")
	}
	delete(body, "type")
	delete(body, "generate")
	delete(body, "previous_response_id")
	// Codex treats call_id as an opaque association key and emits native
	// custom_tool_call/custom_tool_call_output pairs. Keep the client payload
	// byte-for-byte semantic here. Compatibility adaptation is applied only to
	// server-collected replay items, or after an explicit upstream rejection.
	body["stream"] = true
	return json.Marshal(body)
}

// bridgeCallIDMapper 维护桥接 turn 内 codex call_id ↔ 上游 call_id 的映射。
// codex 客户端用 call_id 把 custom_tool_call 与 custom_tool_call_output 配对；
// OpenAI 上游执行工具会自生成新的 fc_ call_id（不 echo 请求里的），导致 codex
// 拿 fc_ 匹配不到它记录的 ctco_ 报 orphan。这里按工具名配对：请求里 codex 的
// (name → codex call_id) 队列，与上游响应里 (name → 上游 call_id) 顺序配对，
// 把上游 call_id 还原成 codex call_id 回传。
type bridgeCallIDMapper struct {
	// codexByCallName: name -> 请求里按出现顺序的 codex call_id 队列
	codexByCallName map[string][]string
	// upstreamToCodex: 上游 fc_ call_id -> codex call_id（响应里逐事件累积）
	upstreamToCodex map[string]string
}

func newBridgeCallIDMapper(body []byte) *bridgeCallIDMapper {
	m := &bridgeCallIDMapper{
		codexByCallName: make(map[string][]string),
		upstreamToCodex: make(map[string]string),
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return m
	}
	items, _ := parsed["input"].([]any)

	// 第一遍：收集已有配对的 output call_id。历史重放进来的 function_call
	// 一定带对应的 function_call_output（重放收集器成对注入）；这些是已执行
	// 完的调用，上游响应里不会再出现它们的 function_call，不能进配对队列。
	outputCallIDs := make(map[string]struct{})
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := item["type"].(string)
		if !isOpenAIReplayCallOutputType(typ) {
			continue
		}
		if callID, _ := item["call_id"].(string); callID != "" {
			outputCallIDs[callID] = struct{}{}
		}
	}

	// 第二遍：只收集本轮新增（无配对 output）的调用入队。
	// codex 真实场景下工具名可能全部相同（如 exec），name 队列只能靠
	// "本轮新增"过滤 + 上游按序返回 function_call 来配对。
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := item["type"].(string)
		if !isCodexToolCallContextItemType(typ) {
			continue
		}
		callID, _ := item["call_id"].(string)
		if callID == "" {
			continue
		}
		if _, hasOutput := outputCallIDs[callID]; hasOutput {
			// 已配对：历史调用，跳过
			continue
		}
		name, _ := item["name"].(string)
		if name == "" {
			continue
		}
		m.codexByCallName[name] = append(m.codexByCallName[name], callID)
	}
	return m
}

// observeUpstreamCall 记录上游响应里的 function_call（name + 上游 call_id），
// 按 name 顺序配对到请求里的 codex call_id。
func (m *bridgeCallIDMapper) observeUpstreamCall(name, upstreamCallID string) {
	if m == nil || name == "" || upstreamCallID == "" {
		return
	}
	queue := m.codexByCallName[name]
	if len(queue) == 0 {
		return
	}
	codexCallID := queue[0]
	m.codexByCallName[name] = queue[1:]
	if _, exists := m.upstreamToCodex[upstreamCallID]; !exists {
		m.upstreamToCodex[upstreamCallID] = codexCallID
	}
}

// restoreCallID 把上游 call_id 还原为 codex call_id；无映射时原样返回。
func (m *bridgeCallIDMapper) restoreCallID(upstreamCallID string) string {
	if m == nil {
		return upstreamCallID
	}
	if codexCallID, ok := m.upstreamToCodex[upstreamCallID]; ok {
		return codexCallID
	}
	return upstreamCallID
}

// restoreBridgeResponseCallIDs 改写响应 SSE 事件里 function_call /
// function_call_output 的 call_id，使其匹配 codex 客户端记录的 codex call_id。
// function_call 事件：先按 name 建立上游→codex 映射，再把 call_id 还原（客户端
// 历史与下一轮 custom_tool_call_output 都认 codex call_id）。function_call_output
// 事件按已建立的映射还原。返回 (改写后的 bytes, 是否变化)。
func (m *bridgeCallIDMapper) restoreBridgeResponseCallIDs(msg []byte) ([]byte, bool) {
	if m == nil || len(msg) == 0 {
		return msg, false
	}
	itemType := gjson.GetBytes(msg, "item.type").String()
	itemPath := "item"
	if itemType == "" {
		itemType = gjson.GetBytes(msg, "response.output.0.type").String()
		if itemType != "" {
			itemPath = "response.output.0"
		}
	}
	switch itemType {
	case "function_call",
		"custom_tool_call",
		"local_shell_call",
		"mcp_tool_call",
		"tool_call",
		"tool_search_call":
		name := gjson.GetBytes(msg, itemPath+".name").String()
		callID := gjson.GetBytes(msg, itemPath+".call_id").String()
		if name == "" || callID == "" {
			// Terminal snapshot may only carry output[] without a name on every
			// path; still try restore when mapping already exists.
			if callID == "" {
				return msg, false
			}
			codexCallID := m.restoreCallID(callID)
			if codexCallID == callID {
				return msg, false
			}
			return bytes.Replace(msg, []byte(callID), []byte(codexCallID), 1), true
		}
		m.observeUpstreamCall(name, callID)
		codexCallID := m.restoreCallID(callID)
		if codexCallID == callID {
			return msg, false
		}
		return bytes.Replace(msg, []byte(callID), []byte(codexCallID), 1), true
	case "function_call_output",
		"custom_tool_call_output",
		"local_shell_call_output",
		"mcp_tool_call_output",
		"tool_search_output":
		callID := gjson.GetBytes(msg, itemPath+".call_id").String()
		if callID == "" {
			return msg, false
		}
		codexCallID := m.restoreCallID(callID)
		if codexCallID == callID {
			return msg, false
		}
		return bytes.Replace(msg, []byte(callID), []byte(codexCallID), 1), true
	default:
		// response.completed may pack many output items; rewrite every mapped call_id.
		if !gjson.GetBytes(msg, "response.output").IsArray() || len(m.upstreamToCodex) == 0 {
			return msg, false
		}
		changed := false
		out := msg
		for upstreamCallID, codexCallID := range m.upstreamToCodex {
			if upstreamCallID == "" || codexCallID == "" || upstreamCallID == codexCallID {
				continue
			}
			if !bytes.Contains(out, []byte(upstreamCallID)) {
				continue
			}
			out = bytes.ReplaceAll(out, []byte(upstreamCallID), []byte(codexCallID))
			changed = true
		}
		return out, changed
	}
}

// normalizeOpenAIWSBridgeToolTypes 把 codex 私有工具类型归一化为 OpenAI
// Responses 认识的 function_call/function_call_output。多轮重放会把历史里的
// local_shell_call/custom_tool_call/mcp_tool_call/tool_call 等原样注入下一轮
// input，OpenAI 官方上游不认识这些私有 type，会报
// "Unknown parameter: 'input[N].arguments'"；同时这些私有项的 id（lc_/ctco_/
// tsc_ 前缀）不满足 OpenAI 对 function_call* 的 fc_ 前缀要求，会报
// "Invalid 'input[N].id': ... Expected an ID that begins with 'fc'"。
// 归一化后 type 合法、id 满足前缀、arguments 保留，上游可正常接受。
// call_id 是 function_call 与 function_call_output 的关联键，保持不变。
func normalizeOpenAIWSBridgeToolTypes(body map[string]any) {
	rawInput, ok := body["input"]
	if !ok {
		return
	}
	items, ok := rawInput.([]any)
	if !ok {
		return
	}
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := item["type"].(string)
		switch typ {
		case "local_shell_call", "custom_tool_call", "mcp_tool_call", "tool_call", "tool_search_call":
			// custom/freeform 工具的 input 自由文本包进 {"input": ...} 参数
			// （与 chatcompletions bridge 的降级一致）。
			if raw, has := item["input"]; has && typ == "custom_tool_call" {
				encoded, _ := json.Marshal(map[string]any{"input": raw})
				item["arguments"] = string(encoded)
				delete(item, "input")
			}
			item["type"] = "function_call"
			normalizeOpenAIWSBridgeItemID(item)
		case "local_shell_call_output", "custom_tool_call_output", "mcp_tool_call_output", "tool_search_output":
			item["type"] = "function_call_output"
			normalizeOpenAIWSBridgeItemID(item)
		}
	}
}

// normalizeOpenAIWSBridgeItemID 把 codex 私有前缀 id 改为 fc_ 前缀，满足
// OpenAI Responses 对 function_call / function_call_output 项的 id 校验
// （Expected an ID that begins with 'fc'）。call_id 不动。
func normalizeOpenAIWSBridgeItemID(item map[string]any) {
	id, _ := item["id"].(string)
	if id == "" || strings.HasPrefix(id, "fc_") {
		return
	}
	for _, prefix := range []string{"lc_", "ctco_", "ctc_", "tsco_", "tsc_", "mcp_", "call_", "sh_", "ct_", "ts_"} {
		if strings.HasPrefix(id, prefix) {
			id = strings.TrimPrefix(id, prefix)
			break
		}
	}
	item["id"] = "fc_" + id
}

// ensureOpenAIWSBridgeInputArguments 兜底多轮会话重放时 function_call 系列项的
// arguments 字段。上游 OpenAI 对每个 tool 调用项都要求 arguments（缺失或空会
// 返回 400 "Missing required parameter: 'input[N].arguments'"）。重放路径会把
// 上一轮从 response.output_item.done 抓到的 function_call 项原样注入下一轮
// input，若该事件抓到的是分片中间态（arguments 尚未到齐），就会原样转发触发
// 400。此处对缺失或空字符串的 arguments 统一补 "{}"，对象形态（新版 codex）
// 保持原样。
func ensureOpenAIWSBridgeInputArguments(body map[string]any) {
	rawInput, ok := body["input"]
	if !ok {
		return
	}
	items, ok := rawInput.([]any)
	if !ok {
		return
	}
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := item["type"].(string)
		if !isCodexToolCallContextItemType(typ) {
			continue
		}
		args, has := item["arguments"]
		if !has {
			item["arguments"] = "{}"
			continue
		}
		if s, ok := args.(string); ok && strings.TrimSpace(s) == "" {
			item["arguments"] = "{}"
		}
	}
}

type openAIWSToolCallReplayCollector struct {
	items            []json.RawMessage
	seen             map[string]struct{}
	terminalSnapshot bool
}

func (c *openAIWSToolCallReplayCollector) AddEvent(eventType string, message []byte) {
	switch strings.TrimSpace(eventType) {
	case "response.output_item.done":
		if c.terminalSnapshot {
			return
		}
		c.addItem(gjson.GetBytes(message, "item"))
	case "response.completed", "response.done":
		output := gjson.GetBytes(message, "response.output")
		if !output.IsArray() {
			return
		}
		// Prefer the terminal snapshot when it is a real, complete output list.
		// ChatGPT/Codex streams often emit full custom_tool_call items on
		// output_item.done, then a completed event whose output is empty or
		// omits tool calls. Wholesale-clearing here drops the only copy of the
		// call; the next http_bridge turn then ships a bare
		// custom_tool_call_output and upstream returns
		// "No tool call found for custom tool call output".
		terminal := &openAIWSToolCallReplayCollector{}
		for _, item := range output.Array() {
			terminal.addItem(item)
		}
		if len(output.Array()) == 0 {
			// Empty terminal output is not authoritative for tool replay.
			// Keep candidates collected from output_item.done.
			c.terminalSnapshot = true
			return
		}
		if openAIWSReplayCollectorToolCallCount(terminal) == 0 &&
			openAIWSReplayCollectorToolCallCount(c) > 0 {
			// Terminal kept messages/reasoning but lost tool calls. Merge the
			// previously collected tool-call items after the terminal snapshot
			// so call/output pairing survives into the next bridge turn.
			merged := &openAIWSToolCallReplayCollector{}
			for _, item := range terminal.items {
				merged.addItem(gjson.ParseBytes(item))
			}
			for _, item := range c.items {
				if !isCodexToolCallContextItemType(strings.TrimSpace(gjson.GetBytes(item, "type").String())) {
					continue
				}
				merged.addItem(gjson.ParseBytes(item))
			}
			c.items = merged.items
			c.seen = merged.seen
			c.terminalSnapshot = true
			return
		}
		c.items = terminal.items
		c.seen = terminal.seen
		c.terminalSnapshot = true
	}
}

func openAIWSReplayCollectorToolCallCount(c *openAIWSToolCallReplayCollector) int {
	if c == nil {
		return 0
	}
	count := 0
	for _, item := range c.items {
		if isCodexToolCallContextItemType(strings.TrimSpace(gjson.GetBytes(item, "type").String())) {
			count++
		}
	}
	return count
}

func (c *openAIWSToolCallReplayCollector) Items() []json.RawMessage {
	return cloneOpenAIWSRawMessages(c.items)
}

// addItem keeps prior-turn output items that must be re-injected into the next
// HTTP bridge request when the bridge cannot rely on server-side response state.
func (c *openAIWSToolCallReplayCollector) addItem(item gjson.Result) {
	if !item.Exists() || item.Type != gjson.JSON {
		return
	}
	raw := strings.TrimSpace(item.Raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return
	}
	itemType := strings.TrimSpace(item.Get("type").String())
	switch {
	case isCodexToolCallContextItemType(itemType):
		// keep as-is
	case itemType == "message" || (itemType == "" && strings.EqualFold(item.Get("role").String(), "assistant")):
		// Always normalize so assistant content parts become output_text.
		// If normalization fails (empty/unusable content), drop the item rather
		// than replaying the original shape (which previously caused 400s).
		normalized := normalizeOpenAIWSReplayMessageItem(item)
		if len(normalized) == 0 {
			return
		}
		raw = string(normalized)
	case itemType == "reasoning":
		normalized := normalizeOpenAIWSReplayReasoningItem(item)
		if len(normalized) == 0 {
			return
		}
		raw = string(normalized)
	default:
		return
	}
	key := strings.TrimSpace(item.Get("id").String())
	if key == "" {
		key = strings.TrimSpace(item.Get("call_id").String())
	}
	if key == "" {
		key = raw
	}
	if c.seen == nil {
		c.seen = make(map[string]struct{})
	}
	if _, ok := c.seen[key]; ok {
		return
	}
	c.seen[key] = struct{}{}
	c.items = append(c.items, json.RawMessage(raw))
}

func openAIWSReplayContentPartType(role string) string {
	// OpenAI Responses input items: assistant/tool content parts must use
	// output_text/refusal; user/system use input_text. Replaying assistant
	// turns with input_text yields 400:
	//   Invalid value: 'input_text'. Supported values are: 'output_text' and 'refusal'.
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "tool":
		return "output_text"
	default:
		return "input_text"
	}
}

func normalizeOpenAIWSReplayMessageItem(item gjson.Result) []byte {
	role := strings.TrimSpace(item.Get("role").String())
	if role == "" {
		role = "assistant"
	}
	partType := openAIWSReplayContentPartType(role)
	textParts := make([]map[string]any, 0, 2)
	content := item.Get("content")
	if content.IsArray() {
		for _, part := range content.Array() {
			text := strings.TrimSpace(firstNonEmpty(
				part.Get("text").String(),
				part.Get("output_text").String(),
				part.Get("input_text").String(),
			))
			if text == "" {
				continue
			}
			textParts = append(textParts, map[string]any{
				"type": partType,
				"text": text,
			})
		}
	} else if content.Type == gjson.String {
		if text := strings.TrimSpace(content.String()); text != "" {
			textParts = append(textParts, map[string]any{
				"type": partType,
				"text": text,
			})
		}
	}
	if len(textParts) == 0 {
		return nil
	}
	encoded, err := json.Marshal(map[string]any{
		"type":    "message",
		"role":    role,
		"content": textParts,
	})
	if err != nil {
		return nil
	}
	return encoded
}

func normalizeOpenAIWSReplayReasoningItem(item gjson.Result) []byte {
	texts := make([]string, 0, 2)
	summary := item.Get("summary")
	if summary.IsArray() {
		for _, part := range summary.Array() {
			text := strings.TrimSpace(firstNonEmpty(part.Get("text").String(), part.Get("summary_text").String()))
			if text != "" {
				texts = append(texts, text)
			}
		}
	}
	if len(texts) == 0 {
		return nil
	}
	// Reasoning is always re-emitted as an assistant note for input replay.
	encoded, err := json.Marshal(map[string]any{
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{{
			"type": openAIWSReplayContentPartType("assistant"),
			"text": "[reasoning] " + strings.Join(texts, "\n"),
		}},
	})
	if err != nil {
		return nil
	}
	return encoded
}

func buildOpenAIWSHTTPBridgeErrorEvent(statusCode int, message string) []byte {
	message = strings.TrimSpace(message)
	if message == "" {
		message = http.StatusText(statusCode)
	}
	if message == "" {
		message = "upstream request failed"
	}
	event := map[string]any{
		"type":   "error",
		"status": statusCode,
		"error": map[string]any{
			"type":    "upstream_error",
			"message": message,
		},
	}
	body, err := json.Marshal(event)
	if err != nil {
		return []byte(`{"type":"error","error":{"type":"upstream_error","message":"upstream request failed"}}`)
	}
	return body
}

func openAIWSHTTPBridgeUpstreamContext(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	detachedCtx, releaseDetachedCtx := detachUpstreamContext(ctx)
	deadlineCtx, cancelDeadline := context.WithDeadline(detachedCtx, deadline)
	return deadlineCtx, func() {
		cancelDeadline()
		releaseDetachedCtx()
	}
}

func (s *OpenAIGatewayService) proxyOpenAIWSHTTPBridgeTurn(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	payload []byte,
	payloadBytes int,
	originalModel string,
	imageBillingModel string,
	imageSizeTier string,
	imageInputSize string,
	turn int,
	writeClientMessage func([]byte) error,
) (*OpenAIForwardResult, error) {
	if s == nil {
		return nil, errors.New("service is nil")
	}
	if s.httpUpstream == nil {
		return nil, errors.New("openai http upstream is nil")
	}
	if account == nil {
		return nil, errors.New("account is nil")
	}
	if writeClientMessage == nil {
		return nil, errors.New("client websocket writer is nil")
	}

	body, err := prepareOpenAIWSHTTPBridgeBody(payload)
	if err != nil {
		return nil, fmt.Errorf("prepare http bridge body: %w", err)
	}
	// 构建 codex call_id ↔ 上游 call_id 映射（本 turn 内）。客户端 payload 保持
	// 原样（prepare 不做归一化），但 codex 的 custom_tool_call 等私有类型在
	// 重放路径会被归一化为 function_call；无论哪种形态，上游执行工具后都会
	// 自生成新的 fc_ call_id（不 echo 请求里的），导致 codex 客户端用 fc_ 匹配
	// 不到它记录的 ctco_ 报 orphan。这里按工具名把上游 call_id 还原为 codex call_id。
	callIDMapper := newBridgeCallIDMapper(body)
	// HTTP bridge talks to ChatGPT over HTTP SSE, which does not accept native
	// Codex namespace tools. Reuse the Forward-path flatten/restore so WS clients
	// on http_bridge stay compatible with multi-turn namespace tool calls.
	if account.Type == AccountTypeOAuth {
		body, err = flattenOpenAIResponsesNamespaces(c, body)
		if err != nil {
			return nil, fmt.Errorf("flatten http bridge namespace tools: %w", err)
		}
	}

	turnStart := time.Now()
	maxDuration := openAIWSHTTPBridgeMaxDurationDefault
	streamDeadline := turnStart.Add(maxDuration)
	upstreamCtx, releaseUpstreamCtx := openAIWSHTTPBridgeUpstreamContext(ctx, streamDeadline)
	defer releaseUpstreamCtx()
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	if c != nil {
		c.Set("openai_passthrough", true)
		c.Set("openai_ws_http_bridge", true)
	}

	rejectedFieldRetryState := newOpenAIResponsesRejectedFieldRetryState(body)
	var resp *http.Response
	for {
		upstreamReq, buildErr := s.buildUpstreamRequestOpenAIPassthrough(upstreamCtx, c, account, body, token)
		if buildErr != nil {
			return nil, buildErr
		}
		if account.Platform != PlatformGrok && isOpenAIResponsesLiteWebSocketPayload(payload) {
			upstreamReq.Header.Set(responsesLiteHeader, "true")
		}

		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			safeErr := sanitizeUpstreamErrorMessage(err.Error())
			statusCode := http.StatusBadGateway
			clientMessage := "Upstream request failed"
			if errors.Is(err, context.DeadlineExceeded) {
				statusCode = http.StatusGatewayTimeout
				clientMessage = "Upstream response timed out"
			}
			_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(statusCode, clientMessage))
			return nil, fmt.Errorf("upstream http bridge request failed: %s", safeErr)
		}
		if resp.StatusCode < http.StatusBadRequest {
			break
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, openAIWSHTTPBridgeErrorBodyLimitBytes))
		_ = resp.Body.Close()
		retryBody, reason, changed, retryErr := normalizeOpenAIResponsesRejectedFieldRetryBody(resp.StatusCode, body, respBody)
		if retryErr != nil {
			return nil, fmt.Errorf("normalize http bridge compatibility retry body: %w", retryErr)
		}
		if changed && rejectedFieldRetryState.AllowForReason(retryBody, reason) {
			body = retryBody
			logOpenAIWSModeInfo(
				"ingress_ws_http_bridge_compatibility_retry account_id=%d turn=%d reason=%s",
				account.ID,
				turn,
				truncateOpenAIWSLogValue(reason, openAIWSLogValueMaxLen),
			)
			continue
		}

		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if upstreamMsg == "" {
			upstreamMsg = http.StatusText(resp.StatusCode)
		}
		_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(resp.StatusCode, upstreamMsg))
		return nil, fmt.Errorf("upstream http bridge error: status=%d message=%s", resp.StatusCode, upstreamMsg)
	}
	defer func() { _ = resp.Body.Close() }()

	logOpenAIWSModeInfo(
		"ingress_ws_http_bridge_upstream_ok account_id=%d turn=%d status=%d payload_bytes=%d header_ms=%d",
		account.ID,
		turn,
		resp.StatusCode,
		payloadBytes,
		time.Since(turnStart).Milliseconds(),
	)

	responseID := ""
	usage := OpenAIUsage{}
	imageCounter := newOpenAIImageOutputCounter()
	var firstTokenMs *int
	reqStream := openAIWSPayloadBoolFromRaw(body, "stream", true)
	eventCount := 0
	tokenEventCount := 0
	terminalEventCount := 0
	replayCollector := &openAIWSToolCallReplayCollector{}
	firstEventType := ""
	lastEventType := ""
	sawDone := false
	wroteDownstream := false
	clientDisconnected := false
	mappedModel := ""
	needModelReplace := false
	var mappedModelBytes []byte
	if originalModel != "" {
		mappedModel = normalizeOpenAIModelForUpstream(account, account.GetMappedModel(originalModel))
		needModelReplace = mappedModel != "" && mappedModel != originalModel
		if needModelReplace {
			mappedModelBytes = []byte(mappedModel)
		}
	}

	resultWithUsage := func() *OpenAIForwardResult {
		imageCount := imageCounter.Count()
		payloadBytesValue := int64(payloadBytes)
		eventCountValue := eventCount
		result := &OpenAIForwardResult{
			RequestID:       responseID,
			Usage:           usage,
			Model:           originalModel,
			UpstreamModel:   mappedModel,
			ServiceTier:     extractOpenAIServiceTierFromBody(body),
			ReasoningEffort: ApplyThinkingEnabledFallback(extractOpenAIReasoningEffortFromBody(body, firstNonEmpty(mappedModel, originalModel)), body, mappedModel),
			Stream:          reqStream,
			OpenAIWSMode:    true,
			ResponseHeaders: cloneHeader(resp.Header),
			Duration:        time.Since(turnStart),
			FirstTokenMs:    firstTokenMs,
			WSPayloadBytes:  &payloadBytesValue,
			WSEventCount:    &eventCountValue,
		}
		if replayInput := normalizeOpenAIReplayItemsForAccount(account, replayCollector.Items()); len(replayInput) > 0 {
			result.wsReplayInput = replayInput
			result.wsReplayInputExists = true
		}
		if imageCount > 0 {
			result.ImageCount = imageCount
			result.ImageSize = imageSizeTier
			result.ImageInputSize = imageInputSize
			result.ImageOutputSizes = imageCounter.Sizes()
			result.BillingModel = imageBillingModel
		}
		return result
	}

	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	// Prevent WS bridge turns from hanging forever when upstream accepts the
	// request but never emits another SSE line.
	if streamInterval <= 0 {
		streamInterval = 90 * time.Second
	}

	type streamLine struct {
		line string
		err  error
		done bool
	}
	// Cancelable reader: early returns (idle/max-duration/max-token) must stop the
	// scanner goroutine. Otherwise it can block forever on a full channel after
	// resp.Body is closed by defer, leaking the goroutine and scanner buffer.
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	lines := make(chan streamLine, 16)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(resp.Body)
		scanBuf := getSSEScannerBuf64K()
		scanner.Buffer(scanBuf[:0], maxLineSize)
		defer putSSEScannerBuf64K(scanBuf)
		send := func(item streamLine) bool {
			select {
			case lines <- item:
				return true
			case <-readCtx.Done():
				return false
			}
		}
		for scanner.Scan() {
			if !send(streamLine{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = send(streamLine{err: err, done: true})
			return
		}
		_ = send(streamLine{done: true})
	}()

	maxTokenEvents := openAIWSHTTPBridgeMaxTokenEventsDefault

	// Reuse one timer instead of time.After in the hot loop to avoid leaking
	// timers under high event rates.
	var waitTimer *time.Timer
	stopWaitTimer := func() {
		if waitTimer == nil {
			return
		}
		if !waitTimer.Stop() {
			select {
			case <-waitTimer.C:
			default:
			}
		}
	}
	defer stopWaitTimer()
	armWaitTimer := func(d time.Duration) <-chan time.Time {
		if d <= 0 {
			d = time.Millisecond
		}
		if waitTimer == nil {
			waitTimer = time.NewTimer(d)
			return waitTimer.C
		}
		stopWaitTimer()
		waitTimer.Reset(d)
		return waitTimer.C
	}
	abortMaxDuration := func() (*OpenAIForwardResult, error) {
		msg := fmt.Sprintf("upstream stream exceeded max duration %s", maxDuration)
		_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(http.StatusGatewayTimeout, msg))
		logOpenAIWSModeInfo(
			"ingress_ws_http_bridge_stream_max_duration account_id=%d turn=%d max_duration=%s events=%d token_events=%d first_event=%s last_event=%s",
			account.ID,
			turn,
			maxDuration.String(),
			eventCount,
			tokenEventCount,
			truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
			truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
		)
		return resultWithUsage(), errors.New(msg)
	}

	lastRead := time.Now()
	for {
		var item streamLine
		wait := streamInterval
		if remaining := time.Until(streamDeadline); remaining > 0 && (wait <= 0 || remaining < wait) {
			wait = remaining
		}
		if wait <= 0 {
			return abortMaxDuration()
		}
		select {
		case item = <-lines:
		case <-armWaitTimer(wait):
			if time.Now().After(streamDeadline) {
				return abortMaxDuration()
			}
			if streamInterval > 0 && time.Since(lastRead) >= streamInterval {
				msg := fmt.Sprintf("upstream stream idle for %s", streamInterval)
				_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(http.StatusGatewayTimeout, msg))
				logOpenAIWSModeInfo(
					"ingress_ws_http_bridge_stream_idle_timeout account_id=%d turn=%d interval=%s events=%d first_event=%s last_event=%s",
					account.ID,
					turn,
					streamInterval.String(),
					eventCount,
					truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
				)
				return resultWithUsage(), errors.New(msg)
			}
			continue
		case <-ctx.Done():
			return resultWithUsage(), ctx.Err()
		}
		if item.err != nil {
			return resultWithUsage(), fmt.Errorf("read upstream http bridge stream: %w", item.err)
		}
		if item.done && item.line == "" {
			break
		}
		lastRead = time.Now()

		data, ok := extractOpenAISSEDataLine(item.line)
		if !ok {
			continue
		}
		trimmedData := strings.TrimSpace(data)
		if trimmedData == "" {
			continue
		}
		if trimmedData == "[DONE]" {
			sawDone = true
			continue
		}

		upstreamMessage := []byte(trimmedData)
		eventType, eventResponseID, _ := parseOpenAIWSEventEnvelope(upstreamMessage)
		if responseID == "" && eventResponseID != "" {
			responseID = eventResponseID
		}
		if eventType != "" {
			eventCount++
			if firstEventType == "" {
				firstEventType = eventType
			}
			lastEventType = eventType
		}
		if isOpenAIWSTokenEvent(eventType) {
			tokenEventCount++
			if firstTokenMs == nil {
				ms := int(time.Since(turnStart).Milliseconds())
				firstTokenMs = &ms
			}
			if tokenEventCount > maxTokenEvents {
				msg := fmt.Sprintf("upstream stream exceeded max token events %d", maxTokenEvents)
				_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(http.StatusGatewayTimeout, msg))
				logOpenAIWSModeInfo(
					"ingress_ws_http_bridge_stream_max_token_events account_id=%d turn=%d max_token_events=%d events=%d token_events=%d first_event=%s last_event=%s",
					account.ID,
					turn,
					maxTokenEvents,
					eventCount,
					tokenEventCount,
					truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
				)
				return resultWithUsage(), errors.New(msg)
			}
		}
		if time.Now().After(streamDeadline) {
			msg := fmt.Sprintf("upstream stream exceeded max duration %s", maxDuration)
			_ = writeClientMessage(buildOpenAIWSHTTPBridgeErrorEvent(http.StatusGatewayTimeout, msg))
			logOpenAIWSModeInfo(
				"ingress_ws_http_bridge_stream_max_duration account_id=%d turn=%d max_duration=%s events=%d token_events=%d first_event=%s last_event=%s",
				account.ID,
				turn,
				maxDuration.String(),
				eventCount,
				tokenEventCount,
				truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
				truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
			)
			return resultWithUsage(), errors.New(msg)
		}
		if openAIWSEventShouldParseUsage(eventType) {
			parseOpenAIWSResponseUsageFromCompletedEvent(upstreamMessage, &usage)
		}
		imageCounter.AddSSEData(upstreamMessage)

		if needModelReplace && len(mappedModelBytes) > 0 && openAIWSEventMayContainModel(eventType) && strings.Contains(trimmedData, mappedModel) {
			upstreamMessage = replaceOpenAIWSMessageModel(upstreamMessage, mappedModel, originalModel)
		}
if s.toolCorrector != nil && openAIWSEventMayContainToolCalls(eventType) && openAIWSMessageLikelyContainsToolCalls(upstreamMessage) {
				if corrected, changed := s.toolCorrector.CorrectToolCallsInSSEBytes(upstreamMessage); changed {
					upstreamMessage = corrected
				}
			}

			restoredMessage, restoreErr := restoreOpenAIResponsesNamespacePayload(c, upstreamMessage)
			if restoreErr != nil {
				return resultWithUsage(), fmt.Errorf("restore http bridge namespace response: %w", restoreErr)
			}
			upstreamMessage = restoredMessage

			// 上游自生成的 fc_ call_id 还原为 codex 的 call_id（工具名配对），
			// 必须在 replayCollector 之前完成：重放序列注入下一轮 input 时也要
			// 带 codex call_id，否则客户端 custom_tool_call_output(call_*) 与
			// 重放 function_call(fc_*) 断链 → 上游 No tool call found。
			if callIDMapper != nil {
				if restored, changed := callIDMapper.restoreBridgeResponseCallIDs(upstreamMessage); changed {
					upstreamMessage = restored
				}
			}
			replayCollector.AddEvent(eventType, upstreamMessage)

		if !clientDisconnected {
			if err := writeClientMessage(upstreamMessage); err != nil {
				if isOpenAIWSClientDisconnectError(err) {
					clientDisconnected = true
					closeStatus, closeReason := summarizeOpenAIWSReadCloseError(err)
					logOpenAIWSModeInfo(
						"ingress_ws_http_bridge_client_disconnected_drain account_id=%d turn=%d close_status=%s close_reason=%s",
						account.ID,
						turn,
						closeStatus,
						truncateOpenAIWSLogValue(closeReason, openAIWSHeaderValueMaxLen),
					)
				} else {
					return nil, wrapOpenAIWSIngressTurnError(
						"write_client",
						fmt.Errorf("write client websocket event: %w", err),
						wroteDownstream,
					)
				}
			} else {
				wroteDownstream = true
			}
		}

		if eventType == "error" {
			errCodeRaw, errTypeRaw, errMsgRaw := parseOpenAIWSErrorEventFields(upstreamMessage)
			s.persistOpenAIWSRateLimitSignal(ctx, account, resp.Header, upstreamMessage, errCodeRaw, errTypeRaw, errMsgRaw)
			errMessage := strings.TrimSpace(errMsgRaw)
			if errMessage == "" {
				errMessage = "upstream error event"
			}
			return resultWithUsage(), errors.New(errMessage)
		}
		if isOpenAIWSTerminalEvent(eventType) {
			terminalEventCount++
			firstTokenMsValue := -1
			if firstTokenMs != nil {
				firstTokenMsValue = *firstTokenMs
			}
			logOpenAIWSModeInfo(
				"ingress_ws_http_bridge_turn_completed account_id=%d turn=%d response_id=%s payload_bytes=%d duration_ms=%d events=%d token_events=%d terminal_events=%d first_event=%s last_event=%s first_token_ms=%d client_disconnected=%v",
				account.ID,
				turn,
				truncateOpenAIWSLogValue(responseID, openAIWSIDValueMaxLen),
				payloadBytes,
				time.Since(turnStart).Milliseconds(),
				eventCount,
				tokenEventCount,
				terminalEventCount,
				truncateOpenAIWSLogValue(firstEventType, openAIWSLogValueMaxLen),
				truncateOpenAIWSLogValue(lastEventType, openAIWSLogValueMaxLen),
				firstTokenMsValue,
				clientDisconnected,
			)
			return resultWithUsage(), nil
		}
	}
	if sawDone && eventCount > 0 {
		return resultWithUsage(), nil
	}
	return resultWithUsage(), errors.New("upstream http bridge stream ended before terminal event")
}
