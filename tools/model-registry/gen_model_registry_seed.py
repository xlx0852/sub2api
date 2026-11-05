#!/usr/bin/env python3
"""从 catalog.json + 线上售价行生成 model registry 的种子迁移。

用法：
  python3 tools/model-registry/gen_model_registry_seed.py > backend/migrations/216_seed_model_registry.sql

设计约束（见 openspec/changes/move-model-registry-to-db/design.md）：
  * catalog 的 models → models（含上下文/能力/默认 chat-test/upstream 映射）
  * catalog 的 aliases / id_overrides / retired_ids → model_aliases
  * catalog 的 fallback_pricing → model_prices(group_id IS NULL)
  * channel_model_pricing 里有价的行 → model_prices(group_id = 该表绑定的分组)
  * 全部 ON CONFLICT DO NOTHING，可重复执行
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
CATALOG = REPO / "backend/internal/pkg/modelcatalog/catalog.json"


def sql_str(v: str) -> str:
    return "'" + v.replace("'", "''") + "'"


def sql_num(v) -> str:
    if v is None:
        return "NULL"
    if isinstance(v, str):
        return sql_str(v)
    return repr(float(v)) if isinstance(v, float) else str(v)


def main() -> None:
    cat = json.loads(CATALOG.read_text())
    platforms = cat["platforms"]
    fallback = cat.get("fallback_pricing", {})

    models: dict[str, dict] = {}

    def add_model(model_id: str, **fields) -> None:
        cur = models.setdefault(model_id, {"platform": "", "media": "{}"})
        cur.update({k: v for k, v in fields.items() if v not in (None, "", False)})

    # 1) catalog 模型清单
    for platform, cfg in platforms.items():
        default_chat = (cfg.get("default_chat_model") or "").strip()
        default_test = (cfg.get("default_test_model") or "").strip()
        mapping = cfg.get("default_mapping") or {}
        for m in cfg.get("models") or []:
            mid = (m.get("id") or "").strip()
            if not mid:
                continue
            add_model(
                mid,
                platform=platform,
                display_name=m.get("display_name") or "",
                family=m.get("family") or "",
                owned_by=m.get("owned_by") or "",
                context_window=m.get("context_window"),
                max_context_window=m.get("max_context_window"),
                media=json.dumps(m.get("media") or {}, ensure_ascii=False, sort_keys=True),
                is_reasoning=bool(m.get("is_reasoning")),
                is_default_chat=(mid == default_chat),
                is_default_test=(mid == default_test),
                upstream_id=(mapping.get(mid) or ""),
            )

    # 2) 只有价、没有平台元数据的兼容 id（deepseek-flash / k3 / glm-5 …）
    for key in fallback:
        if key not in models:
            add_model(key, notes="price-only compat id")

    # 3) 线上售价行的模型名（这些 id 代码里没有，先按 openai 平台登记）
    live = [
        ("deepseek-v4-1-flash-1017-uncensored", "openai", 1048576),
        ("deepseek-v4.1-flash-1017-uncensored", "openai", 1048576),
        ("deepseek-v4.1-flash-abliterated", "openai", 1048576),
        ("deepseek-v4.1-flash-abliterated-free", "openai", 1048576),
        ("deepseek-v4.1-flash-fast", "openai", 1048576),
        ("glm-5.3-abliterated", "openai", 1048576),
        ("glm-5.3-flash-abliterated", "openai", 1048576),
        ("minimax-m3.1-flash", "openai", 1048576),
        ("hinoki-offensive", "openai", 1048576),
    ]
    for mid, platform, ctx in live:
        add_model(
            mid,
            platform=platform,
            display_name=mid,
            context_window=ctx,
            media=json.dumps({"image": True}, sort_keys=True),
            notes="imported from sell rows (not in catalog)",
        )

    out = sys.stdout
    w = out.write
    w("-- 216: 从 catalog.json + 线上售价行播种 model registry。\n")
    w("-- 由 tools/model-registry/gen_model_registry_seed.py 生成，请勿手改；catalog 变更后重新生成。\n")
    w("-- 全部 ON CONFLICT DO NOTHING，可重复执行；末尾带一致性断言。\n\n")

    w("-- models\n")
    for mid in sorted(models):
        f = models[mid]
        w(
            "INSERT INTO models (id, platform, display_name, family, owned_by, context_window, "
            "max_context_window, media, is_reasoning, enabled, retired, is_default_chat, "
            "is_default_test, upstream_id, notes) VALUES ("
            f"{sql_str(mid)}, {sql_str(f.get('platform', ''))}, {sql_str(f.get('display_name', ''))}, "
            f"{sql_str(f.get('family', ''))}, {sql_str(f.get('owned_by', ''))}, "
            f"{sql_num(f.get('context_window'))}, {sql_num(f.get('max_context_window'))}, "
            f"{sql_str(f.get('media', '{}'))}::jsonb, {sql_num(bool(f.get('is_reasoning')))}, "
            f"TRUE, FALSE, {sql_num(bool(f.get('is_default_chat')))}, "
            f"{sql_num(bool(f.get('is_default_test')))}, {sql_str(f.get('upstream_id', ''))}, "
            f"{sql_str(f.get('notes', ''))}) ON CONFLICT (id) DO NOTHING;\n"
        )

    w("\n-- 别名：aliases / id_overrides / id_reverse_overrides / retired_ids\n")
    seen_alias: set[tuple[str, str]] = set()

    def emit_alias(alias: str, platform: str, model_id: str, kind: str) -> None:
        alias, platform = alias.strip(), platform.strip()
        if not alias or model_id not in models or (alias, platform) in seen_alias:
            return
        seen_alias.add((alias, platform))
        w(
            "INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ("
            f"{sql_str(alias)}, {sql_str(platform)}, {sql_str(model_id)}, {sql_str(kind)}) "
            "ON CONFLICT (alias, platform) DO NOTHING;\n"
        )

    for platform, cfg in platforms.items():
        for alias, target in (cfg.get("aliases") or {}).items():
            emit_alias(alias, platform, target, "alias")
        for src, dst in (cfg.get("id_overrides") or {}).items():
            emit_alias(src, platform, dst, "id_override")
        for src, dst in (cfg.get("id_reverse_overrides") or {}).items():
            emit_alias(dst, platform, src, "id_reverse")
        for rid in cfg.get("retired_ids") or []:
            emit_alias(rid, platform, rid, "retired")

    w("\n-- fallback_pricing 里的 alias_of → model_aliases(kind=alias)，价格按目标模型解析\n")
    for alias_key in sorted(fallback):
        entry = fallback[alias_key] or {}
        target = (entry.get("alias_of") or "").strip()
        if not target:
            continue
        platform = (models.get(target) or {}).get("platform", "")
        w(
            "INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ("
            f"{sql_str(alias_key)}, {sql_str(platform)}, {sql_str(target)}, 'alias') "
            "ON CONFLICT (alias, platform) DO NOTHING;\n"
        )

    w("\n-- 平台默认价（group_id IS NULL），来自 catalog.fallback_pricing\n")
    for mid in sorted(fallback):
        e = fallback[mid] or {}
        if e.get("alias_of"):
            continue
        w(
            "INSERT INTO model_prices (model_id, group_id, billing_mode, currency, fx_mode, "
            "input_price, output_price, cache_write_price, cache_read_price, "
            "input_price_priority, output_price_priority, image_output_price) VALUES ("
            f"{sql_str(mid)}, NULL, 'token', {sql_str((e.get('currency') or 'USD'))}, 'manual', "
            f"{sql_num(e.get('input_cost_per_token'))}, {sql_num(e.get('output_cost_per_token'))}, "
            f"{sql_num(e.get('cache_creation_input_token_cost'))}, {sql_num(e.get('cache_read_input_token_cost'))}, "
            f"{sql_num(e.get('input_cost_per_token_priority'))}, {sql_num(e.get('output_cost_per_token_priority'))}, "
            f"{sql_num(e.get('output_cost_per_image_token'))}) "
            "ON CONFLICT (model_id) WHERE group_id IS NULL DO NOTHING;\n"
        )

    w("\n-- 各组售价：把 channel_model_pricing 里有价的行搬进 model_prices\n")
    w("-- 通过 groups.sell_price_policy_id 反查该行属于哪个分组；纯模型清单行（无价）不搬。\n")
    w(
        """INSERT INTO model_prices (model_id, group_id, billing_mode, currency, fx_mode,
                           cost_id, fx_rate, markup_pct,
                           input_price, output_price, cache_write_price, cache_read_price,
                           image_output_price, per_request_price)
SELECT m.model_name, g.id, p.billing_mode, p.currency, p.fx_mode,
       p.cost_id, p.fx_rate, p.markup_pct,
       p.input_price, p.output_price, p.cache_write_price, p.cache_read_price,
       p.image_output_price, p.per_request_price
FROM channel_model_pricing p
JOIN groups g ON g.sell_price_policy_id = p.channel_id AND g.deleted_at IS NULL
CROSS JOIN LATERAL jsonb_array_elements_text(p.models) AS m(model_name)
WHERE p.input_price IS NOT NULL
   OR p.output_price IS NOT NULL
   OR p.cache_read_price IS NOT NULL
   OR p.cache_write_price IS NOT NULL
   OR p.per_request_price IS NOT NULL
ON CONFLICT (model_id, group_id) WHERE group_id IS NOT NULL DO NOTHING;
"""
    )

    w("\n-- 一致性断言：搬过来的每组价格必须与原售价行逐项相等（每 token 1e-12）。\n")
    w(
        """DO $$
DECLARE
    mismatch int;
BEGIN
    SELECT count(*) INTO mismatch
    FROM channel_model_pricing p
    JOIN groups g ON g.sell_price_policy_id = p.channel_id AND g.deleted_at IS NULL
    CROSS JOIN LATERAL jsonb_array_elements_text(p.models) AS m(model_name)
    LEFT JOIN model_prices mp ON mp.model_id = m.model_name AND mp.group_id = g.id
    WHERE COALESCE(p.input_price, 0) <> COALESCE(mp.input_price, 0)
       OR COALESCE(p.output_price, 0) <> COALESCE(mp.output_price, 0)
       OR COALESCE(p.cache_read_price, 0) <> COALESCE(mp.cache_read_price, 0)
       OR COALESCE(p.cache_write_price, 0) <> COALESCE(mp.cache_write_price, 0);
    IF mismatch > 0 THEN
        RAISE EXCEPTION 'model_prices import mismatch on % rows — aborting', mismatch;
    END IF;

    -- 行存在但四种 token 价全空是非法状态（历史上正是它导致 0 元计费）
    SELECT count(*) INTO mismatch
    FROM model_prices
    WHERE input_price IS NULL AND output_price IS NULL
      AND cache_read_price IS NULL AND cache_write_price IS NULL
      AND per_request_price IS NULL AND image_tiers = '[]'::jsonb;
    IF mismatch > 0 THEN
        RAISE EXCEPTION '% model_prices rows have no price at all — aborting', mismatch;
    END IF;
END $$;
"""
    )


if __name__ == "__main__":
    main()
