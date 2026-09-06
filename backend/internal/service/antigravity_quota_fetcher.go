package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

const (
	forbiddenTypeValidation = "validation"
	forbiddenTypeViolation  = "violation"
	forbiddenTypeForbidden  = "forbidden"

	// 机器可读的错误码
	errorCodeForbidden       = "forbidden"
	errorCodeUnauthenticated = "unauthenticated"
	errorCodeRateLimited     = "rate_limited"
	errorCodeNetworkError    = "network_error"
)

var antigravityQuotaIDPattern = regexp.MustCompile(`[^a-z0-9]+`)

// AntigravityQuotaFetcher 从 Antigravity API 获取额度
type AntigravityQuotaFetcher struct {
	proxyRepo ProxyRepository
}

// NewAntigravityQuotaFetcher 创建 AntigravityQuotaFetcher
func NewAntigravityQuotaFetcher(proxyRepo ProxyRepository) *AntigravityQuotaFetcher {
	return &AntigravityQuotaFetcher{proxyRepo: proxyRepo}
}

// CanFetch 检查是否可以获取此账户的额度
func (f *AntigravityQuotaFetcher) CanFetch(account *Account) bool {
	if account.Platform != PlatformAntigravity {
		return false
	}
	accessToken := account.GetCredential("access_token")
	return accessToken != ""
}

// FetchQuota 获取 Antigravity 账户额度信息
func (f *AntigravityQuotaFetcher) FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error) {
	accessToken := account.GetCredential("access_token")
	projectID, err := resolveAntigravityProjectID(account)
	if err != nil {
		return nil, err
	}

	client, err := antigravity.NewClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create antigravity client failed: %w", err)
	}

	// retrieveUserQuotaSummary is the quota-window source used by CPA Management
	// Center. FetchAvailableModels remains available for model discovery only.
	quotaResp, quotaRaw, err := client.RetrieveUserQuotaSummary(ctx, accessToken, projectID)
	if err != nil {
		// 403 Forbidden: 不报错，返回 is_forbidden 标记
		var forbiddenErr *antigravity.ForbiddenError
		if errors.As(err, &forbiddenErr) {
			now := time.Now()
			fbType := classifyForbiddenType(forbiddenErr.Body)
			return &QuotaResult{
				UsageInfo: &UsageInfo{
					UpdatedAt:       &now,
					IsForbidden:     true,
					ForbiddenReason: forbiddenErr.Body,
					ForbiddenType:   fbType,
					ValidationURL:   extractValidationURL(forbiddenErr.Body),
					NeedsVerify:     fbType == forbiddenTypeValidation,
					IsBanned:        fbType == forbiddenTypeViolation,
					ErrorCode:       errorCodeForbidden,
				},
			}, nil
		}
		return nil, err
	}

	// 调用 LoadCodeAssist 获取订阅等级和 AI Credits 余额（非关键路径，失败不影响主流程）
	tierRaw, tierNormalized, loadResp := f.fetchSubscriptionTier(ctx, client, accessToken)

	// 转换为 UsageInfo
	usageInfo := f.buildQuotaSummaryUsageInfo(quotaResp, tierRaw, tierNormalized, loadResp)

	return &QuotaResult{
		UsageInfo: usageInfo,
		Raw:       quotaRaw,
	}, nil
}

// fetchSubscriptionTier 获取账号订阅等级，失败返回空字符串。
// 同时返回 LoadCodeAssistResponse，以便提取 AI Credits 余额。
func (f *AntigravityQuotaFetcher) fetchSubscriptionTier(ctx context.Context, client *antigravity.Client, accessToken string) (raw, normalized string, loadResp *antigravity.LoadCodeAssistResponse) {
	loadResp, _, err := client.LoadCodeAssist(ctx, accessToken)
	if err != nil {
		slog.Warn("failed to fetch subscription tier", "error", err)
		return "", "", nil
	}
	if loadResp == nil {
		return "", "", nil
	}

	raw = loadResp.GetTier() // 已有方法：paidTier > currentTier
	normalized = normalizeTier(raw)
	return raw, normalized, loadResp
}

// normalizeTier 将原始 tier 字符串归一化为 FREE/PRO/ULTRA/ULTRA_LITE/UNKNOWN
func normalizeTier(raw string) string {
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "ultra-lite") || strings.Contains(lower, "ultra_lite"):
		return "ULTRA_LITE"
	case strings.Contains(lower, "ultra"):
		return "ULTRA"
	case strings.Contains(lower, "pro"):
		return "PRO"
	case strings.Contains(lower, "free"):
		return "FREE"
	default:
		return "UNKNOWN"
	}
}

// buildQuotaSummaryUsageInfo preserves the upstream group/bucket/window shape.
func (f *AntigravityQuotaFetcher) buildQuotaSummaryUsageInfo(quotaResp *antigravity.RetrieveUserQuotaSummaryResponse, tierRaw, tierNormalized string, loadResp *antigravity.LoadCodeAssistResponse) *UsageInfo {
	now := time.Now()
	info := &UsageInfo{
		Source:              "active",
		UpdatedAt:           &now,
		AntigravityQuota:    make(map[string]*AntigravityModelQuota),
		SubscriptionTier:    tierNormalized,
		SubscriptionTierRaw: tierRaw,
	}
	if quotaResp != nil {
		if quotaResp.ServerTime != nil {
			info.AntigravityServerTime = quotaResp.ServerTime.UTC().Format(time.RFC3339)
		}
		for groupIndex, group := range quotaResp.Groups {
			if !isTextGeminiQuotaGroup(group.DisplayName, group.Description) {
				continue
			}
			groupID := stableQuotaID(group.DisplayName, fmt.Sprintf("quota-group-%d", groupIndex+1))
			converted := AntigravityQuotaGroup{ID: groupID, Label: group.DisplayName, Description: group.Description}
			for bucketIndex, bucket := range group.Buckets {
				if bucket.RemainingFraction == nil {
					continue
				}
				remaining := clampQuotaFraction(*bucket.RemainingFraction)
				bucketID := strings.TrimSpace(bucket.BucketID)
				if bucketID == "" {
					bucketID = fmt.Sprintf("%s-bucket-%d", groupID, bucketIndex+1)
				}
				converted.Buckets = append(converted.Buckets, AntigravityQuotaBucket{
					ID: bucketID, Label: bucket.DisplayName, Window: bucket.Window,
					RemainingFraction: remaining, ResetTime: bucket.ResetTime, Description: bucket.Description,
				})
				// Populate the legacy per-model shape during rolling frontend
				// upgrades. Antigravity's Gemini quota is a shared group, so both
				// supported model labels intentionally point at the same strictest
				// bucket until the browser reloads the group-aware bundle.
				legacyUtilization := int(roundUsagePercent2((1 - remaining) * 100))
				for _, model := range []string{"gemini-3.1-pro-high", "gemini-3.8-flash"} {
					existing := info.AntigravityQuota[model]
					if existing == nil || legacyUtilization > existing.Utilization {
						info.AntigravityQuota[model] = &AntigravityModelQuota{Utilization: legacyUtilization, ResetTime: bucket.ResetTime}
					} else if existing.ResetTime == "" || (bucket.ResetTime != "" && bucket.ResetTime < existing.ResetTime) {
						existing.ResetTime = bucket.ResetTime
					}
				}
				if info.FiveHour == nil && isFiveHourQuotaWindow(bucket.Window, bucket.DisplayName) {
					progress := &UsageProgress{Utilization: roundUsagePercent2((1 - remaining) * 100), WindowLabel: "5h"}
					if resetTime, errReset := time.Parse(time.RFC3339, bucket.ResetTime); errReset == nil {
						progress.ResetsAt = &resetTime
						progress.RemainingSeconds = max(0, int(time.Until(resetTime).Seconds()))
					}
					info.FiveHour = progress
				}
			}
			if len(converted.Buckets) > 0 {
				sort.SliceStable(converted.Buckets, func(i, j int) bool {
					return quotaBucketSortWeight(converted.Buckets[i].Window, converted.Buckets[i].Label) <
						quotaBucketSortWeight(converted.Buckets[j].Window, converted.Buckets[j].Label)
				})
				info.AntigravityQuotaGroups = append(info.AntigravityQuotaGroups, converted)
			}
		}
	}
	if len(info.AntigravityQuota) == 0 {
		info.AntigravityQuota = nil
	}

	if loadResp != nil {
		for _, credit := range loadResp.GetAvailableCredits() {
			info.AICredits = append(info.AICredits, AICredit{
				CreditType:     credit.CreditType,
				Amount:         credit.GetAmount(),
				MinimumBalance: credit.GetMinimumAmount(),
			})
		}
	}

	return info
}

func clampQuotaFraction(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func stableQuotaID(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = antigravityQuotaIDPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return fallback
	}
	return value
}

func isTextGeminiQuotaGroup(label, description string) bool {
	labelText := strings.ToLower(label)
	if strings.Contains(labelText, "image") || strings.Contains(labelText, "imagen") {
		return false
	}
	text := strings.ToLower(label + " " + description)
	return strings.Contains(text, "gemini")
}

func isFiveHourQuotaWindow(window, label string) bool {
	value := strings.ToLower(strings.TrimSpace(window + " " + label))
	return strings.Contains(value, "5h") || strings.Contains(value, "5 hour") || strings.Contains(value, "5-hour") || strings.Contains(value, "five hour")
}

func quotaBucketSortWeight(window, label string) int {
	value := strings.ToLower(strings.TrimSpace(window + " " + label))
	if strings.Contains(value, "5h") || strings.Contains(value, "5 hour") || strings.Contains(value, "5-hour") || strings.Contains(value, "five hour") {
		return 1
	}
	if strings.Contains(value, "1d") || strings.Contains(value, "daily") || strings.Contains(value, "day") {
		return 2
	}
	if strings.Contains(value, "7d") || strings.Contains(value, "weekly") || strings.Contains(value, "week") {
		return 3
	}
	return 4
}

// GetProxyURL 获取账户的代理 URL
func (f *AntigravityQuotaFetcher) GetProxyURL(ctx context.Context, account *Account) string {
	if account.ProxyID == nil || f.proxyRepo == nil {
		return ""
	}
	proxy, err := f.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || proxy == nil {
		return ""
	}
	return proxy.URL()
}

// classifyForbiddenType 根据 403 响应体判断禁止类型
func classifyForbiddenType(body string) string {
	lower := strings.ToLower(body)
	switch {
	case strings.Contains(lower, "validation_required") ||
		strings.Contains(lower, "verify your account") ||
		strings.Contains(lower, "validation_url"):
		return forbiddenTypeValidation
	case strings.Contains(lower, "terms of service") ||
		strings.Contains(lower, "violation"):
		return forbiddenTypeViolation
	default:
		return forbiddenTypeForbidden
	}
}

// urlPattern 用于从 403 响应体中提取 URL（降级方案）
var urlPattern = regexp.MustCompile(`https://[^\s"'\\]+`)

// extractValidationURL 从 403 响应 JSON 中提取验证/申诉链接
func extractValidationURL(body string) string {
	// 1. 尝试结构化 JSON 提取: /error/details[*]/metadata/validation_url 或 appeal_url
	var parsed struct {
		Error struct {
			Details []struct {
				Metadata map[string]string `json:"metadata"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &parsed) == nil {
		for _, detail := range parsed.Error.Details {
			if u := detail.Metadata["validation_url"]; u != "" {
				return u
			}
			if u := detail.Metadata["appeal_url"]; u != "" {
				return u
			}
		}
	}

	// 2. 降级：正则匹配 URL
	lower := strings.ToLower(body)
	if !strings.Contains(lower, "validation") &&
		!strings.Contains(lower, "verify") &&
		!strings.Contains(lower, "appeal") {
		return ""
	}
	// 先解码常见转义再匹配
	normalized := strings.ReplaceAll(body, `\u0026`, "&")
	if m := urlPattern.FindString(normalized); m != "" {
		return m
	}
	return ""
}
