# Change: 完整退役 Vertex Service Account 供应

## Why

Vertex Service Account 同时贯穿 Anthropic/Gemini 网关转发、Google token 交换、模型映射、账号测试、调度和 Vertex 批图 Provider。只隐藏新增入口仍会让存量账号继续承载流量，也会保留完整的凭据与协议维护面。

本 change 按完整供应退役处理：禁止新增和重新激活，将存量账号幂等置为 inactive，并删除所有生产执行路径；历史账号、usage、结算和审计记录继续保留。

## What Changes

- **BREAKING**：控制面不再接受 `type=service_account` 的新增、类型转换、重新激活、恢复、清错、凭据测试或批量启用操作。
- 新增幂等迁移，将存量 `service_account` 账号置为 inactive/non-schedulable；不删除账号、凭据密文、usage、结算或审计记录。
- 删除 Anthropic/Gemini Vertex 请求构造、Service Account JWT/token 交换、模型映射、账号探测、调度与转发分支。
- 删除 Vertex 批图 Provider、GCS/Batch Prediction 客户端、配置、文档和前端类型。
- 从账号创建/编辑、筛选、容量/用量和操作菜单移除 Vertex；历史 inactive 行仍可识别和删除。
- 修正 active change `remove-gemini-api-batch-provider` 中“Vertex 批图继续保留”的相反要求和测试。

## Impact

- Affected specs:
  - `provider-supply-lifecycle`
- Affected code:
  - `backend/internal/domain`、`backend/internal/service`、`backend/internal/handler`、`backend/internal/repository`
  - `backend/internal/config`、modelcatalog、migrations、Vertex 批图实现
  - `frontend/src/features/admin-accounts`、账号管理组件、类型与 i18n
  - `docs/BATCH_IMAGE_MVP.md`、架构/运维文档
  - `openspec/changes/remove-gemini-api-batch-provider`
- Preserved behavior:
  - Gemini API Key/OAuth、Anthropic API Key/OAuth 及其他平台账号创建不变。
  - 历史 Vertex 账号、usage、结算、账单和审计数据不删除。
  - 非 Vertex 的协议、调度、计费与结算语义不变。

## Non-Goals

- 不转换存量凭据为 API Key 或 OAuth 凭据。
- 不删除历史账号、usage、settlement、billing 或 audit 数据。
- 不删除仍由其他 Google 供应使用的共享依赖。
- 不把 Gemini 平台整体退役、Bedrock 退役或批图架构重构并入本 change。
