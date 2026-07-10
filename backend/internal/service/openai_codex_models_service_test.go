package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type codexModelsTokenCacheStub struct {
	token string
}

func (s *codexModelsTokenCacheStub) GetAccessToken(context.Context, string) (string, error) {
	return s.token, nil
}

func (s *codexModelsTokenCacheStub) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func (s *codexModelsTokenCacheStub) DeleteAccessToken(context.Context, string) error {
	return nil
}

func (s *codexModelsTokenCacheStub) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (s *codexModelsTokenCacheStub) ReleaseRefreshLock(context.Context, string) error {
	return nil
}

func newCodexModelsTestAccount() *Account {
	return &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acc-123",
		},
	}
}

func TestFetchCodexModelsManifestPassthrough(t *testing.T) {
	manifestBody := `{"models":[{"slug":"gpt-5.5","display_name":"GPT-5.5"}]}`

	var gotAuth, gotAccountID, gotOriginator, gotClientVersion, gotUserAgent, gotVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccountID = r.Header.Get("chatgpt-account-id")
		gotOriginator = r.Header.Get("Originator")
		gotClientVersion = r.URL.Query().Get("client_version")
		gotUserAgent = r.Header.Get("User-Agent")
		gotVersion = r.Header.Get("Version")
		w.Header().Set("ETag", `W/"abc123"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(manifestBody))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	s := &OpenAIGatewayService{}
	manifest, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.137.0", "")
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}

	if string(manifest.Body) != manifestBody {
		t.Errorf("body not passed through verbatim: got %q", manifest.Body)
	}
	if manifest.ETag != `W/"abc123"` {
		t.Errorf("etag not passed through: got %q", manifest.ETag)
	}
	if gotAuth != "Bearer test-access-token" {
		t.Errorf("authorization header: got %q", gotAuth)
	}
	if gotAccountID != "acc-123" {
		t.Errorf("chatgpt-account-id header: got %q", gotAccountID)
	}
	if gotOriginator != "codex_cli_rs" {
		t.Errorf("originator header: got %q", gotOriginator)
	}
	if gotClientVersion != "0.137.0" {
		t.Errorf("client_version query: got %q", gotClientVersion)
	}
	if gotVersion != "0.137.0" {
		t.Errorf("Version header: got %q", gotVersion)
	}
	if !strings.Contains(gotUserAgent, "codex_cli_rs/0.137.0") {
		t.Errorf("User-Agent should include client version, got %q", gotUserAgent)
	}
}

func TestFetchCodexModelsManifestDefaultClientVersion(t *testing.T) {
	var gotClientVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClientVersion = r.URL.Query().Get("client_version")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	s := &OpenAIGatewayService{}
	if _, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "", ""); err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if gotClientVersion != openAICodexProbeVersion {
		t.Errorf("default client_version: got %q, want %q", gotClientVersion, openAICodexProbeVersion)
	}
}

func TestFetchCodexModelsManifestNotModified(t *testing.T) {
	var gotIfNoneMatch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `W/"abc123"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	s := &OpenAIGatewayService{}
	manifest, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.137.0", `W/"abc123"`)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}
	if !manifest.NotModified {
		t.Error("expected NotModified to be true")
	}
	if gotIfNoneMatch != `W/"abc123"` {
		t.Errorf("if-none-match header: got %q", gotIfNoneMatch)
	}
}

func TestFetchCodexModelsManifestUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"boom"}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	s := &OpenAIGatewayService{}
	if _, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.137.0", ""); err == nil {
		t.Fatal("expected error for upstream 500, got nil")
	}
}

func TestFetchCodexModelsManifestMissingToken(t *testing.T) {
	account := newCodexModelsTestAccount()
	delete(account.Credentials, "access_token")

	s := &OpenAIGatewayService{}
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", ""); err == nil {
		t.Fatal("expected error for missing access token, got nil")
	}
}

func TestFetchCodexModelsManifestUsesTokenProvider(t *testing.T) {
	manifestBody := `{"models":[{"slug":"gpt-5.6-sol"}]}`

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(manifestBody))
	}))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original }()

	account := newCodexModelsTestAccount()
	delete(account.Credentials, "access_token")

	s := &OpenAIGatewayService{
		openAITokenProvider: &OpenAITokenProvider{
			tokenCache: &codexModelsTokenCacheStub{token: "provider-access-token"},
		},
	}
	manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", "")
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
	}

	if string(manifest.Body) != manifestBody {
		t.Fatalf("body = %q, want %q", manifest.Body, manifestBody)
	}
	if gotAuth != "Bearer provider-access-token" {
		t.Fatalf("authorization header = %q, want provider token", gotAuth)
	}
}

func TestInspectCodexManifestScore_PrefersGPT56Family(t *testing.T) {
	score := inspectCodexManifestScore([]byte(`{"models":[{"slug":"gpt-5.6-sol"},{"slug":"gpt-5.6-terra"},{"slug":"gpt-5.5"}]}`))
	if score.LatestFamilyCount != 2 {
		t.Fatalf("LatestFamilyCount = %d, want 2", score.LatestFamilyCount)
	}
	if score.TotalModelCount != 3 {
		t.Fatalf("TotalModelCount = %d, want 3", score.TotalModelCount)
	}
	if !score.richEnoughForCodexPicker() {
		t.Fatal("expected richEnoughForCodexPicker for gpt-5.6 family")
	}
}

func TestPickBestCodexManifest_PrefersRicherManifest(t *testing.T) {
	accounts := []*Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
	}

	best, err := pickBestCodexManifest(context.Background(), accounts, func(_ context.Context, account *Account) (*CodexModelsManifest, error) {
		switch account.ID {
		case 1:
			return &CodexModelsManifest{
				Body: []byte(`{"models":[{"slug":"gpt-5.5"},{"slug":"gpt-5.4"}]}`),
				ETag: `W/"older"`,
			}, nil
		case 2:
			return &CodexModelsManifest{
				Body: []byte(`{"models":[{"slug":"gpt-5.6-sol"},{"slug":"gpt-5.6-terra"},{"slug":"gpt-5.5"}]}`),
				ETag: `W/"latest"`,
			}, nil
		default:
			t.Fatalf("unexpected account id %d", account.ID)
			return nil, nil
		}
	})
	if err != nil {
		t.Fatalf("pickBestCodexManifest returned error: %v", err)
	}
	if best == nil {
		t.Fatal("pickBestCodexManifest returned nil manifest")
	}
	if best.ETag != `W/"latest"` {
		t.Fatalf("etag = %q, want latest manifest", best.ETag)
	}
	if !strings.Contains(string(best.Body), "gpt-5.6-sol") {
		t.Fatalf("best manifest body = %s, want gpt-5.6 manifest", string(best.Body))
	}
}

func TestPickBestCodexManifest_EarlyExitOnLatestFamily(t *testing.T) {
	accounts := []*Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
	}

	var probed atomic.Int32
	best, err := pickBestCodexManifest(context.Background(), accounts, func(_ context.Context, account *Account) (*CodexModelsManifest, error) {
		probed.Add(1)
		if account.ID == 1 {
			return &CodexModelsManifest{
				Body: []byte(`{"models":[{"slug":"gpt-5.6-sol"},{"slug":"gpt-5.5"}]}`),
				ETag: `W/"rich"`,
			}, nil
		}
		t.Fatalf("should early-exit before probing account %d", account.ID)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("pickBestCodexManifest returned error: %v", err)
	}
	if best == nil || best.ETag != `W/"rich"` {
		t.Fatalf("unexpected best manifest: %#v", best)
	}
	if probed.Load() != 1 {
		t.Fatalf("probed = %d, want 1 (early exit)", probed.Load())
	}
}

func TestCodexCLIUserAgentForVersion(t *testing.T) {
	ua := codexCLIUserAgentForVersion("0.144.0")
	if !strings.HasPrefix(ua, "codex_cli_rs/0.144.0 ") {
		t.Fatalf("user agent = %q, want codex_cli_rs/0.144.0 prefix", ua)
	}
}

func TestPickBestCodexManifest_CapsProbeCount(t *testing.T) {
	// 5 candidates, but only the first codexModelsMaxAccountsToProbe are consulted.
	// Account 4 would be richest; if the cap works we must NOT see it.
	accounts := make([]*Account, 0, 5)
	for id := int64(1); id <= 5; id++ {
		accounts = append(accounts, &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth})
	}

	var probed []int64
	best, err := pickBestCodexManifest(context.Background(), accounts, func(_ context.Context, account *Account) (*CodexModelsManifest, error) {
		probed = append(probed, account.ID)
		switch account.ID {
		case 1, 2, 3:
			return &CodexModelsManifest{
				Body: []byte(`{"models":[{"slug":"gpt-5.5"},{"slug":"gpt-5.4"}]}`),
				ETag: `W/"capped"`,
			}, nil
		case 4:
			return &CodexModelsManifest{
				Body: []byte(`{"models":[{"slug":"gpt-5.6-sol"},{"slug":"gpt-5.6-terra"}]}`),
				ETag: `W/"richest-but-beyond-cap"`,
			}, nil
		default:
			t.Fatalf("unexpected account id %d", account.ID)
			return nil, nil
		}
	})
	if err != nil {
		t.Fatalf("pickBestCodexManifest returned error: %v", err)
	}
	if len(probed) != codexModelsMaxAccountsToProbe {
		t.Fatalf("probed = %v (len=%d), want exactly %d accounts", probed, len(probed), codexModelsMaxAccountsToProbe)
	}
	for _, id := range probed {
		if id > int64(codexModelsMaxAccountsToProbe) {
			t.Fatalf("probed beyond cap: %v", probed)
		}
	}
	if best == nil || best.ETag != `W/"capped"` {
		t.Fatalf("best = %#v, want capped (not beyond-cap richest)", best)
	}
	if strings.Contains(string(best.Body), "gpt-5.6") {
		t.Fatalf("cap broken: selected body %s", string(best.Body))
	}
}
