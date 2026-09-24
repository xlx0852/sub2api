package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ApplyMirasimModelMapping 端点测试。
//
// 关键约束（改动前请先读）：本端点的风险不在「写不写映射」，而在
//  1. 凭据保全：service 层 merge 语义是「非敏感键完全由 incoming 决定」，
//     所以必须断言 base_url 等非敏感键被原样保留、敏感键不进 incoming；
//  2. 绝不写空映射：上游清单为空时必须报错而不是落库空映射；
//  3. 幂等/覆盖语义：已有映射时默认 409，overwrite=true 才替换。
//
// 这里用 *service.AccountTestService 之外的桩注入（见 upstreamModelsFetcher 注释）。

type mirasimApplyAdminService struct {
	*stubAdminService
	account      *service.Account
	getErr       error
	updateErr    error
	updateCalls  int
	lastUpdateID int64
	lastUpdate   *service.UpdateAccountInput
}

func (s *mirasimApplyAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.account == nil || s.account.ID != id {
		return nil, errors.New("account not found")
	}
	return s.account, nil
}

func (s *mirasimApplyAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	s.updateCalls++
	s.lastUpdateID = id
	s.lastUpdate = input
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	updated := *s.account
	updated.Credentials = input.Credentials
	return &updated, nil
}

type stubUpstreamModels struct {
	models []string
	err    error
	calls  int
}

func (s *stubUpstreamModels) FetchUpstreamSupportedModels(_ context.Context, _ *service.Account) ([]string, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.models, nil
}

func setupApplyMirasimModelMappingRouter(adminSvc service.AdminService, fetcher upstreamModelsFetcher) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.upstreamModels = fetcher
	router.POST("/api/v1/admin/accounts/:id/models/apply-mapping", handler.ApplyMirasimModelMapping)
	return router
}

// mirasimRefreshTokenCredentialKey 与 service 内部常量 mirasimCredentialRefreshToken 同值。
// 跨包单测拿不到未导出常量，这里写死字面量：如果服务端改名，本测试会立刻失败（这正是期望）。
const mirasimRefreshTokenCredentialKey = "mirasim_refresh_token"

func mirasimRelayAccountFixture(credentials map[string]any) *service.Account {
	creds := map[string]any{mirasimRefreshTokenCredentialKey: "rt-secret"}
	for key, value := range credentials {
		creds[key] = value
	}
	return &service.Account{
		ID:          140,
		Name:        "mirasim-relay",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Credentials: creds,
	}
}

func decodeApplyMappingResponse(t *testing.T, rec *httptest.ResponseRecorder) (models []string, changed bool) {
	t.Helper()
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Models  []string `json:"models"`
			Changed bool     `json:"changed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	return payload.Data.Models, payload.Data.Changed
}

func TestApplyMirasimModelMapping_WritesIdentityMappingAndPreservesCredentials(t *testing.T) {
	adminSvc := &mirasimApplyAdminService{
		stubAdminService: newStubAdminService(),
		account: mirasimRelayAccountFixture(map[string]any{
			"base_url":           "https://relay.example.com",
			"mirasim_relay_url":  "https://relay.example.com",
			"mirasim_device_key": "device-secret",
		}),
	}
	fetcher := &stubUpstreamModels{models: []string{"deepseek-flash", "kimi-k3", " kimi-k3 "}}
	router := setupApplyMirasimModelMappingRouter(adminSvc, fetcher)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/140/models/apply-mapping", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	models, changed := decodeApplyMappingResponse(t, rec)
	require.True(t, changed)
	require.Equal(t, []string{"deepseek-flash", "kimi-k3"}, models, "响应里的模型清单应去重且有序")

	require.Equal(t, 1, adminSvc.updateCalls)
	require.Equal(t, int64(140), adminSvc.lastUpdateID)
	require.NotNil(t, adminSvc.lastUpdate)

	incoming := adminSvc.lastUpdate.Credentials
	// 非敏感键必须保全（merge 语义下漏传即等于删除）。
	require.Equal(t, "https://relay.example.com", incoming["base_url"])
	require.Equal(t, "https://relay.example.com", incoming["mirasim_relay_url"])
	// 敏感键不得进入 incoming（由 merge 保留 existing）。
	require.NotContains(t, incoming, mirasimRefreshTokenCredentialKey)
	require.NotContains(t, incoming, "mirasim_device_key")

	mapping, ok := incoming["model_mapping"].(map[string]any)
	require.True(t, ok, "model_mapping 必须是 map[string]any（domain 层只认这个类型）")
	require.Equal(t, map[string]any{
		"deepseek-flash": "deepseek-flash",
		"kimi-k3":        "kimi-k3",
	}, mapping)
	for key, value := range mapping {
		_, isString := value.(string)
		require.True(t, isString, "映射值必须是 string: %s", key)
	}
}

func TestApplyMirasimModelMapping_RejectsNonMirasimAccount(t *testing.T) {
	adminSvc := &mirasimApplyAdminService{
		stubAdminService: newStubAdminService(),
		account: &service.Account{
			ID:          7,
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Credentials: map[string]any{"api_key": "openai-key"},
		},
	}
	fetcher := &stubUpstreamModels{models: []string{"gpt-5"}}
	router := setupApplyMirasimModelMappingRouter(adminSvc, fetcher)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/7/models/apply-mapping", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, adminSvc.updateCalls)
	require.Equal(t, 0, fetcher.calls, "非 relay 账号不应发起上游请求")
}

func TestApplyMirasimModelMapping_NotFoundWhenAccountMissing(t *testing.T) {
	adminSvc := &mirasimApplyAdminService{
		stubAdminService: newStubAdminService(),
		getErr:           errors.New("no rows"),
	}
	router := setupApplyMirasimModelMappingRouter(adminSvc, &stubUpstreamModels{models: []string{"m"}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/999/models/apply-mapping", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, 0, adminSvc.updateCalls)
}

func TestApplyMirasimModelMapping_ConflictWhenMappingExistsWithoutOverwrite(t *testing.T) {
	adminSvc := &mirasimApplyAdminService{
		stubAdminService: newStubAdminService(),
		account: mirasimRelayAccountFixture(map[string]any{
			"model_mapping": map[string]any{"deepseek-v4.1-flash": "deepseek-flash"},
		}),
	}
	fetcher := &stubUpstreamModels{models: []string{"deepseek-flash"}}
	router := setupApplyMirasimModelMappingRouter(adminSvc, fetcher)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/140/models/apply-mapping", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, 0, adminSvc.updateCalls, "默认不覆盖人工维护的别名映射")
}

func TestApplyMirasimModelMapping_OverwriteReplacesExistingMapping(t *testing.T) {
	adminSvc := &mirasimApplyAdminService{
		stubAdminService: newStubAdminService(),
		account: mirasimRelayAccountFixture(map[string]any{
			"model_mapping": map[string]any{"deepseek-v4.1-flash": "deepseek-flash"},
		}),
	}
	router := setupApplyMirasimModelMappingRouter(adminSvc, &stubUpstreamModels{models: []string{"deepseek-flash"}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/140/models/apply-mapping",
		strings.NewReader(`{"overwrite":true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	_, changed := decodeApplyMappingResponse(t, rec)
	require.True(t, changed)
	require.Equal(t, 1, adminSvc.updateCalls)
	require.Equal(t, map[string]any{"deepseek-flash": "deepseek-flash"},
		adminSvc.lastUpdate.Credentials["model_mapping"])
}

func TestApplyMirasimModelMapping_IdempotentWhenMappingMatchesUpstream(t *testing.T) {
	adminSvc := &mirasimApplyAdminService{
		stubAdminService: newStubAdminService(),
		account: mirasimRelayAccountFixture(map[string]any{
			"model_mapping": map[string]any{
				"deepseek-flash": "deepseek-flash",
				"kimi-k3":        "kimi-k3",
			},
		}),
	}
	router := setupApplyMirasimModelMappingRouter(adminSvc, &stubUpstreamModels{models: []string{"kimi-k3", "deepseek-flash"}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/140/models/apply-mapping",
		strings.NewReader(`{"overwrite":true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	models, changed := decodeApplyMappingResponse(t, rec)
	require.False(t, changed)
	require.Equal(t, []string{"deepseek-flash", "kimi-k3"}, models)
	require.Equal(t, 0, adminSvc.updateCalls, "逐键一致时不应写库")
}

func TestApplyMirasimModelMapping_EmptyUpstreamListNeverWritesEmptyMapping(t *testing.T) {
	adminSvc := &mirasimApplyAdminService{
		stubAdminService: newStubAdminService(),
		account:          mirasimRelayAccountFixture(nil),
	}
	router := setupApplyMirasimModelMappingRouter(adminSvc, &stubUpstreamModels{models: []string{" ", ""}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/140/models/apply-mapping", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Equal(t, 0, adminSvc.updateCalls, "空清单写入空映射等于恢复「认领所有模型」")
	require.Contains(t, rec.Body.String(), "no available models")
}

func TestApplyMirasimModelMapping_UpstreamErrorDoesNotExposeUpstreamBody(t *testing.T) {
	adminSvc := &mirasimApplyAdminService{
		stubAdminService: newStubAdminService(),
		account:          mirasimRelayAccountFixture(nil),
	}
	fetcher := &stubUpstreamModels{err: &service.UpstreamModelSyncError{
		Kind:    service.UpstreamModelSyncErrorUpstream,
		Message: "Failed to fetch mirasim available models (upstream)",
		Err:     errors.New("SECRET_UPSTREAM_BODY"),
	}}
	router := setupApplyMirasimModelMappingRouter(adminSvc, fetcher)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/140/models/apply-mapping", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.NotContains(t, rec.Body.String(), "SECRET_UPSTREAM_BODY")
	require.Equal(t, 0, adminSvc.updateCalls)
}

func TestApplyMirasimModelMapping_ConfigErrorReturnsBadRequest(t *testing.T) {
	adminSvc := &mirasimApplyAdminService{
		stubAdminService: newStubAdminService(),
		account:          mirasimRelayAccountFixture(nil),
	}
	fetcher := &stubUpstreamModels{err: &service.UpstreamModelSyncError{
		Kind:    service.UpstreamModelSyncErrorConfiguration,
		Message: "Mirasim relay credentials are not configured",
	}}
	router := setupApplyMirasimModelMappingRouter(adminSvc, fetcher)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/140/models/apply-mapping", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, adminSvc.updateCalls)
}

func TestApplyMirasimModelMapping_MissingFetcherReturnsServerError(t *testing.T) {
	adminSvc := &mirasimApplyAdminService{
		stubAdminService: newStubAdminService(),
		account:          mirasimRelayAccountFixture(nil),
	}
	router := setupApplyMirasimModelMappingRouter(adminSvc, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/140/models/apply-mapping", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, 0, adminSvc.updateCalls)
}

func TestApplyMirasimModelMapping_InvalidAccountID(t *testing.T) {
	router := setupApplyMirasimModelMappingRouter(&mirasimApplyAdminService{stubAdminService: newStubAdminService()}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/abc/models/apply-mapping", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
