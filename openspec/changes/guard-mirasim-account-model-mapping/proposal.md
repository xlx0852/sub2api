# Change: Guard Mirasim Account Model Mapping

## Why

生产事故（2026-09-25）：账号 `140`（mirasim-relay，`platform=anthropic` + `type=apikey`，
`priority=0`，组 34）的 `credentials.model_mapping` 为空。`Account.IsModelSupported`
（`backend/internal/domain/account_entity.go:888`）对空映射的语义是「允许所有模型」，
于是这个优先级最高、实际只能服务 5 个模型的账号被调度去认领 `deepseek-v4.1-flash`
（mirasim 侧真实 id 是 `deepseek-flash`，`v4.1-flash` 只是 label），
上游返回 `422 model "deepseek-v4.1-flash" is not supported at this time`，
网关把它映射成 **502** 返回给客户端（近 8h 命中 6 次，全部集中在修复前的那 27 分钟内）。

账号配置本身已修正（`updated_at=2026-09-25T01:15:59+08:00`，5 条 identity 映射），
修复后同账号 `gateway.forward_failed` 归零。但**这个坑没有任何护栏**：

1. 管理端看不出「这个 mirasim 账号的映射是空的」——响应里 `model_mapping` 就是 `{}`/缺省，
   与「故意不限制」无法区分，前端也不会提示风险。
2. 新建 mirasim relay 账号（OAuth onboarding 只产凭据、不落库，
   最终走 `POST /api/v1/admin/accounts`）默认没有映射，等于**开箱即踩同一个坑**。
3. 手动填映射需要人肉去 `/v1/models` 抄 5–8 个 id，抄错一个就是下一个 422。

本变更不改「空映射＝允许所有」的全局调度语义（改动会让调度依赖动态上游缓存、
失败模式不确定，风险远大于收益），而是在管理端补**可见性**（派生告警位）与
**一键写入**（以账号维度可用模型生成 identity 映射）。

## What Changes

- **新增**管理端账号响应字段 `mirasim_model_mapping_missing`（派生布尔，只读）：
  仅当账号是 mirasim relay 账号且 `model_mapping` 为空时为 `true`；
  其它平台/普通 anthropic apikey 恒为 `false`。基于服务层凭据原文判定，
  不新增任何凭据字段的暴露（仍是脱敏后的 `credentials` + `credentials_status`）。
- **新增**管理端端点 `POST /api/v1/admin/accounts/:id/models/apply-mapping`：
  以 force 拉取该账号的 `GET /v1/models`（账号维度、只读），以 **identity** 映射
  （`id -> id`）写入 `credentials.model_mapping`。护栏：
  - 非 mirasim relay 账号 → `400`；
  - 上游返回空清单 → 拒绝写入（绝不允许把 `model_mapping` 写成空，
    那等于「认领所有模型」，正是本变更要防的状态）；
  - 已有非空映射时默认 `409`，需显式 `overwrite=true` 才覆盖；
  - 幂等：目标映射与现值一致时返回 `changed=false` 且不写库。
- **新增**前端提示与按钮：`EditAccountModal.vue` 的 mirasim relay 区块在
  `mirasim_model_mapping_missing === true` 时显示风险提示，并提供
  「以可用模型填充映射」按钮（调用上述端点，成功后回填白名单）。

## Impact

- 后端（新增文件）：`backend/internal/service/mirasim_model_mapping.go`、
  `backend/internal/handler/admin/account_mirasim_model_mapping.go`。
- 后端（修改）：`backend/internal/handler/dto/types.go`（`Account` 新增派生字段）、
  `backend/internal/handler/dto/mappers.go`（`AccountFromServiceShallow` 填充）、
  `backend/internal/server/routes/admin.go`（注册路由）、
  `backend/internal/service/mirasim_relay.go`（导出识别帮助函数）。
- 前端（修改）：`frontend/src/types/index.ts`、`frontend/src/api/admin/accounts.ts`、
  `frontend/src/components/account/EditAccountModal.vue`、i18n 词条。
- **不改动**：`IsModelSupported` / `GetMappedModel` 的调度语义、mirasim 凭据读写、
  生产账号数据、mirasim 元数据缓存行为（模型清单与 `model_mapping` 无关）。
- 兼容性：新增响应字段是纯增量；新增端点不影响既有路由。
- 无迁移、无配置变更。
