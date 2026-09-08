package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDeviceSessionStore struct {
	mu      sync.Mutex
	session *service.ProviderOAuthSession
}

func newMockDeviceSessionStore() *mockDeviceSessionStore {
	return &mockDeviceSessionStore{}
}

func (s *mockDeviceSessionStore) Create(_ context.Context, session *service.ProviderOAuthSession, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *session
	s.session = &copy
	return nil
}

func (s *mockDeviceSessionStore) Get(_ context.Context, _ string) (*service.ProviderOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return nil, nil
	}
	copy := *s.session
	return &copy, nil
}

func (s *mockDeviceSessionStore) AcquirePollLease(_ context.Context, _ string, now time.Time, _ time.Duration) (*service.ProviderOAuthSessionLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return &service.ProviderOAuthSessionLease{}, nil
	}
	copy := *s.session
	if copy.Status != service.ProviderOAuthSessionPending || copy.NextPollAtUnixMilli > now.UnixMilli() {
		return &service.ProviderOAuthSessionLease{Session: &copy}, nil
	}
	copy.Version++
	copy.PollLeaseID = "lease"
	s.session = &copy
	leaseCopy := copy
	return &service.ProviderOAuthSessionLease{Session: &leaseCopy, ID: "lease", Held: true}, nil
}

func (s *mockDeviceSessionStore) CommitPoll(_ context.Context, lease *service.ProviderOAuthSessionLease, updated *service.ProviderOAuthSession) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil || s.session.Status != service.ProviderOAuthSessionPending || lease == nil || !lease.Held || s.session.Version != lease.Session.Version {
		return false, nil
	}
	copy := *updated
	copy.Version = s.session.Version + 1
	copy.PollLeaseID = ""
	s.session = &copy
	return true, nil
}

func (s *mockDeviceSessionStore) Cancel(_ context.Context, _ string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return false, nil
	}
	s.session.Status = service.ProviderOAuthSessionCancelled
	return true, nil
}

func (s *mockDeviceSessionStore) ConsumeAuthorized(_ context.Context, _ string) (*service.ProviderOAuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil || s.session.Status != service.ProviderOAuthSessionAuthorized {
		return nil, nil
	}
	copy := *s.session
	s.session = nil
	return &copy, nil
}

func (s *mockDeviceSessionStore) Delete(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = nil
	return nil
}

type mockAdminService struct {
	service.AdminService
	createdAccount *service.Account
	updatedAccount *service.Account
	account        *service.Account
}

func (m *mockAdminService) CreateAccount(ctx context.Context, input *service.CreateAccountInput) (*service.Account, error) {
	acc := &service.Account{
		ID:          101,
		Name:        input.Name,
		Platform:    input.Platform,
		Type:        input.Type,
		Credentials: input.Credentials,
		ProxyID:     input.ProxyID,
		Concurrency: input.Concurrency,
		Priority:    input.Priority,
	}
	m.createdAccount = acc
	return acc, nil
}

func (m *mockAdminService) GetAccount(ctx context.Context, id int64) (*service.Account, error) {
	if m.account != nil && m.account.ID == id {
		return m.account, nil
	}
	return nil, service.ErrAccountNotFound
}

func (m *mockAdminService) UpdateAccount(ctx context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	if m.account != nil {
		if input.Credentials != nil {
			m.account.Credentials = input.Credentials
		}
		m.updatedAccount = m.account
		return m.account, nil
	}
	return nil, service.ErrAccountNotFound
}

func (m *mockAdminService) ClearAccountError(ctx context.Context, id int64) (*service.Account, error) {
	return m.account, nil
}

func TestCursorOAuthHandler_StartAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newMockDeviceSessionStore()
	oauthSvc := service.NewCursorOAuthService(nil, store)
	adminSvc := &mockAdminService{}
	handler := NewCursorOAuthHandler(oauthSvc, adminSvc, nil)

	// 1. Start authorization
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/cursor/oauth/device/start", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.StartDeviceAuthorization(c)
	assert.Equal(t, http.StatusOK, rec.Code)

	var startResp struct {
		Code int                               `json:"code"`
		Data service.CursorAuthorizationResult `json:"data"`
	}
	err := json.Unmarshal(rec.Body.Bytes(), &startResp)
	require.NoError(t, err)
	assert.NotEmpty(t, startResp.Data.SessionID)
	assert.NotEmpty(t, startResp.Data.LoginURL)
	assert.Equal(t, service.ProviderOAuthSessionPending, startResp.Data.Status)

	// 2. Query status
	recStatus := httptest.NewRecorder()
	cStatus, _ := gin.CreateTestContext(recStatus)
	cStatus.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/cursor/oauth/device/status?session_id="+startResp.Data.SessionID, nil)

	handler.DeviceAuthorizationStatus(cStatus)
	assert.Equal(t, http.StatusOK, recStatus.Code)
}

func TestCursorOAuthHandler_CreateAccountFromDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := newMockDeviceSessionStore()
	oauthSvc := service.NewCursorOAuthService(nil, store)
	adminSvc := &mockAdminService{}
	handler := NewCursorOAuthHandler(oauthSvc, adminSvc, nil)

	// Manually insert an authorized session
	sessionID := "cur_test_session"
	payloadBytes, _ := json.Marshal(map[string]any{
		"token": map[string]any{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"machine_id":    "mach-999",
			"email":         "user@example.com",
			"expires_at":    time.Now().Add(24 * time.Hour).Unix(),
		},
	})
	_ = store.Create(context.Background(), &service.ProviderOAuthSession{
		ID:                 sessionID,
		Provider:           service.PlatformCursor,
		Status:             service.ProviderOAuthSessionAuthorized,
		Payload:            payloadBytes,
		ExpiresAtUnixMilli: time.Now().Add(10 * time.Minute).UnixMilli(),
	}, 10*time.Minute)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"session_id":"cur_test_session","name":"My Cursor Account","concurrency":2,"priority":1}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/cursor/oauth/create-from-device", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.CreateAccountFromDevice(c)
	assert.Equal(t, http.StatusOK, rec.Code)

	require.NotNil(t, adminSvc.createdAccount)
	assert.Equal(t, "My Cursor Account", adminSvc.createdAccount.Name)
	assert.Equal(t, service.PlatformCursor, adminSvc.createdAccount.Platform)
	assert.Equal(t, "test-access-token", adminSvc.createdAccount.Credentials["access_token"])
	assert.Equal(t, "test-refresh-token", adminSvc.createdAccount.Credentials["refresh_token"])
	assert.Equal(t, "mach-999", adminSvc.createdAccount.Credentials["machine_id"])
}
