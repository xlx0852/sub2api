package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// 生产 config.yaml 里有 gateway 段但不含 upstream_gzip_* 键，
// 这里验证默认值仍会被合并进来（viper 嵌套默认值的常见坑）。
func TestLoadUpstreamGzipDefaultsWhenGatewaySectionExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("gateway:\n    stream_data_interval_timeout: 300\n"), 0o600))

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")
	setDefaults()
	require.NoError(t, viper.ReadInConfig())

	var cfg Config
	require.NoError(t, viper.Unmarshal(&cfg))

	require.EqualValues(t, 1<<20, cfg.Gateway.UpstreamGzipMinBytes, "默认阈值应为 1MiB")
	require.Equal(t, []string{"o10.top"}, cfg.Gateway.UpstreamGzipHosts, "默认白名单应包含 o10.top")
}

func TestLoadUpstreamGzipExplicitDisable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("gateway:\n    upstream_gzip_min_bytes: 0\n    upstream_gzip_hosts: []\n"), 0o600))

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")
	setDefaults()
	require.NoError(t, viper.ReadInConfig())

	var cfg Config
	require.NoError(t, viper.Unmarshal(&cfg))
	require.EqualValues(t, 0, cfg.Gateway.UpstreamGzipMinBytes, "显式 0 应保持关闭")
	require.Empty(t, cfg.Gateway.UpstreamGzipHosts)
}
