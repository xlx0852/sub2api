# Design: model registry in the database

## 1. Schema

### 1.1 `models` (1 row per canonical model)

```sql
CREATE TABLE models (
    id                 TEXT PRIMARY KEY,                     -- canonical id, e.g. glm-5.3-abliterated
    platform           VARCHAR(32) NOT NULL,                 -- openai|anthropic|gemini|antigravity|grok|kimi|deepseek
    display_name       TEXT NOT NULL DEFAULT '',
    family             TEXT NOT NULL DEFAULT '',
    owned_by           TEXT NOT NULL DEFAULT '',
    context_window     BIGINT,
    max_context_window BIGINT,
    media              JSONB NOT NULL DEFAULT '{}'::jsonb,  -- {"image":true,"video":false,"image_input":true}
    is_reasoning       BOOLEAN NOT NULL DEFAULT FALSE,
    enabled            BOOLEAN NOT NULL DEFAULT TRUE,
    retired            BOOLEAN NOT NULL DEFAULT FALSE,        -- 出现在 /v1/models 之外，但保留解析
    is_default_chat    BOOLEAN NOT NULL DEFAULT FALSE,        -- 每平台唯一（部分唯一索引）
    is_default_test    BOOLEAN NOT NULL DEFAULT FALSE,        -- 每平台唯一
    version            BIGINT NOT NULL DEFAULT 1,             -- 乐观锁
    notes              TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX models_one_default_chat_per_platform ON models (platform) WHERE is_default_chat;
CREATE UNIQUE INDEX models_one_default_test_per_platform ON models (platform) WHERE is_default_test;
```

`retired` mirrors the catalog's `retired_ids`：仍可解析（老请求不断），但不出现在模型列表里。

### 1.2 `model_aliases` (1 row per spelling)

```sql
CREATE TABLE model_aliases (
    alias    TEXT NOT NULL,
    platform VARCHAR(32) NOT NULL,
    model_id TEXT NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    kind     VARCHAR(16) NOT NULL DEFAULT 'alias',   -- alias | id_override | id_reverse | retired
    PRIMARY KEY (alias, platform)
);
```

合并来源：catalog 的 `aliases`、`id_overrides`、`id_reverse_overrides`、`retired_ids`，
以及历史上散落在代码里的拼音/短名（`kimi3.1`、`k3`、`deepseek-flash` …）。加载时展开成
两张内存 map（alias→canonical、canonical→upstream id），保持现有调用点 O(1)。

### 1.3 `model_prices` (1 row per model × group, plus one default row per model)

```sql
CREATE TABLE model_prices (
    id                     BIGSERIAL PRIMARY KEY,
    model_id               TEXT NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    group_id               BIGINT REFERENCES groups(id) ON DELETE CASCADE,  -- NULL = 平台默认价（兜底）
    billing_mode           VARCHAR(20) NOT NULL DEFAULT 'token',
    currency               VARCHAR(3) NOT NULL DEFAULT 'USD',               -- 仅展示，不参与扣费
    fx_mode                VARCHAR(16) NOT NULL DEFAULT 'manual',
    cost_id                BIGINT REFERENCES price_costs(id) ON DELETE SET NULL,
    fx_rate                NUMERIC(12,6),
    markup_pct             NUMERIC(6,3) NOT NULL DEFAULT 0,
    input_price            NUMERIC(20,12),
    output_price           NUMERIC(20,12),
    cache_write_price      NUMERIC(20,12),
    cache_read_price       NUMERIC(20,12),
    input_price_priority   NUMERIC(20,12),          -- 新增：Flex/Priority 档位
    output_price_priority  NUMERIC(20,12),
    image_output_price     NUMERIC(20,8),
    per_request_price      NUMERIC(20,10),
    context_tiers          JSONB NOT NULL DEFAULT '[]'::jsonb,
    image_tiers            JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled                BOOLEAN NOT NULL DEFAULT TRUE,
    version                BIGINT NOT NULL DEFAULT 1,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (model_id, group_id)
);
CREATE UNIQUE INDEX model_prices_one_default_per_model ON model_prices (model_id) WHERE group_id IS NULL;
```

规则：

- `group_id IS NULL` 就是"内置默认价"，取代 `catalog.json` 的 `fallback_pricing`；
- 删掉某组的行 = 回落默认价；删掉默认行 = 该模型 fail-closed，并在后台标红；
- 所有价格列可空，但**"行存在且四种 token 价全为空"是非法状态**，由保存时校验与
  一次数据巡检拦掉（历史上 11 行这种容器行导致了 0 元计费）。

## 2. Resolution

```
resolve_price(model, group):
  canonical = alias_table.resolve(model)            # 例外：无别名时等于自身
  row = model_prices(model=canonical, group_id=group, enabled)
       ?? model_prices(model=canonical, group_id IS NULL, enabled)
  if row == nil: return ErrModelPricingUnavailable  # fail-closed，语义与现在一致
  if row.fx_mode == 'fx_auto': row = derive(row, price_costs[row.cost_id], row.fx_rate, row.markup_pct)
  return row
```

`MODEL_PRICING_SOURCE=legacy|dual|model_prices`：

- `legacy`：完全走现有链路（channel 行 → 快照 → catalog fallback → 代码族匹配）；
- `dual`：算出两套结果，只记 `pricing_source.mismatch` 日志，扣费仍用 legacy；
- `model_prices`：只用新表。

`MODEL_REGISTRY_SOURCE=file|dual|db` 同理，作用于 `modelcatalog.Load()`：
`db` 时从三张表构建同一个 `Catalog` 结构；`dual` 时两者都构建并比对（模型集合、
别名、默认模型、状态），差异逐条打点。

## 3. Admin surface

**新增 `/admin/models`（模型）页**

- 列表：搜索、平台筛选；列 = id / 平台 / 显示名 / 上下文 / 默认价 / 已配分组价数量 / 状态；
  未配默认价的模型标红（`MISSING_DEFAULT_PRICE`）。
- 详情抽屉三个区块：
  1. **身份与能力**：display_name、family、context_window、media、is_reasoning、enabled/retired、默认 chat/test；
  2. **别名**：增删别名（platform + alias → 本模型），显示被哪些历史名命中；
  3. **各组价格**：一行"平台默认价"，其下列出每个绑定分组的价格行（售价、派生模式、成本基、汇率、加价、priority 价），可内联编辑；未绑定分组的分组显示为灰色"未配 → 走默认价"。
- 「从供应商登记」按钮：输入账号（或自动按平台选）后拉 `/v1/models`，把 id / context 长度 / 能力 / 价格写进注册表草稿，人工确认后入库——让新增模型不再依赖发版。

**`/admin/pricing`（定价/比价页）保留**，但降级为只读视图：跨模型价目矩阵、分叉检测、
按分组实付预览；编辑统一跳到模型页（避免两个编辑入口）。

## 4. Guards

- 保存价格行时校验 `model_id` 存在于 `models`，否则 400 `MODEL_NOT_REGISTERED`；
- 保存价格行时校验"四种 token 价至少一个非空"或 `per_request_price`/`image_tiers` 非空；
- 删除默认价行需二次确认，并在响应里报告受影响的绑定分组数；
- 别名冲突（同一 alias 指向不同 model）拒绝保存；
- `enabled=false` 的模型不进模型列表，但已有价格仍可解析（灰度下线）。

## 5. Migration and rollback

1. 迁移 215：建三张表。
2. 迁移 216（可选，双读通过后）：从 `catalog.json` 两份副本导入 models / aliases / 默认价；
   从 `channel_model_pricing`（现存 2 张表、18 个模型名）导入每组价格行，保留
   `fx_mode`、`cost_id`、`fx_rate`、`markup_pct`。导入在事务内完成，失败整体回滚。
3. 数据校验：导入后断言"每个已配置分组的实付价格"在切换前后一致（复用 0.1.392 那次
   `pricing-summary` 的逐模型 diff 方式）。
4. 灰度：`MODEL_REGISTRY_SOURCE=dual` + `MODEL_PRICING_SOURCE=dual` 跑 24h，差异为零后
   切 `db` / `model_prices`。
5. 回滚：两个开关切回 `file` / `legacy`；三张新表保留（不动老表），无需数据回退。

## 6. Rejected alternatives

- **价格表兼任注册表（单表 + JSON 列）**：可行但需要元数据按组复制、价格失去独立审计、
  JSON 内部无法加约束，且别名仍需第二张表；收益只是"少一张表"。
- **继续用 catalog 文件 + 只把价格搬进 DB**：能解决分组价，但"未登记也能上线"、
  "别名散落在代码"这两个根源仍在。
