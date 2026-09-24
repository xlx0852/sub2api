# Tasks: Guard Mirasim Account Model Mapping

> 纪律：只勾选**已实现且已验证**的条目（验证命令与结论见每条末尾），
> 不许预先勾选。发布相关条目在真正发布并通过核验后才勾。

## 1. 后端：派生标记（R1）

- [x] 1.1 `backend/internal/service/mirasim_model_mapping.go` 新增：
      `IsMirasimRelayAccount(*Account) bool`（导出包装 `isMirasimRelayAccount`）、
      `MirasimRelayAccountNeedsModelMapping(*Account) bool`、
      `BuildMirasimIdentityModelMapping([]string) map[string]string`
      （去空白、去重、identity，纯函数）、
      `MirasimModelMappingEqual(map[string]string, map[string]string) bool`（幂等比较）。
      —— 已实现；`go test -tags unit ./internal/service/`（隔离目录）通过。
- [x] 1.2 `backend/internal/handler/dto/types.go` 的 `Account` 新增只读字段
      `MirasimModelMappingMissing bool \`json:"mirasim_model_mapping_missing"\``
      （无 `omitempty`，恒出现）。
      —— 已实现；`TestAccountFromServiceShallow_MirasimFlagSerializedWhenFalse` 断言
      `"mirasim_model_mapping_missing":false` 出现在 JSON 里。
- [x] 1.3 `backend/internal/handler/dto/mappers.go` 的 `AccountFromServiceShallow`
      填充该字段（基于服务层凭据原文，`AccountFromService` 自动继承）。
      —— 已实现；`backend/internal/handler/dto/account_mapper_mirasim_mapping_test.go`
      覆盖「relay 空映射 = true」「已有映射 = false」「kill switch = false」
      「普通 apikey = false」「其它平台 = false」「详情继承 = true」六种情况。
- [x] 1.4 注释写明「关键约束（改动前请先读）」：空映射语义不变、不得在此改写映射、
      字段由凭据原文派生且不得泄漏凭据。
      —— 已写入 `mirasim_model_mapping.go` 文件头与本 change 的 `design.md` D1。

## 2. 后端：一键写入端点（R2）

- [x] 2.1 `backend/internal/handler/admin/account_mirasim_model_mapping.go` 新增
      `ApplyMirasimModelMapping`：
      解析 `:id` → `GetAccount`（不存在 404）→ 非 mirasim 账号 400 →
      校验上游模型来源已注入 → 读取 `{"overwrite": bool}`（容忍空 body）→
      `FetchUpstreamSupportedModels`（账号维度 `GET /v1/models`，force=true）→
      错误分类映射（configuration/unsupported → 400，其余 → 502，固定文案）→
      空清单 → 502 且不写库 → 构造 identity 映射 →
      与现值比较：一致则 `changed=false` 直接返回；非空现值且 `!overwrite` → 409 →
      拷贝现有**非敏感**凭据子键 + 覆盖 `model_mapping`（`map[string]any`）→
      `adminService.UpdateAccount` → 返回 `{"models": [...], "changed": true}`。
      —— 已实现；11 个处理器用例全绿（见 4.2）。
- [x] 2.2 日志只用 `slog` 事件名 + `account_id` / 模型数 / 错误类别；
      绝不 log 凭据、上游正文或 `err.Error()`。
      —— `mirasim_apply_model_mapping_update_failed` / `mirasim_apply_model_mapping_applied`
      仅含 `account_id`、`model_count`、`error_class`。
- [x] 2.3 `backend/internal/server/routes/admin.go` 注册
      `accounts.POST("/:id/models/apply-mapping", h.Admin.Account.ApplyMirasimModelMapping)`
      （放在 `/:id/models/sync-upstream` 之后）。—— 已注册。
- [x] 2.4 不改 `NewAccountHandler` 参数列表、不改 `wire_gen.go`（复用既有字段）。
      —— 仅新增未导出字段 `upstreamModels upstreamModelsFetcher`；
      `NewAccountHandler` 内部 `if accountTestService != nil` 赋值以避免 typed-nil 接口。
      `go build ./...` 通过。

## 3. 前端：风险提示 + 一键填充（R3）

- [x] 3.1 `frontend/src/types/index.ts` 的 `Account` 新增
      `mirasim_model_mapping_missing?: boolean`（含注释说明是后端派生只读字段）。
      —— 已实现。
- [x] 3.2 `frontend/src/api/admin/accounts.ts` 新增
      `applyMirasimModelMapping(id, overwrite?)` → `POST /admin/accounts/:id/models/apply-mapping`，
      返回 `{ models: string[]; changed: boolean }`；并导出到默认导出对象。
      —— 已实现。
- [x] 3.3 `frontend/src/components/account/EditAccountModal.vue`：
      在 mirasim relay 区块内、`account.mirasim_model_mapping_missing === true` 时
      显示风险提示（空映射＝认领所有模型 → 上游 422 → 网关 502）+
      「以可用模型填充映射」按钮（`data-testid="edit-mirasim-apply-mapping"`），
      成功后回填白名单并提示（`changed=false` → 「无变化」提示），
      失败只提示错误、不破坏编辑态。新增 `isApplyingMirasimMapping` 防重入。
      —— 已实现；本地已有编辑态时自动带 `overwrite=true`。
- [x] 3.4 i18n：`frontend/src/i18n/locales/zh/admin/accounts.ts` 与
      `frontend/src/i18n/locales/en/admin/accounts.ts` 的
      `mirasimRelay.*` 补充 `mappingMissingTitle` / `mappingMissingDesc` /
      `applyMapping` / `applyMappingLoading` / `applyMappingSuccess` /
      `applyMappingNoChanges` / `applyMappingEmpty` / `applyMappingFailed` /
      `applyMappingFailedWithMessage`（9 个键，中英各一份）。
      —— 已实现。（原任务文本写的是 `src/locales/zh-CN.ts`，与本仓实际的
      分域 i18n 结构不符，这里已更正为真实路径。）

## 4. 验证（R4）

- [x] 4.1 服务层纯函数单测 `backend/internal/service/mirasim_model_mapping_test.go`：
      识别三态（mirasim 有/无映射、普通 anthropic apikey、kill switch 关闭）、
      identity 生成（去空白、去重、跳过空串）、幂等比较、派生标记与非 mirasim 恒 false。
      —— 已实现；隔离目录 `go test -tags unit ./internal/service/` → `ok`。
- [x] 4.2 处理器测试（`backend/internal/handler/admin/account_mirasim_model_mapping_test.go`）：
      空映射一键填充 200/`changed=true` 且凭据保全；重复调用 `changed=false`；
      非空映射无 `overwrite` → 409；`overwrite=true` → 覆盖；
      空清单 → 非 2xx 且不写库；非 mirasim 账号 → 400；账号不存在 → 404；
      上游失败响应不含上游正文。
      —— 11 个用例；`go test ./internal/handler/admin/ -count=1` → `ok`。
- [x] 4.3 `cd backend && go build ./...` 通过。—— BUILD_OK（exit 0）。
- [x] 4.4 隔离目录单测（`/tmp/mrsverify` 硬链接副本 + 移走 522 个陈旧 `*_test.go`）：
      `go vet -tags unit ./internal/service/` 与 `go test -tags unit ./internal/service/`
      全绿；`go vet ./internal/handler/admin/ ./internal/handler/dto/ ./internal/service/`
      通过。—— 全部 exit 0。
- [x] 4.5 前端：`vue-tsc --noEmit` 通过；在
      `frontend/src/components/account/__tests__/EditAccountModal.spec.ts` 的
      `EditAccountModal Mirasim relay wiring` 内新增 5 个用例（提示显示/隐藏、
      一键填充成功回填白名单、本地已有映射时带 `overwrite=true`、
      `changed=false` → info、失败不破坏编辑态）；全量 `vitest run`
      仅剩既有失败（`UserPlatformQuotaModal.spec.ts` 2 条、`AccountsView.sparkShadow.spec.ts` 1 条）。
      —— 该 spec 40/40 通过；全量 174 passed / 2 failed（失败文件与 HEAD 相同）。
- [x] 4.6 路由清单护栏：`go test ./internal/server` 曾因本变更新增
      `POST /api/v1/admin/accounts/:id/models/apply-mapping` 而失败
      （另有 5 条 mirasim oauth 路由在 HEAD 上就已漏更新 golden），
      已用 `-update` 重生成 `internal/server/testdata/routes.golden`（+6 行，纯新增）。
      —— `go test ./internal/server -count=1` → `ok`；HEAD worktree 反证该测试在
      本变更前即为 FAIL。
- [x] 4.7 全量回归：`go test ./... -count=1`（无 tag）在本分支仅
      `internal/archtest`、`internal/service` 失败，两者失败用例与 HEAD worktree 完全一致
      （6 条 pre-existing 失败），本变更未引入新失败；且本变更修好了 HEAD 上
      失败中的 `internal/server`（路由 golden）。
- [ ] 4.8 生产只读核验（发布后执行）：对账号 `140` 调
      `POST /api/v1/admin/accounts/140/models/apply-mapping` —— 该端点幂等：
      期望 `changed=false`，响应模型数 = 5，账号 `model_mapping` 与调用前逐键相同。
      注意：线上该账号的空映射此前已被外部操作补齐，故本次核验的是「幂等 + 不破坏现有映射」，
      而不是「从空填满」。

## 5. 生产发布

- [ ] 5.1 前端先构建（`pnpm` + `vite build`）并落位发布目录，记录文件数与 sha。
- [ ] 5.2 后端 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed ...`，
      记录 sha256 前缀与备份路径 `sub2api.bak-<日期>-<tag>`。
- [ ] 5.3 发布后核验：账号列表响应含 `mirasim_model_mapping_missing`（账号 140 为
      `false`，即已修好）；日志无新增 502；无凭据泄漏。
- [ ] 5.4 写发布记录 `deploy-<日期>-mirasim-mapping-guard.md`（放在本 change 目录）。

## 生产发布记录（<待填>）

- 代码提交：`<见 git log>`（`feat(mirasim): guard empty account model mapping`）。
- 前端：`frontend/releases/<ver>-<hash>/`，`current` 已重指，回滚点指向前一版本。
- 后端：`backend sha256=<前缀>`，备份 `sub2api.bak-<日期>-<tag>`。
- 核验：账号 140 `mirasim_model_mapping_missing=false`；`apply-mapping` 幂等返回
  `changed=false`、5 个模型；发布后日志无 `account_id=140` 的 `forward_failed`。
- 详见 `deploy-<日期>-mirasim-mapping-guard.md`。
