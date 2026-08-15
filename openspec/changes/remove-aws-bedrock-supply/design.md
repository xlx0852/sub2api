## Context

Bedrock 当前以 `platform=anthropic,type=bedrock` 存在，既支持 SigV4，也支持 Bedrock API key。它拥有独立请求准备、签名、EventStream 解码、错误/failover、CC 兼容、模型映射和账号探测路径，并通过通用 `IsAPIKeyOrBedrock` 分支参与配额、调度和计费。前端同时把它作为可创建账号类型暴露，差分测试床也把 Bedrock JSON/SSE 当作两条短路径。

因此退役不能只删除 `forwardBedrock`：必须先封住全部写路径，再在迁移中停用存量账号，最后删除所有生产分支和 UI 暴露。历史数据与存量策略配置仍需可读取，避免把供应退役扩大成账务或配置破坏。

## Goals / Non-Goals

### Goals

1. 部署新版本后不存在可被调度、探测或转发的 Bedrock 账号。
2. 新建、类型变更、单条状态更新、批量更新、恢复和清错等路径都不能重新激活 Bedrock。
3. 存量账号保留但处于 inactive；历史 usage、结算、审计和加密凭据不变。
4. 删除 Bedrock 专用协议实现与前端入口，缩小生产和回归矩阵。
5. 保留 beta-policy `bedrock` scope 的配置兼容性，但让它永不匹配。

### Non-Goals

- 自动把 AWS 凭据转换成其他上游凭据。
- 删除通用 AWS SDK 或 S3 备份能力。
- 改变其他平台的定价、配额、调度或协议行为。
- 清理所有文本中的历史 Bedrock 名称；迁移说明和历史审计证据可以保留。

## Decisions

### Decision 1: 先封写路径，再删运行时路径

service 层提供统一的 retired-type 判定，供公开 AccountService 和 adminService 使用。HTTP binding 同时从 create/update 的 oneof 中移除 `bedrock`，但 service guard 仍是非 HTTP 调用、批量更新和未来入口的最终防线。

对于存量 Bedrock 账号：允许读取、审计和删除；拒绝把状态设为 active、恢复、清错后激活、测试连接或变更为可运行供应。不得通过给 service 类型别名增加兼容方法实现守卫。

### Decision 2: 使用 additive migration 198

当前最新迁移为 197，因此新增 `198_retire_aws_bedrock_accounts.sql`。迁移只更新 `accounts.type='bedrock' AND status<>'inactive'` 的行，将 status 置为 inactive 并更新时间戳；重复执行无额外影响，不修改其他账号，也不删除任何关联数据。

迁移在服务启动加载账号缓存和接收流量之前完成。应用层守卫负责迁移后长期不变量，不能只依赖一次 SQL。

### Decision 3: 彻底删除供应实现，保留历史数据识别

删除 `gateway_bedrock.go`、`bedrock_request.go`、`bedrock_signer.go`、`bedrock_stream.go` 及其专用测试，并移除转发、count_tokens、账号测试、调度、usage billing、上游模型和 CC 兼容分支。

后端生产逻辑不再需要 `AccountTypeBedrock`、`IsBedrock`、`IsBedrockAPIKey` 或 `IsAPIKeyOrBedrock`。通用逻辑改为明确的 API-key 判定。历史数据库行以字符串值保存，不要求运行时保留一个可调度的类型常量；管理 DTO 若需要展示历史类型，只按不受支持的原始字符串回显。

### Decision 4: scope 配置兼容但永不匹配

`BetaPolicyScopeBedrock` 继续属于合法配置值，解析、保存、API schema/前端类型和已有 UI 回显不得把它判为非法或静默改写。删除 `betaPolicyScopeMatches` 和 OpenAI request-body 中的 Bedrock account 判定后，该 scope 对所有运行账号返回 false。

默认 beta-policy 中只为 Bedrock 模型 ID 存在的白名单模式可以删除；scope 枚举本身必须保留。这两个概念不得混淆。

### Decision 5: 模型目录与价格数据分开处理

删除 modelcatalog 的 `bedrock` 平台、默认映射和供应模型列表，以及两份 catalog 镜像中的 Bedrock 段。`backend/data/model_pricing.json` 和 resources 下的导入价格文件不是账号供应注册表；其中 `litellm_provider=bedrock` 的条目若仍为其他 Anthropic 路径提供价格，必须保留。只有定向价格回归证明无影响时才可删除独占条目。

### Decision 6: 测试床按真实生产面收敛

删除 fake upstream 的 Bedrock provider/EventStream 模拟、TLS SAN、种子账号和两个短路径 case。`run-short-path-diff` 的预期由 6/6 改为 4/4；`run-diff`、disconnect、OpenAI WS、故障注入和 usage snapshot 比较不放宽。

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| 存量账号被普通更新重新激活 | service 统一 guard + 单条/批量/恢复/清错回归测试 |
| 删除通用分支时误伤 Anthropic API key | 将 `IsAPIKeyOrBedrock` 显式收敛为 API-key，并逐个调用点做计费/配额/调度回归 |
| 删除 catalog/价格数据导致其他供应缺价 | 区分供应目录与导入价格元数据，跑 modelcatalog 与 billing 定向测试 |
| handler oneof 先于 service 返回通用校验错误 | API 只承诺明确拒绝；service 调用仍返回稳定 retired-type 错误码/消息 |
| 存量 beta-policy 无法保存 | 保留 scope enum、解析、序列化和前端 round-trip 测试 |
| 升级期间旧进程仍持有 Bedrock 缓存 | 迁移随新实例启动，切流前验证无 active Bedrock；使用成对回滚而非混跑不同 schema 语义 |

## Migration Plan

1. 先补 retired-type 写路径测试与迁移测试，再实现 service/handler 守卫。
2. 添加迁移 198，并验证只影响 Bedrock、幂等且保留关联历史数据。
3. 删除核心转发与通用分支的 Bedrock 半边，逐域运行调度、计费、探测和模型目录测试。
4. 删除前端创建/编辑/筛选/展示入口，保留 scope 配置 round-trip。
5. 收敛差分测试床为 4 条短路径，重跑全套门禁。
6. 以独立提交部署 staging，验证建号拒绝、存量 inactive、非 Bedrock 数据面与回滚；生产部署后观察通过才归档。

## Rollback

应用回滚使用退役前后端与前端成对版本。迁移 198 不删除数据，因此旧版本若必须回滚，可在明确人工审批后选择性恢复原 Bedrock 账号状态；默认回滚不自动重新激活供应，避免在故障处理中意外恢复已退役流量。

## Open Questions

- 无。用户已决定彻底删除供应、存量账号置 inactive、保留 scope 配置值。
