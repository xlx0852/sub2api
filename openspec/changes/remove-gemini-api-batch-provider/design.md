## Context

本 change 最初只退役 `gemini_api` 批图 Provider，并假设 Vertex 批图与通用 Worker 继续保留。后续 `retire-vertex-service-account-supply` 已整包退役 Vertex Service Account，批图实现栈（service/repository/ports/domain/API）也已删除。本文记录该最终状态，不再把 Vertex 当作仍可用的批图供应。

美国生产库在 2026-08-16 与后续减法部署前的只读快照均为：

| Table | Rows |
|---|---:|
| `batch_image_jobs` | 0 |
| `batch_image_items` | 0 |
| `batch_image_events` | 0 |

该证据只代表当时快照；删除实现栈前必须重查，不得把提案内的数字当作永久不变量。

## Goals / Non-Goals

### Goals

- 删除 `gemini_api` 批图上游能力和专用代码。
- 与 Vertex 供应退役对齐：不再保留 Vertex 批图 Provider 或通用批图运行时。
- 保证不存在历史任务被静默遗弃或 hold 未释放。

### Non-Goals

- 不退役 Gemini/Antigravity 普通模型请求。
- 不清理 groups 表批图列或通用 `batch_image.*` 配置键（避免 schema 大改）。
- 不删除历史批图表行（生产已为 0 行）。

## Decisions

### Decision 1: Gemini API 按 Provider 退役；Vertex 由后续 change 整包删除

`gemini_api` 先从 registry 移除。Vertex 批图不在本 change 的原始范围内，但不得再把“保留 Vertex”写成现行不变量。最终运行时无任何 Google 批图 Provider。

### Decision 2: 退役后显式拒绝 `gemini_api`

调用方显式传入 `gemini_api` 时，复用 unsupported-provider 语义，不自动降级到 Vertex 或其他供应。

### Decision 3: 数据门禁优先于自动资金迁移

部署前查询非终态任务、未结算 hold 和输出未清理任务。任意一项非零都阻止部署。不使用 migration 直接把任务改为 failed/cancelled。

### Decision 4: 测试不再依赖已删除的批图实现

通用 repository/worker/settlement 测试随实现栈删除，而不是改挂到 Vertex fixtures。

## Risks / Trade-offs

- 老客户端仍传 `gemini_api` 或 `vertex` 会收到拒绝。这是显式 breaking 行为，不做隐式 provider 切换。
- 文档若仍写 Vertex-only 会误导运维。缓解：规格与任务注明批图运行时已删除，仅保留 schema/配置键。

## Migration Plan

1. 删除 Gemini API 批图 Provider。
2. Vertex 供应退役后删除批图实现栈，保留 groups 列与 `batch_image.*` 键。
3. 生产切流前重查三张表为 0 行。
