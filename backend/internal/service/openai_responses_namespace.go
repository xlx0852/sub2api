package service

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

const openAIResponsesNamespaceNamesContextKey = "openai_responses_namespace_names"

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
	if account == nil || account.Type != AccountTypeOAuth {
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
	if len(names) == 0 || !json.Valid(payload) {
		return payload, nil
	}
	restored, changed, err := apicompat.RestoreResponsesNamespaceCalls(payload, names)
	if err != nil {
		return payload, err
	}
	if changed {
		return restored, nil
	}
	return payload, nil
}
