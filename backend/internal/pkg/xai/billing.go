package xai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// Billing endpoints live on the Grok CLI proxy (same as CPAMC / OpenCodex).
	DefaultBillingBaseURL = DefaultCLIBaseURL

	// SuperGrok monthly included credit limits, in cents (CPAMC heuristics).
	SuperGrokMonthlyLimitCents      int64 = 15_000  // US$150
	SuperGrokHeavyMonthlyLimitCents int64 = 150_000 // US$1,500

	PlanSuperGrok      = "SuperGrok"
	PlanSuperGrokHeavy = "SuperGrok Heavy"

	GrokCLITokenAuthHeader  = "x-xai-token-auth"
	GrokCLITokenAuthValue   = "xai-grok-cli"
	GrokCLIVersionHeader    = "x-grok-client-version"
	GrokCLIVersionValue     = "0.2.91"
	GrokCLIUserAgent        = "grok-pager/0.2.91 grok-shell/0.2.91 (macos; aarch64)"
	GrokCLIUserIDHeader     = "x-userid"
)

// BillingProductUsage is one product row from xAI billing (e.g. GrokBuild / Api / GrokChat).
type BillingProductUsage struct {
	Product      string   `json:"product"`
	UsagePercent *float64 `json:"usage_percent,omitempty"`
}

// BillingSnapshot is the merged view of /v1/billing and /v1/billing?format=credits.
// Field semantics mirror Cli-Proxy-API-Management-Center xai_quota parsing.
type BillingSnapshot struct {
	PeriodType          string                `json:"period_type,omitempty"` // weekly | monthly | unknown
	UsagePercent        *float64              `json:"usage_percent,omitempty"`
	PeriodStart         string                `json:"period_start,omitempty"`
	PeriodEnd           string                `json:"period_end,omitempty"`
	ProductUsage        []BillingProductUsage `json:"product_usage,omitempty"`
	MonthlyLimitCents   *int64                `json:"monthly_limit_cents,omitempty"`
	UsedCents           *int64                `json:"used_cents,omitempty"`
	IncludedUsedCents   *int64                `json:"included_used_cents,omitempty"`
	OnDemandCapCents    *int64                `json:"on_demand_cap_cents,omitempty"`
	OnDemandUsedCents   *int64                `json:"on_demand_used_cents,omitempty"`
	OnDemandUsedPercent *float64              `json:"on_demand_used_percent,omitempty"`
	BillingPeriodStart  string                `json:"billing_period_start,omitempty"`
	BillingPeriodEnd    string                `json:"billing_period_end,omitempty"`
	UsedPercent         *float64              `json:"used_percent,omitempty"` // monthly included used %
	Plan                string                `json:"plan,omitempty"`
	StatusCode          int                   `json:"status_code,omitempty"`
	FetchedAt           string                `json:"fetched_at,omitempty"`
	Source              string                `json:"source,omitempty"`
}

func (s *BillingSnapshot) HasData() bool {
	if s == nil {
		return false
	}
	return s.UsagePercent != nil ||
		s.UsedPercent != nil ||
		s.MonthlyLimitCents != nil ||
		s.UsedCents != nil ||
		s.OnDemandCapCents != nil ||
		len(s.ProductUsage) > 0 ||
		s.PeriodEnd != "" ||
		s.BillingPeriodEnd != "" ||
		s.Plan != ""
}

// BuildBillingURL returns the Grok CLI billing endpoint.
// When formatCredits is true, appends ?format=credits (weekly/product breakdown).
func BuildBillingURL(formatCredits bool) (string, error) {
	base, err := ValidateBaseURL(DefaultBillingBaseURL)
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(base, "/") + "/billing"
	if formatCredits {
		url += "?format=credits"
	}
	return url, nil
}

// ApplyGrokCLIBillingHeaders sets headers expected by cli-chat-proxy billing.
func ApplyGrokCLIBillingHeaders(req *http.Request, accessToken, userID string) {
	if req == nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set(GrokCLITokenAuthHeader, GrokCLITokenAuthValue)
	req.Header.Set(GrokCLIVersionHeader, GrokCLIVersionValue)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", GrokCLIUserAgent)
	if id := strings.TrimSpace(userID); id != "" {
		req.Header.Set(GrokCLIUserIDHeader, id)
	}
}

// ParseBillingResponse parses a full billing HTTP body into a snapshot.
// Accepts either { "config": {...} } or a bare config object.
func ParseBillingResponse(body []byte) (*BillingSnapshot, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, fmt.Errorf("empty billing body")
	}
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("invalid billing json: %w", err)
	}
	config := extractBillingConfig(root)
	if config == nil {
		return nil, fmt.Errorf("billing config missing")
	}
	snapshot := parseBillingConfig(config)
	if snapshot == nil || !snapshot.HasData() {
		return nil, fmt.Errorf("billing config empty")
	}
	snapshot.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return snapshot, nil
}

func extractBillingConfig(root any) map[string]any {
	obj, ok := root.(map[string]any)
	if !ok || obj == nil {
		return nil
	}
	if cfg, ok := obj["config"].(map[string]any); ok && cfg != nil {
		return cfg
	}
	// Some responses may return the config object directly.
	if _, hasLimit := obj["monthlyLimit"]; hasLimit {
		return obj
	}
	if _, hasLimit := obj["monthly_limit"]; hasLimit {
		return obj
	}
	if _, hasPeriod := obj["currentPeriod"]; hasPeriod {
		return obj
	}
	if _, hasPeriod := obj["current_period"]; hasPeriod {
		return obj
	}
	if _, hasProducts := obj["productUsage"]; hasProducts {
		return obj
	}
	if _, hasProducts := obj["product_usage"]; hasProducts {
		return obj
	}
	return nil
}

func parseBillingConfig(cfg map[string]any) *BillingSnapshot {
	if cfg == nil {
		return nil
	}
	out := &BillingSnapshot{}

	currentPeriod := asObject(firstAny(cfg, "currentPeriod", "current_period"))
	periodType := classifyPeriodType(asObject(currentPeriod))
	creditUsagePercent := parseFloatPtr(firstAny(cfg, "creditUsagePercent", "credit_usage_percent"))
	periodStart := firstString(currentPeriod, "start")
	if periodStart == "" {
		periodStart = firstString(cfg, "billingPeriodStart", "billing_period_start")
	}
	periodEnd := firstString(currentPeriod, "end")
	if periodEnd == "" {
		periodEnd = firstString(cfg, "billingPeriodEnd", "billing_period_end")
	}

	products := parseProductUsage(firstAny(cfg, "productUsage", "product_usage"))
	monthlyLimit := parseCents(firstAny(cfg, "monthlyLimit", "monthly_limit"))
	used := parseCents(firstAny(cfg, "used"))
	onDemandCap := parseCents(firstAny(cfg, "onDemandCap", "on_demand_cap"))
	onDemandUsed := parseCents(firstAny(cfg, "onDemandUsed", "on_demand_used"))
	billingStart := firstString(cfg, "billingPeriodStart", "billing_period_start")
	billingEnd := firstString(cfg, "billingPeriodEnd", "billing_period_end")

	var includedUsed *int64
	if used != nil {
		if monthlyLimit != nil && *monthlyLimit > 0 && *used > *monthlyLimit {
			v := *monthlyLimit
			includedUsed = &v
		} else {
			v := *used
			includedUsed = &v
		}
	}
	var overflowUsed *int64
	if used != nil && monthlyLimit != nil && *used > *monthlyLimit {
		v := *used - *monthlyLimit
		overflowUsed = &v
	}
	if onDemandUsed == nil {
		onDemandUsed = overflowUsed
	}

	var monthlyUsedPercent *float64
	if monthlyLimit != nil && *monthlyLimit > 0 && includedUsed != nil {
		p := float64(*includedUsed) / float64(*monthlyLimit) * 100
		monthlyUsedPercent = &p
	}
	var onDemandPercent *float64
	if onDemandCap != nil && *onDemandCap > 0 && onDemandUsed != nil {
		p := float64(*onDemandUsed) / float64(*onDemandCap) * 100
		onDemandPercent = &p
	}

	hasWeekly := creditUsagePercent != nil || periodType == "weekly" || len(products) > 0
	hasMonthly := monthlyLimit != nil || used != nil || (!hasWeekly && (onDemandCap != nil || billingEnd != ""))
	if !hasWeekly && !hasMonthly {
		return nil
	}

	if hasWeekly {
		if periodType == "unknown" {
			out.PeriodType = "weekly"
		} else {
			out.PeriodType = periodType
		}
		out.UsagePercent = creditUsagePercent
		out.PeriodStart = periodStart
		out.PeriodEnd = periodEnd
	} else {
		out.PeriodType = "monthly"
		out.UsagePercent = monthlyUsedPercent
		out.PeriodStart = billingStart
		out.PeriodEnd = billingEnd
	}
	out.ProductUsage = products
	out.MonthlyLimitCents = monthlyLimit
	out.UsedCents = used
	out.IncludedUsedCents = includedUsed
	out.OnDemandCapCents = onDemandCap
	out.OnDemandUsedCents = onDemandUsed
	out.OnDemandUsedPercent = onDemandPercent
	if hasMonthly || monthlyLimit != nil || used != nil || billingEnd != "" {
		out.BillingPeriodStart = billingStart
		out.BillingPeriodEnd = billingEnd
	}
	out.UsedPercent = monthlyUsedPercent
	out.Plan = InferPlanFromMonthlyLimit(monthlyLimit)
	return out
}

// MergeBillingSnapshots prefers non-empty fields from a, filling gaps from b.
// Call with format=credits first, plain billing second (CPAMC order).
func MergeBillingSnapshots(a, b *BillingSnapshot) *BillingSnapshot {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	out := *a
	if out.PeriodType == "" || out.PeriodType == "unknown" {
		if b.PeriodType != "" {
			out.PeriodType = b.PeriodType
		}
	}
	if out.UsagePercent == nil {
		out.UsagePercent = b.UsagePercent
	}
	if out.PeriodStart == "" {
		out.PeriodStart = b.PeriodStart
	}
	if out.PeriodEnd == "" {
		out.PeriodEnd = b.PeriodEnd
	}
	if len(out.ProductUsage) == 0 {
		out.ProductUsage = b.ProductUsage
	}
	if out.MonthlyLimitCents == nil {
		out.MonthlyLimitCents = b.MonthlyLimitCents
	}
	if out.UsedCents == nil {
		out.UsedCents = b.UsedCents
	}
	if out.IncludedUsedCents == nil {
		out.IncludedUsedCents = b.IncludedUsedCents
	}
	if out.OnDemandCapCents == nil {
		out.OnDemandCapCents = b.OnDemandCapCents
	}
	if out.OnDemandUsedCents == nil {
		out.OnDemandUsedCents = b.OnDemandUsedCents
	}
	if out.OnDemandUsedPercent == nil {
		out.OnDemandUsedPercent = b.OnDemandUsedPercent
	}
	if out.BillingPeriodStart == "" {
		out.BillingPeriodStart = b.BillingPeriodStart
	}
	if out.BillingPeriodEnd == "" {
		out.BillingPeriodEnd = b.BillingPeriodEnd
	}
	if out.UsedPercent == nil {
		out.UsedPercent = b.UsedPercent
	}
	if out.Plan == "" {
		out.Plan = b.Plan
	}
	if out.Plan == "" {
		out.Plan = InferPlanFromMonthlyLimit(out.MonthlyLimitCents)
	}
	if out.FetchedAt == "" {
		out.FetchedAt = b.FetchedAt
	}
	if out.StatusCode == 0 {
		out.StatusCode = b.StatusCode
	}
	return &out
}

// InferPlanFromMonthlyLimit maps known SuperGrok credit caps to plan names.
func InferPlanFromMonthlyLimit(cents *int64) string {
	if cents == nil {
		return ""
	}
	switch *cents {
	case SuperGrokMonthlyLimitCents:
		return PlanSuperGrok
	case SuperGrokHeavyMonthlyLimitCents:
		return PlanSuperGrokHeavy
	default:
		return ""
	}
}

func parseProductUsage(raw any) []BillingProductUsage {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	out := make([]BillingProductUsage, 0, len(arr))
	for i, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok || obj == nil {
			continue
		}
		product := firstString(obj, "product")
		if product == "" {
			product = fmt.Sprintf("Product %d", i+1)
		}
		out = append(out, BillingProductUsage{
			Product:      product,
			UsagePercent: parseFloatPtr(firstAny(obj, "usagePercent", "usage_percent")),
		})
	}
	return out
}

func classifyPeriodType(period map[string]any) string {
	if period == nil {
		return "unknown"
	}
	t := strings.ToLower(firstString(period, "type"))
	switch {
	case strings.Contains(t, "weekly"):
		return "weekly"
	case strings.Contains(t, "monthly"):
		return "monthly"
	default:
		return "unknown"
	}
}

// parseCents accepts a bare number or { "val": number } (CLIProxyAPI shape).
func parseCents(raw any) *int64 {
	if raw == nil {
		return nil
	}
	if obj, ok := raw.(map[string]any); ok {
		return parseBillingInt64(firstAny(obj, "val", "value", "amount"))
	}
	return parseBillingInt64(raw)
}

func parseBillingInt64(raw any) *int64 {
	switch v := raw.(type) {
	case nil:
		return nil
	case float64:
		n := int64(v)
		return &n
	case float32:
		n := int64(v)
		return &n
	case int:
		n := int64(v)
		return &n
	case int64:
		n := v
		return &n
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return &i
		}
		if f, err := v.Float64(); err == nil {
			n := int64(f)
			return &n
		}
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		var n int64
		if _, err := fmt.Sscan(s, &n); err == nil {
			return &n
		}
	}
	return nil
}

func parseFloatPtr(raw any) *float64 {
	switch v := raw.(type) {
	case nil:
		return nil
	case float64:
		return &v
	case float32:
		f := float64(v)
		return &f
	case int:
		f := float64(v)
		return &f
	case int64:
		f := float64(v)
		return &f
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return &f
		}
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		if strings.HasSuffix(s, "%") {
			s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
		}
		var f float64
		if _, err := fmt.Sscan(s, &f); err == nil {
			return &f
		}
	}
	return nil
}

func asObject(raw any) map[string]any {
	if raw == nil {
		return nil
	}
	if obj, ok := raw.(map[string]any); ok {
		return obj
	}
	return nil
}

func firstAny(obj map[string]any, keys ...string) any {
	if obj == nil {
		return nil
	}
	for _, key := range keys {
		if v, ok := obj[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

func firstString(obj map[string]any, keys ...string) string {
	raw := firstAny(obj, keys...)
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		// ignore numeric non-strings
		return ""
	default:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
