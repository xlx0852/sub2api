# Change: 退役 AWS Bedrock 账号供应

## Why

AWS Bedrock 不是孤立的转发分支，而是贯穿账号类型、调度、计费、探测、模型目录、管理前端和差分测试床的生产供应路径。继续保留已不再运营的供应会扩大协议与计费回归面，也会让管理员继续创建实际上不应承载流量的账号。

本 change 以显式 breaking migration 彻底退役 Bedrock 供应：禁止新增或重新激活 Bedrock 账号，将存量账号幂等置为 inactive，删除运行时转发与专用协议代码，同时保留历史账号、usage、结算记录和已持久化的 beta-policy `bedrock` scope 值。

## What Changes

- **BREAKING**：控制面不再接受 `type=bedrock` 的新账号、类型变更或重新激活操作；管理员需要改用 Anthropic API key 或其他仍受支持的供应。
- 新增 `198_retire_aws_bedrock_accounts.sql`，把所有存量 `type='bedrock'` 账号幂等置为 `status='inactive'`，不删除账号、凭据密文、usage、结算或审计记录。
- 删除 Bedrock SigV4/API-key 转发、EventStream 解码、请求改写、模型映射、连通性探测、调度/计费专用分支和模型目录供应声明。
- 从账号创建/编辑界面、筛选、徽标、容量/用量展示和平台 registry 中移除 Bedrock；历史 inactive 行仍可在管理数据中识别和删除。
- 从双版本差分测试床移除 Bedrock JSON/SSE 短路径夹具，短路径矩阵由 6 项收敛为 4 项；其他 HTTP/SSE/WS、断连、故障和结算差分保持不变。
- 保留 `BetaPolicyScopeBedrock = "bedrock"` 的解析、校验、序列化和管理端回显，使存量规则继续可读取；该 scope 在运行时永不匹配任何账号。
- 保留 AWS SDK 依赖，因为 S3 备份仍在使用。导入的模型价格数据只有在证明是 Bedrock 独占且删除不影响其他供应定价时才允许清理。

## Impact

- Affected specs:
  - `provider-supply-lifecycle`（新）
- Affected code:
  - `backend/internal/domain`、`backend/internal/service`、`backend/internal/handler`、`backend/internal/repository`
  - `backend/internal/pkg/modelcatalog`、`backend/resources/model-catalog`
  - `backend/migrations`
  - `frontend/src/features/admin-accounts`、账号管理组件、类型与 i18n
  - `tools/usage_settlement_*`、`tools/refactor_dual_version_reconcile.py`
  - `docs/ARCHITECTURE.md`、`docs/ARCHITECTURE_CN.md`
- Preserved behavior:
  - 非 Bedrock 平台的协议 I/O、调度/计费语义、倍率、YAML/env 键名、默认分离部署和升级流程不变。
  - 历史 Bedrock usage/结算/审计数据及 beta-policy scope 配置不丢失。
- OpenAPI audit:
  - 当前 `openapi/control-plane.yaml` 未枚举 Bedrock 账号类型；真实约束位于 handler binding、service 写路径和前端类型/registry。本 change 仍须补契约回归，防止未来重新暴露该类型。

## Non-Goals

- 不迁移或转换存量 Bedrock 凭据为 Anthropic API-key 凭据。
- 不删除历史账号行、usage、settlement、audit 或已加密凭据。
- 不删除 `aws-sdk-go-v2`，不改变 S3 备份。
- 不修改非 Bedrock 供应的模型价格或计费结果。
- 不把计费持久性、全局状态实例化或其他架构债务并入本 change。
