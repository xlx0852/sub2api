# Change: 新增用户储值与账号供给预测

## Why

利润分析目前只能回看已发生的收入、成本和利润，不能回答“用户账户里已经存了多少待消费余额”以及“按当前消费速度，未来需要多少上游账号或采购预算才能承接”。缺少这些数据会使订阅号采购和上游充值依赖经验判断。

## What Changes

- 在利润分析中增加独立的“供给预测”页签，不阻塞现有利润总览加载。
- 展示活跃普通用户的可消费余额、冻结余额、最近 7/30 天日均储值消耗、余额可支撑天数和指定规划期内的预计消耗。
- 只将余额计费请求纳入储值消耗速度；订阅用户的免余额扣费请求不得重复计入。
- 以最近 7 天与 30 天较高的日均消耗作为规划需求，支持 7/30/60/90 天规划期和可配置安全余量，同时返回全部假设与样本窗口。
- 按历史余额消费的平台占比拆分未来需求，避免将 OpenAI、Grok、Anthropic 等异构供给混为一个数字。
- 对 OAuth / Setup Token 订阅供给，使用历史单账号活跃日收入的 P75 作为保守单号日承载量，计算所需账号数、当前可调度数、缺口/富余和置信度。
- 对 API Key 按量供给，按历史上游成本/客户收入比估算规划期采购预算，不伪造“所需账号数”。
- 对混合平台分别展示订阅承载需求与按量采购预算，并在平台级去重当前可调度账号。
- 新增独立预测 API，使用 15 分钟服务端快照、singleflight 和手动刷新；页签首次打开时再延迟请求，不增加利润首屏延迟。
- 显示预测生成时间、数据完整性、置信度和不可估算原因；数据不足时不以 0 代替。

## Impact

- Affected specs: `balance-supply-forecast`
- Affected code: `backend/internal/repository/` 预测聚合查询、`backend/internal/service/` 预测模型与公式、`backend/internal/handler/admin/` 快照 API、`frontend/src/api/admin/`、`frontend/src/views/admin/ProfitView.vue`、中英文文案与测试。
- No database migration is required for the initial implementation; forecasts are derived from existing users, usage logs, accounts, account groups, and subscription-cycle data.
- **Behavioral note**: 预测是基于历史消费与承载的经营估算，不是财务负债确认或上游官方额度承诺。
