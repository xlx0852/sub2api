package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
)

const (
	kimiQuotaUpstreamTimeout    = 20 * time.Second
	kimiUsageURL                = kimi.CodingBaseURL + "/v1/usages"
	kimiQuotaDefaultConcurrency = 10
	// Legacy temp-unsched reason prefix (pre rate-limit alignment). Still cleared on recovery.
	kimiQuotaPauseReasonPrefix = "kimi quota exhausted:"
	// Marker written when official window exhaustion is mirrored into rate_limit_reset_at,
	// so recovery can clear it without wiping unrelated 429 cooldowns.
	kimiQuotaRateLimitUntilKey = "kimi_quota_rate_limit_until"
)

// KimiAccessTokenProvider is the subset of the Kimi token provider needed by
// the quota client. Keeping this boundary small makes the 401 recovery path
// deterministic and easy to exercise without touching OAuth state in tests.
type KimiAccessTokenProvider interface {
	GetAccessToken(ctx context.Context, account *Account) (string, error)
	ForceRefresh(ctx context.Context, account *Account) (string, error)
}

// KimiQuotaQuerier is consumed by AccountUsageService so quota acquisition can
// be cached independently from the HTTP implementation.
type KimiQuotaQuerier interface {
	QueryUsage(ctx context.Context, accountID int64) (*KimiQuotaUsage, error)
}

// KimiQuotaUsage is the normalized projection of /coding/v1/usages used by the
// account usage API. Kimi's `usage` field is the weekly window; the 300-minute
// entry in `limits` is the rolling five-hour window.
type KimiQuotaUsage struct {
	FiveHour         *UsageProgress `json:"five_hour,omitempty"`
	SevenDay         *UsageProgress `json:"seven_day,omitempty"`
	SubscriptionTier string         `json:"subscription_tier,omitempty"`
	SubscriptionKind string         `json:"subscription_kind,omitempty"`
	FetchedAt        int64          `json:"fetched_at"`
}

type KimiQuotaService struct {
	accountRepo   AccountRepository
	proxyRepo     ProxyRepository
	tokenProvider KimiAccessTokenProvider
	httpUpstream  HTTPUpstream
	usageURL      string
}

func NewKimiQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider KimiAccessTokenProvider,
	httpUpstream HTTPUpstream,
) *KimiQuotaService {
	return &KimiQuotaService{
		accountRepo:   accountRepo,
		proxyRepo:     proxyRepo,
		tokenProvider: tokenProvider,
		httpUpstream:  httpUpstream,
		usageURL:      kimiUsageURL,
	}
}

// QueryUsage fetches Kimi's official rolling quota windows. A single 401 is
// recovered with the shared Kimi OAuth refresh machinery before failing.
func (s *KimiQuotaService) QueryUsage(ctx context.Context, accountID int64) (*KimiQuotaUsage, error) {
	account, err := s.loadKimiOAuthAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if s.tokenProvider == nil || s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "KIMI_QUOTA_NOT_CONFIGURED", "kimi quota service is not configured")
	}

	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil || strings.TrimSpace(token) == "" {
		if err == nil {
			err = errors.New("access token is empty")
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_QUOTA_TOKEN_UNAVAILABLE", "failed to acquire access token: %v", err)
	}

	proxyURL := s.resolveProxyURL(ctx, account)
	callCtx, cancel := context.WithTimeout(ctx, kimiQuotaUpstreamTimeout)
	defer cancel()

	for recovered := false; ; {
		statusCode, body, requestErr := s.fetchUsage(callCtx, account, token, proxyURL)
		if requestErr != nil {
			return nil, requestErr
		}
		if statusCode == http.StatusUnauthorized && !recovered {
			recovered = true
			refreshed, refreshErr := s.tokenProvider.ForceRefresh(ctx, account)
			if refreshErr != nil || strings.TrimSpace(refreshed) == "" {
				if refreshErr == nil {
					refreshErr = errors.New("refreshed access token is empty")
				}
				return nil, infraerrors.Newf(http.StatusUnauthorized, "KIMI_QUOTA_UNAUTHENTICATED", "Kimi quota API returned 401 and token refresh failed: %v", refreshErr)
			}
			token = refreshed
			continue
		}
		if statusCode < 200 || statusCode >= 300 {
			return nil, kimiQuotaHTTPError(statusCode)
		}

		usage, parseErr := parseKimiQuotaUsage(body, time.Now())
		if parseErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_QUOTA_PARSE_FAILED", "failed to parse Kimi quota response: %v", parseErr)
		}
		usage.FetchedAt = time.Now().Unix()
		syncKimiQuotaSchedulingState(ctx, s.accountRepo, account, usage, time.Now())
		return usage, nil
	}
}

func syncKimiQuotaSchedulingState(ctx context.Context, repo AccountRepository, account *Account, usage *KimiQuotaUsage, now time.Time) {
	if repo == nil || account == nil || usage == nil {
		return
	}
	updates := map[string]any{"kimi_quota_updated_at": now.UTC().Format(time.RFC3339)}
	var blockUntil time.Time
	for _, window := range []struct {
		name     string
		progress *UsageProgress
	}{{"5h", usage.FiveHour}, {"7d", usage.SevenDay}} {
		if window.progress == nil {
			continue
		}
		updates["kimi_quota_"+window.name+"_utilization"] = window.progress.Utilization
		if window.progress.ResetsAt == nil {
			updates["kimi_quota_"+window.name+"_reset_at"] = nil
			continue
		}
		updates["kimi_quota_"+window.name+"_reset_at"] = window.progress.ResetsAt.UTC().Format(time.RFC3339)
		if window.progress.Utilization >= 100 && window.progress.ResetsAt.After(now) && window.progress.ResetsAt.After(blockUntil) {
			blockUntil = *window.progress.ResetsAt
		}
	}

	// Align with OpenAI UX: exhausted official windows → rate_limit_reset_at + countdown UI.
	// Also migrate legacy temp-unschedulable pauses created by earlier builds.
	if !blockUntil.IsZero() {
		updates[kimiQuotaRateLimitUntilKey] = blockUntil.UTC().Format(time.RFC3339)
		_ = repo.UpdateExtra(ctx, account.ID, updates)
		_ = repo.SetRateLimited(ctx, account.ID, blockUntil)
		if isKimiQuotaTempUnschedReason(account.TempUnschedulableReason) {
			_ = repo.ClearTempUnschedulable(ctx, account.ID)
		}
		return
	}

	if hasKimiQuotaRateLimitMarker(account.Extra) || hasKimiQuotaRateLimitMarker(updates) {
		updates[kimiQuotaRateLimitUntilKey] = nil
	}
	_ = repo.UpdateExtra(ctx, account.ID, updates)
	if isKimiQuotaTempUnschedReason(account.TempUnschedulableReason) {
		_ = repo.ClearTempUnschedulable(ctx, account.ID)
	}
	if hasKimiQuotaRateLimitMarker(account.Extra) {
		_ = repo.ClearRateLimit(ctx, account.ID)
	}
}

func isKimiQuotaTempUnschedReason(reason string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(reason)), kimiQuotaPauseReasonPrefix)
}

func hasKimiQuotaRateLimitMarker(extra map[string]any) bool {
	if extra == nil {
		return false
	}
	v, ok := extra[kimiQuotaRateLimitUntilKey]
	if !ok || v == nil {
		return false
	}
	switch n := v.(type) {
	case string:
		return strings.TrimSpace(n) != ""
	default:
		return true
	}
}

func (s *KimiQuotaService) fetchUsage(ctx context.Context, account *Account, token, proxyURL string) (int, []byte, error) {
	usageURL := kimiUsageURL
	if s != nil && strings.TrimSpace(s.usageURL) != "" {
		usageURL = s.usageURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return 0, nil, infraerrors.Newf(http.StatusInternalServerError, "KIMI_QUOTA_REQUEST_BUILD_FAILED", "failed to build Kimi quota request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	applyKimiCodingHeaders(req.Header, account)
	if account != nil {
		account.ApplyHeaderOverrides(req.Header)
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, kimiQuotaAccountConcurrency(account))
	if err != nil {
		return 0, nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_QUOTA_REQUEST_FAILED", "Kimi quota request failed: %v", err)
	}
	if resp == nil {
		return 0, nil, infraerrors.New(http.StatusBadGateway, "KIMI_QUOTA_REQUEST_FAILED", "Kimi quota request returned an empty response")
	}
	if resp.Body == nil {
		return resp.StatusCode, nil, nil
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, infraerrors.Newf(http.StatusBadGateway, "KIMI_QUOTA_READ_FAILED", "failed to read Kimi quota response: %v", err)
	}
	return resp.StatusCode, body, nil
}

func kimiQuotaAccountConcurrency(account *Account) int {
	if account == nil || account.Concurrency <= 0 {
		return kimiQuotaDefaultConcurrency
	}
	return account.Concurrency
}

func kimiQuotaHTTPError(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return infraerrors.New(http.StatusUnauthorized, "KIMI_QUOTA_UNAUTHENTICATED", "Kimi quota API returned 401; re-authorize the Kimi account")
	case http.StatusForbidden:
		return infraerrors.New(http.StatusForbidden, "KIMI_QUOTA_FORBIDDEN", "Kimi quota API returned 403")
	case http.StatusTooManyRequests:
		return infraerrors.New(http.StatusTooManyRequests, "KIMI_QUOTA_RATE_LIMITED", "Kimi quota API is rate limited")
	default:
		return infraerrors.Newf(mapUpstreamStatus(statusCode), "KIMI_QUOTA_UPSTREAM_ERROR", "Kimi quota API returned %d", statusCode)
	}
}

func (s *KimiQuotaService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	if account.Proxy != nil {
		return account.Proxy.URL()
	}
	if s != nil && s.proxyRepo != nil {
		if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			return proxy.URL()
		}
	}
	return ""
}

func (s *KimiQuotaService) loadKimiOAuthAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "KIMI_QUOTA_NOT_CONFIGURED", "kimi quota service is not configured")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		if err == nil {
			err = errors.New("account not found")
		}
		return nil, infraerrors.Newf(http.StatusNotFound, "KIMI_QUOTA_ACCOUNT_NOT_FOUND", "Kimi account not found: %v", err)
	}
	if account.Platform != PlatformKimi {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_QUOTA_INVALID_PLATFORM", "account is not a Kimi account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "KIMI_QUOTA_INVALID_TYPE", "account is not a Kimi OAuth account")
	}
	return account, nil
}

type kimiQuotaResponse struct {
	Usage   *kimiQuotaDetail `json:"usage"`
	Limits  []kimiQuotaLimit `json:"limits"`
	SubType string           `json:"subType"`
	User    *kimiQuotaUser   `json:"user"`
}

type kimiQuotaUser struct {
	Membership *kimiQuotaMembership `json:"membership"`
}

type kimiQuotaMembership struct {
	Level string `json:"level"`
}

type kimiQuotaLimit struct {
	Window *kimiQuotaWindow `json:"window"`
	Detail *kimiQuotaDetail `json:"detail"`
}

type kimiQuotaWindow struct {
	Duration json.RawMessage `json:"duration"`
	TimeUnit string          `json:"timeUnit"`
}

type kimiQuotaDetail struct {
	Limit     json.RawMessage `json:"limit"`
	Used      json.RawMessage `json:"used"`
	Remaining json.RawMessage `json:"remaining"`
	ResetTime json.RawMessage `json:"resetTime"`
}

func parseKimiQuotaUsage(body []byte, now time.Time) (*KimiQuotaUsage, error) {
	var payload kimiQuotaResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	usage := &KimiQuotaUsage{SubscriptionKind: strings.TrimSpace(payload.SubType)}
	if payload.User != nil && payload.User.Membership != nil {
		usage.SubscriptionTier = strings.TrimSpace(payload.User.Membership.Level)
	}
	if usage.SubscriptionTier == "" {
		usage.SubscriptionTier = usage.SubscriptionKind
	}
	usage.SevenDay = buildKimiUsageProgress(payload.Usage, now)
	for _, limit := range payload.Limits {
		if limit.Window == nil || limit.Detail == nil {
			continue
		}
		duration, ok := kimiRawNumber(limit.Window.Duration)
		if !ok || !isKimiFiveHourWindow(duration, limit.Window.TimeUnit) {
			continue
		}
		usage.FiveHour = buildKimiUsageProgress(limit.Detail, now)
		break
	}
	if usage.FiveHour == nil && usage.SevenDay == nil {
		return nil, errors.New("response contains no supported quota windows")
	}
	return usage, nil
}

func isKimiFiveHourWindow(duration float64, unit string) bool {
	normalized := strings.ToLower(strings.TrimSpace(unit))
	return (normalized == "minute" || normalized == "minutes" || strings.Contains(normalized, "minute")) && duration == 300
}

func buildKimiUsageProgress(detail *kimiQuotaDetail, now time.Time) *UsageProgress {
	if detail == nil {
		return nil
	}
	limit, hasLimit := kimiRawNumber(detail.Limit)
	used, hasUsed := kimiRawNumber(detail.Used)
	remaining, hasRemaining := kimiRawNumber(detail.Remaining)
	if !hasLimit || limit <= 0 {
		if hasUsed && hasRemaining && used+remaining > 0 {
			limit = used + remaining
			hasLimit = true
		}
	}
	if !hasLimit || limit <= 0 {
		return nil
	}

	if !hasUsed && hasRemaining {
		used = limit - remaining
		hasUsed = true
	}
	if !hasUsed {
		used = 0
	}
	utilization := used / limit * 100
	if !hasUsed && hasRemaining {
		utilization = (limit - remaining) / limit * 100
	}
	if utilization < 0 {
		utilization = 0
	}
	if utilization > 100 {
		utilization = 100
	}

	var resetAt *time.Time
	remainingSeconds := 0
	if rawReset, ok := kimiRawString(detail.ResetTime); ok {
		if parsed, err := parseKimiResetTime(rawReset); err == nil {
			resetAt = &parsed
			remainingSeconds = int(parsed.Sub(now).Seconds())
			if remainingSeconds < 0 {
				remainingSeconds = 0
			}
		}
	}
	return &UsageProgress{
		Utilization:      utilization,
		ResetsAt:         resetAt,
		RemainingSeconds: remainingSeconds,
	}
}

func kimiRawNumber(raw json.RawMessage) (float64, bool) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return 0, false
	}
	if strings.HasPrefix(value, `"`) {
		var quoted string
		if err := json.Unmarshal(raw, &quoted); err != nil {
			return 0, false
		}
		value = strings.TrimSpace(quoted)
	}
	parsed, err := strconv.ParseFloat(value, 64)
	return parsed, err == nil
}

func kimiRawString(raw json.RawMessage) (string, bool) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return "", false
	}
	if strings.HasPrefix(value, `"`) {
		var quoted string
		if err := json.Unmarshal(raw, &quoted); err != nil {
			return "", false
		}
		return strings.TrimSpace(quoted), true
	}
	return strings.TrimSpace(value), true
}

func parseKimiResetTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("reset time is empty")
	}
	// Some Kimi responses contain more than nanosecond precision. Go's time
	// package only retains nanoseconds, so truncate the fractional part before
	// parsing rather than rejecting an otherwise valid reset timestamp.
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		end := len(value)
		for i := dot + 1; i < len(value); i++ {
			if value[i] == 'Z' || value[i] == 'z' || value[i] == '+' || value[i] == '-' {
				end = i
				break
			}
		}
		fraction := value[dot+1 : end]
		if len(fraction) > 9 {
			value = value[:dot+1] + fraction[:9] + value[end:]
		}
	}
	return time.Parse(time.RFC3339Nano, value)
}
