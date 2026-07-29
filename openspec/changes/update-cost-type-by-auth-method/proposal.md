# Change: 按账号认证类型自动判定利润成本口径

## Why

目前成本配置弹窗允许管理员手动选择“按量计费”或“订阅制”。该选择可以与账号实际认证方式冲突，导致 API Key 账号被错误地按订阅摊销，或 OAuth 登录账号遗漏周期成本，进而使利润报表失真。

## What Changes

- 将成本类型改为由账号认证类型自动判定：`oauth` / `setup-token` 为订阅制，`apikey` 为按量计费。
- 利润服务以该判定作为唯一成本口径，不再接受历史配置中的 `cost_type` 覆盖。
- 对 API Key 按量账号，成本固定采用每条使用记录快照中的“账号计费倍率”（上游采购折扣）乘以原始上游模型成本；用户收入仍采用实际扣费金额。
- 在利润界面明确区分“上游采购折扣”和“客户销售倍率”，避免将低价采购误判为亏损。
- 移除“窗口基准收入”的人工输入；系统使用该账号历史实际扣费自动计算 5 小时窗口收入基准，仅用于展示窗口变现效率，不参与利润计算。
- 为订阅账号引入“订阅周期账本”：每次实际充值单独记录起始日、周期费用、周期天数、币种和备注。
- 利润按查询范围与每条真实订阅周期的交集汇总成本；两个充值周期之间的停用空档成本为 0，不假设自动续订。
- 计费窗口优先使用账本中的当前有效周期；仅在旧数据没有账本时，才允许按凭据 `subscription_expires_at` 生成可识别的回退视图。OAuth 的 `expires_at` 不得作为订阅周期日期。
- 在订阅周期录入界面保留“一键推算日期”辅助按钮：优先按 `subscription_expires_at` 推算；必要时可按 OAuth `expires_at` 生成未确认草稿，但不得自动保存或参与利润计算。
- 成本配置界面移除成本类型切换：订阅账号只配置周期费用与周期天数；API Key 账号说明成本直接来自历史使用记录。
- 保留已有成本配置记录及订阅费用，不自动删除或篡改历史使用账本；不匹配认证类型的旧 `cost_type` 不再参与利润计算。
- 补充认证类型映射、旧配置兼容及界面展示的回归测试。
- 优化利润接口的数据访问路径：全局汇总与趋势共用一次按账号、按日聚合，订阅周期、窗口基准和当前周期收入全部批量读取，查询次数不再随账号数量线性增长。
- 新增兼容性聚合接口供利润页一次加载汇总、趋势和账号明细；保留现有 summary/trend 接口，避免破坏已有调用方。
- 将批量订阅配置从逐账号 Upsert 改为单次批量写入，并增加查询预算与聚合结果回归测试。

## Impact

- Affected specs: `account-profit-accounting`（新增能力规格）
- Affected code: `backend/migrations/`、`backend/internal/repository/profit_repo.go`、`backend/internal/service/profit_service.go`、成本周期 repository/handler/service、`frontend/src/api/admin/profit.ts`、`frontend/src/components/admin/account/AccountCostConfigDialog.vue`、`frontend/src/views/admin/ProfitView.vue`
- **Behavioral change**: 已存在但认证类型与手动成本类型不匹配的账号，利润口径将切换为认证类型对应的规则。
