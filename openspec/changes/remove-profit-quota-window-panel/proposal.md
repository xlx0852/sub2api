# Change: 移除利润分析中的配额窗口面板与账号抽屉

## Why

管理员利润分析页底部的「配额窗口」时间轴（以及只能从该面板打开的账号利润抽屉）在实际运营中没有被使用：它与利润总览、趋势图重复占用页面，且窗口经济账需要额外的实时计算。运营侧决定移除这一展示面。

窗口账本本身要继续保留：账号用量详情的「满额估算」（`account-quota-estimation`）依赖同一张观测表，上游额度展示也依赖同一批探测服务。

## What Changes

- 删除利润分析页的配额窗口时间轴面板（`QuotaWindowPanel.vue`）及其专用前端模块（`quotaTimeline.ts`）。
- 删除只能由该面板打开的账号利润抽屉（`AccountProfitDrawer.vue`）以及它在 `ProfitView.vue` 中的状态、加载函数与事件绑定。
- 删除只服务抽屉的 `POST /admin/profit/accounts/:account_id/window-economics` 接口（路由、handler、service 方法、请求/响应 DTO）与对应测试。
- 清理随之不再使用的 i18n 键与前端类型；利润分析页保留概览卡片与趋势图。
- **不改变**：窗口账本（`account_quota_windows` / `account_quota_usage_observations`）、账本写入方（请求观察、用量刷新、后台 sweep）、`account-quota-estimation` 的满额估算、账号用量页的上游额度展示。
- **不改变**：利润会计本身（计费窗口来源、break-even、摊销参照窗口）与其在 `AccountProfitSummary` 中的字段；这些字段在本次改动后暂未被前端消费，保留待后续决定。

## Impact

- 受影响规范：`account-profit-quota-windows`（移除时间轴相关的两条需求）、`account-profit-accounting`（把窗口投影的可观测面从时间轴改为利润分析结果）。
- 代码：`frontend/src/components/admin/profit/`、`frontend/src/features/admin/views/ProfitView.vue`、`frontend/src/api/admin/profit.ts`、`frontend/src/i18n/locales/{zh,en}/admin/profit.ts`、`backend/internal/handler/admin/profit_handler.go`、`backend/internal/service/profit_service.go`、`backend/internal/server/routes/admin.go`、`backend/internal/server/testdata/routes.golden`。
- 无数据库迁移、无新增或修改环境变量；窗口账本数据与后台任务保持不变。
- 回滚：还原上述文件即可；被删除的接口没有其它调用方。
