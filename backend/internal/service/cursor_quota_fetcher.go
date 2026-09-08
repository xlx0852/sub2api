package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/google/uuid"
)

type CursorSandUsageStatus struct {
	UsagePercent          float64 `json:"usage_percent"`
	HasAvailableUsage     bool    `json:"has_available_usage"`
	CurrentPeriodStart    string  `json:"current_period_start,omitempty"`
	NextResetTimestampUtc string  `json:"next_reset_timestamp_utc,omitempty"`
	IncludedUsagePlan     string  `json:"included_usage_plan,omitempty"`
	GrokPlanLabel         string  `json:"grok_plan_label,omitempty"`
}

type CursorQuotaFetcher struct {
	proxyRepo  ProxyRepository
	backendURL string
	now        func() time.Time
}

func NewCursorQuotaFetcher(proxyRepo ProxyRepository) *CursorQuotaFetcher {
	return &CursorQuotaFetcher{
		proxyRepo:  proxyRepo,
		backendURL: cursor.DefaultBackendURL,
		now:        time.Now,
	}
}

func (f *CursorQuotaFetcher) CanFetch(account *Account) bool {
	return account != nil && account.Platform == PlatformCursor && account.Type == AccountTypeOAuth
}

func (f *CursorQuotaFetcher) GetProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil || f.proxyRepo == nil {
		return ""
	}
	proxy, err := f.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || proxy == nil {
		return ""
	}
	return proxy.URL()
}

func (f *CursorQuotaFetcher) createHTTPClient(proxyURL string) (*http.Client, error) {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if proxyURL != "" {
		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("parse proxy url: %w", err)
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsedURL); err != nil {
			return nil, fmt.Errorf("configure proxy: %w", err)
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

func (f *CursorQuotaFetcher) callUnaryConnect(ctx context.Context, httpClient *http.Client, token, machineID, method string) ([]byte, error) {
	endpoint := fmt.Sprintf("%s/aiserver.v1.DashboardService/%s", strings.TrimRight(f.backendURL, "/"), method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("create %s request: %w", method, err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(cursor.HeaderCursorChecksum, cursor.CreateCursorChecksum(machineID, f.now()))
	req.Header.Set(cursor.HeaderCursorClientType, cursor.DefaultClientType)
	req.Header.Set(cursor.HeaderCursorClientVersion, cursor.DefaultClientVersion)
	req.Header.Set(cursor.HeaderSandBoxNamespace, cursor.DefaultBoxNamespace)
	req.Header.Set(cursor.HeaderGhostMode, "false")
	req.Header.Set(cursor.HeaderRequestID, uuid.NewString())
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", method, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", method, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d: %s", method, resp.StatusCode, string(body))
	}
	return body, nil
}

func (f *CursorQuotaFetcher) FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error) {
	if account == nil {
		return nil, fmt.Errorf("account is nil")
	}

	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token == "" {
		return nil, fmt.Errorf("access token is empty")
	}
	machineID := strings.TrimSpace(account.GetCredential("machine_id"))
	if machineID == "" {
		machineID = uuid.NewString()
	}

	httpClient, err := f.createHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}

	rawResult := make(map[string]any)
	now := f.now()
	usageInfo := &UsageInfo{
		Source:    "active",
		UpdatedAt: &now,
	}

	// 1. GetMe
	if meBytes, err := f.callUnaryConnect(ctx, httpClient, token, machineID, "GetMe"); err == nil {
		var meData map[string]any
		if json.Unmarshal(meBytes, &meData) == nil {
			rawResult["profile"] = meData
			if email, ok := meData["email"].(string); ok && email != "" {
				if account.Credentials != nil && account.GetCredential("email") == "" {
					account.Credentials["email"] = email
				}
			}
		}
	}

	// 2. GetSandUsageStatus
	if sandBytes, err := f.callUnaryConnect(ctx, httpClient, token, machineID, "GetSandUsageStatus"); err == nil {
		var sandData struct {
			CurrentPeriodStart         string  `json:"currentPeriodStart"`
			NextResetTimestampUtc     string  `json:"nextResetTimestampUtc"`
			UsagePercent              float64 `json:"usagePercent"`
			HasAvailableUsage         bool    `json:"hasAvailableUsage"`
			IncludedUsageSuperGrokPlan string  `json:"includedUsageSuperGrokPlan"`
			GrokPlanLabel             string  `json:"grokPlanLabel"`
		}
		if json.Unmarshal(sandBytes, &sandData) == nil {
			var rawSand map[string]any
			_ = json.Unmarshal(sandBytes, &rawSand)
			rawResult["sand_usage_status"] = rawSand

			status := &CursorSandUsageStatus{
				UsagePercent:          sandData.UsagePercent,
				HasAvailableUsage:     sandData.HasAvailableUsage,
				CurrentPeriodStart:    sandData.CurrentPeriodStart,
				NextResetTimestampUtc: sandData.NextResetTimestampUtc,
				IncludedUsagePlan:     sandData.IncludedUsageSuperGrokPlan,
				GrokPlanLabel:         sandData.GrokPlanLabel,
			}
			usageInfo.CursorSandUsage = status

			tier := sandData.GrokPlanLabel
			if tier == "" {
				tier = sandData.IncludedUsageSuperGrokPlan
			}
			if tier != "" {
				usageInfo.SubscriptionTier = strings.ToUpper(tier)
				usageInfo.SubscriptionTierRaw = tier
			}

			// Map to standard SevenDay progress
			var resetsAt *time.Time
			remainingSec := 0
			if sandData.NextResetTimestampUtc != "" {
				if t, parseErr := time.Parse(time.RFC3339Nano, sandData.NextResetTimestampUtc); parseErr == nil {
					resetsAt = &t
					rem := int(time.Until(t).Seconds())
					if rem > 0 {
						remainingSec = rem
					}
				}
			}
			windowMin := 7 * 24 * 60
			usageInfo.SevenDay = &UsageProgress{
				Utilization:      sandData.UsagePercent,
				ResetsAt:         resetsAt,
				RemainingSeconds: remainingSec,
				WindowMinutes:    &windowMin,
				WindowLabel:      "7d",
			}
		}
	}

	// 3. GetCurrentPeriodUsage
	if periodBytes, err := f.callUnaryConnect(ctx, httpClient, token, machineID, "GetCurrentPeriodUsage"); err == nil {
		var periodData map[string]any
		if json.Unmarshal(periodBytes, &periodData) == nil {
			rawResult["current_period_usage"] = periodData
		}
	}

	return &QuotaResult{
		UsageInfo: usageInfo,
		Raw:       rawResult,
	}, nil
}
