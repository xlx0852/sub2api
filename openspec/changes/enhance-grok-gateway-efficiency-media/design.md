## Context

sub2api 已具备 Grok OAuth/API Key 转发、Codex 双向工具翻译、多账号粘连、额度探测、图片/视频生成和媒体计费。此次改动应复用现有职责边界，避免把请求优化、调度状态和媒体任务状态混合。

## Goals / Non-Goals

- Goals:
  - 降低 Grok 多轮请求体积，同时保留最近上下文和工具链标识。
  - 使用新鲜额度快照改善账号选择，但不让缺失或陈旧数据影响可用性。
  - 以最小增量补齐视频编辑 API。
- Non-Goals:
  - 不实现 xAI Files 自动 offload。
  - 不实现合成工具调用或过早结束自动恢复。
  - 不改变非 Grok 平台行为。
  - 不实现完整 Grok Build 桌面代理平面。

## Decisions

### 请求体优化

- 新增 `gateway.grok_payload` 配置，默认启用精确重复图片去重和图片请求 `store=false`；历史文本截断及软硬预算使用保守默认值并允许用 `0` 关闭。
- 只处理 Grok 上游请求。精确重复 `data:image/...` 保留最新副本；远程 URL、非完全相同图片和当前输入不去重。
- 只有序列化体积超过软预算时才截断历史工具输出；保留最新工具输出、call ID、UTF-8 边界及明确截断标记。
- 超过硬预算且无法安全缩减时返回 `413 invalid_request_error`，不静默删除当前输入或工具链锚点。

### 额度软调度

- 复用现有 scheduler `quota_headroom` 权重，不新增独立选号器。
- sticky 命中和硬耗尽排除保持现有优先级；额度只参与其后的候选评分。
- 优先使用 24 小时内成功获取的 billing 剩余比例；缺失时使用仍在有效窗口内的 API requests/tokens 最保守剩余比例。
- 缺失、解析失败、过期或陈旧数据返回中性权重，不排除账号。
- scheduler metadata 保留必要 Grok quota 字段，并将观测快照更新视为 scheduler-neutral 单账号刷新。

### 视频编辑

- 新增独立 endpoint 类型，但复用 `handleGrokMedia`。
- 视频编辑属于异步视频提交：成功后绑定 request ID 到选中账号，状态查询继续沿用现有路由。
- 请求体默认原样透传；不应用文本生成专属模型降级。
- 计费优先使用请求或完成任务中可验证的时长/分辨率，无法解析时沿用现有视频默认逻辑。

## Risks / Trade-offs

- 历史截断可能损失上下文：仅超过软预算后处理历史项，保留最新项并支持关闭。
- quota 数据可能陈旧：限定 freshness，陈旧数据中性化。
- 视频编辑 schema 可能变化：保持透传，仅做 endpoint 和通用媒体字段解析。
- 当前工作树改动较多：逐文件小补丁并运行聚焦及完整包测试。

## Migration Plan

- 配置有默认值，无数据库迁移。
- 新路由向后兼容。
- 可通过关闭 `gateway.grok_payload` 各阈值和现有 scheduler quota 权重快速回退。
