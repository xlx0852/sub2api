# Design: GPT-5.5 family official pricing

## D1. 官方价的核实来源与方法

**结论**：`gpt-5.5` 短上下文（<272K）为 $5.00 in / $0.50 cached / `-` cache writes / $30.00 out；
长上下文为 $10.00 / $1.00 / `-` / $45.00；Fast 档为 $12.50 / $1.25 / `-` / $75.00。
`gpt-5.5-pro` 短上下文为 $30.00 / `-` / `-` / $180.00，长上下文 $60.00 / `-` / `-` / $270.00，
**没有 Fast 档**。

**来源与抓取方法**（可复现）：

| 制品 | 来源 | sha256 |
| --- | --- | --- |
| `/tmp/oai-pricing-20260930.md`（22407 B） | OpenAI 官方定价页（docs/pricing），抓取于 2026-09-30 | `b271bf08244492bcaa2a169a86c1c486ea8f4b1713ee990ce05b49435cd8c510` |
| `/tmp/oai-models-gpt55.md`（4132 B） | OpenAI 官方模型页 `docs/models/gpt-5.5` | `fdb0fc8fe9ea7f276716c2a5b49b902e7e676ff262bb8adfdf2d5b8911bad3ba` |

官方页段落定位：`### Standard pricing data`（第 30 行起）中 `gpt-5.5` 为 `:41`、`gpt-5.5-pro` 为 `:42`；
`### Fast pricing data`（第 170 行起）中 `gpt-5.5` 为 `:181`，`gpt-5.5-pro` 无行；
长上下文阈值说明在 `:211`（Short ≤272K / Long >272K）；长上下文倍率原文在
`/tmp/oai-models-gpt55.md:34`：「For GPT-5.5, prompts with >272K input tokens are priced at 2x input and
1.5x output for the full session for standard, batch, and flex.」

**交叉核对**：仓库内 mirror（`backend/resources/model-pricing/model_prices_and_context_window.json`）
与独立 LiteLLM 派生文件（`/tmp/prod_pricing_litellm.json`，218 键）对 `gpt-5.5` / `gpt-5.5-pro` 的
数值与官方一致（除 pro 的 cached input，见 D4）；外仓 `/tmp/remote_pricing.json` 亦一致。

**方法约束**：不凭猜测填价；所有进入常量与 catalog 的数字都能在上述制品中定位到行号。官方表格中的
`-` 表示该档位不提供/不适用，一律**不填价格**，而不是照抄相邻数值。

## D2. 生产生效层判定（本变更 scope 的依据）

三方证据互相印证，结论是**静态常量层**为生产对 `gpt-5.5` 的实际决定层：

1. **运行期证据**：近 72h 日志 346 次 `gpt-5.5 -> gpt-5.4(static)`。
2. **代码路径**：`BillingService.GetModelPricing`（`billing_service.go:492`）先走
   `PricingService.GetModelPricing`（`pricing_service.go:828`）：数据层未命中 → `matchOpenAIModel`
   静态分支返回非 nil → catalog 兜底（`getFallbackPricing`，`billing_service.go:221`）不触达。
   即三层顺序为 **运行期数据层 → 静态常量层 → catalog 兜底层**。
3. **容器实况**：`/app/resources` 与 `/app/data/pricing` 的定价文件是运维自撰的 85 键 alias 文件
   （21101 B，sha256 `a1977c93…`），`gpt-5.5 = {alias_of: gpt-5.5}` 为自引用别名 → 数据层为空；
   `/app/release-model-catalog/catalog.json` 不存在 → 生效 catalog 是二进制内嵌的 v16（其中
   `gpt-5.5 = alias_of gpt-5.4`）。容器 env：`MODEL_CATALOG_DATA_DIR=/app/release-model-catalog`、
   `PRICING_DATA_DIR=/app/data/pricing`、`PRICING_REMOTE_ENABLED=false`、`MODEL_CATALOG_REMOTE_ENABLED=false`。

**推论**：计费正确性必须由**静态常量 + catalog 双层**对齐官方价；运维侧文件不是修复生效的必要条件，
本变更不触碰它们（见 D9）。同时因为 `remote` 关闭，`modelcatalog.Load` 的候选顺序（显式 File → cwd
`resources/...` → `backend/resources/...` → 内嵌）最终落到内嵌副本，所以 catalog 两份副本必须字节一致。

## D3. Fast 档定价方式：用声明的 `*_Priority` 单价，不改统一倍率

`serviceTierCostMultiplier`（`billing_service.go`）只保留 flex 的 0.5×，其源码注释已明确官方 Fast 表
**按模型分别定价**（GPT-6 与 5.6 为 2×、5.5 为 2.5×），且 cyber、`*-pro` 等模型**根本没有 Fast 档**，
因此统一倍率思路是错的。`usePriorityServiceTierPricing`（`billing_service.go:52`）要求模型声明了
`*_Priority` 单价且 `service_tier == priority` 时才生效，生效时 `tierMultiplier` 保持 1.0。

- `gpt-5.5` 声明三档 Fast 单价（1.25e-5 / 7.5e-5 / 1.25e-6），与官方 Fast 行逐项一致。
- `gpt-5.5-pro` **不声明** Fast 单价 → 即使收到 `service_tier=priority` 也只能按标准价结算。
  这是刻意的：官方无该档位，凭空加价即过度收费。
- `SupportsServiceTier` 字段不参与计费决策（全仓非测试代码仅在 `pricing_service.go` 的
  LiteLLM 映射处出现，`ModelPricing` 结构无此字段），设为 `true` 只为家族一致性；**真正的开关是
  `*_Priority` 单价是否非零**。

## D4. `gpt-5.5-pro` 的 cached input：取 3e-6（官方为 `-`）

官方表格 cached input 为 `-`（忠实读法即不提供缓存读取档，按 3e-5 输入价处理）；mirror 与
LiteLLM 派生文件均为 3e-6（输入价的 10%，LiteLLM 惯例）。

**决策**：catalog 与静态常量都取 **3e-6**。理由：与已发布 mirror 及前身模型（`gpt-5.4-pro` 亦为
3e-6）行为一致，避免层间不一致；该模型 30 天生产 0 行，不存在历史影响面。

**替代方案（记为 open question）**：改取 3e-5 以严格贴合官方 `-`。若采纳，需同时改 mirror，属独立
变更；届时 `gpt-5.5-pro` 的缓存读取会贵 10×。

## D5. `gpt-5.5-cyber`：本轮不加专属常量（待裁决）

事实：`gpt-5.5-cyber` 在 catalog `fallback_pricing`、运行期数据文件、内嵌 mirror 三处**都已是**
官网 Grouped 价（$12.50 / $1.25 / `-` / $75.00，`/tmp/oai-pricing-20260930.md:247`），
所以在数据层或 catalog 层它**不是缺口**。但静态兜底会经 `strings.Contains(model, "gpt-5.5")` 归一
（`normalizeKnownOpenAICodexModel`）后落入新的 `gpt-5.5` 分支，得到 5e-6 / 5e-7 / 3e-5，
即修正后仍**约 2.5× 偏低**（5e-6 vs 1.25e-5）。

**决策**：本轮**不改** cyber，保持「只修 `gpt-5.5` 与 `gpt-5.5-pro`」的最小侵入范围；上述数字如实
记录，是否补 cyber 常量由用户裁决。若补，改动量约 12 行：新增
`openAIGPT55CyberFallbackPricing`（input 1.25e-5、output 7.5e-5、cache_read 1.25e-6、无 cache_creation、
无 `*_Priority`、长上下文 272000/2.0/1.5）并把 `gpt-5.5-cyber` 前缀分支插到 `gpt-5.5` 分支之前。
注意这不是回归：现状 2.5e-6 → 修正后 5e-6，方向正确，只是仍未到位。

## D6. 新常量显式写入长上下文字段（自包含）

`applyModelSpecificPricingPolicy`（`billing_service.go:875`，注入段 870-935）对
`isOpenAIGPT54Model` 家族（`gpt-5.5` / `gpt-5.5-pro` 经归一后命中）在长上下文字段 <= 0 时会自动注入
`openAIGPT54LongContext*`（272000 / 2.0 / 1.5，`billing_service.go:43-46`），因此几何上天然对齐官方。
但新常量仍**显式写入**这三个字段（沿用 `openAIGPT56SolFallbackPricing` 直接引用
`openAIGPT54LongContext*` 的写法），使常量自包含、不依赖隐式注入路径。

## D7. 新条目不带 cache_creation 价（别名继承陷阱）

官方对 `gpt-5.5` / `gpt-5.5-pro` 的 cache writes 均为 `-`。catalog 现有 `gpt-5.4` 条目**带**
`cache_creation_input_token_cost: 2.5e-06`，因此旧写法 `gpt-5.5 → alias_of gpt-5.4` 一旦可达就会
继承出新的 cache write 收费。新条目必须**自包含**且不含该字段，也不设 `supports_cache_breakdown`。
生产数据可佐证：`gpt-5.5` 全部行的 `cache_creation_cost` 为 0。

## D8. 两份 catalog 与版本号 / 外仓发布时序

- `backend/resources/model-catalog/catalog.json` 是规范源，改后必须 `cp` 覆盖
  `backend/internal/pkg/modelcatalog/catalog.json`，两文件**字节一致**（由
  `skills/update-model-catalog/scripts/verify_catalog.py` 强制校验）。
- `version` 16 → 17，`updated_at` 取发布时刻 UTC RFC 3339；`versions/<version>/catalog.json` 与
  `catalog.sha256`（格式 `<64 hex>  catalog.json` + 换行）发布后**不可变**。
- 外仓 `xlx0852/model-catalog` 的发布必须等本次 bump 完成后一次进行，避免同一版本号对应两份不同字节。
  生产 `data_dir` 里残留的 v7 快照因单调版本规则（candidate `version < active.Version` 时忽略）不会
  覆盖 v17。

## D9. 不触碰运维侧文件

生产 `/opt/sub2api/data/pricing/model_pricing.json` 与
`/opt/sub2api/api/releases/<ver>/resources/model-pricing/...` 由运维维护（部署时替换了仓库 mirror），
其刷新属独立运维动作，需另行提出与批准。本变更只保证：**即使这些文件保持现状，计费也正确**（静态常量
层生效）；若运维日后刷成 mirror 值，静态常量与数据层互为双保险。

仓库内 mirror `backend/resources/model-pricing/model_prices_and_context_window.json` 本身已是正确官方价
（`gpt-5.5` 含 priority 三价、`gpt-5.5-pro` 为 3e-5 / 3e-6 / 1.8e-4），**本变更不动它**。

## D10. 与未归档变更 `update-openai-gpt61-sol-catalog` 的冲突修正

该变更（commit `9c79008af`）的 delta 中有两处断言建立在「`gpt-5.5` 走 `gpt-5.4` 别名」之上，本修正
使其失效，需随本变更一并更正（否则「别名链无自引用」要求与「`gpt-5.5` 经 `gpt-5.4` 解析」场景自相矛盾）：

1. 「No self-referential aliases」场景中把 `gpt-5.5 → gpt-5.4` 当作正当别名链的例子 → 换成仍成立的
   深链（如 `claude-opus-4.8 → claude-opus-4.5`）。
2. 「Restored GPT-5.4 fallback entries keep effective prices」场景中要求 `gpt-5.5-pro` 取 2.5 / 15
   → 删除该子句，只保留 `gpt-5.4-mini` 的断言，`gpt-5.5` 家族的价由本变更的 requirement 规定。

## 已知限制

- priority 与长上下文叠加按乘法复合（2.5× · 2× = 5×）：官方未发布该叠加档位，属未文档化边界，
  本变更不改变该语义。
- `openAIGPT54FallbackPricing` 缺 `*Priority` 字段（catalog `gpt-5.4` 有 5e-6 / 3e-5 / 5e-7）：
  同类潜在缺陷，**本轮不修**，保持聚焦。
- `gpt-5.5-cyber` 静态兜底仍偏低约 2.5×（见 D5）。
- 区域处理（data residency）端点 10% 加价（`gpt-5.5` / `gpt-5.5-pro` 均适用）在仓库内无字段表达，
  本轮不建模；外仓 `/tmp/remote_pricing.json` 带 `regional_processing_uplift_multiplier_eu/us: 1.1`。
- 官方 Flex 档 `gpt-5.5-pro` 长上下文列为 `-`（官方数据缺口），本变更不为 pro 设 flex 专属价；
  统一 flex 0.5× 语义未受影响。
- 官方 Fast 档 `gpt-5.5` 无长上下文行，本变更不为 Fast 档单独建模长上下文。

## Open questions

1. `gpt-5.5-cyber` 是否补静态常量（D5）？
2. `gpt-5.5-pro` cached input 是否改为严格贴合官方 `-`（3e-5，D4）？
3. 运维侧定价文件是否需要刷新以消除层间分歧（D9，需单独批准）？

## Risks

- **分支顺序**：`gpt-5.5-pro` 分支若排在 `gpt-5.5` 之后，pro 会被 5.5 价吞掉（偏低 6×）。测试必须
  断言 pro 用自己的常量。
- **别名继承**：若新条目误写成别名或误带 cache_creation，会在数据层可达时产生新的过度收费（D7）。
- **版本单调**：catalog 版本未递增会导致旧快照覆盖新值；发布后需确认生效版本。
- **部署门控**：修复只有随新二进制上线才生效（生产生效层是内嵌常量与内嵌 catalog）。
