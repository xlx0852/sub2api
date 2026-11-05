# Change: Price the GPT-5.5 family from OpenAI's official table

## Why

生产上 `gpt-5.5` 与 `gpt-5.5-pro` 按 GPT-5.4 的静态兜底常量结算，而官方价恰为其两倍；`gpt-5.5-pro`
偏差更大（约 12 倍），且 Fast/priority 档完全没有计价。以下先固定「生产实际决定层」，再给出官方价与
量化影响。

### 生产实际决定层是静态常量层（不是 catalog，也不是运行期数据文件）

- 代码路径：`BillingService.GetModelPricing`（`backend/internal/service/billing_service.go:492`）第 1 步调用
  `PricingService.GetModelPricing`（`backend/internal/service/pricing_service.go:828`）。该函数先查运行期数据层，
  未命中即进入 `matchOpenAIModel`，`gpt-5.5` 前缀分支（当前工作树 `pricing_service.go:1154-1158`）返回
  `openAIGPT54FallbackPricing` 且**非 nil**，因此第 2 步的 catalog 兜底（`getFallbackPricing`，
  `billing_service.go:221`）对 `gpt-5.5` 永不触达。有效优先级：运行期数据层 → 静态常量层 → catalog 兜底层。
- 运行期日志（生产容器近 72h）：`[Pricing] OpenAI fallback matched gpt-5.5 -> gpt-5.4(static)` 共 **346 次**
  （caller 行号 `service/pricing_service.go:1130` 属已部署二进制，对应当前工作树 `pricing_service.go:1154-1158`）；
  同窗口无 `gpt-5.5-pro` / `gpt-5.5-cyber` 命中。
- 容器实况：`/app/resources` 与 `/app/data/pricing` 的定价文件均为运维自撰的 85 键 alias 文件
  （21101 B，sha256 `a1977c93…`），其中 `gpt-5.5` 是自引用别名（`alias_of: gpt-5.5`）解析失败 →
  运行期数据层为空 → 落静态常量。`/app/release-model-catalog/catalog.json` 不存在 → 生效的是二进制内嵌
  catalog（v16），而其中 `gpt-5.5` 同样写作 `alias_of: gpt-5.4`。

结论：**计费正确性必须由代码（静态常量）+ catalog `fallback_pricing` 双层对齐官方价**；运维侧定价文件
属于独立运维范畴，不是本修复生效的必要条件。

### 官方价（核实来源与方法见 `design.md` D1）

| 模型 | 档位 | 输入 | 缓存读取 | 缓存写入 | 输出 |
| --- | --- | ---: | ---: | ---: | ---: |
| `gpt-5.5` | Standard (<272K) | $5.00 | $0.50 | `-` | $30.00 |
| `gpt-5.5` | Standard (>272K) | $10.00 | $1.00 | `-` | $45.00 |
| `gpt-5.5` | Fast (priority) | $12.50 | $1.25 | `-` | $75.00 |
| `gpt-5.5-pro` | Standard (<272K) | $30.00 | `-` | `-` | $180.00 |
| `gpt-5.5-pro` | Standard (>272K) | $60.00 | `-` | `-` | $270.00 |

（单位 USD / 1M tokens。来源：`/tmp/oai-pricing-20260930.md:41`、`:42`、`:181`、`:211`；
`gpt-5.5-pro` 在官方 Fast 表中没有行；长上下文规则见 `/tmp/oai-models-gpt55.md:34`。）

### 影响量化（生产 `usage_logs` 30 天，只读查询）

- `gpt-5.5`：15,339 请求；输入 256,022,134 tok、缓存读取 1,521,590,400 tok、缓存写入 0、输出 11,748,784 tok；
  当前计费 $1,980.04。官方价在输入 / 缓存读取 / 输出三项上**恰为当前的两倍** → 官方口径约 **$3,960**，
  即 30 天**少记约 $1,980（≈50%）**。
- `gpt-5.5-pro` / `gpt-5.5-cyber`：30 天 **0 行** → pro 修正属预防性（当前兜底 2.5e-6 / 1.5e-5 vs 官方
  3e-5 / 1.8e-4，**偏低约 12 倍**）。
- `service_tier` 分布（`gpt-5.5` 系）：`priority` 21 行 / $4.96，说明 Fast 有真实流量；静态常量没有
  `*_Priority` 单价 → `usePriorityServiceTierPricing`（`billing_service.go:52`）不成立 →
  `tierMultiplier` 保持 1.0 → **Fast 请求按标准价结算，欠收**（官方 Fast = 2.5×，不是统一倍率）。

## What Changes

1. **静态常量层**（`backend/internal/service/pricing_service.go`）：新增
   `openAIGPT55FallbackPricing` 与 `openAIGPT55ProFallbackPricing`，并在静态分支区插入两个前缀分支。
   `gpt-5.5-pro` 分支**必须排在 `gpt-5.5` 分支之前**，否则 pro 会被 5.5 分支吞掉。
   - `gpt-5.5`：input 5e-6、output 3e-5、cache_read 5e-7、priority input 1.25e-5、
     priority output 7.5e-5、priority cache_read 1.25e-6，长上下文 272000 / 2.0 / 1.5。
   - `gpt-5.5-pro`：input 3e-5、output 1.8e-4、cache_read 3e-6，长上下文 272000 / 2.0 / 1.5，
     **不设 `*_Priority`**（官方无 Fast 档）。
   - 两者**均不设 cache_creation 价**（官方为 `-`）。
2. **catalog 层**（`backend/resources/model-catalog/catalog.json` 与内嵌副本
   `backend/internal/pkg/modelcatalog/catalog.json`，`version` 16 → 17）：把 `gpt-5.5` 与 `gpt-5.5-pro`
   由别名改为自包含条目，值与上表一致，并同样**不带** `cache_creation_input_token_cost`
   （旧别名写法会从 `gpt-5.4` 继承 2.5e-6 的 cache write 价，一旦可达即成新过度收费）。
3. **修正未归档变更的冲突断言**：`update-openai-gpt61-sol-catalog` 的 delta 里有两处断言
   （「`gpt-5.5` 经 `gpt-5.4` 解析」、「`gpt-5.5-pro` 有效价 2.5/15」）与本修正直接冲突，随本变更一并更正。
4. **测试**：补服务层单测覆盖标准价、Fast 档、长上下文、日期后缀变体、无数据文件时的静态兜底；
   补 catalog 校验（别名链无自引用、`gpt-5.5`/`gpt-5.5-pro` 自包含）。

明确不改：`serviceTierCostMultiplier`（Fast 加价只由 `*_Priority` 单价表达，保持现状）、前端模型白名单、
`normalizeOpenAICodexCompactReasoningEffort`、镜像中既有 `gpt-6-sol` 条目、运维侧定价文件。

## Documented limitations and open questions

- `gpt-5.5-cyber`：catalog、运行期数据文件、内嵌 mirror 三处均已是官网 Grouped 价
  （1.25e-5 / 7.5e-5 / 1.25e-6），因此**不是缺口**；但它的静态兜底仍会落进 `gpt-5.5` 前缀分支，
  修正后仍有约 2.5× 偏低（5e-6 vs 1.25e-5）。本轮**不新增** cyber 专属常量（保持最小侵入），
  是否补需用户裁决，方案见 `design.md` D5。
- `gpt-5.5-pro` 的缓存读取价：官方表格为 `-`，mirror 为 3e-6（输入价的 10%，LiteLLM 惯例）。
  本轮取 3e-6（与已发布 mirror 及前身行为一致），分歧与替代方案记入 `design.md` D4。
- priority 与长上下文叠加按乘法复合（2.5× · 2× = 5×）：官方无此叠加档位，属未文档化边界，
  记入 `design.md`「已知限制」，本变更不改变该复合语义。
- `openAIGPT54FallbackPricing` 缺 `*Priority` 字段（catalog `gpt-5.4` 有 5e-6 / 3e-5 / 5e-7）：
  同类潜在缺陷，**本轮不修**，`design.md` 记为已知限制。
- 区域处理（data residency）端点对 `gpt-5.5` / `gpt-5.5-pro` 加价 10%，仓库内无对应字段表达，
  本轮不建模。

## Impact

- Affected specs: `model-catalog`（新增 GPT-5.5 家族官方价与 Fast 档要求）。
- Affected code: `backend/internal/service/pricing_service.go`、两份 catalog
  （`backend/resources/model-catalog/catalog.json`、`backend/internal/pkg/modelcatalog/catalog.json`）、
  新增/调整的服务层与 catalog 测试。
- 无 API 变更、无数据库迁移、无前端变更；`gpt-5.4` 及其它家族的价格不变。
- **部署门控**：生产生效层是二进制内嵌常量与内嵌 catalog，本修复必须随新二进制上线才生效；
  静态常量层一旦生效，运行期数据文件是否刷新对 `gpt-5.5` 计费不再敏感。
- 发布顺序提示：`xlx0852/model-catalog` 外仓发布须待本变更 bump `version` 后进行（一次性发布
  v17），避免同一版本号对应两份不同字节。

## Approval gate

本 proposal 为待批复状态：批复后再进入实现（Stage 2），实现完成后单独走部署与 Stage 3 归档。
