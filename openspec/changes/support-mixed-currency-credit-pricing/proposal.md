# Change: 支持混合币种模型定价与无币种额度记账

## Why

平台当前把模型基础价格、用户扣费和账号侧成本都以美元符号展示，但用户充值实际是 `1 元主体 = 1 平台额度`，Codex 等模型又沿用行业的美元官方价数值乘销售倍率。DeepSeek 中文官方价以人民币发布，如果继续按美元数值直接扣费，会把上游汇率风险隐藏在平台逻辑中并产生亏损。

平台不应自动估算上游汇率。模型定价币种只说明基础价格来源，销售与采购的最终数值分别由分组销售倍率和账号综合成本倍率控制。

## What Changes

- 为模型目录回退价格、动态定价和渠道模型定价增加 `USD` / `CNY` 币种元数据；未配置的旧价格兼容为 `USD`。
- 将 DeepSeek V4 Flash / Pro 及其官方别名改为 DeepSeek 中文官方人民币峰谷定价，保留北京时间峰谷时段规则。
- 模型币种不参与自动汇率换算；用户扣费仍为“基础价格数值 × 分组销售倍率”，以无币种的“额度”记录。
- 将账号 `rate_multiplier` 的管理端语义从“采购折扣”改为“综合成本倍率”，由管理员手动纳入汇率、渠道折扣、手续费和风险缓冲。
- API Key 账号成本仍为“账号侧基础成本数值 × 历史综合成本倍率快照”，不读取或自动更新外汇。
- 用量日志快照定价币种，保留现有 `total_cost`、`actual_cost`、`rate_multiplier` 和 `account_rate_multiplier` 数值；不重算历史日志。
- 管理端用量表中，模型基础单价和原始价必须按快照显示 `$` 或 `¥`，所有旧模型和未显式配置币种的价格默认补齐 `$`，不出现空单位；用户扣费和账号成本只显示数字，不显示货币符号，也不追加“额度”或“成本额度”文字。
- 现有用户余额、充值比例、API Key 配额、Codex 销售倍率和历史账单均不进行迁移或回算。

## Impact

- Affected specs:
  - `mixed-currency-credit-pricing` （新增）
  - `account-profit-accounting` （修改 API Key 成本倍率语义）
- Related active changes:
  - `normalize-balance-denomination-1-to-1`
  - `update-cost-type-by-auth-method`
  - `add-public-pricing-catalog`
  - `unify-usage-billing-core`
- Affected code:
  - 模型目录、LiteLLM 定价解析、渠道定价配置与定价解析器
  - DeepSeek 官方回退价格与峰谷计费
  - `usage_logs` 定价币种快照、结算 payload 与查询 DTO
  - 账号倍率管理文案、用量费用明细和公开/登录态价格展示
  - 计费、利润、API 合同和前端回归测试
- Database migration: 为渠道定价和用量日志增加具有兼容默认值的币种字段，不修改原有金额。
- No external dependency and no automatic FX service.
