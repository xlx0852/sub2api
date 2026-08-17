## Context

批量生图框架同时支持 `gemini_api` 和 `vertex`。当前路由不再注册公开批图入口，但 Wire 仍构造 `BatchImageWorkerRuntime`，registry 仍构造两个 Provider。

美国生产库在 2026-08-16 的只读快照为：

| Table | Rows |
|---|---:|
| `batch_image_jobs` | 0 |
| `batch_image_items` | 0 |
| `batch_image_events` | 0 |

该证据只代表当时快照；部署前必须重查，不得把提案内的数字当作永久不变量。

## Goals / Non-Goals

### Goals

- 删除 `gemini_api` 批图上游能力和专用代码。
- 保留 Vertex 与通用批图基础设施。
- 保证不存在历史任务被静默遗弃或 hold 未释放。

### Non-Goals

- 不删除 Vertex 批图、数据表、Redis 队列或 Worker 框架。
- 不退役 Gemini/Antigravity 普通模型请求。
- 不修改结算、冻结、扣费或输出保留语义。
- 不清理通用批图配置键或数据库 schema。

## Decisions

### Decision 1: 按 Provider 退役，不删整个批图子系统

`vertex` 仍有完整 Provider 和独立配置。因此 registry、Worker、repository、settlement 和 cleanup 继续保留，仅移除 Gemini API 分支。

### Decision 2: 退役后显式拒绝 `gemini_api`

`IsSupportedBatchImageProvider` 和 provider selection order 只返回 `vertex`。调用方显式传入 `gemini_api` 时，复用 `BATCH_IMAGE_UNSUPPORTED_PROVIDER`/`ErrBatchImageUnsupportedProvider` 语义，不自动降级到 Vertex，避免资质、区域与价格误配。

### Decision 3: 数据门禁优先于自动资金迁移

部署前查询 `provider='gemini_api'` 的非终态、未结算 hold 和输出未清理任务。任意一项非零都阻止部署，先由现有 Worker/结算路径恢复。

不使用 migration 直接把任务改为 failed/cancelled，因为这会绕过 hold 释放、usage log 和审计事件。

### Decision 4: 测试 fixtures 使用 Vertex 或通用 fake

repository、settlement、download 等通用测试不应依赖已退役 provider 常量。它们迁移到 `vertex` 或本地 fake provider；仅用于 Gemini HTTP/JSONL 实现的测试随实现删除。

## Risks / Trade-offs

- 其他私有部署可能存在 Gemini 批图历史任务。缓解：部署前数据门禁，非零即停止。
- 老客户端仍传 `gemini_api` 会收到拒绝。这是显式 breaking 行为，不做隐式 provider 切换。
- 文档可能让运维人员误以为批图整体下线。缓解：文档明确 Vertex 仍保留。

## Migration Plan

1. 实现前重跑定向测试并记录现有 provider 行为。
2. 移除 Gemini Provider/客户端，registry 及对外类型只保留 Vertex。
3. 运行批图 repository/worker/settlement/download/cleanup 回归，验证 Vertex 不变。
4. staging 再次只读检查三张表，验证 `gemini_api` 拒绝和 Vertex 正常。
5. 生产切流前立即重查数据门禁；保留成对旧后端二进制回滚。
