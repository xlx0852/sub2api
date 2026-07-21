package service

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const (
	grokQuotaUpstreamTimeout = 20 * time.Second
	grokBillingSnapshotKey   = "grok_billing_snapshot"
)

type GrokQuotaProbeResult struct {
	Source          string               `json:"source"`
	Billing         *xai.BillingSnapshot `json:"billing,omitempty"`
	Snapshot        *xai.QuotaSnapshot   `json:"snapshot,omitempty"` // legacy rate-limit headers (optional)
	StatusCode      int                  `json:"status_code,omitempty"`
	HeadersObserved bool                 `json:"headers_observed"`
	ResetSupported  bool                 `json:"reset_supported"`
	FetchedAt       int64                `json:"fetched_at"`
}

type GrokQuotaResetResult struct {
	Supported bool   `json:"supported"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type GrokQuotaService struct {
	accountRepo   AccountRepository
	proxyRepo     ProxyRepository
	tokenProvider *GrokTokenProvider
	httpUpstream  HTTPUpstream
}

func NewGrokQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *GrokTokenProvider,
	httpUpstream HTTPUpstream,
) *GrokQuotaService {
	return &GrokQuotaService{
		accountRepo:   accountRepo,
		proxyRepo:     proxyRepo,
		tokenProvider: tokenProvider,
		httpUpstream:  httpUpstream,
	}
}

// ProbeUsage actively queries xAI Grok CLI billing endpoints (same source as CPAMC).
// It sequentially hits /v1/billing?format=credits then /v1/billing, merges the
// config payloads, and persists the snapshot on the account for passive display.
func (s *GrokQuotaService) ProbeUsage(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error) {
	account, token, proxyURL, err := s.prepareProbe(ctx, accountID)
	if err != nil {
		return nil, err
	}

	userID := resolveGrokBillingUserID(account)
	creditsURL, err := xai.BuildBillingURL(true)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_BILLING_URL_INVALID", "invalid billing url: %v", err)
	}
	plainURL, err := xai.BuildBillingURL(false)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_BILLING_URL_INVALID", "invalid billing url: %v", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, grokQuotaUpstreamTimeout)
	defer cancel()

	// Sequential: reuses the same account concurrency slot and keeps mocks simple.
	// Merge semantics match CPAMC (credits first, plain fills monthly gaps).
	creditsSnap, creditsStatus, creditsErr := s.fetchBilling(callCtx, creditsURL, token, userID, proxyURL, account)
	plainSnap, plainStatus, plainErr := s.fetchBilling(callCtx, plainURL, token, userID, proxyURL, account)

	merged := xai.MergeBillingSnapshots(creditsSnap, plainSnap)
	if merged == nil {
		// Both failed: surface the more useful error.
		if creditsErr != nil {
			return nil, creditsErr
		}
		if plainErr != nil {
			return nil, plainErr
		}
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_BILLING_EMPTY", "billing response contained no quota data")
	}

	// Prefer status from a successful snapshot so a failed credits call (e.g. 401)
	// does not poison a successful plain billing result as NeedsReauth.
	statusCode := pickSuccessfulBillingStatus(creditsSnap, creditsStatus, plainSnap, plainStatus)

	merged.StatusCode = statusCode
	merged.Source = "billing_probe"
	if merged.FetchedAt == "" {
		merged.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	}

	// Persist for account list passive display.
	updates := map[string]any{
		grokBillingSnapshotKey: merged,
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("grok_billing_persist_failed",
			"account_id", account.ID,
			"error", err,
		)
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_BILLING_PERSIST_FAILED", "billing fetched but failed to persist: %v", err)
	}

	// Keep legacy rate-limit header snapshot if present (still useful for 429 UI).
	legacy, _ := grokQuotaSnapshotFromExtra(account.Extra)

	return &GrokQuotaProbeResult{
		Source:          "billing_probe",
		Billing:         merged,
		Snapshot:        legacy,
		StatusCode:      statusCode,
		HeadersObserved: legacy != nil && legacy.HeadersObserved,
		ResetSupported:  false,
		FetchedAt:       time.Now().Unix(),
	}, nil
}

// pickSuccessfulBillingStatus returns the HTTP status from a successful snapshot.
// When both succeed, prefers a 2xx status; when only one succeeds, uses that side.
func pickSuccessfulBillingStatus(creditsSnap *xai.BillingSnapshot, creditsStatus int, plainSnap *xai.BillingSnapshot, plainStatus int) int {
	isOK := func(code int) bool { return code >= 200 && code < 300 }

	switch {
	case creditsSnap != nil && plainSnap != nil:
		if isOK(creditsStatus) {
			return creditsStatus
		}
		if isOK(plainStatus) {
			return plainStatus
		}
		if creditsStatus != 0 {
			return creditsStatus
		}
		return plainStatus
	case creditsSnap != nil:
		return creditsStatus
	case plainSnap != nil:
		return plainStatus
	default:
		if creditsStatus != 0 {
			return creditsStatus
		}
		return plainStatus
	}
}

func (s *GrokQuotaService) fetchBilling(
	ctx context.Context,
	targetURL, token, userID, proxyURL string,
	account *Account,
) (*xai.BillingSnapshot, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 0, infraerrors.Newf(http.StatusInternalServerError, "GROK_BILLING_REQUEST_BUILD_FAILED", "failed to build billing request: %v", err)
	}
	xai.ApplyGrokCLIBillingHeaders(req, token, userID)
	// Custom upstream relays may require extra admission headers; apply after defaults.
	if account != nil {
		account.ApplyHeaderOverrides(req.Header)
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return nil, 0, infraerrors.Newf(http.StatusBadGateway, "GROK_BILLING_REQUEST_FAILED", "billing request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, infraerrors.Newf(http.StatusBadGateway, "GROK_BILLING_READ_FAILED", "failed to read billing body: %v", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, resp.StatusCode, infraerrors.New(http.StatusUnauthorized, "GROK_BILLING_UNAUTHENTICATED", "billing API returned 401; re-authorize the Grok account")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, resp.StatusCode, infraerrors.New(http.StatusForbidden, "GROK_BILLING_FORBIDDEN", "billing API returned 403")
	}
	if resp.StatusCode >= 400 {
		bodyText := truncate(strings.TrimSpace(string(bodyBytes)), 240)
		slog.Warn("grok_billing_probe_failed", "account_id", account.ID, "status", resp.StatusCode, "url", targetURL, "body", bodyText)
		return nil, resp.StatusCode, infraerrors.Newf(mapUpstreamStatus(resp.StatusCode), "GROK_BILLING_UPSTREAM_ERROR", "billing API returned %d: %s", resp.StatusCode, bodyText)
	}

	snapshot, err := xai.ParseBillingResponse(bodyBytes)
	if err != nil {
		return nil, resp.StatusCode, infraerrors.Newf(http.StatusBadGateway, "GROK_BILLING_PARSE_FAILED", "failed to parse billing response: %v", err)
	}
	snapshot.StatusCode = resp.StatusCode
	return snapshot, resp.StatusCode, nil
}

func (s *GrokQuotaService) ResetQuota(ctx context.Context, accountID int64) (*GrokQuotaResetResult, error) {
	if _, err := s.loadGrokOAuthAccount(ctx, accountID); err != nil {
		return nil, err
	}
	return nil, infraerrors.New(http.StatusNotImplemented, "GROK_QUOTA_RESET_UNSUPPORTED", "xAI does not expose a Grok subscription quota reset endpoint for OAuth accounts")
}

func (s *GrokQuotaService) prepareProbe(ctx context.Context, accountID int64) (*Account, string, string, error) {
	if s == nil || s.tokenProvider == nil || s.httpUpstream == nil {
		return nil, "", "", infraerrors.New(http.StatusInternalServerError, "GROK_QUOTA_NOT_CONFIGURED", "grok quota service is not configured")
	}
	account, err := s.loadGrokOAuthAccount(ctx, accountID)
	if err != nil {
		return nil, "", "", err
	}

	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, "", "", infraerrors.Newf(http.StatusBadGateway, "GROK_QUOTA_TOKEN_UNAVAILABLE", "failed to acquire access token: %v", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, "", "", infraerrors.New(http.StatusBadGateway, "GROK_QUOTA_TOKEN_UNAVAILABLE", "access token is empty")
	}

	return account, token, s.resolveProxyURL(ctx, account), nil
}

func (s *GrokQuotaService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	switch {
	case account.Proxy != nil:
		return account.Proxy.URL()
	case s != nil && s.proxyRepo != nil:
		if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			return proxy.URL()
		}
	}
	return ""
}

func (s *GrokQuotaService) loadGrokOAuthAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "GROK_QUOTA_NOT_CONFIGURED", "grok quota service is not configured")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusNotFound, "GROK_QUOTA_ACCOUNT_NOT_FOUND", "account not found: %v", err)
	}
	if account == nil {
		return nil, infraerrors.New(http.StatusNotFound, "GROK_QUOTA_ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.Platform != PlatformGrok {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_QUOTA_INVALID_PLATFORM", "account is not a Grok account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_QUOTA_INVALID_TYPE", "account is not an OAuth account")
	}
	return account, nil
}

func resolveGrokBillingUserID(account *Account) string {
	if account == nil {
		return ""
	}
	for _, key := range []string{"sub", "subject", "user_id", "userId", "id"} {
		if v := strings.TrimSpace(account.GetCredential(key)); v != "" {
			return v
		}
	}
	// Nested oauth / user maps sometimes store subject.
	if raw, ok := account.Credentials["oauth"]; ok {
		if m, ok := raw.(map[string]any); ok {
			for _, key := range []string{"sub", "subject", "user_id", "userId"} {
				if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			}
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
