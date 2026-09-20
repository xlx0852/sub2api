## 1. 审计与冲突收敛

- [x] 1.1 只读审计生产 `service_account` 账号数、状态、分组绑定、近期 usage 和可替代供应容量。
  - 备注：生产 `type=service_account` 全库 0 行，近 30 天 usage 0；现网仅 apikey/oauth。
- [x] 1.2 只读审计批图任务表，确认不存在 Vertex 非终态任务或制定终结方案。
  - 备注：`batch_image_jobs` 0 行，非终态 0。
- [x] 1.3 修正 `remove-gemini-api-batch-provider` 中“保留 Vertex”规格、设计、任务和测试边界。
  - 备注：proposal/design/spec 已改为最终无 Google 批图 Provider；groups 列与 `batch_image.*` 键仍保留。
- [x] 1.4 补扫 OpenAPI、modelcatalog、价格、配置、前端、文档和工具中的 Vertex/Service Account 足迹。
  - 备注：控制面/前端建号入口已收口；Vertex 实现文件仍在磁盘，运行时 fail-closed。

## 2. 控制面与写路径封锁

- [x] 2.1 将错误语义升级为稳定的 Vertex Service Account retired 错误。
- [x] 2.2 在 AccountService/adminService/repository 封锁创建、类型转换、恢复、清错、启用、schedulable 和批量重新激活。
- [x] 2.3 封锁账号测试、token refresh、上游模型同步和其他会调用存量凭据的控制面路径。
- [x] 2.4 增加 handler/service/repository 写路径矩阵回归，证明账号不能重新上线且无关联副作用。

## 3. 存量数据迁移

- [x] 3.1 新增幂等迁移，仅将 `type=service_account` 存量账号置 inactive/non-schedulable。
- [x] 3.2 增加迁移回归：重复执行无害、非 Vertex 不变、账号/凭据/usage/settlement/audit 不删除。
- [x] 3.3 增加应用层不变量测试，证明迁移后的账号不能被任何写路径恢复。

## 4. 普通网关运行时删除

- [x] 4.1 删除 Service Account JWT/token exchange、缓存、代理和 Vertex URL/body 构造实现及专用测试。
- [x] 4.2 删除 Anthropic/Gemini pipeline、token provider、模型映射、beta filter 和上游模型中的 Vertex 分支。
- [x] 4.3 删除调度、账号探测、容量/用量和计费中的 Service Account 运行分支。
- [x] 4.4 删除 modelcatalog/config 中 Vertex 独占供应声明，保留经审计仍由其他 Google 供应使用的共享能力。
  - 备注：已删除 `batch_image.vertex_*` 配置项与校验；Gemini 官方配额文档链接保留。

## 5. Vertex 批图删除

- [x] 5.1 删除 Vertex Batch Prediction/GCS Provider、client、registry、配置校验和专用测试。
- [x] 5.2 将批图公共类型、worker/queue/settlement/download/cleanup 收敛到退役后的真实 Provider 集合。
  - 备注：无可用 Provider 后删除 service/repository/ports 批图实现；分组字段与 `batch_image.*` 配置键保留以免动 schema。
- [x] 5.3 更新批图文档和前端 API 类型，删除 Vertex 设置与操作入口。
  - 备注：配置入口已删；批图公开 API 仍可识别历史 `provider=vertex` 字符串，但不再支持提交。

## 6. 前端与历史兼容

- [x] 6.1 从 Create/Edit/Bulk、筛选、容量/用量和操作菜单移除 Vertex 运行入口与凭据字段。
- [ ] 6.2 保留历史 inactive 行的最小识别、导出和删除能力，不展示恢复/测试/启用动作。
- [x] 6.3 删除仅用于 Vertex 运行的 zh/en 文案、类型和 registry 元数据。
  - 备注：建号/编辑文案与 location 常量已删；`AccountType` 仍保留 `service_account` 以便识别历史行。

## 7. 回归、提交与发布

- [ ] 7.1 运行账号、调度、Gemini/Anthropic、批图、计费、migration 定向与 race 测试。
- [ ] 7.2 运行非 Vertex HTTP/SSE/WS、故障切换、usage/结算差分，证明其他供应语义不变。
- [ ] 7.3 运行前端 typecheck/build、后端全仓编译、archtest、`git diff --check` 和 OpenSpec strict。
- [ ] 7.4 以明确文件集合独立提交，不混入当前 dirty worktree 的其他 WIP。
- [ ] 7.5 使用 `both + db_change` 在 staging 验证迁移、全部拒绝路径、非 Vertex 流量与补偿回滚。
- [ ] 7.6 蓝绿发布生产，验证存量账号 inactive、无 Vertex 运行路径及公网网关后保留回滚资产。
