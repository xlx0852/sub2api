package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

const (
	schedulerScoringRolloutCacheTTL  = 5 * time.Second
	schedulerScoringRolloutDBTimeout = 2 * time.Second
	schedulerScoringRolloutCacheKey  = "scheduler_scoring_rollout"
)

type cachedSchedulerScoringRollout struct {
	config    scheduling.RolloutConfig
	expiresAt int64
}

func (s *SettingService) schedulerScoringRollout(ctx context.Context) scheduling.RolloutConfig {
	if s == nil || s.settingRepo == nil {
		return scheduling.ParseRolloutConfig("")
	}
	if cached, ok := s.schedulerScoringRolloutCache.Load().(*cachedSchedulerScoringRollout); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.config.Clone()
		}
	}

	loaded, _, _ := s.schedulerScoringRolloutSF.Do(schedulerScoringRolloutCacheKey, func() (any, error) {
		if cached, ok := s.schedulerScoringRolloutCache.Load().(*cachedSchedulerScoringRollout); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.config.Clone(), nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), schedulerScoringRolloutDBTimeout)
		defer cancel()
		raw, err := s.settingRepo.GetValue(dbCtx, SettingKeySchedulerScoringRollout)
		if err != nil {
			raw = ""
		}
		config := scheduling.ParseRolloutConfig(raw)
		s.schedulerScoringRolloutCache.Store(&cachedSchedulerScoringRollout{
			config:    config.Clone(),
			expiresAt: time.Now().Add(schedulerScoringRolloutCacheTTL).UnixNano(),
		})
		return config, nil
	})
	config, _ := loaded.(scheduling.RolloutConfig)
	return config.Clone()
}

func (s *SettingService) GetSchedulerScoringRollout(ctx context.Context) scheduling.RolloutConfig {
	return s.schedulerScoringRollout(ctx)
}

func (s *SettingService) SetSchedulerScoringRollout(ctx context.Context, config scheduling.RolloutConfig) error {
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("scheduler scoring rollout setting repository is unavailable")
	}
	if err := config.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal scheduler scoring rollout: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeySchedulerScoringRollout, string(raw)); err != nil {
		return fmt.Errorf("set scheduler scoring rollout: %w", err)
	}
	s.schedulerScoringRolloutSF.Forget(schedulerScoringRolloutCacheKey)
	s.schedulerScoringRolloutCache.Store(&cachedSchedulerScoringRollout{
		config:    config.Clone(),
		expiresAt: time.Now().Add(schedulerScoringRolloutCacheTTL).UnixNano(),
	})
	return nil
}

func normalizeSchedulerScoringRolloutRaw(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = `{"default":"off"}`
	}
	config := scheduling.RolloutConfig{}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return "", fmt.Errorf("invalid scheduler scoring rollout json: %w", err)
	}
	if err := config.Validate(); err != nil {
		return "", err
	}
	normalized, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal scheduler scoring rollout: %w", err)
	}
	return string(normalized), nil
}
