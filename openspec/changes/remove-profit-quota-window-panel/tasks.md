## 1. 规范

- [x] 1.1 `account-profit-quota-windows` 移除时间轴两条需求（REMOVED：Quota timeline rendering remains bounded / Timeline optimization preserves quota semantics）
- [x] 1.2 `account-profit-accounting` 把窗口投影的可观测面改为利润分析结果（MODIFIED，场景改为 profit analysis 不再报告越界窗口）
- [x] 1.3 `openspec validate remove-profit-quota-window-panel --strict` → valid

## 2. 前端

- [x] 2.1 删除 `QuotaWindowPanel.vue`(828) 、`AccountProfitDrawer.vue`(379) 、`quotaTimeline.ts`(256) 与其测试（786+158 行）
- [x] 2.2 `ProfitView.vue` 移除面板、抽屉、drawer 状态与 `openAccountDrawer`/`loadDrawer*`/`expandLedgerWindows`；补回被一并切掉的 `</AppLayout>`
- [x] 2.3 `api/admin/profit.ts` 移除 `ProfitQuotaWindow`、`ProfitWindowEconomics*` 与 `windowEconomics()`，删除 `quota_windows` / `drawer_quota_*` 字段声明
- [x] 2.4 zh/en `admin.profit` 删除 36 个仅被上述组件使用的键（含 `quotaWindow*`、`windowHistory*`、`drawerQuota*`、`drawerBillingCycle` 等）；95 个在用的键零缺失
- [x] 2.5 更新 `ProfitView.spec.ts`（去掉组件 stub 与 windowEconomics mock，断言面板不存在）

## 3. 后端

- [x] 3.1 删除 `profit.POST("/accounts/:account_id/window-economics")` 与 `routes.golden` 对应行
- [x] 3.2 删除 `ProfitHandler.GetAccountWindowEconomics`
- [x] 3.3 删除 `ProfitService.GetAccountWindowEconomics`（133 行）与 `ProfitWindowEconomicsQuery/Item/Response`
- [x] 3.4 删除 `profit_window_economics_test.go`，全仓库无残留引用

## 4. 门禁

- [x] 4.1 `go build ./...` 通过；`go test ./internal/server/ -run 'Route|Inventory|Golden'`、`./internal/service/ -run Profit`、`./internal/handler/admin/` 全部 ok
- [x] 4.2 `npx vue-tsc --noEmit` 通过；`pnpm build` 通过；`ProfitView.spec.ts` 通过
- [x] 4.3 `npx vitest run` = `2 failed | 173 passed (175)`、`3 failed | 1111 passed (1114)`，失败集与基线（`UserPlatformQuotaModal` ×2、`AccountsView.sparkShadow` ×1）完全一致，用例数因删除组件测试减少 18

## 5. 发布

- [x] 5.1 前端 release `0.1.391-756ee2ecc`（聚合 `02c771e3…`）；`current` 已切换、`rollback` → `0.1.391-0c48b9f6b`
- [x] 5.2 后端二进制 `sha256=5339fd48…`（`vcs.revision=756ee2ecc`）热替换；备份 `sub2api.bak-20260925-profitqwin`
- [x] 5.3 核验：两个下线接口 404、`/health` 200、5 分钟 5xx=0、容器 healthy 且无重启
- [x] 5.4 发布记录 `deploy-2026-09-25-profit-quota-window-panel.md`
