package service

// Mirasim relay 账号的模型映射护栏（可见性与一键修复）。
//
// 背景（生产事故 2026-09-25，账号 140）：
//   - relay 账号的 model_mapping 为空时，Account.IsModelSupported 的语义是
//     「允许所有模型」，于是该账号会被调度去认领上游根本不支持的模型
//     （如 label deepseek-v4.1-flash，mirasim 侧真实 id 是 deepseek-flash），
//     上游返回 422，网关把它变成 502 返回给客户端。
//   - 该账号 priority=0，即组内优先级最高，因此总是第一个认领。
//
// 关键约束（改动前请先读）：
//   - 本文件只做「识别 + 派生标记 + 生成 identity 映射」，**绝不**修改
//     domain 层 IsModelSupported / GetMappedModel 的空映射语义（空映射仍等价
//     「允许所有模型」）。是否写入映射由管理端显式操作决定。
//   - 判定「是不是 mirasim relay 账号」必须复用 isMirasimRelayAccount，
//     不要另写一套宽松规则：宽松判定只允许出现在只读元数据/模型清单路径
//     （mirasimRelayAccountCandidate），否则会把没有凭据的账号也当成 relay 账号。
//   - 输出只含模型 id，不得包含任何凭据内容。

import "strings"

// IsMirasimRelayAccount 导出 mirasim relay 账号的权威识别规则。
//
// 管理端需要它来拒绝「对非 relay 账号执行 relay 专属操作」，内部实现
// 仍然只有 isMirasimRelayAccount 一份，避免两套规则漂移。
func IsMirasimRelayAccount(account *Account) bool {
	return isMirasimRelayAccount(account)
}

// MirasimRelayAccountNeedsModelMapping 判断账号是否需要补模型映射。
//
// 语义：是 mirasim relay 账号，且解析后的 model_mapping 为空。
// 空映射此刻等价于「认领所有模型」，是 422 → 502 的直接前提。
// 该函数是只读派生判断，不会对账号做任何写入。
func MirasimRelayAccountNeedsModelMapping(account *Account) bool {
	if !isMirasimRelayAccount(account) {
		return false
	}
	return len(account.GetModelMapping()) == 0
}

// BuildMirasimIdentityModelMapping 把上游可用模型清单转成 identity 映射。
//
// 产物形态与前端 useModelWhitelist.buildModelMappingObject 的白名单模式一致：
// mapping[id] = id（值为 string，domain 层 resolveModelMapping 只接受 string 值）。
//
// 刻意只做 identity：上游既然把这批 id 列为可用，「原样转发」必然被接受；
// 别名映射需要人工知识，机器猜错会把「漏配」升级成「配错」。
//
// 规范化：去首尾空白、丢弃空串、去重。清单为空或全为空白时返回空 map，
// 调用方必须据此拒绝写入（写入空映射等于恢复成「认领所有模型」）。
func BuildMirasimIdentityModelMapping(models []string) map[string]string {
	if len(models) == 0 {
		return map[string]string{}
	}
	mapping := make(map[string]string, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model)
		if id == "" {
			continue
		}
		mapping[id] = id
	}
	return mapping
}

// MirasimModelMappingEqual 比较两份 identity 映射是否逐键相同。
//
// 仅用于「幂等」判定：目标映射与账号现值完全一致时跳过写库。
// 两边长度不同或任一键/值不同即视为不等。
func MirasimModelMappingEqual(current, target map[string]string) bool {
	if len(current) != len(target) {
		return false
	}
	for key, value := range target {
		if existing, ok := current[key]; !ok || existing != value {
			return false
		}
	}
	return true
}
