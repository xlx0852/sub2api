package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const grokQuotaSnapshotExtraKey = "grok_usage_snapshot"

type GrokQuotaFetcher struct{}

func NewGrokQuotaFetcher() *GrokQuotaFetcher {
	return &GrokQuotaFetcher{}
}

func (f *GrokQuotaFetcher) BuildUsageInfo(account *Account) *UsageInfo {
	now := time.Now()
	usage := &UsageInfo{
		Source:    "passive",
		UpdatedAt: &now,
	}
	if account == nil {
		usage.ErrorCode = "quota_unknown"
		usage.Error = "Grok quota is unknown until billing is queried or an upstream response includes xAI rate-limit headers"
		return usage
	}

	// Prefer official billing snapshot (CPAMC-style SuperGrok / weekly / products / monthly credits).
	if billing, err := grokBillingSnapshotFromExtra(account.Extra); err == nil && billing != nil && billing.HasData() {
		usage.GrokBilling = billing
		usage.GrokQuotaSnapshotState = "billing"
		usage.Source = "billing"
		if billing.Plan != "" {
			usage.SubscriptionTier = billing.Plan
			usage.SubscriptionTierRaw = billing.Plan
		}
		if parsedAt, err := time.Parse(time.RFC3339, billing.FetchedAt); err == nil {
			usage.UpdatedAt = &parsedAt
		}
		usage.GrokLastQuotaProbeAt = billing.FetchedAt
		usage.GrokLastStatusCode = billing.StatusCode
		switch billing.StatusCode {
		case 401:
			usage.NeedsReauth = true
			usage.ErrorCode = "unauthenticated"
		case 403:
			usage.IsForbidden = true
			usage.ForbiddenType = "forbidden"
			usage.ErrorCode = "forbidden"
		}
		// Still attach legacy rate-limit header snapshot if present.
		if snapshot, err := grokQuotaSnapshotFromExtra(account.Extra); err == nil && snapshot != nil {
			attachGrokHeaderSnapshot(usage, snapshot, false)
		}
		return usage
	}

	snapshot, err := grokQuotaSnapshotFromExtra(account.Extra)
	if err != nil || snapshot == nil {
		usage.ErrorCode = "quota_unknown"
		usage.Error = "Grok quota is unknown until billing is queried or an upstream response includes xAI rate-limit headers"
		return usage
	}

	attachGrokHeaderSnapshot(usage, snapshot, true)
	return usage
}

func attachGrokHeaderSnapshot(usage *UsageInfo, snapshot *xai.QuotaSnapshot, primary bool) {
	if usage == nil || snapshot == nil {
		return
	}
	if parsedAt, err := time.Parse(time.RFC3339, snapshot.UpdatedAt); err == nil && primary {
		usage.UpdatedAt = &parsedAt
	}
	usage.GrokRequestQuota = snapshot.Requests
	usage.GrokTokenQuota = snapshot.Tokens
	usage.GrokRetryAfterSeconds = snapshot.RetryAfterSeconds
	if usage.SubscriptionTier == "" {
		usage.SubscriptionTier = snapshot.SubscriptionTier
		usage.SubscriptionTierRaw = snapshot.SubscriptionTier
	}
	if usage.GrokEntitlementStatus == "" {
		usage.GrokEntitlementStatus = snapshot.EntitlementStatus
	}
	if usage.GrokLastQuotaProbeAt == "" {
		usage.GrokLastQuotaProbeAt = snapshot.LastProbeAt
	}
	usage.GrokLastHeadersSeenAt = snapshot.LastHeadersSeenAt
	if usage.GrokLastStatusCode == 0 {
		usage.GrokLastStatusCode = snapshot.StatusCode
	}
	if primary {
		if snapshot.HasObservedHeaders() {
			usage.GrokQuotaSnapshotState = "observed"
		} else {
			usage.GrokQuotaSnapshotState = "no_headers"
			usage.ErrorCode = "quota_unknown"
			usage.Error = "No xAI quota headers observed on the latest Grok probe"
		}
	}

	if primary {
		switch snapshot.StatusCode {
		case 401:
			usage.NeedsReauth = true
			usage.ErrorCode = "unauthenticated"
		case 403:
			usage.IsForbidden = true
			usage.ForbiddenType = "forbidden"
			usage.ErrorCode = "forbidden"
			if usage.GrokEntitlementStatus == "" {
				usage.GrokEntitlementStatus = "forbidden"
			}
		case 429:
			usage.ErrorCode = "rate_limited"
		}
	}
}

func grokQuotaSnapshotFromExtra(extra map[string]any) (*xai.QuotaSnapshot, error) {
	return decodeExtraAs[xai.QuotaSnapshot](extra, grokQuotaSnapshotExtraKey)
}

func grokBillingSnapshotFromExtra(extra map[string]any) (*xai.BillingSnapshot, error) {
	return decodeExtraAs[xai.BillingSnapshot](extra, grokBillingSnapshotKey)
}

func decodeExtraAs[T any](extra map[string]any, key string) (*T, error) {
	if extra == nil {
		return nil, nil
	}
	raw, ok := extra[key]
	if !ok || raw == nil {
		return nil, nil
	}
	switch snapshot := raw.(type) {
	case *T:
		return snapshot, nil
	case T:
		return &snapshot, nil
	case map[string]any:
		data, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		var out T
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", key, err)
		}
		var out T
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
}
