package service

// Mirasim 签名 roster（`GET /v1/model-roster`）的最小解析：只取决定 thinking 形态的
// `adaptive` 标记，**不参与**模型白名单（白名单仍以账号维度 `GET /v1/models` 为唯一来源）。
//
// 关键约束（改动前请先读）：
//   - `adaptive` 只有**显式出现**时才算数（AdaptiveSet 语义）：缺字段 ≠ false，
//     否则会把所有模型误判成预算型；
//   - `agents[family]` 里的条目覆盖 `models` 共享定义（官方同源规则）；
//   - 解析失败/字段缺失一律当「没有该模型的形态信息」处理，调用方回落到默认的自适应型，
//     不得因为 roster 问题拒绝请求。

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// mirasimRosterAgentFamilies 是 roster 里需要展开的 agent 家族（与官方一致）。
var mirasimRosterAgentFamilies = []string{"claude", "codex", "dsh", "zcode", "kimi"}

// mirasimRosterFamilyPrefix 是各家族条目 id 的合法前缀（防止把别家模型塞进来）。
var mirasimRosterFamilyPrefix = map[string]string{
	"claude": "claude-",
	"codex":  "gpt-",
	"dsh":    "deepseek-",
	"zcode":  "glm-",
	"kimi":   "kimi-",
}

// parseMirasimRosterAdaptive 解析 roster，返回「模型 id → 是否自适应型」的**显式**标记表。
func parseMirasimRosterAdaptive(raw []byte) (map[string]bool, error) {
	var envelope struct {
		Version string                       `json:"version"`
		Agents  map[string][]json.RawMessage `json:"agents"`
		Models  map[string]json.RawMessage   `json:"models"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("mirasim roster: 响应解析失败: %w", err)
	}
	if strings.TrimSpace(envelope.Version) == "" {
		return nil, errors.New("mirasim roster: 响应缺少 version")
	}

	adaptive := make(map[string]bool)
	// 先铺共享定义，再由 agent 条目覆盖（官方同源规则）。
	for id, entry := range envelope.Models {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" {
			continue
		}
		if flag, ok := mirasimSpecAdaptiveFlag(entry); ok {
			adaptive[id] = flag
		}
	}
	for _, family := range mirasimRosterAgentFamilies {
		prefix := mirasimRosterFamilyPrefix[family]
		for _, entry := range envelope.Agents[family] {
			id, flag, ok := mirasimAgentAdaptiveFlag(entry, prefix)
			if !ok {
				continue
			}
			adaptive[id] = flag
		}
	}
	if len(adaptive) == 0 {
		// 能解析但没有一个显式 adaptive：属正常情况（全部保持默认形态），不报错。
		return adaptive, nil
	}
	return adaptive, nil
}

// mirasimSpecAdaptiveFlag 读取 specs 条目里的 adaptive（仅显式出现时 ok=true）。
func mirasimSpecAdaptiveFlag(raw json.RawMessage) (bool, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false, false
	}
	value, ok := fields["adaptive"]
	if !ok {
		return false, false
	}
	var flag bool
	if json.Unmarshal(value, &flag) != nil {
		return false, false
	}
	return flag, true
}

// mirasimAgentAdaptiveFlag 读取 agent 家族条目：id 必须匹配家族前缀，且 adaptive 显式出现。
func mirasimAgentAdaptiveFlag(raw json.RawMessage, prefix string) (string, bool, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return "", false, false
	}
	var id string
	if json.Unmarshal(fields["id"], &id) != nil {
		return "", false, false
	}
	id = strings.ToLower(strings.TrimSpace(id))
	// kimi 家族在 roster 里用 kimi-code/k3 表示 kimi-k3（官方同源映射）。
	if prefix == "kimi-" && id == "kimi-code/k3" {
		id = "kimi-k3"
	}
	if id == "" || !strings.HasPrefix(id, prefix) {
		return "", false, false
	}
	flag, ok := mirasimSpecAdaptiveFlag(raw)
	if !ok {
		return "", false, false
	}
	return id, flag, true
}

// mirasimThinkingShapeForModel 依据 roster 显式标记决定模型形态。
// 没有该模型的显式标记时返回 mirasimShapeUnknown（调用方用默认的自适应型）。
func mirasimThinkingShapeForModel(adaptive map[string]bool, model string) mirasimThinkingShape {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" || len(adaptive) == 0 {
		return mirasimShapeUnknown
	}
	if flag, ok := adaptive[model]; ok {
		if flag {
			return mirasimShapeAdaptive
		}
		return mirasimShapeBudget
	}
	return mirasimShapeUnknown
}
