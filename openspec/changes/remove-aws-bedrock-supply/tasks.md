## 1. 契约与写路径封锁

- [x] 1.1 补扫 OpenAPI、docs、modelcatalog、价格资源、前端 registry 和差分工具的 Bedrock 足迹，记录保留/删除清单
  - 备注：删除供应目录/运行路径；保留 beta-policy scope 往返、S3 AWS SDK 依赖和可被其他 Anthropic 定价使用的导入价格行。
- [x] 1.2 增加统一 retired Bedrock 类型错误，并在 AccountService/adminService 创建路径拒绝 `type=bedrock`
  - 备注：新增 `BEDROCK_ACCOUNT_RETIRED`，公开与 admin 创建路径共用同一守卫。
- [x] 1.3 从 handler create/update oneof 和前端 AccountType/registry 中移除 Bedrock，同时补 HTTP 建号拒绝回归
  - 备注：handler binding 和 registry 均不再暴露 Bedrock，已增 HTTP 绑定回归。
- [x] 1.4 封锁单条更新、状态更新、批量更新、恢复、清错和账号测试等重新激活/调用存量 Bedrock 的路径
  - 备注：service 与 repository 双层封锁；限流恢复/token refresh 等直连 repository 的清错路径也无法重新激活。

## 2. 存量数据迁移

- [x] 2.1 新增幂等迁移 `198_retire_aws_bedrock_accounts.sql`，仅将存量 Bedrock 账号置 inactive
- [x] 2.2 增加迁移回归：重复执行无害、非 Bedrock 不变、账号/usage/settlement/关联引用不删除
  - 备注：SQL 仅 UPDATE 匹配行，二次执行不再匹配；静态回归禁止 DELETE/DROP 及 usage/settlement 写入。
- [x] 2.3 增加应用层不变量测试，证明迁移后的 Bedrock 不能被任何写路径重新激活
  - 备注：覆盖 create/update/status/bulk/restore/clear-error/schedulable/test，并补 repository 绕过 service 的回归。

## 3. 后端供应实现删除

- [x] 3.1 删除 `gateway_bedrock.go`、`bedrock_request.go`、`bedrock_signer.go`、`bedrock_stream.go` 及专用测试
- [x] 3.2 删除 Gateway 转发、CC 兼容、count_tokens、GetAccessToken、账号探测、上游模型和响应处理中的 Bedrock 分支
- [x] 3.3 删除调度、runtime stats、quota/usage billing/repository mapper 中的 Bedrock 半边，将通用逻辑显式收敛为 API-key 语义
- [x] 3.4 删除 domain/service Bedrock 账号类型辅助、默认模型映射和 modelcatalog 的 Bedrock 平台/目录段
- [x] 3.5 审查导入价格数据；保留仍服务其他供应的条目，仅删除经测试证明为 Bedrock 独占的元数据
  - 备注：导入价格行不是供应注册表，仍可为 Anthropic 价格规范化服务，因此不删除。

## 4. scope 与配置兼容

- [x] 4.1 保留 `BetaPolicyScopeBedrock` 的合法枚举、解析、序列化、API/前端类型和存量配置回显
- [x] 4.2 删除运行时 Bedrock scope 匹配与 Bedrock 专用默认模型白名单，使该 scope 永不命中
- [x] 4.3 增加存量 beta/fast policy `scope=bedrock` 的读取、编辑和保存 round-trip 回归

## 5. 前端收口

- [x] 5.1 删除 Create/Edit/Bulk 的 Bedrock 选择与凭据字段，不影响 registry 中其他平台字段
- [x] 5.2 删除账号筛选、平台徽标、模型白名单预设、容量/用量/操作菜单中的 Bedrock 运行逻辑
- [x] 5.3 删除仅用于 Bedrock 账号/渠道的 zh/en 文案；保留 scope 配置兼容文案
- [x] 5.4 增加 registry、类型检查和 i18n 回归，证明 UI 不再暴露 Bedrock 建号入口

## 6. 差分测试床与文档

- [x] 6.1 从 reconcile/crash harness/fake upstream 删除 Bedrock key、种子、TLS SAN、provider 和 EventStream 模拟
- [x] 6.2 将 `run-short-path-diff` 从 6/6 更新为 4/4，保持其他差分比较与失败退出门禁不变
  - 备注：用例矩阵已收敛为 4 条；真实 dual-version 执行结果在 7.2 单独记录。
- [x] 6.3 更新中英文架构文档，删除 Bedrock 生产短路径说明并记录账号退役迁移
- [x] 6.4 确认 `aws-sdk-go-v2` 仍由 S3 备份使用，依赖不删除
  - 备注：`backup_s3_store.go` 仍导入 config/credentials/s3/signer，`go.mod` 不改。

## 7. 回归、提交与部署

- [x] 7.1 调度、计费、账号、探测、modelcatalog、migration 相关定向测试与 `-race` 全绿
  - 备注：Bedrock retired/service/handler/migration/scope 定向与 race 通过；repository testcontainers 通过 Colima socket 实际运行 3/3 通过。
- [x] 7.2 `run-diff`、`run-disconnect-diff` 和 4/4 `run-short-path-diff` 全绿，响应/usage/settlement 比较不放宽
  - 备注：本机隔离双实例实测为 10/10、5/5、4/4；schema/usage snapshot 全部相等，未请求远程环境。
- [x] 7.3 archtest、unit/integration 全仓编译、`git diff --check`、根 `make build` 和 `openspec validate remove-aws-bedrock-supply --strict` 全绿
  - 备注：`-tags=unit`/`-tags=integration` 通过 `-exec /usr/bin/true` 完成全仓编译；archtest、vue-tsc/Vite、根构建、diff-check 与 strict validate 均通过。普通全量执行仍有既有 WS/OAuth/默认模型断言失败，未修改这些 dirty WIP 测试。
- [x] 7.4 以明确文件集合独立提交，不使用 `git add -A`，不混入 fingerprint 或其他 WIP
  - 备注：按后端、前端、工具/文档/OpenSpec 三个明确文件集分批提交，全程未使用 `git add -A`。
- [ ] 7.5 staging 验证建号拒绝、存量 inactive、非 Bedrock HTTP/SSE/WS、计费与回滚；生产部署后观察通过才 archive
