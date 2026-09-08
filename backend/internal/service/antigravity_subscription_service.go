package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

const antigravitySubscriptionAbnormal = "abnormal"

// AntigravitySubscriptionResult 表示订阅检测后的规范化结果。
type AntigravitySubscriptionResult struct {
	PlanType           string
	SubscriptionStatus string
	SubscriptionError  string
}

// NormalizeAntigravitySubscription 从 LoadCodeAssistResponse 提取 plan_type + 异常状态。
// 使用 GetTier()（返回 tier ID）+ TierIDToPlanType 映射。
func NormalizeAntigravitySubscription(resp *antigravity.LoadCodeAssistResponse) AntigravitySubscriptionResult {
	if resp == nil {
		return AntigravitySubscriptionResult{PlanType: "Free"}
	}

	tierID := resp.GetTier()
	planType := antigravity.TierIDToPlanType(tierID)

	// 检查是否存在阻断性的不合格层级。
	// 注意：如果账号拥有有效付费订阅（PaidTier，如 g1-pro-tier / g1-ultra-tier）或处于有效 standard-tier，
	// 谷歌即便在 ineligibleTiers 中返回 free-tier（例如因直连 IP 判定免费层受限，或已开通 Pro 不再适用免费层），
	// 账号本身作为 Pro/Ultra 也是完全正常可调用的，不能粗暴误判为 Abnormal。
	for _, it := range resp.IneligibleTiers {
		if it == nil {
			continue
		}
		ineligibleID := it.GetTierID()
		// 1. 若不合格层明确匹配当前生效的 tier（无论付费还是免费），属于异常
		isTierIneligible := (ineligibleID != "" && strings.EqualFold(ineligibleID, tierID))
		// 2. 若用户并无付费层（纯 free 账号），且 free-tier 自身在 ineligibleTiers 中或全局受限
		isFreeBlocked := (resp.PaidTier == nil && (ineligibleID == "" || strings.EqualFold(ineligibleID, "free-tier")))

		if isTierIneligible || isFreeBlocked {
			return AntigravitySubscriptionResult{
				PlanType:           "Abnormal",
				SubscriptionStatus: antigravitySubscriptionAbnormal,
				SubscriptionError:  strings.TrimSpace(it.ReasonMessage),
			}
		}
	}

	return AntigravitySubscriptionResult{
		PlanType: planType,
	}
}
