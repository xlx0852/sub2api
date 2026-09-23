package service

import "testing"

// 网关转发设置的测试辅助（原位于 setting_service_claude_oauth_system_prompt_test.go，
// 该文件随 system 提示词注入一起删除后，这两个 helper 仍被其它测试使用，故迁移至此）。

// resetGatewayForwardingSettingsCacheForTest 预留的缓存重置入口。
// 缓存挂在 SettingService 实例上，新建实例即为空缓存，因此这里是空实现。
func resetGatewayForwardingSettingsCacheForTest(t *testing.T) {
	t.Helper()
}

// invalidateGatewayForwardingCache 丢弃进程内缓存的网关转发设置，
// 让下一次读取回源到 settingRepo（测试里改完 repo 数据后调用）。
func invalidateGatewayForwardingCache(s *SettingService) {
	if s == nil {
		return
	}
	s.gatewayForwardingSF.Forget("gateway_forwarding")
	s.gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{})
}
