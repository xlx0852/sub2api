package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ExternalBridgeUsage is a provider-neutral snapshot returned by a deployed
// Cursor/Devin sidecar. Fields are intentionally additive so a sidecar can
// expose the upstream's native credit model without pretending it is tokens.
type ExternalBridgeUsage struct {
	Kind                   string     `json:"kind"`
	Status                 string     `json:"status,omitempty"`
	PlanName               string     `json:"plan_name,omitempty"`
	AvailablePromptCredits float64    `json:"available_prompt_credits,omitempty"`
	AvailableFlowCredits   float64    `json:"available_flow_credits,omitempty"`
	AvailableFlexCredits   float64    `json:"available_flex_credits,omitempty"`
	UsedPromptCredits      float64    `json:"used_prompt_credits,omitempty"`
	UsedFlowCredits        float64    `json:"used_flow_credits,omitempty"`
	UsedFlexCredits        float64    `json:"used_flex_credits,omitempty"`
	DailyQuotaRemaining    *float64   `json:"daily_quota_remaining_percent,omitempty"`
	WeeklyQuotaRemaining   *float64   `json:"weekly_quota_remaining_percent,omitempty"`
	DailyQuotaReset        *time.Time `json:"daily_quota_reset_at,omitempty"`
	WeeklyQuotaReset       *time.Time `json:"weekly_quota_reset_at,omitempty"`
	CapacityAvailable      *bool      `json:"capacity_available,omitempty"`
}

type externalBridgeUsageCache struct {
	usage     *UsageInfo
	timestamp time.Time
}

const externalBridgeUsageCacheTTL = 30 * time.Second

func (s *AccountUsageService) getExternalBridgeUsage(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	if account == nil || !account.IsExternalBridge() {
		now := time.Now()
		return &UsageInfo{UpdatedAt: &now}, nil
	}
	if !force && s.cache != nil {
		if raw, ok := s.cache.externalBridge.Load(account.ID); ok {
			if cached, ok := raw.(*externalBridgeUsageCache); ok && time.Since(cached.timestamp) < externalBridgeUsageCacheTTL {
				return cached.usage, nil
			}
		}
	}

	baseURL := strings.TrimRight(strings.TrimSpace(account.GetOpenAIBaseURL()), "/")
	if baseURL == "" {
		return externalBridgeDegraded(account, "external bridge base_url is empty"), nil
	}
	endpoint := baseURL + "/api/account"
	if account.ExternalBridgeKind() == "devin" {
		endpoint = baseURL + "/panel/api/status"
	}
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return externalBridgeDegraded(account, err.Error()), nil
	}
	if token := account.GetOpenAIApiKey(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return externalBridgeDegraded(account, fmt.Sprintf("bridge usage request failed: %v", err)), nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return externalBridgeDegraded(account, fmt.Sprintf("read bridge usage response: %v", err)), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return externalBridgeDegraded(account, fmt.Sprintf("bridge usage returned HTTP %d", resp.StatusCode)), nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return externalBridgeDegraded(account, fmt.Sprintf("decode bridge usage response: %v", err)), nil
	}
	usage := parseExternalBridgeUsage(account.ExternalBridgeKind(), payload)
	now := time.Now()
	result := &UsageInfo{Source: "active", UpdatedAt: &now, ExternalBridge: usage}
	if usage.WeeklyQuotaRemaining != nil {
		remaining := clampPercent(*usage.WeeklyQuotaRemaining)
		utilization := 100 - remaining
		minutes := 7 * 24 * 60
		result.SevenDay = &UsageProgress{Utilization: utilization, WindowMinutes: &minutes, WindowLabel: "7d", ResetsAt: usage.WeeklyQuotaReset, RemainingSeconds: secondsUntil(usage.WeeklyQuotaReset)}
	}
	if usage.DailyQuotaRemaining != nil {
		remaining := clampPercent(*usage.DailyQuotaRemaining)
		utilization := 100 - remaining
		minutes := 24 * 60
		result.FiveHour = &UsageProgress{Utilization: utilization, WindowMinutes: &minutes, WindowLabel: "24h", ResetsAt: usage.DailyQuotaReset, RemainingSeconds: secondsUntil(usage.DailyQuotaReset)}
	}
	if usage.PlanName != "" {
		result.SubscriptionTier = strings.ToUpper(usage.PlanName)
		result.SubscriptionTierRaw = usage.PlanName
	}
	if s.cache != nil {
		s.cache.externalBridge.Store(account.ID, &externalBridgeUsageCache{usage: result, timestamp: now})
	}
	return result, nil
}

func externalBridgeDegraded(account *Account, message string) *UsageInfo {
	now := time.Now()
	return &UsageInfo{Source: "active", UpdatedAt: &now, ExternalBridge: &ExternalBridgeUsage{Kind: account.ExternalBridgeKind(), Status: "error"}, Error: message}
}

func parseExternalBridgeUsage(kind string, payload map[string]any) *ExternalBridgeUsage {
	usage := &ExternalBridgeUsage{Kind: kind, Status: "ok"}
	if kind == "devin" {
		plan := nestedMap(payload, "plan_status", "planStatus")
		info := nestedMap(plan, "plan_info", "planInfo")
		usage.PlanName = firstString(info, "plan_name", "planName")
		usage.AvailablePromptCredits = firstNumber(plan, "available_prompt_credits", "availablePromptCredits")
		usage.AvailableFlowCredits = firstNumber(plan, "available_flow_credits", "availableFlowCredits")
		usage.AvailableFlexCredits = firstNumber(plan, "available_flex_credits", "availableFlexCredits")
		usage.UsedPromptCredits = firstNumber(plan, "used_prompt_credits", "usedPromptCredits")
		usage.UsedFlowCredits = firstNumber(plan, "used_flow_credits", "usedFlowCredits")
		usage.UsedFlexCredits = firstNumber(plan, "used_flex_credits", "usedFlexCredits")
		usage.DailyQuotaRemaining = numberPtr(plan, "daily_quota_remaining", "daily_quota_remaining_percent", "dailyQuotaRemainingPercent")
		usage.WeeklyQuotaRemaining = numberPtr(plan, "weekly_quota_remaining", "weekly_quota_remaining_percent", "weeklyQuotaRemainingPercent")
		usage.DailyQuotaReset = unixTimePtr(plan, "daily_quota_reset", "daily_quota_reset_at_unix", "dailyQuotaResetAtUnix")
		usage.WeeklyQuotaReset = unixTimePtr(plan, "weekly_quota_reset", "weekly_quota_reset_at_unix", "weeklyQuotaResetAtUnix")
		return usage
	}

	sand := nestedMap(payload, "sandUsageStatus", "sand_usage_status")
	usage.PlanName = firstString(sand, "grokPlanLabel", "grok_plan_label", "includedUsageSuperGrokPlan", "included_usage_plan")
	percent := numberPtr(sand, "usagePercent", "usage_percent")
	if percent != nil {
		usage.WeeklyQuotaRemaining = externalFloatPtr(100 - clampPercent(*percent))
	}
	usage.WeeklyQuotaReset = stringTimePtr(sand, "nextResetTimestampUtc", "next_reset_timestamp_utc")
	available := externalBoolPtr(sand, "hasAvailableUsage", "has_available_usage")
	usage.CapacityAvailable = available
	return usage
}

func nestedMap(root map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value, ok := root[key].(map[string]any); ok {
			return value
		}
	}
	return map[string]any{}
}

func firstString(root map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := root[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
func firstNumber(root map[string]any, keys ...string) float64 {
	if value := numberPtr(root, keys...); value != nil {
		return *value
	}
	return 0
}
func numberPtr(root map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		switch value := root[key].(type) {
		case float64:
			return &value
		case json.Number:
			parsed, _ := value.Float64()
			return &parsed
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err == nil {
				return &parsed
			}
		}
	}
	return nil
}
func externalBoolPtr(root map[string]any, keys ...string) *bool {
	for _, key := range keys {
		if value, ok := root[key].(bool); ok {
			return &value
		}
	}
	return nil
}
func externalFloatPtr(value float64) *float64 { return &value }
func stringTimePtr(root map[string]any, keys ...string) *time.Time {
	value := firstString(root, keys...)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &parsed
}
func unixTimePtr(root map[string]any, keys ...string) *time.Time {
	value := numberPtr(root, keys...)
	if value == nil || *value <= 0 {
		return nil
	}
	parsed := time.Unix(int64(*value), 0).UTC()
	return &parsed
}
func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
func secondsUntil(value *time.Time) int {
	if value == nil {
		return 0
	}
	remaining := int(time.Until(*value).Seconds())
	if remaining < 0 {
		return 0
	}
	return remaining
}
