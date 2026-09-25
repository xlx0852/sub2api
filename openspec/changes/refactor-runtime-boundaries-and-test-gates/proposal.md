# Change: 收敛运行时边界并恢复架构门禁

## Why

当前后端的核心业务、HTTP/SSE 传输、持久化和运行态缓存边界已经发生泄漏：`internal/service` 中有 80 个非测试文件直接使用 `gin.Context`，28 个文件直接依赖 Ent，8 个文件包含裸 SQL；OAuth handler 也直接查询 Ent。与此同时，`-tags unit` 构建配置无法编译，架构测试检测到新增包级可变全局状态，Redis key、settings KV 解析和 usage_log 多路径写入缺少集中契约。

这些问题会造成协议实现重复、跨入口行为漂移、多实例状态不一致、测试门禁失效，以及数据库/缓存修改难以安全演进。

## What Changes

本 change 分阶段修复以下架构缺陷，不改变公开 API、计费口径或默认生产部署拓扑：

1. **恢复门禁**：修复 `unit` build tag 测试隔离，令 unit/vet 门可编译；将新增包级可变全局改为构造注入并让 `internal/archtest` 回绿。
2. **传输边界**：引入 transport-neutral 的流事件/输出接口，逐步把 SSE/HTTP writer、Flush 和协议帧编码从 service 移到 transport/handler；先完成 Claude/OpenAI HTTP 主路径，再迁移 WS、图片和兼容入口。
3. **持久化边界**：把 handler 直接 Ent 查询和 service 裸 SQL 收回 repository/ports；保留 domain 纯净和 inward-only 依赖。
4. **调度与网关边界**：把重复的账号选择公共逻辑收敛到 `AccountSelector` primitives，平台差异保留为 policy/strategy，不改变现有候选集、sticky、cooldown、quota 和失败惩罚语义。
5. **数据契约集中化**：集中 Redis key builder/失效操作；抽取单一 usage log column schema/builder；把 settings 按领域拆成类型化解析器，保持已有 key/value 兼容。
6. **包边界与测试拆分**：按能力逐步拆分 `internal/service`，测试迁移到对应子包/测试目录；禁止继续新增跨能力依赖和超大 god object。
7. **仓库卫生**：补齐构建产物、凭据文件、虚拟环境和临时目录的忽略规则，并清理未跟踪产物（不删除用户明确保留的源代码）。

## Impact

- Affected specs: `architecture-governance`
- Affected code: `backend/internal/service`、`backend/internal/handler`、`backend/internal/repository`、`backend/internal/ports`、`backend/internal/domain`、`backend/internal/archtest`、Redis key helpers、settings/usage persistence
- **BREAKING**: 无公开 API、配置 key、计费口径或部署拓扑 breaking change；内部 service 包路径可能在迁移提交中调整。
- Migration: 采用小步、可回滚提交；每个能力先补行为/差分测试，再迁移实现，最后删除旧路径。
