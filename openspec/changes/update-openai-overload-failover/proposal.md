# Change: 改进 OpenAI 流式过载故障切换

## Why

生产中 OpenAI `gpt-5.5` 会在多个账号同步返回 `server_is_overloaded`。当前网关对部分流式终态只能原账号重试或直接暴露 503，导致账号池存在但用户仍看到大量失败。

## What Changes

- 对尚未向客户端输出可见内容的可重试上游过载错误，优先切换到同分组的其他账号。
- 同一请求不重复选择已经失败的账号，并对失败账号施加短暂、带抖动的冷却。
- 保留已产生可见输出后的安全边界：不重放可能造成重复内容或重复扣费的流。
- 为重试次数、账号切换、失败阶段和冷却原因增加结构化日志/审计字段。
- 默认保持现有并发限制和计费语义不变；不自动切换到其他模型，除非已有分组回退策略明确允许。

## Impact

- Affected specs: `openai-gateway`（若现有规范不存在，则新增网关可靠性要求）
- Affected code: `backend/internal/handler/openai_gateway_handler.go`, OpenAI failover/scheduler service code, related tests
- Compatibility: 对客户端 API 形状保持兼容；可能减少用户可见 503，但会增加少量上游重试请求和尾延迟
- Rollout: 先以现有 `test` 分组灰度，观察暴露 503、上游过载、重试次数和重复计费指标后再扩大范围
