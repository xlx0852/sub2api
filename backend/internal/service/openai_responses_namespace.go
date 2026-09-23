package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

// externalBridgeToolDescriptionLimit 是外部桥能接受的最长工具描述（UTF-8 字节数）。
//
// 实测 Devin 的 GetChatMessage：单条 ASCII 工具描述 ≤4096 字节可正常处理，≥5120 会以
// permission_denied「Unable to process request due to an MCP configuration issue.」
// 拒绝整个请求（codex 的 exec 工具描述约 10.5KB，正好命中）。该报错文案与真实原因
// 无关，因此这里留出余量按 4000 字符裁剪，保证 codex 的工具声明可用。
const externalBridgeToolDescriptionLimit = 4000

const (
	openAIResponsesNamespaceNamesContextKey  = "openai_responses_namespace_names"
	openAIResponsesCustomToolNamesContextKey = "openai_responses_custom_tool_names"
)

// shouldFlattenOpenAIResponsesNamespaces 判定 HTTP Forward 路径是否摊平
// Codex namespace 工具。
//
// 仅影响 OpenAIGatewayService.Forward：
//   - OAuth + HTTP/SSE（含 compact 强制 HTTP）→ 摊平，回程 restore
//   - OAuth + HTTP body passthrough → 摊平（透传仍走 HTTP 上游）
//   - OAuth + 将实际走 WSv2 且非 HTTP passthrough → 不摊平（WS egress 无 restore）
//
// 不覆盖 WS 入站：
//   - ProxyResponsesWebSocketFromClient / ctx_pool / passthrough 不调用 Forward
//   - http_bridge 在 proxyOpenAIWSHTTPBridgeTurn 内单独调用 flatten/restore
func shouldFlattenOpenAIResponsesNamespaces(account *Account, transport OpenAIUpstreamTransport, passthroughEnabled bool) bool {
	if account == nil {
		return false
	}
	// OAuth Codex 上游与 external bridge（Devin/Cursor 等）都不吃 namespace 工具树。
	// 需要在出站前摊平为顶层 function，回程再 restore，否则桥接侧会把整个 tools 丢掉，
	// 模型就会回复「当前没有可调用工具」。
	supportsFlatten := account.Type == AccountTypeOAuth || account.IsExternalBridge()
	if !supportsFlatten {
		return false
	}
	if transport == OpenAIUpstreamTransportResponsesWebsocketV2 && !passthroughEnabled {
		return false
	}
	return true
}

func flattenOpenAIResponsesNamespaces(c *gin.Context, body []byte) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"namespace"`)) {
		return body, nil
	}
	var requestBody map[string]any
	if err := json.Unmarshal(body, &requestBody); err != nil {
		return body, fmt.Errorf("decode OpenAI namespace body: %w", err)
	}
	names, changed, err := apicompat.FlattenResponsesNamespacesExcept(requestBody, map[string]bool{"image_gen": true})
	if err != nil {
		return body, err
	}
	if !changed {
		return body, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, fmt.Errorf("encode OpenAI namespace body: %w", err)
	}
	setOpenAIResponsesNamespaceNames(c, names)
	return rebuilt, nil
}

// downgradeExternalBridgeResponsesTools 让外部桥（Devin/Cursor 等）能吃下 Codex 的工具声明：
//
//  1. namespace 工具树摊平为顶层 function（桥接侧不理解 namespace）；
//  2. custom/freeform 工具（codex 核心执行工具 exec 即此类）降级为 function + 单一 input
//     字符串 schema —— 部分上游会以 permission_denied「MCP configuration issue」拒绝
//     custom 工具声明（实测 Devin 的 GetChatMessage 如此）。
//
// 回程由 restoreOpenAIResponsesNamespacePayload 依据这里存下的名字把 namespace 与
// custom_tool_call 还原，对客户端完全透明。
func downgradeExternalBridgeResponsesTools(c *gin.Context, body []byte, sanitizeDevinTools bool) ([]byte, error) {
	if len(body) == 0 || !bytes.Contains(body, []byte(`"tools"`)) {
		return body, nil
	}
	hasNamespace := bytes.Contains(body, []byte(`"namespace"`))
	hasCustom := sanitizeDevinTools && bytes.Contains(body, []byte(`"custom"`))
	if !hasNamespace && !hasCustom && !sanitizeDevinTools {
		return body, nil
	}
	var requestBody map[string]any
	if err := json.Unmarshal(body, &requestBody); err != nil {
		return body, fmt.Errorf("decode external bridge tools body: %w", err)
	}
	changed := false
	if hasNamespace {
		names, flattened, err := apicompat.FlattenResponsesNamespacesExcept(requestBody, map[string]bool{"image_gen": true})
		if err != nil {
			return body, err
		}
		if flattened {
			setOpenAIResponsesNamespaceNames(c, names)
			changed = true
		}
	}
	if hasCustom {
		customNames, downgraded, err := apicompat.DowngradeResponsesCustomTools(requestBody)
		if err != nil {
			return body, err
		}
		if downgraded {
			setOpenAIResponsesCustomToolNames(c, customNames)
			changed = true
		}
	}
	if sanitizeDevinTools && truncateExternalBridgeToolDescriptions(requestBody) {
		changed = true
	}
	if !changed {
		return body, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, fmt.Errorf("encode external bridge tools body: %w", err)
	}
	return rebuilt, nil
}

// externalBridgeToolDescriptionTruncationMarker 明确告知模型描述被截断，避免
// 它把句子中断处当成完整语义。
const externalBridgeToolDescriptionTruncationMarker = "\n... (description truncated by the gateway)"

// truncateExternalBridgeToolDescription 按行边界把超长描述裁到上限内，并追加截断标记。
//
// 行边界优于硬切：codex 的 exec 描述在 4000 字符处正好落在嵌套工具的 TypeScript
// 签名中间，硬切会给出半句类型声明；退到上一个换行再标记，语义上更干净。
func truncateExternalBridgeToolDescription(description string) string {
	marker := externalBridgeToolDescriptionTruncationMarker
	budget := externalBridgeToolDescriptionLimit - len([]byte(marker))
	if budget <= 0 {
		budget = externalBridgeToolDescriptionLimit
	}
	raw := []byte(description)
	if len(raw) <= externalBridgeToolDescriptionLimit {
		return description
	}
	head := raw[:budget]
	if idx := bytes.LastIndexByte(head, '\n'); idx > budget/2 {
		head = head[:idx]
	}
	for len(head) > 0 && !utf8.Valid(head) {
		head = head[:len(head)-1]
	}
	return strings.TrimRight(string(head), " \t\r\n") + marker
}

// truncateExternalBridgeToolDescriptions 把超长工具描述裁剪到外部桥可接受的
// 上限（按 UTF-8 字节数），返回是否有改动。
func truncateExternalBridgeToolDescriptions(requestBody map[string]any) bool {
	tools, _ := requestBody["tools"].([]any)
	changed := false
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		description, ok := tool["description"].(string)
		if !ok {
			continue
		}
		// 保守按 UTF-8 字节限制；只测过 ASCII 边界，不能假定多字节字符同样安全。
		if len([]byte(description)) <= externalBridgeToolDescriptionLimit {
			continue
		}
		tool["description"] = truncateExternalBridgeToolDescription(description)
		changed = true
	}
	return changed
}

func setOpenAIResponsesCustomToolNames(c *gin.Context, names map[string]bool) {
	if c != nil && len(names) > 0 {
		c.Set(openAIResponsesCustomToolNamesContextKey, names)
	}
}

func openAIResponsesCustomToolNames(c *gin.Context) map[string]bool {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIResponsesCustomToolNamesContextKey)
	if !ok {
		return nil
	}
	names, _ := value.(map[string]bool)
	return names
}

func setOpenAIResponsesNamespaceNames(c *gin.Context, names map[string]apicompat.ResponsesNamespaceName) {
	if c != nil && len(names) > 0 {
		c.Set(openAIResponsesNamespaceNamesContextKey, names)
	}
}

func openAIResponsesNamespaceNames(c *gin.Context) map[string]apicompat.ResponsesNamespaceName {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIResponsesNamespaceNamesContextKey)
	if !ok {
		return nil
	}
	names, _ := value.(map[string]apicompat.ResponsesNamespaceName)
	return names
}

func restoreOpenAIResponsesNamespacePayload(c *gin.Context, payload []byte) ([]byte, error) {
	names := openAIResponsesNamespaceNames(c)
	customNames := openAIResponsesCustomToolNames(c)
	if (len(names) == 0 && len(customNames) == 0) || !json.Valid(payload) {
		return payload, nil
	}
	restored, changed, err := apicompat.RestoreResponsesNamespaceCalls(payload, names, customNames)
	if err != nil {
		return payload, err
	}
	if changed {
		return restored, nil
	}
	return payload, nil
}
