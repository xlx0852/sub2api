//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// settingModelConcurrencyRepoStub 为 GetModelConcurrencyLimits 提供 GetValue 实现的桩。
type settingModelConcurrencyRepoStub struct {
	values map[string]string
}

func (s *settingModelConcurrencyRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingModelConcurrencyRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (s *settingModelConcurrencyRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingModelConcurrencyRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *settingModelConcurrencyRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingModelConcurrencyRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingModelConcurrencyRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSanitizeModelConcurrencyLimits(t *testing.T) {
	got := sanitizeModelConcurrencyLimits(map[string]int{
		"gpt-5.6-luna":                      8,
		"":                                  5,         // 空模型键丢弃
		"gpt-5.6-sol":                       0,         // 非正数丢弃（显式不限制语义由 0 表达）
		"   gpt-5.6-terra   ":               3,         // 首尾空格裁剪
		"very-long-model-" + padString(200): 2,         // 超长模型键丢弃
		"gpt-5.6-luna-2026-08-01":           1_000_001, // 超上限 clamp
	})
	require.Equal(t, map[string]int{
		"gpt-5.6-luna":            8,
		"gpt-5.6-terra":           3,
		"gpt-5.6-luna-2026-08-01": 1_000_000,
	}, got)
}

func padString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

func TestSettingService_ParseSettings_ModelConcurrencyLimits(t *testing.T) {
	svc := NewSettingService(&settingUpdateRepoStub{}, &config.Config{})

	got := svc.parseSettings(map[string]string{
		SettingKeyModelConcurrencyLimits: `{"gpt-5.6-luna":8,"gpt-5.6-sol":0}`,
	})

	require.Equal(t, map[string]int{"gpt-5.6-luna": 8}, got.ModelConcurrencyLimits)

	// 缺失时保持 nil（= 未配置预算）
	gotEmpty := svc.parseSettings(map[string]string{})
	require.Nil(t, gotEmpty.ModelConcurrencyLimits)

	// 非法 JSON → 优雅降级为空
	gotBad := svc.parseSettings(map[string]string{
		SettingKeyModelConcurrencyLimits: `{not-json`,
	})
	require.Nil(t, gotBad.ModelConcurrencyLimits)
}

func TestSettingService_UpdateSettings_ModelConcurrencyLimits(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	updates, err := svc.buildSystemSettingsUpdates(context.Background(), &SystemSettings{
		ModelConcurrencyLimits: map[string]int{"gpt-5.6-luna": 12, "gpt-5.6-sol": 0},
	})
	require.NoError(t, err)

	raw, ok := updates[SettingKeyModelConcurrencyLimits]
	require.True(t, ok)
	parsed := map[string]int{}
	require.NoError(t, json.Unmarshal([]byte(raw), &parsed))
	// 0 条目被清洗丢弃
	require.Equal(t, map[string]int{"gpt-5.6-luna": 12}, parsed)
}

func TestSettingService_GetModelConcurrencyLimits(t *testing.T) {
	repo := &settingModelConcurrencyRepoStub{values: map[string]string{
		SettingKeyModelConcurrencyLimits: `{"gpt-5.6-luna":8}`,
	}}
	svc := NewSettingService(repo, &config.Config{})

	got := svc.GetModelConcurrencyLimits(context.Background())
	require.Equal(t, map[string]int{"gpt-5.6-luna": 8}, got)

	// 缓存命中：换 repo 值也不变（TTL 内）
	repo.values[SettingKeyModelConcurrencyLimits] = `{"gpt-5.6-luna":100}`
	got2 := svc.GetModelConcurrencyLimits(context.Background())
	require.Equal(t, 8, got2["gpt-5.6-luna"])

	// SetModelConcurrencyLimitsCache 主动刷新：立即生效
	svc.SetModelConcurrencyLimitsCache(map[string]int{"gpt-5.6-luna": 100})
	got3 := svc.GetModelConcurrencyLimits(context.Background())
	require.Equal(t, 100, got3["gpt-5.6-luna"])
}

func TestSettingService_GetModelConcurrencyLimits_NotFound(t *testing.T) {
	svc := NewSettingService(&settingModelConcurrencyRepoStub{values: map[string]string{}}, &config.Config{})

	got := svc.GetModelConcurrencyLimits(context.Background())
	require.Empty(t, got) // 未配置 = 空 map（等价于不限制）
}
