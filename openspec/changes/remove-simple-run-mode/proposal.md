# Change: 移除简洁运行模式

## Why

简洁模式同时改变认证、计费、配额、分组调度、后台菜单和初始化数据，形成一套长期分叉的运行语义。继续保留会扩大测试矩阵，并可能让同一 API Key 在不同运行模式下产生不同的扣费和调度结果。

## What Changes

- **BREAKING**：删除 `simple` 运行模式，仅保留 `standard`。
- **BREAKING**：配置为 `RUN_MODE=simple` 或 `run_mode: simple` 时拒绝启动并输出迁移提示，不静默回退到标准模式。
- 删除简洁模式下对认证、余额、配额、计费、分组隔离、调度快照、定时任务和初始化数据的旁路逻辑。
- 删除前端 `run_mode` / `isSimpleMode` 状态、简洁模式路由守卫、菜单过滤、组件分支和引导分支。
- 删除简洁模式专用初始化与一次性维护代码；保留已经写入数据库的普通业务数据，不执行破坏性清理。
- 更新配置示例、README、API 契约和相关测试。

## Impact

- Affected specs: `runtime-mode`
- Affected code: `backend/internal/config`, `backend/internal/server/middleware`, `backend/internal/service`, `backend/internal/repository`, `backend/internal/setup`, `backend/cmd/server`, `frontend/src/stores`, `frontend/src/router`, `frontend/src/components`, `frontend/src/views`, deployment examples and documentation
- Migration: 当前使用简洁模式的部署必须先补齐标准模式所需的用户、分组、渠道、计费和配额配置，然后将 `RUN_MODE` 改为 `standard` 或删除该配置，再升级启动。
