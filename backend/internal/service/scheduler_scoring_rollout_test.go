package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type writableSchedulerRolloutRepo struct {
	*openAIAdvancedSchedulerSettingRepoStub
}

func (r *writableSchedulerRolloutRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func TestOpenAIGatewayServiceSchedulerScoringRolloutModeReadsDatabaseScope(t *testing.T) {
	groupID := int64(25)
	repo := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeySchedulerScoringRollout: `{"default":"off","platforms":{"openai":"shadow"},"groups":{"openai":{"25":"enforce"}}}`,
	}}
	service := &OpenAIGatewayService{
		settingService: NewSettingService(repo, &config.Config{}),
	}

	require.Equal(t, scheduling.RolloutEnforce, service.schedulerScoringRolloutMode(context.Background(), PlatformOpenAI, &groupID))
	require.Equal(t, scheduling.RolloutShadow, service.schedulerScoringRolloutMode(context.Background(), PlatformOpenAI, nil))
	require.Equal(t, scheduling.RolloutOff, service.schedulerScoringRolloutMode(context.Background(), PlatformGrok, &groupID))
}

func TestOpenAIGatewayServiceSchedulerScoringRolloutModeFailsClosed(t *testing.T) {
	service := &OpenAIGatewayService{}
	require.Equal(t, scheduling.RolloutOff, service.schedulerScoringRolloutMode(context.Background(), PlatformOpenAI, nil))
}

func TestSettingServiceSetSchedulerScoringRolloutPersistsAndRefreshesCacheImmediately(t *testing.T) {
	repo := &writableSchedulerRolloutRepo{openAIAdvancedSchedulerSettingRepoStub: &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeySchedulerScoringRollout: `{"platforms":{"openai":"shadow"}}`,
	}}}
	settingService := NewSettingService(repo, &config.Config{})
	require.Equal(t, scheduling.RolloutShadow, settingService.GetSchedulerScoringRollout(context.Background()).Resolve(PlatformOpenAI, nil))

	updated := scheduling.RolloutConfig{
		Default:   scheduling.RolloutOff,
		Platforms: map[string]scheduling.RolloutMode{PlatformOpenAI: scheduling.RolloutOff},
	}
	require.NoError(t, settingService.SetSchedulerScoringRollout(context.Background(), updated))
	updated.Platforms[PlatformOpenAI] = scheduling.RolloutEnforce
	require.Equal(t, scheduling.RolloutOff, settingService.GetSchedulerScoringRollout(context.Background()).Resolve(PlatformOpenAI, nil))

	var persisted scheduling.RolloutConfig
	require.NoError(t, json.Unmarshal([]byte(repo.values[SettingKeySchedulerScoringRollout]), &persisted))
	require.Equal(t, scheduling.RolloutOff, persisted.Resolve(PlatformOpenAI, nil))
}
