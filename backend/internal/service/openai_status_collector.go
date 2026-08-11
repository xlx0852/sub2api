package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	openAIStatusCollectorJobName = "openai_status_collector"
	openAIStatusCollectorLockKey = "ops:provider-status:openai:leader"
)

var openAIStatusCollectorReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type OpenAIStatusCollectorService struct {
	repo        OpsRepository
	redisClient *redis.Client
	cfg         *config.Config
	client      *OpenAIStatusClient
	instanceID  string

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

func NewOpenAIStatusCollectorService(repo OpsRepository, redisClient *redis.Client, cfg *config.Config) *OpenAIStatusCollectorService {
	statusCfg := openAIStatusConfig(cfg)
	return &OpenAIStatusCollectorService{
		repo:        repo,
		redisClient: redisClient,
		cfg:         cfg,
		client:      NewOpenAIStatusClient(time.Duration(statusCfg.RequestTimeoutSeconds)*time.Second, int64(statusCfg.MaxBodyBytes)),
		instanceID:  uuid.NewString(),
	}
}

func ProvideOpenAIStatusCollectorService(repo OpsRepository, redisClient *redis.Client, cfg *config.Config) *OpenAIStatusCollectorService {
	svc := NewOpenAIStatusCollectorService(repo, redisClient, cfg)
	svc.Start()
	return svc
}

func openAIStatusConfig(cfg *config.Config) config.OpsProviderStatusConfig {
	result := config.OpsProviderStatusConfig{
		Enabled: true, PollIntervalSeconds: 60, StaleAfterSeconds: 180,
		RequestTimeoutSeconds: 5, MaxBodyBytes: 512 * 1024,
	}
	if cfg != nil {
		result = cfg.Ops.ProviderStatus
	}
	if result.PollIntervalSeconds < 30 {
		result.PollIntervalSeconds = 30
	}
	if result.PollIntervalSeconds > 3600 {
		result.PollIntervalSeconds = 3600
	}
	if result.StaleAfterSeconds < result.PollIntervalSeconds*2 {
		result.StaleAfterSeconds = result.PollIntervalSeconds * 3
	}
	if result.RequestTimeoutSeconds < 1 || result.RequestTimeoutSeconds > 30 {
		result.RequestTimeoutSeconds = 5
	}
	if result.MaxBodyBytes < 16*1024 || result.MaxBodyBytes > 2*1024*1024 {
		result.MaxBodyBytes = 512 * 1024
	}
	return result
}

func (s *OpenAIStatusCollectorService) Start() {
	if s == nil || s.repo == nil || !openAIStatusConfig(s.cfg).Enabled {
		return
	}
	s.startOnce.Do(func() {
		s.stopCh = make(chan struct{})
		s.wg.Add(1)
		go s.run()
	})
}

func (s *OpenAIStatusCollectorService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
	s.wg.Wait()
}

func (s *OpenAIStatusCollectorService) run() {
	defer s.wg.Done()
	interval := time.Duration(openAIStatusConfig(s.cfg).PollIntervalSeconds) * time.Second
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			s.collectOnce()
			timer.Reset(interval)
		case <-s.stopCh:
			return
		}
	}
}

func (s *OpenAIStatusCollectorService) collectOnce() {
	if s == nil || s.repo == nil || s.client == nil {
		return
	}
	statusCfg := openAIStatusConfig(s.cfg)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(statusCfg.RequestTimeoutSeconds+2)*time.Second)
	defer cancel()
	release, ok := s.acquireLock(ctx, time.Duration(statusCfg.PollIntervalSeconds+20)*time.Second)
	if !ok {
		return
	}
	if release != nil {
		defer release()
	}
	startedAt := time.Now().UTC()
	record, err := s.client.Fetch(ctx, startedAt)
	if err == nil {
		var inserted bool
		inserted, err = s.repo.InsertProviderStatusSnapshot(ctx, record)
		if err == nil {
			s.syncTransitionEvent(ctx, record, inserted)
			result := fmt.Sprintf("indicator=%s changed=%t", record.OverallIndicator, inserted)
			duration := time.Since(startedAt).Milliseconds()
			_ = s.repo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
				JobName: openAIStatusCollectorJobName, LastRunAt: &startedAt, LastSuccessAt: &startedAt,
				LastDurationMs: &duration, LastResult: &result,
			})
			return
		}
	}
	duration := time.Since(startedAt).Milliseconds()
	message := "unknown provider status collection error"
	if err != nil {
		message = err.Error()
	}
	_ = s.repo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName: openAIStatusCollectorJobName, LastRunAt: &startedAt, LastErrorAt: &startedAt,
		LastError: &message, LastDurationMs: &duration,
	})
	logger.LegacyPrintf("service.openai_status_collector", "collection failed: %v", err)
}

func (s *OpenAIStatusCollectorService) acquireLock(ctx context.Context, ttl time.Duration) (func(), bool) {
	if s.redisClient == nil {
		return nil, true
	}
	ok, err := s.redisClient.SetNX(ctx, openAIStatusCollectorLockKey, s.instanceID, ttl).Result()
	if err != nil {
		logger.LegacyPrintf("service.openai_status_collector", "leader lock failed: %v", err)
		return nil, false
	}
	if !ok {
		return nil, false
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = openAIStatusCollectorReleaseScript.Run(releaseCtx, s.redisClient, []string{openAIStatusCollectorLockKey}, s.instanceID).Result()
	}, true
}

func (s *OpenAIStatusCollectorService) syncTransitionEvent(ctx context.Context, current *ProviderStatusSnapshotRecord, inserted bool) {
	if !inserted || current == nil {
		return
	}
	history, err := s.repo.ListProviderStatusSnapshots(ctx, &ProviderStatusHistoryFilter{Provider: current.Provider, Limit: 2})
	if err != nil || len(history) < 2 {
		return
	}
	previous := history[1]
	wasOperational := strings.EqualFold(previous.OverallIndicator, "none")
	isOperational := strings.EqualFold(current.OverallIndicator, "none")
	if wasOperational == isOperational {
		return
	}
	events, _ := s.repo.ListAlertEvents(ctx, &OpsAlertEventFilter{Limit: 100, Status: OpsAlertStatusFiring})
	if isOperational {
		now := time.Now().UTC()
		for _, event := range events {
			if event != nil && event.Dimensions["event_type"] == "provider_status" && event.Dimensions["provider"] == current.Provider {
				_ = s.repo.UpdateAlertEventStatus(ctx, event.ID, OpsAlertStatusResolved, &now)
			}
		}
		return
	}
	for _, event := range events {
		if event != nil && event.Dimensions["event_type"] == "provider_status" && event.Dimensions["provider"] == current.Provider {
			return
		}
	}
	_, _ = s.repo.CreateAlertEvent(ctx, &OpsAlertEvent{
		Severity: "P2", Status: OpsAlertStatusFiring,
		Title:       "OpenAI official status degraded",
		Description: current.OverallDescription,
		Dimensions:  map[string]any{"event_type": "provider_status", "provider": current.Provider, "indicator": current.OverallIndicator},
		FiredAt:     current.FetchedAt,
	})
}
