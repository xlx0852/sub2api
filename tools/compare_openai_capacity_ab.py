#!/usr/bin/env python3
"""Compare OpenAI capacity vs ok dump samples written by the gateway."""

from __future__ import annotations

import argparse
import json
import sys
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any


INTERESTING_HEADER_KEYS = (
    "x-request-id",
    "cf-ray",
    "openai-processing-ms",
    "x-codex-primary-used-percent",
    "x-codex-primary-window-minutes",
    "x-codex-primary-reset-after-seconds",
    "x-codex-secondary-used-percent",
    "x-codex-secondary-window-minutes",
    "x-codex-secondary-reset-after-seconds",
    "x-codex-primary-over-secondary-limit-percent",
    "retry-after",
    "originator",
)

INTERESTING_SNAPSHOT_KEYS = (
    "primary_used_percent",
    "secondary_used_percent",
    "used_5h_percent",
    "used_7d_percent",
    "window_5h_minutes",
    "window_7d_minutes",
    "primary_window_minutes",
    "secondary_window_minutes",
)

INTERESTING_QUOTA_KEYS = (
    "codex_5h_used_percent",
    "codex_7d_used_percent",
    "codex_primary_used_percent",
    "codex_secondary_used_percent",
    "codex_usage_updated_at",
)


def load_records(dump_dir: Path) -> list[dict[str, Any]]:
    records: list[dict[str, Any]] = []
    for path in sorted(dump_dir.glob("*.json")):
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            print(f"skip {path.name}: {exc}", file=sys.stderr)
            continue
        data["_file"] = path.name
        records.append(data)
    return records


def group(records: list[dict[str, Any]]) -> dict[str, list[dict[str, Any]]]:
    out: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for rec in records:
        out[str(rec.get("label") or "unknown")].append(rec)
    return out


def freq(values: list[Any], limit: int = 8) -> str:
    counts = Counter(str(v) if v not in (None, "") else "<empty>" for v in values)
    parts = [f"{k}×{n}" for k, n in counts.most_common(limit)]
    return ", ".join(parts) if parts else "<none>"


def collect(records: list[dict[str, Any]], getter) -> list[Any]:
    return [getter(rec) for rec in records]


def nested(rec: dict[str, Any], *keys: str) -> Any:
    cur: Any = rec
    for key in keys:
        if not isinstance(cur, dict):
            return None
        cur = cur.get(key)
    return cur


def print_section(title: str) -> None:
    print()
    print(title)
    print("-" * len(title))


def summarize_side(label: str, records: list[dict[str, Any]]) -> None:
    print_section(f"{label} n={len(records)}")
    if not records:
        print("  (no samples)")
        return
    print("  entry          ", freq(collect(records, lambda r: r.get("entry"))))
    print("  http_status    ", freq(collect(records, lambda r: r.get("http_status"))))
    print("  semantic_status", freq(collect(records, lambda r: r.get("semantic_status"))))
    print("  model          ", freq(collect(records, lambda r: r.get("model"))))
    print("  event_type     ", freq(collect(records, lambda r: r.get("event_type"))))
    print("  error_code     ", freq(collect(records, lambda r: r.get("error_code"))))
    print("  error_type     ", freq(collect(records, lambda r: r.get("error_type"))))
    print("  error_message  ", freq(collect(records, lambda r: (r.get("error_message") or "")[:80])))
    print("  capacity_shed  ", freq(collect(records, lambda r: r.get("capacity_shed"))))
    print("  originator     ", freq(collect(records, lambda r: r.get("originator"))))
    for key in INTERESTING_HEADER_KEYS:
        values = collect(records, lambda r, k=key: nested(r, "headers", k))
        if any(v not in (None, "") for v in values):
            print(f"  header.{key:42s} {freq(values)}")
    for key in INTERESTING_SNAPSHOT_KEYS:
        values = collect(records, lambda r, k=key: nested(r, "codex_snapshot", k))
        if any(v is not None for v in values):
            print(f"  snapshot.{key:40s} {freq(values)}")
    for key in INTERESTING_QUOTA_KEYS:
        values = collect(records, lambda r, k=key: nested(r, "account_quota", k))
        if any(v is not None for v in values):
            print(f"  quota.{key:44s} {freq(values)}")


def diff_presence(ok_recs: list[dict[str, Any]], cap_recs: list[dict[str, Any]]) -> None:
    print_section("presence diff (ok vs capacity)")
    if not ok_recs or not cap_recs:
        print("  need both ok and capacity samples")
        return

    def present_rate(records: list[dict[str, Any]], getter) -> float:
        n = sum(1 for rec in records if getter(rec) not in (None, "", {}, []))
        return n / len(records)

    rows = [
        ("x-codex-primary-used-percent", lambda r: nested(r, "headers", "x-codex-primary-used-percent")),
        ("x-codex-secondary-used-percent", lambda r: nested(r, "headers", "x-codex-secondary-used-percent")),
        ("cf-ray", lambda r: nested(r, "headers", "cf-ray")),
        ("retry-after", lambda r: nested(r, "headers", "retry-after")),
        ("codex_snapshot", lambda r: r.get("codex_snapshot")),
        ("account_quota.5h", lambda r: nested(r, "account_quota", "codex_5h_used_percent")),
        ("account_quota.7d", lambda r: nested(r, "account_quota", "codex_7d_used_percent")),
        ("error_code", lambda r: r.get("error_code")),
        ("error_message", lambda r: r.get("error_message")),
        ("body_excerpt", lambda r: r.get("body_excerpt")),
    ]
    print(f"  {'field':36s} {'ok%':>8s} {'cap%':>8s} note")
    for name, getter in rows:
        ok_p = present_rate(ok_recs, getter)
        cap_p = present_rate(cap_recs, getter)
        note = ""
        if ok_p >= 0.5 and cap_p < 0.2:
            note = "missing on capacity"
        elif cap_p >= 0.5 and ok_p < 0.2:
            note = "capacity-only"
        print(f"  {name:36s} {ok_p:8.0%} {cap_p:8.0%} {note}")


def first_values(records: list[dict[str, Any]], key: str) -> str:
    vals = sorted({str(r.get(key) if r.get(key) not in (None, "") else "<empty>") for r in records})
    return ",".join(vals) if vals else "<none>"


def unique_stream_events(records: list[dict[str, Any]]) -> str:
    seen: list[str] = []
    for rec in records:
        for event in rec.get("stream_events") or []:
            name = str(event)
            if name not in seen:
                seen.append(name)
    return ">".join(seen) if seen else "<none>"


def pick_contrast(grouped: dict[str, list[dict[str, Any]]]) -> tuple[str, list[dict[str, Any]]]:
    if grouped.get("ok"):
        return "ok", grouped["ok"]
    for label, recs in grouped.items():
        if label != "capacity" and recs:
            return label, recs
    return "", []


def diff_wire_fields(cap_recs: list[dict[str, Any]], other_recs: list[dict[str, Any]], other_label: str) -> list[str]:
    """Return named wire-level differences between capacity and a contrast set."""
    if not cap_recs or not other_recs:
        return []
    rows = [
        ("error_code", lambda recs: first_values(recs, "error_code")),
        ("error_type", lambda recs: first_values(recs, "error_type")),
        ("capacity_shed", lambda recs: first_values(recs, "capacity_shed")),
        ("semantic_status", lambda recs: first_values(recs, "semantic_status")),
        ("cli_mapped_message", lambda recs: first_values(recs, "cli_mapped_message")),
        ("stream_events", unique_stream_events),
        ("upstream_model", lambda recs: first_values(recs, "upstream_model")),
    ]
    diffs: list[str] = []
    print_section(f"wire diff (capacity vs {other_label})")
    print(f"  {'field':22s} {'capacity':40s} {other_label}")
    for name, getter in rows:
        left = getter(cap_recs)
        right = getter(other_recs)
        mark = "DIFF" if left != right else "same"
        print(f"  {name:22s} {left:40s} {right}  [{mark}]")
        if left != right:
            diffs.append(f"{name}: capacity={left} {other_label}={right}")
    if diffs:
        print("  named_differences:")
        for item in diffs:
            print(f"    - {item}")
    else:
        print("  named_differences: none")
    return diffs


def quota_vs_capacity(cap_recs: list[dict[str, Any]]) -> None:
    print_section("capacity vs cached quota (is this rate-limit?)")
    if not cap_recs:
        print("  no capacity samples")
        return
    exhausted = 0
    headroom = 0
    unknown = 0
    for rec in cap_recs:
        quota = rec.get("account_quota") or {}
        snap = rec.get("codex_snapshot") or {}
        used = []
        for key in ("used_5h_percent", "used_7d_percent", "primary_used_percent", "secondary_used_percent"):
            v = snap.get(key)
            if isinstance(v, (int, float)):
                used.append(float(v))
        for key in ("codex_5h_used_percent", "codex_7d_used_percent", "codex_primary_used_percent", "codex_secondary_used_percent"):
            v = quota.get(key)
            try:
                if v is not None:
                    used.append(float(v))
            except (TypeError, ValueError):
                pass
        if not used:
            unknown += 1
            continue
        hottest = max(used)
        if hottest >= 99:
            exhausted += 1
        else:
            headroom += 1
        print(
            f"  {rec.get('_file')}: hottest_used={hottest:.1f}% "
            f"http={rec.get('http_status')} code={rec.get('error_code') or '-'} "
            f"msg={(rec.get('error_message') or '')[:60]}"
        )
    print(f"  summary: quota_exhausted={exhausted} quota_headroom={headroom} quota_unknown={unknown}")
    if headroom and not exhausted:
        print("  conclusion: capacity is independent of 5h/7d quota exhaustion in this sample set")
    elif exhausted and not headroom:
        print("  conclusion: capacity coincided with exhausted quota; cannot separate the two yet")
    elif exhausted and headroom:
        print("  conclusion: mixed; capacity can fire with remaining quota")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "dump_dir",
        nargs="?",
        default="backend/tmp/openai-capacity-ab",
        help="directory of capacity_*.json / ok_*.json dumps",
    )
    args = parser.parse_args()
    dump_dir = Path(args.dump_dir)
    if not dump_dir.is_dir():
        print(f"dump dir not found: {dump_dir}", file=sys.stderr)
        print("Enable gateway.openai_capacity_ab_dump_dir or SUB2API_OPENAI_CAPACITY_AB_DUMP, restart, then send A/B traffic.", file=sys.stderr)
        return 2

    records = load_records(dump_dir)
    grouped = group(records)
    print(f"dir={dump_dir.resolve()} files={len(records)} labels={ {k: len(v) for k, v in grouped.items()} }")
    if not records:
        print("no dump files yet. Restart the gateway with dump enabled and send:")
        print("  A) same account + model, successful short prompt")
        print("  B) same account + model until 'Selected model is at capacity' / server_is_overloaded")
        return 1

    summarize_side("ok", grouped.get("ok", []))
    summarize_side("capacity", grouped.get("capacity", []))
    extra = {k: v for k, v in grouped.items() if k not in {"ok", "capacity"}}
    if extra:
        for label, recs in extra.items():
            summarize_side(label, recs)
    contrast_label, contrast_recs = pick_contrast(grouped)
    if grouped.get("ok"):
        diff_presence(grouped["ok"], grouped.get("capacity", []))
    elif contrast_label:
        print_section("presence diff (ok vs capacity)")
        print(f"  no ok samples; contrasting capacity vs {contrast_label} (success unavailable)")
        diff_wire_fields(grouped.get("capacity", []), contrast_recs, contrast_label)
    else:
        diff_presence([], grouped.get("capacity", []))
    if contrast_label and grouped.get("ok"):
        diff_wire_fields(grouped.get("capacity", []), contrast_recs, contrast_label)
    quota_vs_capacity(grouped.get("capacity", []))
    if not grouped.get("capacity") or not contrast_label:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
