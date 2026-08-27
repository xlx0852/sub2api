package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/stretchr/testify/require"
)

type retrospectiveRiskTestEvidenceStore struct {
	payload []byte
	getErr  error
	deleted []string
}

func (s *retrospectiveRiskTestEvidenceStore) Configured() bool          { return true }
func (s *retrospectiveRiskTestEvidenceStore) CurrentKeyVersion() string { return "v1" }
func (s *retrospectiveRiskTestEvidenceStore) Put(context.Context, string, string, []byte) (int64, error) {
	return 0, nil
}
func (s *retrospectiveRiskTestEvidenceStore) Get(context.Context, string, string) ([]byte, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return append([]byte(nil), s.payload...), nil
}

type retrospectiveRiskRunTestRepo struct {
	ports.RetrospectiveRiskRepository
	items          []domain.RetrospectiveEvidence
	claimCalls     int
	lockCalls      int
	failCalls      int
	profileUpdates int
	completed      int
	updates        []domain.RetrospectiveReviewRun
	expired        []domain.RetrospectiveEvidence
	markedDeleted  []string
}

func (r *retrospectiveRiskRunTestRepo) TryReviewLock(context.Context) (func(), bool, error) {
	r.lockCalls++
	return func() {}, true, nil
}

func (r *retrospectiveRiskRunTestRepo) CreateReviewRun(_ context.Context, key string, day time.Time, manual bool, policy string, _ []byte) (*domain.RetrospectiveReviewRun, bool, error) {
	return &domain.RetrospectiveReviewRun{ID: 1, RunKey: key, ScheduledDay: day, Manual: manual, PolicyVersion: policy, Status: "running", StartedAt: time.Now()}, true, nil
}

func (r *retrospectiveRiskRunTestRepo) ClaimEvidence(context.Context, int64, string, int) ([]domain.RetrospectiveEvidence, error) {
	r.claimCalls++
	if r.claimCalls > 1 {
		return nil, nil
	}
	return append([]domain.RetrospectiveEvidence(nil), r.items...), nil
}

func (r *retrospectiveRiskRunTestRepo) FailReviewItem(context.Context, int64, string, string, string) error {
	r.failCalls++
	return nil
}

func (r *retrospectiveRiskRunTestRepo) CompleteReviewItem(context.Context, int64, string, domain.RetrospectiveEvidence, domain.RetrospectiveReviewResult) (int64, error) {
	r.completed++
	return int64(r.completed), nil
}

func (r *retrospectiveRiskRunTestRepo) UpdateReviewRun(_ context.Context, run *domain.RetrospectiveReviewRun) error {
	r.updates = append(r.updates, *run)
	return nil
}

func (r *retrospectiveRiskRunTestRepo) UpsertRiskProfileFromFinding(context.Context, domain.RetrospectiveEvidence, int64, domain.RetrospectiveReviewResult, time.Duration, bool) (*domain.UserRiskProfile, error) {
	r.profileUpdates++
	return nil, nil
}

func (r *retrospectiveRiskRunTestRepo) ListExpiredEvidence(context.Context, time.Time, int) ([]domain.RetrospectiveEvidence, error) {
	return append([]domain.RetrospectiveEvidence(nil), r.expired...), nil
}

func (r *retrospectiveRiskRunTestRepo) MarkEvidenceDeleted(_ context.Context, id string) error {
	r.markedDeleted = append(r.markedDeleted, id)
	return nil
}

func retrospectiveRunTestModeration(t *testing.T, riskCfg RetrospectiveRiskConfig) *ContentModerationService {
	t.Helper()
	cfg := defaultContentModerationConfig()
	cfg.Retrospective = riskCfg
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	return NewContentModerationService(&contentModerationTestSettingRepo{values: map[string]string{SettingKeyContentModerationConfig: string(raw)}}, nil, nil, nil, nil, nil, nil)
}
func (s *retrospectiveRiskTestEvidenceStore) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}
func (s *retrospectiveRiskTestEvidenceStore) List(context.Context, string, int) (domain.EvidenceObjectPage, error) {
	return domain.EvidenceObjectPage{}, nil
}
func (s *retrospectiveRiskTestEvidenceStore) Probe(context.Context) error { return nil }

func TestRetrospectiveDailySampleIsStableWithinDay(t *testing.T) {
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	first := retrospectiveDailySample(42, now, strings.Repeat("a", 64), 17)
	for i := 0; i < 20; i++ {
		require.Equal(t, first, retrospectiveDailySample(42, now.Add(time.Duration(i)*time.Minute), strings.Repeat("a", 64), 17))
	}
	require.False(t, retrospectiveDailySample(42, now, strings.Repeat("a", 64), 0))
	require.True(t, retrospectiveDailySample(42, now, strings.Repeat("a", 64), 100))
}

func TestTruncateUTF8BytesPreservesValidEncoding(t *testing.T) {
	got, truncated := truncateUTF8Bytes("你好abcdef", 7)
	require.True(t, truncated)
	require.True(t, utf8.ValidString(got))
	require.LessOrEqual(t, len([]byte(got)), 7)
}

func TestRetrospectiveEvidencePayloadExcludesAttribution(t *testing.T) {
	payload, err := json.Marshal(retrospectiveEvidencePayload{
		SchemaVersion: 2, CaseID: "case-1", OriginalContent: "client content",
		Protocol: "openai_responses", Model: "gpt-5.6", CapturedAt: time.Now(),
	})
	require.NoError(t, err)
	text := string(payload)
	require.NotContains(t, text, "email")
	require.NotContains(t, text, "api_key")
	require.NotContains(t, text, "user_id")
	require.NotContains(t, text, "account")
	require.NotContains(t, text, "ip")
}

func TestSanitizeRetrospectiveReviewContentPreservesIntentAndReferences(t *testing.T) {
	secret := "super-secret-token-123456"
	hash := strings.Repeat("a", 64)
	uuid := "123e4567-e89b-12d3-a456-426614174000"
	input := "删除目标数据，Authorization: Bearer " + secret +
		"，请求 https://target.example/admin/delete?access_token=" + secret +
		"，校验 hash=" + hash + " task=" + uuid

	out := sanitizeRetrospectiveReviewContent(input)

	require.NotContains(t, out, secret)
	require.Contains(t, out, "删除目标数据")
	require.Contains(t, out, "https://target.example/admin/delete?access_token=")
	require.Contains(t, out, hash)
	require.Contains(t, out, uuid)
	require.Equal(t, 2, strings.Count(out, "<SECRET:bearer#1>"))
}

func TestSanitizeRetrospectiveReviewContentLeavesOrdinaryEvidenceUnchanged(t *testing.T) {
	input := "分析 https://target.example/path 的响应，hash=" + strings.Repeat("c", 64) +
		" request=123e4567-e89b-12d3-a456-426614174000"

	require.Equal(t, input, sanitizeRetrospectiveReviewContent(input))
}

func TestSanitizeRetrospectiveReviewContentCoversQuotedAndPEMSecrets(t *testing.T) {
	input := "password=\"correct horse battery staple\"\n-----BEGIN PRIVATE KEY-----\nprivate-material\n-----END PRIVATE KEY-----"

	out := sanitizeRetrospectiveReviewContent(input)

	require.NotContains(t, out, "correct horse battery staple")
	require.NotContains(t, out, "private-material")
	require.Contains(t, out, "password=\"<SECRET:password#")
	require.Contains(t, out, "<SECRET:private_key#")
}

func TestReviewEvidenceSendsSemanticCopyAndKeepsEncryptedOriginal(t *testing.T) {
	secret := "super-secret-token-123456"
	hash := strings.Repeat("b", 64)
	original := "使用 Authorization: Bearer " + secret + " 删除 https://target.example/admin?token=" + secret + " hash=" + hash
	payload, err := json.Marshal(retrospectiveEvidencePayload{
		SchemaVersion: 2, CaseID: "case-1", OriginalContent: original,
		Protocol: "openai_responses", Model: "gpt-5.6", CapturedAt: time.Now(),
	})
	require.NoError(t, err)

	receivedInputs := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		receivedInputs <- request.Input
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"flagged":false,"category_scores":{"illicit":0.1}}]}`))
	}))
	defer server.Close()

	moderation := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil)
	moderation.httpClient = server.Client()
	cfg := defaultContentModerationConfig()
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"test-moderation-key"}
	cfg.RetryCount = 0
	cfg.TimeoutMS = 1000
	svc := &RetrospectiveRiskService{
		store:      &retrospectiveRiskTestEvidenceStore{payload: payload},
		moderation: moderation,
	}

	result, recovered, err := svc.reviewEvidence(context.Background(), cfg, domain.RetrospectiveEvidence{
		ID: "case-1", ObjectKey: "risk-evidence/pending/case-1.enc", KeyVersion: "v1",
	})
	require.NoError(t, err)
	require.False(t, result.Flagged)
	receivedInput := <-receivedInputs
	require.JSONEq(t, string(payload), string(recovered))
	require.Contains(t, string(recovered), secret)
	require.NotContains(t, receivedInput, secret)
	require.Contains(t, receivedInput, "https://target.example/admin?token=")
	require.Contains(t, receivedInput, hash)
	require.Equal(t, 2, strings.Count(receivedInput, "<SECRET:bearer#1>"))
}

func TestRetrospectiveConfigNormalizeAppliesHardLimits(t *testing.T) {
	cfg := RetrospectiveRiskConfig{
		BaselineSampleRate: 1000, QueueSize: 999999, PerUserDailyItems: 999,
		PerUserDailyBytes: 99 << 20, MaxItemBytes: 10 << 20,
		PendingRetentionDays: 100, FlaggedRetentionDays: 100,
		ReviewHour: 99, MaxReviewItems: 999999, MaxReviewInputTokens: 999999999,
		RiskTTLHours: 999999,
	}
	cfg.normalize()
	require.Equal(t, 100, cfg.BaselineSampleRate)
	require.Equal(t, retrospectiveRiskMaxQueueSize, cfg.QueueSize)
	require.Equal(t, retrospectiveRiskMaxDailyItems, cfg.PerUserDailyItems)
	require.Equal(t, retrospectiveRiskMaxDailyBytes, cfg.PerUserDailyBytes)
	require.Equal(t, retrospectiveRiskMaxItemBytes, cfg.MaxItemBytes)
	require.Equal(t, 3, cfg.PendingRetentionDays)
	require.Equal(t, 30, cfg.FlaggedRetentionDays)
	require.Equal(t, 2, cfg.ReviewHour)
	require.Equal(t, 10000, cfg.MaxReviewItems)
	require.Equal(t, int64(20_000_000), cfg.MaxReviewInputTokens)
	require.Equal(t, 30*24, cfg.RiskTTLHours)
}

func TestRetrospectiveConfigEnforcementRequiresReviewAndElevation(t *testing.T) {
	cfg := defaultRetrospectiveRiskConfig()
	cfg.EnforcementEnabled = true
	cfg.normalize()
	require.False(t, cfg.EnforcementEnabled)
	cfg.ReviewEnabled = true
	cfg.ElevationEnabled = true
	cfg.EnforcementEnabled = true
	cfg.normalize()
	require.True(t, cfg.EnforcementEnabled)
}

func TestClassifyRetrospectiveReviewError(t *testing.T) {
	require.Equal(t, "retryable: review timeout", classifyRetrospectiveReviewError(context.DeadlineExceeded))
	require.Equal(t, "retryable: review dependency unavailable", classifyRetrospectiveReviewError(errors.New("upstream 503")))
	require.Equal(t, "terminal: invalid evidence", classifyRetrospectiveReviewError(errors.New("invalid evidence payload")))
}

func TestManualReviewFailsClosedWhenReviewDisabledOrUnconfigured(t *testing.T) {
	var nilService *RetrospectiveRiskService
	require.False(t, nilService.TriggerReview().Accepted)
	require.False(t, (&RetrospectiveRiskService{}).TriggerReview().Accepted)
}

func TestRetrospectiveCaptureQueueIsBoundedAndDropsWithoutBlocking(t *testing.T) {
	queue := make(chan retrospectiveCaptureTask, 2)
	require.True(t, enqueueRetrospectiveCapture(queue, 1, retrospectiveCaptureTask{}))
	require.False(t, enqueueRetrospectiveCapture(queue, 1, retrospectiveCaptureTask{}))
	require.Len(t, queue, 1)
}

func TestRetrospectiveReviewCircuitBreakerFailsSafe(t *testing.T) {
	cfg := defaultRetrospectiveRiskConfig()
	cfg.ReviewEnabled = true
	items := make([]domain.RetrospectiveEvidence, 5)
	for i := range items {
		items[i] = domain.RetrospectiveEvidence{ID: fmt.Sprintf("case-%d", i), ObjectKey: fmt.Sprintf("risk-evidence/pending/%d", i), KeyVersion: "v1"}
	}
	repo := &retrospectiveRiskRunTestRepo{items: items}
	svc := &RetrospectiveRiskService{repo: repo, store: &retrospectiveRiskTestEvidenceStore{getErr: errors.New("upstream 503")}, moderation: retrospectiveRunTestModeration(t, cfg)}
	svc.runReview("manual:test-circuit", time.Now(), true)
	require.Equal(t, 5, repo.failCalls)
	require.Zero(t, repo.profileUpdates)
	require.NotEmpty(t, repo.updates)
	require.Equal(t, "failed", repo.updates[len(repo.updates)-1].Status)
	require.Contains(t, repo.updates[len(repo.updates)-1].Error, "circuit open")
}

func TestRetrospectiveReviewSingleProcessGuard(t *testing.T) {
	repo := &retrospectiveRiskRunTestRepo{}
	svc := &RetrospectiveRiskService{repo: repo, store: &retrospectiveRiskTestEvidenceStore{}, moderation: retrospectiveRunTestModeration(t, defaultRetrospectiveRiskConfig())}
	svc.running.Store(true)
	svc.runReview("manual:already-running", time.Now(), true)
	require.Zero(t, repo.lockCalls)
}

func TestRetrospectiveReviewStopsAtItemBudget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"flagged":false,"category_scores":{"illicit":0.1}}]}`))
	}))
	defer server.Close()
	riskCfg := defaultRetrospectiveRiskConfig()
	riskCfg.ReviewEnabled = true
	riskCfg.MaxReviewItems = 1
	cfg := defaultContentModerationConfig()
	cfg.Retrospective = riskCfg
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"test-key"}
	cfg.RetryCount = 0
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)
	moderation := NewContentModerationService(&contentModerationTestSettingRepo{values: map[string]string{SettingKeyContentModerationConfig: string(rawCfg)}}, nil, nil, nil, nil, nil, nil)
	moderation.httpClient = server.Client()
	payload, err := json.Marshal(retrospectiveEvidencePayload{SchemaVersion: 2, CaseID: "case-budget", OriginalContent: "synthetic evidence", CapturedAt: time.Now()})
	require.NoError(t, err)
	repo := &retrospectiveRiskRunTestRepo{items: []domain.RetrospectiveEvidence{{ID: "case-budget", ObjectKey: "risk-evidence/pending/budget", KeyVersion: "v1"}}}
	svc := &RetrospectiveRiskService{repo: repo, store: &retrospectiveRiskTestEvidenceStore{payload: payload}, moderation: moderation}
	svc.runReview("manual:test-budget", time.Now(), true)
	require.Equal(t, 1, repo.completed)
	require.NotEmpty(t, repo.updates)
	require.Equal(t, "budget_limited", repo.updates[len(repo.updates)-1].Status)
}

func TestRetrospectiveRepeatedFindingUsesExactAPIKeyEnforcementChain(t *testing.T) {
	userID, keyID := int64(501), int64(601)
	logRepo := &contentModerationTestRepo{}
	userRepo := &contentModerationTestUserRepo{user: &User{ID: userID, Role: RoleUser, Status: StatusActive}}
	keyRepo := &contentModerationTestAPIKeyRepo{key: &APIKey{ID: keyID, UserID: userID, Key: "sk-current", Status: StatusAPIKeyActive}}
	moderation := NewContentModerationService(nil, logRepo, nil, nil, userRepo, &contentModerationTestAuthCacheInvalidator{}, nil)
	moderation.apiKeyRepo = keyRepo
	svc := &RetrospectiveRiskService{moderation: moderation}
	cfg := defaultContentModerationConfig()
	cfg.AutoDisableAPIKeyEnabled = true
	cfg.AutoBanEnabled = false
	svc.applyRetrospectiveFindingEnforcement(context.Background(), cfg, domain.RetrospectiveEvidence{
		ID: "case-enforce", UserID: userID, APIKeyID: &keyID, RequestID: "request-enforce",
	}, domain.RetrospectiveReviewResult{Flagged: true, HighestCategory: "illicit", HighestScore: 0.9, RedactedSummary: "category=illicit score=0.9000"})
	require.Equal(t, StatusAPIKeyDisabled, keyRepo.key.Status)
	logs := logRepo.snapshotLogs()
	require.Len(t, logs, 1)
	require.Equal(t, ContentModerationActionRetrospective, logs[0].Action)
	require.Equal(t, &keyID, logs[0].APIKeyID)
}

func TestRetrospectiveEnforcementKeepsAdministratorExempt(t *testing.T) {
	userID, keyID := int64(502), int64(602)
	keyRepo := &contentModerationTestAPIKeyRepo{key: &APIKey{ID: keyID, UserID: userID, Key: "sk-admin", Status: StatusAPIKeyActive}}
	moderation := NewContentModerationService(nil, &contentModerationTestRepo{}, nil, nil, &contentModerationTestUserRepo{user: &User{ID: userID, Role: RoleAdmin, Status: StatusActive}}, nil, nil)
	moderation.apiKeyRepo = keyRepo
	cfg := defaultContentModerationConfig()
	cfg.AutoDisableAPIKeyEnabled = true
	(&RetrospectiveRiskService{moderation: moderation}).applyRetrospectiveFindingEnforcement(context.Background(), cfg,
		domain.RetrospectiveEvidence{ID: "case-admin", UserID: userID, APIKeyID: &keyID},
		domain.RetrospectiveReviewResult{Flagged: true, HighestCategory: "illicit", HighestScore: 0.99})
	require.Equal(t, StatusAPIKeyActive, keyRepo.key.Status)
}

func TestRetrospectiveCleanupDeletesExpiredObjectBeforeIndex(t *testing.T) {
	repo := &retrospectiveRiskRunTestRepo{expired: []domain.RetrospectiveEvidence{{ID: "expired-1", ObjectKey: "risk-evidence/pending/expired-1"}}}
	store := &retrospectiveRiskTestEvidenceStore{}
	(&RetrospectiveRiskService{repo: repo, store: store}).cleanupExpiredEvidence()
	require.Equal(t, []string{"risk-evidence/pending/expired-1"}, store.deleted)
	require.Equal(t, []string{"expired-1"}, repo.markedDeleted)
}
