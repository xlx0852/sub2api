package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/google/uuid"
)

const (
	retrospectiveRiskPolicyVersion       = "retrospective-v1"
	retrospectiveRiskMaxQueueSize        = 8192
	retrospectiveRiskMaxItemBytes        = 64 << 10
	retrospectiveRiskMaxDailyItems       = 100
	retrospectiveRiskMaxDailyBytes int64 = 8 << 20
)

type RetrospectiveRiskConfig struct {
	CaptureEnabled       bool    `json:"capture_enabled"`
	ReviewEnabled        bool    `json:"review_enabled"`
	ElevationEnabled     bool    `json:"elevation_enabled"`
	BaselineSampleRate   int     `json:"baseline_sample_rate"`
	QueueSize            int     `json:"queue_size"`
	PerUserDailyItems    int     `json:"per_user_daily_items"`
	PerUserDailyBytes    int64   `json:"per_user_daily_bytes"`
	MaxItemBytes         int     `json:"max_item_bytes"`
	PendingRetentionDays int     `json:"pending_retention_days"`
	FlaggedRetentionDays int     `json:"flagged_retention_days"`
	ReviewHour           int     `json:"review_hour"`
	MaxReviewItems       int     `json:"max_review_items"`
	MaxReviewInputTokens int64   `json:"max_review_input_tokens"`
	MaxReviewCostUnits   float64 `json:"max_review_cost_units"`
	RiskTTLHours         int     `json:"risk_ttl_hours"`
	EnforcementEnabled   bool    `json:"enforcement_enabled"`
}

func defaultRetrospectiveRiskConfig() RetrospectiveRiskConfig {
	return RetrospectiveRiskConfig{
		CaptureEnabled:       false,
		ReviewEnabled:        false,
		ElevationEnabled:     false,
		BaselineSampleRate:   2,
		QueueSize:            1024,
		PerUserDailyItems:    20,
		PerUserDailyBytes:    1 << 20,
		MaxItemBytes:         retrospectiveRiskMaxItemBytes,
		PendingRetentionDays: 3,
		FlaggedRetentionDays: 30,
		ReviewHour:           2,
		MaxReviewItems:       1000,
		MaxReviewInputTokens: 2_000_000,
		MaxReviewCostUnits:   0,
		RiskTTLHours:         7 * 24,
	}
}

func (cfg *RetrospectiveRiskConfig) normalize() {
	defaults := defaultRetrospectiveRiskConfig()
	if cfg.BaselineSampleRate < 0 {
		cfg.BaselineSampleRate = 0
	}
	if cfg.BaselineSampleRate > 100 {
		cfg.BaselineSampleRate = 100
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaults.QueueSize
	}
	if cfg.QueueSize > retrospectiveRiskMaxQueueSize {
		cfg.QueueSize = retrospectiveRiskMaxQueueSize
	}
	if cfg.PerUserDailyItems <= 0 {
		cfg.PerUserDailyItems = defaults.PerUserDailyItems
	}
	if cfg.PerUserDailyItems > retrospectiveRiskMaxDailyItems {
		cfg.PerUserDailyItems = retrospectiveRiskMaxDailyItems
	}
	if cfg.PerUserDailyBytes <= 0 {
		cfg.PerUserDailyBytes = defaults.PerUserDailyBytes
	}
	if cfg.PerUserDailyBytes > retrospectiveRiskMaxDailyBytes {
		cfg.PerUserDailyBytes = retrospectiveRiskMaxDailyBytes
	}
	if cfg.MaxItemBytes <= 0 {
		cfg.MaxItemBytes = defaults.MaxItemBytes
	}
	if cfg.MaxItemBytes > retrospectiveRiskMaxItemBytes {
		cfg.MaxItemBytes = retrospectiveRiskMaxItemBytes
	}
	if cfg.PendingRetentionDays <= 0 || cfg.PendingRetentionDays > 3 {
		cfg.PendingRetentionDays = defaults.PendingRetentionDays
	}
	if cfg.FlaggedRetentionDays <= 0 || cfg.FlaggedRetentionDays > 30 {
		cfg.FlaggedRetentionDays = defaults.FlaggedRetentionDays
	}
	if cfg.ReviewHour < 0 || cfg.ReviewHour > 23 {
		cfg.ReviewHour = defaults.ReviewHour
	}
	if cfg.MaxReviewItems <= 0 {
		cfg.MaxReviewItems = defaults.MaxReviewItems
	}
	if cfg.MaxReviewItems > 10000 {
		cfg.MaxReviewItems = 10000
	}
	if cfg.MaxReviewInputTokens <= 0 {
		cfg.MaxReviewInputTokens = defaults.MaxReviewInputTokens
	}
	if cfg.MaxReviewInputTokens > 20_000_000 {
		cfg.MaxReviewInputTokens = 20_000_000
	}
	if cfg.MaxReviewCostUnits < 0 {
		cfg.MaxReviewCostUnits = 0
	}
	if cfg.RiskTTLHours <= 0 {
		cfg.RiskTTLHours = defaults.RiskTTLHours
	}
	if cfg.RiskTTLHours > 30*24 {
		cfg.RiskTTLHours = 30 * 24
	}
	if !cfg.ReviewEnabled || !cfg.ElevationEnabled {
		cfg.EnforcementEnabled = false
	}
}

type retrospectiveCaptureTask struct {
	Input           ContentModerationCheckInput
	Config          RetrospectiveRiskConfig
	OriginalContent string
	ContentHash     string
	Truncated       bool
	Reason          string
	CapturedAt      time.Time
}

type retrospectiveEvidencePayload struct {
	SchemaVersion   int       `json:"schema_version"`
	CaseID          string    `json:"case_id"`
	OriginalContent string    `json:"original_content,omitempty"`
	LegacyContent   string    `json:"content,omitempty"`
	Protocol        string    `json:"protocol"`
	Model           string    `json:"model"`
	CapturedAt      time.Time `json:"captured_at"`
}

func (p retrospectiveEvidencePayload) originalContent() string {
	if p.OriginalContent != "" {
		return p.OriginalContent
	}
	return p.LegacyContent
}

type RetrospectiveReviewTrigger struct {
	Accepted bool   `json:"accepted"`
	RunKey   string `json:"run_key"`
}

type RetrospectiveRiskService struct {
	repo       ports.RetrospectiveRiskRepository
	store      ports.ContentModerationEvidenceStore
	moderation *ContentModerationService
	queue      chan retrospectiveCaptureTask
	stop       chan struct{}
	stopOnce   sync.Once
	running    atomic.Bool
	enqueued   atomic.Int64
	dropped    atomic.Int64
	stored     atomic.Int64
	errors     atomic.Int64
	bytes      atomic.Int64
	probeMu    sync.Mutex
	probeAt    time.Time
	probeErr   error
}

func NewRetrospectiveRiskService(
	repo ports.RetrospectiveRiskRepository,
	store ports.ContentModerationEvidenceStore,
	moderation *ContentModerationService,
) *RetrospectiveRiskService {
	svc := &RetrospectiveRiskService{
		repo: repo, store: store, moderation: moderation,
		queue: make(chan retrospectiveCaptureTask, retrospectiveRiskMaxQueueSize),
		stop:  make(chan struct{}),
	}
	if moderation != nil {
		moderation.SetRetrospectiveRiskService(svc)
	}
	if repo != nil && store != nil {
		go svc.captureWorker()
		go svc.scheduler()
	}
	return svc
}

func (s *RetrospectiveRiskService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stop) })
}

func (s *RetrospectiveRiskService) TryCapture(ctx context.Context, input ContentModerationCheckInput, cfg RetrospectiveRiskConfig) {
	if s == nil || s.repo == nil || s.store == nil || !s.store.Configured() || !cfg.CaptureEnabled || input.UserID <= 0 {
		return
	}
	cfg.normalize()
	extracted := ExtractContentModerationInput(input.Protocol, input.Body)
	if extracted.IsEmpty() {
		return
	}
	extracted.Normalize()
	originalContent := strings.TrimSpace(extracted.Text)
	if originalContent == "" {
		return
	}
	contentHashBytes := sha256.Sum256([]byte(originalContent))
	contentHash := hex.EncodeToString(contentHashBytes[:])
	reason := "baseline"
	sampleRate := cfg.BaselineSampleRate
	if s.isElevatedCached(ctx, input.UserID) {
		sampleRate = 100
		reason = "risk_profile"
	}
	if !retrospectiveDailySample(input.UserID, time.Now(), contentHash, sampleRate) {
		return
	}
	originalContent, truncated := truncateUTF8Bytes(originalContent, cfg.MaxItemBytes)
	task := retrospectiveCaptureTask{
		Input: input, Config: cfg, OriginalContent: originalContent, ContentHash: contentHash,
		Truncated: truncated, Reason: reason, CapturedAt: time.Now(),
	}
	if enqueueRetrospectiveCapture(s.queue, cfg.QueueSize, task) {
		s.enqueued.Add(1)
		return
	}
	s.dropped.Add(1)
}

func enqueueRetrospectiveCapture(queue chan retrospectiveCaptureTask, limit int, task retrospectiveCaptureTask) bool {
	if queue == nil || limit <= 0 || len(queue) >= limit {
		return false
	}
	select {
	case queue <- task:
		return true
	default:
		return false
	}
}

func (s *RetrospectiveRiskService) isElevatedCached(ctx context.Context, userID int64) bool {
	if s == nil || s.moderation == nil || s.moderation.hashCache == nil || userID <= 0 {
		return false
	}
	known, elevated, err := s.moderation.hashCache.GetElevatedUserSampling(ctx, userID)
	return err == nil && known && elevated
}

func retrospectiveDailySample(userID int64, now time.Time, contentHash string, rate int) bool {
	if rate >= 100 {
		return true
	}
	if rate <= 0 {
		return false
	}
	seed := fmt.Sprintf("%d|%s|%s", userID, now.Format("2006-01-02"), contentHash)
	h := sha256.Sum256([]byte(seed))
	return int(binary.BigEndian.Uint16(h[:2])%100) < rate
}

func truncateUTF8Bytes(value string, max int) (string, bool) {
	if max <= 0 || len(value) <= max {
		return value, false
	}
	raw := []byte(value[:max])
	for len(raw) > 0 && !utf8.Valid(raw) {
		raw = raw[:len(raw)-1]
	}
	return string(raw), true
}

func (s *RetrospectiveRiskService) captureWorker() {
	for {
		select {
		case <-s.stop:
			return
		case task := <-s.queue:
			s.persistCapture(task)
		}
	}
}

func (s *RetrospectiveRiskService) persistCapture(task retrospectiveCaptureTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	id := uuid.NewString()
	requestID := strings.TrimSpace(task.Input.RequestID)
	if requestID == "" {
		requestID = id
	}
	day := time.Date(task.CapturedAt.Year(), task.CapturedAt.Month(), task.CapturedAt.Day(), 0, 0, 0, 0, task.CapturedAt.Location())
	shard := strings.ReplaceAll(id[:2], "-", "0")
	objectKey := fmt.Sprintf("risk-evidence/pending/%04d/%02d/%02d/%s/%s.json.zst.enc", day.Year(), day.Month(), day.Day(), shard, id)
	apiKeyID := task.Input.APIKeyID
	item := &domain.RetrospectiveEvidence{
		ID: id, UserID: task.Input.UserID, RequestID: requestID, Endpoint: task.Input.Endpoint,
		Provider: task.Input.Provider, Model: task.Input.Model, ContentHash: task.ContentHash,
		CaptureDay: day, CaptureReason: task.Reason, ObjectKey: objectKey,
		KeyVersion: s.store.CurrentKeyVersion(), Status: "reserved", PlaintextBytes: int64(len(task.OriginalContent)),
		Truncated: task.Truncated, RedactedExcerpt: "",
		CapturedAt: task.CapturedAt, ExpiresAt: task.CapturedAt.AddDate(0, 0, task.Config.PendingRetentionDays),
	}
	if apiKeyID > 0 {
		item.APIKeyID = &apiKeyID
	}
	reserved, err := s.repo.ReserveEvidence(ctx, item, task.Config.PerUserDailyItems, task.Config.PerUserDailyBytes)
	if err != nil {
		s.errors.Add(1)
		slog.Warn("retrospective_risk.reserve_failed", "user_id", task.Input.UserID, "error", err)
		return
	}
	if !reserved {
		return
	}
	payload, err := json.Marshal(retrospectiveEvidencePayload{
		SchemaVersion: 2, CaseID: id, OriginalContent: task.OriginalContent,
		Protocol: task.Input.Protocol, Model: task.Input.Model, CapturedAt: task.CapturedAt,
	})
	if err != nil {
		_ = s.repo.MarkEvidenceFailed(ctx, id, "encode evidence payload")
		s.errors.Add(1)
		return
	}
	storedBytes, err := s.store.Put(ctx, objectKey, item.KeyVersion, payload)
	if err != nil {
		_ = s.repo.MarkEvidenceFailed(ctx, id, "object storage write failed")
		s.errors.Add(1)
		slog.Warn("retrospective_risk.store_failed", "evidence_id", id, "error", err)
		return
	}
	if err := s.repo.MarkEvidenceStored(ctx, id, storedBytes); err != nil {
		_ = s.store.Delete(ctx, objectKey)
		s.errors.Add(1)
		slog.Warn("retrospective_risk.index_commit_failed", "evidence_id", id, "error", err)
		return
	}
	s.stored.Add(1)
	s.bytes.Add(storedBytes)
}

func (s *RetrospectiveRiskService) scheduler() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			if s.moderation == nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			cfg, err := s.moderation.loadConfig(ctx)
			cancel()
			if err != nil {
				continue
			}
			cfg.Retrospective.normalize()
			if cfg.Retrospective.ReviewEnabled && now.Hour() == cfg.Retrospective.ReviewHour {
				runKey := "scheduled:" + now.Format("2006-01-02") + ":" + retrospectiveRiskPolicyVersion
				go s.runReview(runKey, now, false)
			}
			go s.expireProfiles(cfg)
			if now.Minute()%15 == 0 {
				go s.cleanupExpiredEvidence()
				if now.Hour() == 3 {
					go s.reconcileOrphanEvidence()
				}
			}
		}
	}
}

func (s *RetrospectiveRiskService) reconcileOrphanEvidence() {
	if s == nil || s.repo == nil || s.store == nil || !s.store.Configured() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cursor := ""
	for pages := 0; pages < 100; pages++ {
		page, err := s.store.List(ctx, cursor, 200)
		if err != nil {
			s.errors.Add(1)
			return
		}
		for _, key := range page.Keys {
			if strings.Contains(key, "/probe/") {
				continue
			}
			exists, err := s.repo.EvidenceObjectExists(ctx, key)
			if err != nil {
				return
			}
			if !exists {
				if err := s.store.Delete(ctx, key); err != nil {
					s.errors.Add(1)
				}
			}
		}
		if page.NextCursor == "" {
			return
		}
		cursor = page.NextCursor
	}
}

func (s *RetrospectiveRiskService) cleanupExpiredEvidence() {
	if s == nil || s.repo == nil || s.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	items, err := s.repo.ListExpiredEvidence(ctx, time.Now(), 200)
	if err != nil {
		return
	}
	for _, item := range items {
		if err := s.store.Delete(ctx, item.ObjectKey); err != nil {
			continue
		}
		_ = s.repo.MarkEvidenceDeleted(ctx, item.ID)
	}
}

func (s *RetrospectiveRiskService) TriggerReview() RetrospectiveReviewTrigger {
	if s == nil || s.repo == nil || s.store == nil || !s.store.Configured() || s.moderation == nil || s.running.Load() {
		return RetrospectiveReviewTrigger{Accepted: false}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg, err := s.moderation.loadConfig(ctx)
	if err != nil || !cfg.Retrospective.ReviewEnabled {
		return RetrospectiveReviewTrigger{Accepted: false}
	}
	runKey := "manual:" + uuid.NewString()
	go s.runReview(runKey, time.Now(), true)
	return RetrospectiveReviewTrigger{Accepted: true, RunKey: runKey}
}

func (s *RetrospectiveRiskService) runReview(runKey string, scheduledAt time.Time, manual bool) {
	if s == nil || !s.running.CompareAndSwap(false, true) {
		return
	}
	defer s.running.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	release, acquired, err := s.repo.TryReviewLock(ctx)
	if err != nil || !acquired {
		return
	}
	defer release()
	cfg, err := s.moderation.loadConfig(ctx)
	if err != nil {
		return
	}
	riskCfg := cfg.Retrospective
	riskCfg.normalize()
	if !manual && !riskCfg.ReviewEnabled {
		return
	}
	snapshot, _ := json.Marshal(riskCfg)
	run, created, err := s.repo.CreateReviewRun(ctx, runKey, scheduledAt, manual, retrospectiveRiskPolicyVersion, snapshot)
	if err != nil || !created {
		return
	}
	remaining := riskCfg.MaxReviewItems
	for remaining > 0 {
		if run.InputTokens >= riskCfg.MaxReviewInputTokens || (riskCfg.MaxReviewCostUnits > 0 && run.CostUnits >= riskCfg.MaxReviewCostUnits) {
			run.Status = "budget_limited"
			break
		}
		batch := 100
		if remaining < batch {
			batch = remaining
		}
		items, err := s.repo.ClaimEvidence(ctx, run.ID, retrospectiveRiskPolicyVersion, batch)
		if err != nil {
			run.Status = "failed"
			run.Error = "claim evidence failed"
			break
		}
		if len(items) == 0 {
			run.Status = "completed"
			break
		}
		run.ClaimedCount += int64(len(items))
		remaining -= len(items)
		consecutiveFailures := 0
		for _, item := range items {
			itemCtx, itemCancel := context.WithTimeout(ctx, 30*time.Second)
			result, raw, err := s.reviewEvidence(itemCtx, cfg, item)
			itemCancel()
			if err != nil {
				run.FailedCount++
				consecutiveFailures++
				_ = s.repo.FailReviewItem(ctx, run.ID, item.ID, retrospectiveRiskPolicyVersion, classifyRetrospectiveReviewError(err))
				_ = s.repo.UpdateReviewRun(ctx, run)
				if consecutiveFailures >= 5 {
					run.Status = "failed"
					run.Error = "review dependency circuit open after consecutive failures"
					break
				}
				continue
			}
			consecutiveFailures = 0
			run.InputTokens += result.InputTokens
			run.CostUnits += result.CostUnits
			itemID, err := s.repo.CompleteReviewItem(ctx, run.ID, retrospectiveRiskPolicyVersion, item, result)
			if err != nil {
				run.FailedCount++
				continue
			}
			if result.Flagged {
				profile, profileErr := s.repo.UpsertRiskProfileFromFinding(ctx, item, itemID, result, time.Duration(riskCfg.RiskTTLHours)*time.Hour, riskCfg.ElevationEnabled)
				if profileErr != nil {
					run.FailedCount++
					_ = s.repo.FailReviewItem(ctx, run.ID, item.ID, retrospectiveRiskPolicyVersion, "risk profile update failed")
					continue
				}
				flaggedKey := strings.Replace(item.ObjectKey, "risk-evidence/pending/", "risk-evidence/flagged/", 1)
				if storedBytes, moveErr := s.store.Put(ctx, flaggedKey, item.KeyVersion, raw); moveErr == nil {
					if metadataErr := s.repo.MoveEvidenceObject(ctx, item.ID, flaggedKey, storedBytes, time.Now().AddDate(0, 0, riskCfg.FlaggedRetentionDays)); metadataErr == nil {
						_ = s.store.Delete(ctx, item.ObjectKey)
					} else {
						_ = s.store.Delete(ctx, flaggedKey)
					}
				}
				if profile != nil {
					s.moderation.markElevatedUserSampling(ctx, cfg, item.UserID, true)
					if riskCfg.EnforcementEnabled && (profile.Level == domain.RiskLevelHigh || profile.Level == domain.RiskLevelCritical) {
						s.applyRetrospectiveFindingEnforcement(ctx, cfg, item, result)
					}
				}
				run.FlaggedCount++
			}
			run.ReviewedCount++
			_ = s.repo.UpdateReviewRun(ctx, run)
		}
		if run.Status == "failed" {
			break
		}
		_ = s.repo.UpdateReviewRun(ctx, run)
	}
	if run.Status == "running" {
		if remaining <= 0 {
			run.Status = "budget_limited"
		} else {
			run.Status = "completed"
		}
	}
	_ = s.repo.UpdateReviewRun(ctx, run)
}

func (s *RetrospectiveRiskService) applyRetrospectiveFindingEnforcement(ctx context.Context, cfg *ContentModerationConfig, evidence domain.RetrospectiveEvidence, result domain.RetrospectiveReviewResult) {
	if s == nil || s.moderation == nil || cfg == nil || !result.Flagged {
		return
	}
	userID := evidence.UserID
	log := &ContentModerationLog{
		RequestID: evidence.RequestID, UserID: &userID, UserEmail: evidence.UserEmail,
		APIKeyID: evidence.APIKeyID, Endpoint: evidence.Endpoint, Provider: evidence.Provider, Model: evidence.Model,
		Mode: "retrospective", Action: ContentModerationActionRetrospective, Flagged: true,
		HighestCategory: result.HighestCategory, HighestScore: result.HighestScore,
		CategoryScores:    map[string]float64{result.HighestCategory: result.HighestScore},
		ThresholdSnapshot: cloneFloatMap(cfg.Thresholds), InputExcerpt: result.RedactedSummary,
	}
	// This reuses the existing user/API-key enforcement chain. It never receives
	// or mutates an upstream account identifier, and audit persistence happens
	// before any user-side effect.
	s.moderation.persistContentModerationLog(ctx, cfg, log, "", false, true)
}

func classifyRetrospectiveReviewError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "retryable: review timeout"
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"timeout", "temporar", "connection", "rate limit", "429", "502", "503", "504"} {
		if strings.Contains(message, marker) {
			return "retryable: review dependency unavailable"
		}
	}
	for _, marker := range []string{"invalid evidence payload", "empty retrospective", "decrypt", "exceeds size"} {
		if strings.Contains(message, marker) {
			return "terminal: invalid evidence"
		}
	}
	return "retryable: review dependency failed"
}

func (s *RetrospectiveRiskService) reviewEvidence(ctx context.Context, cfg *ContentModerationConfig, item domain.RetrospectiveEvidence) (domain.RetrospectiveReviewResult, []byte, error) {
	raw, err := s.store.Get(ctx, item.ObjectKey, item.KeyVersion)
	if err != nil {
		return domain.RetrospectiveReviewResult{}, nil, err
	}
	var payload retrospectiveEvidencePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return domain.RetrospectiveReviewResult{}, raw, err
	}
	originalContent := strings.TrimSpace(payload.originalContent())
	if payload.CaseID != item.ID || originalContent == "" {
		return domain.RetrospectiveReviewResult{}, raw, errors.New("invalid evidence payload")
	}
	reviewContent := sanitizeRetrospectiveReviewContent(originalContent)
	if reviewContent == "" {
		return domain.RetrospectiveReviewResult{}, raw, errors.New("empty retrospective review content")
	}
	apiResult, err := s.moderation.callModeration(ctx, cfg, reviewContent)
	if err != nil {
		return domain.RetrospectiveReviewResult{}, raw, err
	}
	flagged, category, score := evaluateModerationScores(apiResult.CategoryScores, cfg.Thresholds)
	reasons := []string{}
	if category != "" {
		reasons = append(reasons, category)
	}
	redactedSummary := ""
	if category != "" {
		redactedSummary = fmt.Sprintf("category=%s score=%.4f", category, score)
	}
	result := domain.RetrospectiveReviewResult{
		Flagged: flagged, HighestCategory: category, HighestScore: score,
		ReasonCodes: reasons, RedactedSummary: redactedSummary,
		InputTokens: int64(len([]rune(reviewContent))/4 + 1), CostUnits: 0,
	}
	return result, raw, nil
}

func (s *RetrospectiveRiskService) expireProfiles(cfg *ContentModerationConfig) {
	if s == nil || s.repo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ids, err := s.repo.ExpireRiskProfiles(ctx, time.Now(), 100)
	if err != nil {
		return
	}
	for _, id := range ids {
		s.moderation.markElevatedUserSampling(ctx, cfg, id, false)
	}
}

func (s *RetrospectiveRiskService) IsElevated(ctx context.Context, userID int64) bool {
	if s == nil || s.repo == nil || userID <= 0 {
		return false
	}
	elevated, err := s.repo.IsRiskElevated(ctx, userID, time.Now())
	return err == nil && elevated
}

func (s *RetrospectiveRiskService) Status(ctx context.Context, cfg RetrospectiveRiskConfig) (*domain.RetrospectiveRiskStatus, error) {
	cfg.normalize()
	status := &domain.RetrospectiveRiskStatus{
		Configured: s.store != nil && s.store.Configured(), CaptureEnabled: cfg.CaptureEnabled,
		ReviewEnabled: cfg.ReviewEnabled, QueueSize: cfg.QueueSize, QueueLength: len(s.queue),
		Enqueued: s.enqueued.Load(), Dropped: s.dropped.Load(), Stored: s.stored.Load(),
		StorageErrors: s.errors.Load(), StoredBytes: s.bytes.Load(),
	}
	if s.repo != nil {
		status.LatestReviewRun, _ = s.repo.LatestReviewRun(ctx)
		status.ActiveRiskProfiles, _ = s.repo.CountActiveRiskProfiles(ctx)
		if items, storedBytes, err := s.repo.EvidenceStats(ctx); err == nil {
			status.EvidenceItems = items
			status.StoredBytes = storedBytes
		}
	}
	if status.Configured {
		s.probeMu.Lock()
		if time.Since(s.probeAt) > 5*time.Minute {
			probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			s.probeErr = s.store.Probe(probeCtx)
			cancel()
			s.probeAt = time.Now()
		}
		probeErr := s.probeErr
		s.probeMu.Unlock()
		status.ConnectivityOK = probeErr == nil
		if probeErr != nil {
			status.ConnectivityError = "OSS evidence probe failed"
		}
	}
	return status, nil
}

func (s *RetrospectiveRiskService) ListRuns(ctx context.Context, page, pageSize int) ([]domain.RetrospectiveReviewRun, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.repo.ListReviewRuns(ctx, pageSize, (page-1)*pageSize)
}

func (s *RetrospectiveRiskService) ListProfiles(ctx context.Context, level string, page, pageSize int) ([]domain.UserRiskProfile, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.repo.ListRiskProfiles(ctx, level, pageSize, (page-1)*pageSize)
}

func (s *RetrospectiveRiskService) ListEvents(ctx context.Context, userID int64, page, pageSize int) ([]domain.RiskEvent, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.repo.ListRiskEvents(ctx, userID, pageSize, (page-1)*pageSize)
}

func (s *RetrospectiveRiskService) ListEvidenceCases(ctx context.Context, userID int64, status string, page, pageSize int) ([]domain.RetrospectiveEvidenceCase, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.repo.ListEvidenceCases(ctx, userID, status, pageSize, (page-1)*pageSize)
}

func (s *RetrospectiveRiskService) AuthorizeEvidenceAccess(ctx context.Context, evidenceID string, adminID int64, password, reason string) (*domain.EvidenceAccessAuthorization, error) {
	reason = strings.TrimSpace(reason)
	if s == nil || s.repo == nil || s.moderation == nil || s.moderation.userRepo == nil || adminID <= 0 || strings.TrimSpace(evidenceID) == "" {
		return nil, errors.New("raw evidence access is unavailable")
	}
	if len([]rune(reason)) < 8 {
		_ = s.repo.AuditOperatorAction(ctx, adminID, "evidence_authorize", evidenceID, 0, reason, "denied")
		return nil, errors.New("an access reason of at least 8 characters is required")
	}
	admin, err := s.moderation.userRepo.GetByID(ctx, adminID)
	if err != nil || admin == nil || !admin.IsAdmin() || !admin.CheckPassword(password) {
		_ = s.repo.AuditOperatorAction(ctx, adminID, "evidence_authorize", evidenceID, 0, reason, "denied")
		return nil, errors.New("secondary authorization failed")
	}
	evidence, err := s.repo.GetEvidence(ctx, evidenceID)
	if err != nil || evidence == nil || evidence.Status == "deleted" || evidence.ExpiresAt.Before(time.Now()) {
		_ = s.repo.AuditOperatorAction(ctx, adminID, "evidence_authorize", evidenceID, 0, reason, "denied")
		return nil, errors.New("evidence is unavailable")
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return nil, errors.New("create evidence access grant")
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	digest := sha256.Sum256([]byte(token))
	expiresAt := time.Now().Add(5 * time.Minute)
	if err := s.repo.CreateEvidenceAccessGrant(ctx, evidenceID, adminID, hex.EncodeToString(digest[:]), reason, expiresAt); err != nil {
		return nil, err
	}
	return &domain.EvidenceAccessAuthorization{Token: token, ExpiresAt: expiresAt}, nil
}

func (s *RetrospectiveRiskService) ReadRawEvidence(ctx context.Context, evidenceID string, adminID int64, token string) (*domain.RawEvidenceView, error) {
	if s == nil || s.repo == nil || s.store == nil || adminID <= 0 || strings.TrimSpace(token) == "" {
		return nil, errors.New("raw evidence access is unavailable")
	}
	digest := sha256.Sum256([]byte(token))
	grant, err := s.repo.ConsumeEvidenceAccessGrant(ctx, evidenceID, adminID, hex.EncodeToString(digest[:]), time.Now())
	if err != nil {
		_ = s.repo.AuditOperatorAction(ctx, adminID, "evidence_read", evidenceID, 0, "invalid or expired grant", "denied")
		return nil, errors.New("evidence access grant is invalid or expired")
	}
	evidence, err := s.repo.GetEvidence(ctx, evidenceID)
	if err != nil {
		_ = s.repo.AuditOperatorAction(ctx, adminID, "evidence_read", evidenceID, 0, grant.Reason, "failed")
		return nil, err
	}
	raw, err := s.store.Get(ctx, evidence.ObjectKey, evidence.KeyVersion)
	if err != nil {
		_ = s.repo.AuditOperatorAction(ctx, adminID, "evidence_read", evidenceID, evidence.UserID, grant.Reason, "failed")
		return nil, errors.New("read encrypted evidence failed")
	}
	var payload retrospectiveEvidencePayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.CaseID != evidenceID {
		_ = s.repo.AuditOperatorAction(ctx, adminID, "evidence_read", evidenceID, evidence.UserID, grant.Reason, "failed")
		return nil, errors.New("invalid evidence payload")
	}
	if err := s.repo.AuditOperatorAction(ctx, adminID, "evidence_read", evidenceID, evidence.UserID, grant.Reason, "success"); err != nil {
		return nil, errors.New("evidence access audit failed")
	}
	return &domain.RawEvidenceView{EvidenceID: evidenceID, Content: payload.originalContent(), Protocol: payload.Protocol, Model: payload.Model, CapturedAt: payload.CapturedAt}, nil
}

func (s *RetrospectiveRiskService) ReviewProfile(ctx context.Context, userID int64, level, reviewStatus, note string, adminID *int64) (*domain.UserRiskProfile, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	valid := map[string]bool{domain.RiskLevelNormal: true, domain.RiskLevelWatch: true, domain.RiskLevelHigh: true, domain.RiskLevelCritical: true}
	if !valid[level] {
		return nil, errors.New("invalid risk level")
	}
	if adminID == nil || *adminID <= 0 {
		return nil, errors.New("missing admin identity")
	}
	if len([]rune(strings.TrimSpace(note))) < 3 {
		return nil, errors.New("review note must contain at least 3 characters")
	}
	if reviewStatus != "unreviewed" && reviewStatus != "confirmed" && reviewStatus != "cleared" {
		return nil, errors.New("invalid review status")
	}
	profile, err := s.repo.SetRiskProfile(ctx, userID, level, reviewStatus, note, adminID, 7*24*time.Hour)
	if err != nil {
		_ = s.repo.AuditOperatorAction(ctx, *adminID, "risk_profile_review", "", userID, note, "failed")
	}
	if err == nil && s.moderation != nil {
		cfg, cfgErr := s.moderation.loadConfig(ctx)
		if cfgErr == nil {
			s.moderation.markElevatedUserSampling(ctx, cfg, userID, level != domain.RiskLevelNormal)
		}
	}
	return profile, err
}
