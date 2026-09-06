package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/tidwall/gjson"
)

const (
	defaultReasoningReplayTTL        = 1 * time.Hour
	defaultReasoningReplayMaxEntries = 10000
	maxSignaturesPerFuncName         = 50
	maxOrderedSignatures             = 100
)

// AntigravityReasoningReplayCacheConfig 配置项
type AntigravityReasoningReplayCacheConfig struct {
	TTL        time.Duration
	MaxEntries int
}

type sessionReplayData struct {
	updatedAt         time.Time
	byCallID          map[string]string
	byFuncName        map[string][]string
	orderedSignatures []string
	latestSignature   string
}

// AntigravityReasoningReplayCache 为 Antigravity Gemini 请求提供会话级思考签名缓存与回放
type AntigravityReasoningReplayCache struct {
	mu         sync.RWMutex
	ttl        time.Duration
	maxEntries int
	sessions   map[string]*sessionReplayData
}

// NewAntigravityReasoningReplayCache 创建新的思考签名缓存实例
func NewAntigravityReasoningReplayCache(configs ...AntigravityReasoningReplayCacheConfig) *AntigravityReasoningReplayCache {
	ttl := defaultReasoningReplayTTL
	maxEntries := defaultReasoningReplayMaxEntries
	if len(configs) > 0 {
		if configs[0].TTL > 0 {
			ttl = configs[0].TTL
		}
		if configs[0].MaxEntries > 0 {
			maxEntries = configs[0].MaxEntries
		}
	}
	return &AntigravityReasoningReplayCache{
		ttl:        ttl,
		maxEntries: maxEntries,
		sessions:   make(map[string]*sessionReplayData),
	}
}

// RecordSignature 记录指定会话中的函数调用签名或思考签名
func (c *AntigravityReasoningReplayCache) RecordSignature(sessionKey, callID, funcName, signature string) {
	signature = strings.TrimSpace(signature)
	sessionKey = strings.TrimSpace(sessionKey)
	if signature == "" || signature == antigravity.DummyThoughtSignature || sessionKey == "" {
		return
	}

	callID = strings.TrimSpace(callID)
	funcName = strings.TrimSpace(funcName)

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	sess, exists := c.sessions[sessionKey]
	if !exists {
		if len(c.sessions) >= c.maxEntries {
			c.evictOldestLocked(128, now)
		}
		sess = &sessionReplayData{
			byCallID:   make(map[string]string),
			byFuncName: make(map[string][]string),
		}
		c.sessions[sessionKey] = sess
	}

	sess.updatedAt = now
	sess.latestSignature = signature

	if callID != "" {
		sess.byCallID[callID] = signature
	}

	if funcName != "" {
		list := sess.byFuncName[funcName]
		if len(list) == 0 || list[len(list)-1] != signature {
			if len(list) >= maxSignaturesPerFuncName {
				list = list[1:]
			}
			sess.byFuncName[funcName] = append(list, signature)
		}
	}

	if len(sess.orderedSignatures) == 0 || sess.orderedSignatures[len(sess.orderedSignatures)-1] != signature {
		if len(sess.orderedSignatures) >= maxOrderedSignatures {
			sess.orderedSignatures = sess.orderedSignatures[1:]
		}
		sess.orderedSignatures = append(sess.orderedSignatures, signature)
	}
}

// RecordSignaturesFromChunk 从上游返回的 SSE chunk 中解析并记录 thoughtSignature
func (c *AntigravityReasoningReplayCache) RecordSignaturesFromChunk(sessionKey string, chunk []byte) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || len(chunk) == 0 {
		return
	}
	if !bytes.Contains(chunk, []byte("thoughtSignature")) && !bytes.Contains(chunk, []byte("thought_signature")) {
		return
	}

	d := json.NewDecoder(bytes.NewReader(chunk))
	d.UseNumber()
	var root map[string]any
	if err := d.Decode(&root); err != nil {
		return
	}

	candidatesRaw, ok := root["candidates"].([]any)
	if !ok {
		// 支持嵌套在 response 下的情况
		if respMap, okResp := root["response"].(map[string]any); okResp {
			candidatesRaw, _ = respMap["candidates"].([]any)
		}
	}

	for _, cand := range candidatesRaw {
		candMap, ok := cand.(map[string]any)
		if !ok {
			continue
		}
		contentMap, ok := candMap["content"].(map[string]any)
		if !ok {
			continue
		}
		parts, ok := contentMap["parts"].([]any)
		if !ok {
			continue
		}
		for _, p := range parts {
			partMap, ok := p.(map[string]any)
			if !ok {
				continue
			}
			sig := extractPartThoughtSignature(partMap)
			if fc, okFC := partMap["functionCall"].(map[string]any); okFC {
				if sig == "" {
					sig = extractPartThoughtSignature(fc)
				}
				if sig != "" && sig != antigravity.DummyThoughtSignature {
					name, id := extractFunctionCallInfo(fc)
					c.RecordSignature(sessionKey, id, name, sig)
				}
			} else if sig != "" && sig != antigravity.DummyThoughtSignature {
				c.RecordSignature(sessionKey, "", "", sig)
			}
		}
	}
}

// GetFunctionCallSignature 获取函数调用的签名（优先按 callID，其次按 funcName 与 occurrence，最后 fallback 最新）
func (c *AntigravityReasoningReplayCache) GetFunctionCallSignature(sessionKey, callID, funcName string, occurrence int) (string, bool) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return "", false
	}

	callID = strings.TrimSpace(callID)
	funcName = strings.TrimSpace(funcName)

	c.mu.RLock()
	defer c.mu.RUnlock()

	sess, exists := c.sessions[sessionKey]
	if !exists {
		return "", false
	}
	if time.Since(sess.updatedAt) > c.ttl {
		return "", false
	}

	if callID != "" {
		if sig, ok := sess.byCallID[callID]; ok && sig != "" {
			return sig, true
		}
	}

	if funcName != "" {
		if list, ok := sess.byFuncName[funcName]; ok && len(list) > 0 {
			if occurrence >= 0 && occurrence < len(list) {
				return list[occurrence], true
			}
			return list[len(list)-1], true
		}
	}

	if sess.latestSignature != "" {
		return sess.latestSignature, true
	}

	return "", false
}

// GetLatestSignature 获取会话中最新的思考签名
func (c *AntigravityReasoningReplayCache) GetLatestSignature(sessionKey string) (string, bool) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return "", false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	sess, exists := c.sessions[sessionKey]
	if !exists {
		return "", false
	}
	if time.Since(sess.updatedAt) > c.ttl {
		return "", false
	}

	if sess.latestSignature != "" {
		return sess.latestSignature, true
	}
	return "", false
}

// RestoreSignatures 遍历 Gemini 请求并自动补齐缺失的 thoughtSignature
func (c *AntigravityReasoningReplayCache) RestoreSignatures(sessionKey string, body []byte) []byte {
	if len(body) == 0 || bytes.IndexByte(body, '{') == -1 {
		return body
	}

	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var root map[string]any
	if err := d.Decode(&root); err != nil {
		return body
	}

	contentsRaw, ok := root["contents"].([]any)
	isRootContents := true
	var reqMap map[string]any
	if !ok {
		if r, okReq := root["request"].(map[string]any); okReq {
			reqMap = r
			contentsRaw, ok = reqMap["contents"].([]any)
			isRootContents = false
		}
	}
	if !ok || len(contentsRaw) == 0 {
		return body
	}

	modified := false
	funcOccurrences := make(map[string]int)

	for _, content := range contentsRaw {
		contentMap, ok := content.(map[string]any)
		if !ok {
			continue
		}
		role, _ := contentMap["role"].(string)
		if role != "model" {
			continue
		}
		parts, ok := contentMap["parts"].([]any)
		if !ok || len(parts) == 0 {
			continue
		}

		for _, p := range parts {
			partMap, ok := p.(map[string]any)
			if !ok {
				continue
			}

			// 1. 处理 functionCall
			if fcRaw, hasFC := partMap["functionCall"]; hasFC {
				fc, okFC := fcRaw.(map[string]any)
				if okFC {
					name, id := extractFunctionCallInfo(fc)
					occ := funcOccurrences[name]
					funcOccurrences[name]++

					currSig := extractPartThoughtSignature(partMap)
					if currSig == "" {
						currSig = extractPartThoughtSignature(fc)
					}

					if currSig == "" || currSig == antigravity.DummyThoughtSignature {
						restored, found := c.GetFunctionCallSignature(sessionKey, id, name, occ)
						if found && restored != "" {
							partMap["thoughtSignature"] = restored
							modified = true
						} else if currSig == "" {
							// 未命中缓存且缺失签名时回退至 dummy sentinel
							partMap["thoughtSignature"] = antigravity.DummyThoughtSignature
							modified = true
						}
					} else if sessionKey != "" {
						// 客户端已带原生签名，顺带记录入缓存
						c.RecordSignature(sessionKey, id, name, currSig)
					}
					continue
				}
			}

			// 2. 处理 thought: true
			isThought := false
			if t, okT := partMap["thought"].(bool); okT && t {
				isThought = true
			}
			if isThought {
				currSig := extractPartThoughtSignature(partMap)
				if currSig == "" || currSig == antigravity.DummyThoughtSignature {
					restored, found := c.GetLatestSignature(sessionKey)
					if found && restored != "" {
						partMap["thoughtSignature"] = restored
						modified = true
					} else if currSig == "" {
						partMap["thoughtSignature"] = antigravity.DummyThoughtSignature
						modified = true
					}
				} else if sessionKey != "" {
					c.RecordSignature(sessionKey, "", "", currSig)
				}
			}
		}
	}

	if !modified {
		return body
	}

	if isRootContents {
		root["contents"] = contentsRaw
	} else if reqMap != nil {
		reqMap["contents"] = contentsRaw
	}

	res, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return res
}

// Clear 清理指定会话的缓存数据
func (c *AntigravityReasoningReplayCache) Clear(sessionKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, strings.TrimSpace(sessionKey))
}

// Size 返回当前缓存中的会话数
func (c *AntigravityReasoningReplayCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.sessions)
}

func (c *AntigravityReasoningReplayCache) evictOldestLocked(count int, now time.Time) {
	// 首先清理过期项
	for k, v := range c.sessions {
		if now.Sub(v.updatedAt) > c.ttl {
			delete(c.sessions, k)
		}
	}
	if len(c.sessions) < c.maxEntries {
		return
	}

	type keyTime struct {
		key string
		t   time.Time
	}
	entries := make([]keyTime, 0, len(c.sessions))
	for k, v := range c.sessions {
		entries = append(entries, keyTime{key: k, t: v.updatedAt})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].t.Before(entries[j].t)
	})

	toDelete := count
	if toDelete > len(entries) {
		toDelete = len(entries)
	}
	for i := 0; i < toDelete; i++ {
		delete(c.sessions, entries[i].key)
	}
}

func extractFunctionCallInfo(fc map[string]any) (name, id string) {
	if n, ok := fc["name"].(string); ok {
		name = strings.TrimSpace(n)
	}
	for _, idKey := range []string{"id", "call_id", "callId"} {
		if idVal, ok := fc[idKey].(string); ok && strings.TrimSpace(idVal) != "" {
			id = strings.TrimSpace(idVal)
			break
		}
	}
	return name, id
}

func extractPartThoughtSignature(m map[string]any) string {
	for _, sigKey := range []string{"thoughtSignature", "thought_signature"} {
		if val, ok := m[sigKey].(string); ok {
			sig := strings.TrimSpace(val)
			if sig != "" {
				return sig
			}
		}
	}
	return ""
}

// deriveGeminiReplayKeyFromBody 从请求体提取或计算稳定的会话标识
func deriveGeminiReplayKeyFromBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	for _, path := range []string{"sessionId", "session_id", "request.sessionId", "request.session_id"} {
		if sid := strings.TrimSpace(gjson.GetBytes(body, path).String()); sid != "" {
			return "session:" + sid
		}
	}

	// 提取首个 user turn 的文本作为对话根标识
	contents := gjson.GetBytes(body, "contents")
	if !contents.IsArray() {
		contents = gjson.GetBytes(body, "request.contents")
	}
	if contents.IsArray() {
		for _, content := range contents.Array() {
			if content.Get("role").String() == "user" {
				firstText := content.Get("parts.0.text").String()
				if firstText != "" {
					h := sha256.Sum256([]byte(firstText))
					return "prompt:" + hex.EncodeToString(h[:8])
				}
			}
		}
	}
	return ""
}
