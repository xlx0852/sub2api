## Context

利润报表使用 `account_cost_configs` 保存周期费用与成本类型。账号本身已有稳定的认证类型（OAuth / Setup Token / API Key），而成本类型本质上由该认证方式决定。继续让两者独立编辑会产生矛盾状态。

## Goals / Non-Goals

- Goals:
  - 认证类型成为利润成本口径的唯一来源。
- OAuth 与 Setup Token 账号继续支持周期费用、周期天数及计费窗口利润。
- 订阅账号可以记录每次实际付款形成的独立周期，不依赖 OAuth 凭据的刷新过期时间，也不假设自动连续续费。
  - API Key 账号始终以历史 `usage_logs` 的账号侧成本作为成本。
  - 不改写用户扣费、Token、原始上游成本或历史使用倍率。
  - 全局利润页只发起一次利润数据请求，服务端数据库往返次数不随账号数量线性增长。
  - 保持现有 summary/trend 响应语义和财务计算结果不变。
  - 利润全局页使用有界时效的服务端快照，避免每次进入页面都实时扫描大范围用量日志。
- Non-Goals:
  - 不自动猜测或填充订阅账号的购买价格。
  - 不变更用户侧套餐和余额扣费规则。

## Decisions

- Decision: 在服务层使用单一映射：`oauth`、`setup-token` → `subscription`；`apikey` → `metered`。
  - Rationale: 认证方式由账号接入方式确定，比人工配置稳定且可审计。
- Decision: 旧成本配置只提供订阅参数（周期费用、周期天数、币种等）；其 `cost_type` 字段不再覆盖映射结果。
  - Rationale: 兼容既有数据，同时避免一次迁移把历史成本配置不可逆地改坏。
- Decision: API Key 账号成本使用 `usage_logs.account_rate_multiplier` 的历史快照乘以原始账号侧模型成本；用户收入使用 `usage_logs.actual_cost`。
  - Rationale: `account_rate_multiplier` 表示与上游谈定的采购折扣，分组/用户倍率表示销售价格，二者必须独立保留才能正确计算毛利。
- Decision: 如果管理员确认某个账号的历史采购折扣曾错误记录，则使用一次范围明确、可审计的数据修正更新该账号对应历史快照；不以当前账号倍率静默重写其他账号历史。
  - Rationale: 采购折扣可能随账号或合同变化，历史记录需要逐账号确认。
- Decision: 新建不可变的 `account_subscription_cycles` 账本。每条记录包含 `account_id`、`starts_at`、`period_fee`、`period_days`、`currency`、`notes`，结束日由起始日加周期天数计算。
  - Rationale: 同一账号可在 7 月充值、8 月停用、9 月再次充值；单一配置字段无法表达这些不连续的成本归属。
- Decision: Grok 的自然月额度重置是容量事件，不是付款或续费事件。若账号于 7 月 17 日付款、8 月 17 日到期，即使 8 月 1 日额度重置，账本仍只有 `[7/17, 8/17)` 这一条采购周期，不在 8 月 1 日新增成本或拆分周期。
  - Rationale: 一笔 Grok 订阅可能跨越两个月度额度窗口，额度刷新提高同一采购成本可承载的销量，但不代表发生第二次付款。其利润逻辑与 OpenAI 订阅一致。
- Decision: Grok quota 的 `billing_period_end` 只用于容量展示与供给预测，绝不作为财务“一键推算日期”的来源；财务推算仍只使用明确的订阅到期日，OAuth token 到期日只能生成风险草稿。
  - Rationale: 月度额度重置日通常与实际付款周年日不同，把它作为账本边界会错误切断真实付款周期。
- Decision: 期间利润只摊销查询区间与账本周期的交集。没有周期记录的停用空档成本为 0；后续充值不会反向填补空档。
  - Rationale: 费用只在实际付款覆盖的有效期内发生。
- Decision: `credentials.subscription_expires_at` 仅作为旧数据的只读回退提示，不自动创建周期账本；`credentials.expires_at` 永不参与。
  - Rationale: 管理员必须确认实际付款日期，避免 Grok OAuth 刷新令牌造成伪周期。
- Decision: 前端提供“一键推算日期”辅助操作。它优先使用 `subscription_expires_at - period_days`；若仅存在 `expires_at`，允许将 `expires_at - period_days` 填入未确认草稿，并展示来源和风险提示。草稿必须由管理员确认保存为周期账本记录后才参与计算。
  - Rationale: 保留快速回忆充值日期的便利性，同时不把易变的 OAuth token 时间当作财务事实。
- Decision: 不再允许人工填写窗口基准收入。系统从该账号的历史 `actual_cost` 中计算任意连续 5 小时的最高已实现收入，作为窗口变现效率的基准；历史样本不足时不展示该效率。
  - Rationale: 管理员通常拿不到官方窗口价值，人工输入会制造伪精确数据；该指标仅用于运营观察，不应影响利润账本。
- Decision: UI 不再展示成本类型二选一。OAuth / Setup Token 显示周期费用表单；API Key 显示只读说明。
  - Rationale: 从界面源头消除无效选择。
- Decision: 新增 overview 聚合接口，一次返回全局汇总、每日趋势和账号明细；现有 summary/trend 接口保留并复用同一批量计算组件。
  - Rationale: 当前利润页并行请求 summary 与 trend，会重复加载账号、周期并扫描同一时间范围的 `usage_logs`；聚合接口可以消除重复 HTTP 和重复扫描，同时保持旧调用方兼容。
- Decision: 仓储层提供按账号、按日的基础聚合，并在服务层从同一份结果派生账号汇总和每日趋势。
  - Rationale: 收入与按量成本的原始口径相同，按 `(account_id, day)` 聚合一次即可同时满足两个视图，无需分别扫描日志表。
- Decision: 订阅周期、窗口基准和当前周期收入使用批量仓储方法，不在账号循环中访问数据库；利润 overview 只加载区间财务计算需要的配置和周期，不加载账号抽屉专用的窗口基准与当前周期收入。
  - Rationale: 当前 summary 最坏执行 `3 + 2S + A` 次数据库查询，trend 执行 `2 + S` 次查询，其中 `S` 为订阅账号数、`A` 为当前有效周期数；批量读取后查询复杂度应为 O(1)，而 overview 无需为未展示字段承担两次额外聚合。
- Decision: 利润 overview 使用 5 分钟进程内快照缓存，复用现有 `snapshotCache` 的 TTL、ETag 和 singleflight 语义，缓存键包含规范化的开始时间、结束时间和时区。
  - Rationale: 利润全局页是经营分析视图，不需要逐请求实时性；项目已有同类快照基础设施，复用它比引入新的 Redis 数据模型更小、更一致。5 分钟能显著降低重复日志扫描，同时把最大陈旧时间控制在可解释范围内。
- Decision: 成本配置、批量成本配置、订阅周期新增或删除成功后清空利润 overview 快照；用量新增依靠 5 分钟 TTL 自然刷新。手动刷新请求绕过旧快照并原子替换当前键的值。
  - Rationale: 财务口径变更必须立即可见；持续写入的用量日志若逐条触发失效会让缓存失去意义，因此使用短 TTL。管理员排查数据时仍可显式获得最新快照。
- Decision: overview 响应包含 `generated_at`，响应头返回缓存命中状态和 ETag；前端展示快照时效并提供刷新操作。
  - Rationale: 有界陈旧必须对管理员可见，不能把缓存数据伪装成实时数据。
- Decision: 利润缓存最多保留有限数量的日期范围键，并在写入时清理过期项，避免任意日期查询导致进程内存无界增长。
  - Rationale: 日期范围是用户输入，缓存必须有容量边界。
- Decision: 批量订阅配置使用单次批量 Upsert，并在同一事务语义下返回实际更新账号。
  - Rationale: 当前逐账号写入产生 N 次往返且中途失败会形成部分完成状态。

## Risks / Trade-offs

- 旧的 API Key 订阅配置会停止按周期摊销，利润会按真实使用成本重算。
  - Mitigation: 发布前列出受影响账号并在界面标示“由认证类型自动判定”。
- 历史账号成本倍率可能曾按默认 1.0 记录，导致历史利润被高估成本。
  - Mitigation: 展示“上游采购折扣”并仅在管理员明确确认后做逐账号历史修正。
- 历史收入不足时自动窗口基准可能没有统计意义。
  - Mitigation: 不展示窗口效率，不填充 0 或要求人工补值。
- OAuth / Setup Token 未配置周期费用时，成本仍为 0。
  - Mitigation: 保留未配置提示与批量补全入口。
- overview 响应体会同时包含账号和每日数据，可能比单独 trend 响应更大。
  - Mitigation: 仅利润全局页使用 overview；账号抽屉继续使用账号级 summary，旧接口保持可用。
- 进程内快照在多实例之间不共享，实例切换或重启后会出现冷缓存，各实例也可能在 TTL 内短暂看到不同快照。
  - Mitigation: 当前目标是消除同一实例上的重复实时扫描；财务配置写入主动失效当前实例，持续用量的最大差异限制为 5 分钟。若未来改为多活实例，再将同一缓存合同迁移到 Redis。
- 单次按账号、按日聚合比单维度聚合返回更多中间行。
  - Mitigation: 服务端流式扫描并立即聚合，限制允许的日期范围；使用基准测试比较总 SQL 时间、返回行数与内存分配。
- 缺少历史充值日期的旧订阅账号无法准确归属完整周期。
  - Mitigation: 明确标记并要求管理员补录周期记录；不从 OAuth token 过期时间猜测。

## Migration Plan

1. 创建 `account_subscription_cycles` 数据库表并发布认证类型映射、周期账本 API 与前端周期管理界面。
2. 在生产环境只读列出认证类型与旧配置不一致、以及缺少周期账本的订阅账号，供管理员补录。
3. 不修改 `usage_logs`；现有 `account_cost_configs` 只保留为旧数据兼容，确认无误后可在独立维护中完成迁移。
4. 性能实现先以查询预算测试验证查询次数与账号数量无关，再用生产近似数据执行 EXPLAIN ANALYZE；若无新增索引需求则保持数据库结构不变。
5. 发布利润快照缓存与轻量 overview；验证首次回源、缓存命中、手动刷新和成本口径写入后的失效行为。

## Performance Validation

- Before: 利润页发起 2 个 HTTP 请求并分别扫描同一范围的 `usage_logs`；summary 最坏为 `3 + 2S + A` 次数据库查询，trend 为 `2 + S` 次，因此整页最坏为 `5 + 3S + A` 次，其中 `S` 是订阅账号数、`A` 是当前有效周期数。
- After: 利润页发起 1 个 overview 请求并对 `usage_logs` 做 1 次按账号、按日聚合；overview 只批量加载账号、成本配置和周期，首次回源最多 4 次数据库查询且与账号数量无关；相同日期范围和时区在 5 分钟内命中快照时执行 0 次利润数据库查询。
- Regression proof: 服务测试以 100 个账号验证 overview 只调用 daily/config/cycle 各 1 次且不调用 best-window/current-window；处理器测试验证缓存命中、并发合并、手动绕过、写入失效、ETag 和容量上限；前端测试验证只调用 1 次 overview 并显示快照时间；仓储测试验证批量配置使用单个 JSONB 参数和单条 SQL。
- Observed baseline: 优化前用户提供的生产近似页面网络记录中，overview 冷查询约为 `4.84s`。
- Implemented hot path: 处理器回归测试验证首次请求为 `miss` 且执行 1 次 loader，第二次同键请求为 `hit` 且 loader 计数保持为 1，即热路径不进入利润仓储。真实生产冷/热耗时在部署后验证。

## Open Questions

- 当前规则将 `setup-token` 与 OAuth 同归为订阅制；如未来存在按量的 Setup Token 上游，需要增加显式例外机制。
- 周期账本是否允许编辑已生效的历史周期。默认仅允许删除明显错误的未结算记录，避免财务口径被静默改写。
