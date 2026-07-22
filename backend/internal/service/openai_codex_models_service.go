package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

// chatgptCodexModelsURL is the ChatGPT Codex models manifest endpoint.
// Package-level variable so tests can point it at a stub server.
var chatgptCodexModelsURL = "https://chatgpt.com/backend-api/codex/models"

const (
	codexModelsManifestBodyLimit int64 = 8 << 20

	// Codex CLI models-manager wraps list_models in a hard 5s timeout
	// (MODELS_REFRESH_TIMEOUT in codex-rs/models-manager/src/manager.rs).
	// The gateway must finish well under that budget or the client silently
	// keeps the bundled/frozen catalog.
	codexModelsClientBudget       = 4 * time.Second
	codexModelsUpstreamTimeout    = 3500 * time.Millisecond
	codexModelsMaxAccountsToProbe = 3
)

// CodexModelsManifest carries the raw upstream manifest payload plus caching
// metadata so handlers can pass both through to the client untouched.
type CodexModelsManifest struct {
	Body        []byte
	ETag        string
	NotModified bool
}

type codexManifestModelEntry struct {
	Slug string `json:"slug"`
	ID   string `json:"id"`
}

type codexManifestEnvelope struct {
	Models []codexManifestModelEntry `json:"models"`
}

type codexManifestScore struct {
	LatestFamilyCount int
	TotalModelCount   int
}

func (s codexManifestScore) betterThan(other codexManifestScore) bool {
	if s.LatestFamilyCount != other.LatestFamilyCount {
		return s.LatestFamilyCount > other.LatestFamilyCount
	}
	return s.TotalModelCount > other.TotalModelCount
}

// richEnoughForCodexPicker reports whether the manifest already contains the
// newest model family Codex clients are trying to discover. Once we see it we
// can stop probing additional accounts inside the 5s client budget.
func (s codexManifestScore) richEnoughForCodexPicker() bool {
	return s.LatestFamilyCount > 0
}

func inspectCodexManifestScore(body []byte) codexManifestScore {
	var envelope codexManifestEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return codexManifestScore{}
	}

	score := codexManifestScore{TotalModelCount: len(envelope.Models)}
	for _, model := range envelope.Models {
		modelID := strings.TrimSpace(model.Slug)
		if modelID == "" {
			modelID = strings.TrimSpace(model.ID)
		}
		if strings.HasPrefix(strings.ToLower(modelID), "gpt-5.6") {
			score.LatestFamilyCount++
		}
	}
	return score
}

func sortUniqueCodexManifestAccounts(ctx context.Context, repo AccountRepository, accounts []Account) []*Account {
	if len(accounts) == 0 {
		return nil
	}

	ordered := make([]*Account, 0, len(accounts))
	seen := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if !account.IsOpenAIOAuth() {
			continue
		}
		credAccount, err := resolveCredentialAccount(ctx, repo, account)
		if err != nil || credAccount == nil || !credAccount.IsOpenAIOAuth() {
			continue
		}
		if _, exists := seen[credAccount.ID]; exists {
			continue
		}
		seen[credAccount.ID] = struct{}{}
		// Keep the schedulable account pointer (for proxy / scheduling metadata)
		// but ensure credentials resolve to the OAuth parent above.
		ordered = append(ordered, account)
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority < ordered[j].Priority
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

func pickBestCodexManifest(
	ctx context.Context,
	accounts []*Account,
	fetch func(context.Context, *Account) (*CodexModelsManifest, error),
) (*CodexModelsManifest, error) {
	var best *CodexModelsManifest
	bestScore := codexManifestScore{}
	var firstErr error
	probed := 0

	for _, account := range accounts {
		if account == nil {
			continue
		}
		if probed >= codexModelsMaxAccountsToProbe {
			break
		}
		if err := ctx.Err(); err != nil {
			if best != nil {
				return best, nil
			}
			if firstErr != nil {
				return nil, firstErr
			}
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "codex models manifest budget exceeded: %v", err)
		}

		probed++
		manifest, err := fetch(ctx, account)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		score := inspectCodexManifestScore(manifest.Body)
		if best == nil || score.betterThan(bestScore) {
			best = manifest
			bestScore = score
		}
		// Codex only needs one rich catalog snapshot. Stop as soon as the
		// newest family is present so we stay under the CLI's 5s timeout.
		if bestScore.richEnoughForCodexPicker() {
			return best, nil
		}
	}

	if best != nil {
		return best, nil
	}
	return nil, firstErr
}

func codexCLIUserAgentForVersion(clientVersion string) string {
	clientVersion = strings.TrimSpace(clientVersion)
	if clientVersion == "" {
		return codexCLIUserAgent
	}
	// Mirror codex_cli_rs/<version> ... shape used by the real CLI.
	if strings.HasPrefix(codexCLIUserAgent, "codex_cli_rs/") {
		if rest, ok := strings.CutPrefix(codexCLIUserAgent, "codex_cli_rs/"); ok {
			if _, after, found := strings.Cut(rest, " "); found {
				return "codex_cli_rs/" + clientVersion + " " + after
			}
		}
	}
	return fmt.Sprintf("codex_cli_rs/%s", clientVersion)
}

func resolveCodexManifestProxyURL(account, credAccount *Account) string {
	for _, candidate := range []*Account{account, credAccount} {
		if candidate == nil {
			continue
		}
		if candidate.ProxyID != nil && candidate.Proxy != nil {
			if proxyURL := candidate.Proxy.URL(); proxyURL != "" {
				return proxyURL
			}
		}
	}
	return ""
}

// FetchPreferredCodexModelsManifest selects a usable OpenAI OAuth account for
// Codex model manifest serving under a hard client budget.
//
// Codex CLI (models-manager) only refreshes the remote catalog under ChatGPT
// auth, then hard-times-out list_models at 5s. This path therefore:
//  1. uses TokenProvider-backed OAuth tokens (same as /responses)
//  2. probes schedulable OAuth accounts in priority order (lower Priority first)
//  3. keeps the best score among the probed set (gpt-5.6* family count, then total)
//  4. early-exits once a "rich enough" catalog (any gpt-5.6*) is found
//  5. caps probes at codexModelsMaxAccountsToProbe so we stay under the budget
//
// Important boundary: this is NOT a strict global "richest entitlement in the
// entire group" guarantee. Accounts beyond the probe cap are not consulted.
func (s *OpenAIGatewayService) FetchPreferredCodexModelsManifest(ctx context.Context, groupID *int64, clientVersion, ifNoneMatch string) (*CodexModelsManifest, error) {
	budgetCtx, cancel := context.WithTimeout(ctx, codexModelsClientBudget)
	defer cancel()

	accounts, err := s.listSchedulableAccounts(budgetCtx, groupID, PlatformOpenAI)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_ACCOUNTS_FAILED", "list schedulable openai accounts: %v", err)
	}

	candidates := sortUniqueCodexManifestAccounts(budgetCtx, s.accountRepo, accounts)
	if len(candidates) == 0 {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_MODELS_ACCOUNT_REQUIRED", "no schedulable OpenAI OAuth accounts for Codex manifest")
	}

	manifest, err := pickBestCodexManifest(budgetCtx, candidates, func(fetchCtx context.Context, account *Account) (*CodexModelsManifest, error) {
		// Intentionally do not forward client If-None-Match to upstream while
		// probing multiple accounts: a 304 from a poorer account would hide a
		// richer catalog on another account.
		return s.FetchCodexModelsManifest(fetchCtx, account, clientVersion, "")
	})
	if err != nil {
		return nil, err
	}

	if ifNoneMatch = strings.TrimSpace(ifNoneMatch); ifNoneMatch != "" && strings.TrimSpace(manifest.ETag) == ifNoneMatch {
		return &CodexModelsManifest{
			ETag:        manifest.ETag,
			NotModified: true,
		}, nil
	}
	return manifest, nil
}

// FetchCodexModelsManifest fetches the live Codex models manifest from the
// ChatGPT backend using the account's OAuth credentials.
//
// The response body is passed through verbatim: the manifest schema evolves
// with Codex client releases (see codex_protocol::openai_models::ModelsResponse),
// and interpreting it here would force the gateway to chase upstream changes.
// Passing it through keeps the gateway schema-agnostic and always reflects the
// account's real entitlements.
func (s *OpenAIGatewayService) FetchCodexModelsManifest(ctx context.Context, account *Account, clientVersion, ifNoneMatch string) (*CodexModelsManifest, error) {
	if account == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_ACCOUNT_REQUIRED", "account is required")
	}
	credAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_CREDENTIALS_FAILED", "resolve credential account: %v", err)
	}

	// Critical: use TokenProvider (cache + refresh), not a raw DB credential
	// snapshot. Codex CLI only refreshes the picker under ChatGPT OAuth; a
	// stale access_token here produces upstream 401 which surfaces as 502 and
	// freezes the client on its bundled catalog.
	// Agent Identity accounts authenticate via assertion headers instead of OAuth.
	var accessToken string
	if credAccount.IsOpenAIAgentIdentity() {
		accessToken = strings.TrimSpace(credAccount.GetOpenAIAccessToken())
	} else {
		token, tokenType, err := s.GetAccessToken(ctx, credAccount)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_TOKEN_MISSING", "get codex backend access token: %v", err)
		}
		if tokenType != "oauth" || strings.TrimSpace(token) == "" {
			return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_MODELS_TOKEN_MISSING", "account has no Codex backend access token")
		}
		accessToken = token
	}
	if accessToken == "" && !credAccount.IsOpenAIAgentIdentity() {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_MODELS_TOKEN_MISSING", "account has no Codex backend access token")
	}

	clientVersion = strings.TrimSpace(clientVersion)
	if clientVersion == "" {
		clientVersion = openAICodexProbeVersion
	}
	requestURL := chatgptCodexModelsURL + "?client_version=" + url.QueryEscape(clientVersion)

	reqCtx, cancel := context.WithTimeout(ctx, codexModelsUpstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_REQUEST_FAILED", "create codex models request: %v", err)
	}
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, credAccount, accessToken)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_AUTH_FAILED", "build Codex models authentication: %v", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("Version", clientVersion)
	req.Header.Set("User-Agent", codexCLIUserAgentForVersion(clientVersion))
	if ifNoneMatch = strings.TrimSpace(ifNoneMatch); ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	setOpenAIChatGPTAccountHeaders(req.Header, credAccount)

	proxyURL := resolveCodexManifestProxyURL(account, credAccount)
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:              proxyURL,
		Timeout:               codexModelsUpstreamTimeout,
		ResponseHeaderTimeout: codexModelsUpstreamTimeout,
	})
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_PROXY_INVALID", "invalid proxy configuration: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "codex models manifest request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		return &CodexModelsManifest{ETag: resp.Header.Get("ETag"), NotModified: true}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "codex models manifest upstream error %d: %s", resp.StatusCode, message)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, codexModelsManifestBodyLimit))
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "read codex models manifest response: %v", err)
	}
	return &CodexModelsManifest{Body: body, ETag: resp.Header.Get("ETag")}, nil
}
