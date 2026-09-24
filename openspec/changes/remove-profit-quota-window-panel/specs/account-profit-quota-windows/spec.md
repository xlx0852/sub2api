## REMOVED Requirements

### Requirement: Quota timeline rendering remains bounded

The Profit quota timeline SHALL keep its rendered grid nodes, projection work, and horizontal content width bounded independently of the number of account lanes and the user's accumulated horizontal navigation distance.

**Reason**: 利润分析页不再提供配额窗口时间轴，页面不再渲染任何账号泳道或时间网格。

**Migration**: 无需迁移。窗口账本（`account_quota_windows` / `account_quota_usage_observations`）、后台观测任务与 `account-quota-estimation` 的满额估算保持不变；依赖时间轴的运营流程改看利润总览与趋势图。

### Requirement: Timeline optimization preserves quota semantics

The optimized timeline SHALL preserve real ledger windows, fallback projections, subscription availability clipping, waiting-activation gaps, drifting windows, and the window-selection payload contract.

**Reason**: 时间轴及其优化实现（`QuotaWindowPanel.vue`、`quotaTimeline.ts`）随面板一并删除，窗口选择回调不再存在。

**Migration**: 无需迁移。窗口语义仍由账本与利润会计维护，只是不再有可视化入口。
