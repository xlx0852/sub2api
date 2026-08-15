#!/usr/bin/env python3
"""Orchestrate isolated settlement durability fixtures and differential runs."""

from __future__ import annotations

import argparse
import concurrent.futures
import http.client
import json
import os
import pathlib
import shlex
import signal
import shutil
import ssl
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from typing import Any

from refactor_dual_version_reconcile import CASES as DIFF_CASES
from refactor_dual_version_reconcile import KEY_ENV as DIFF_KEY_ENV
from refactor_dual_version_reconcile import SHORT_PATH_CASES
from refactor_dual_version_reconcile import compare as compare_transport
from refactor_dual_version_reconcile import request_one
from usage_settlement_fake_upstream import SCENARIOS, make_server


ROOT = pathlib.Path(__file__).resolve().parents[1]
COMPOSE_FILE = ROOT / "tools" / "usage_settlement_crash_compose.yml"
DIFF_TOOL = ROOT / "tools" / "refactor_dual_version_reconcile.py"
COMPOSE_PROJECT = "sub2api-settlement-crash"
NATIVE_ROOT = pathlib.Path("/tmp") / f"sub2api-settlement-crash-{os.getuid()}"
NATIVE_MARKER = NATIVE_ROOT / "backend"
LOCAL_RUNTIME_ROOT = pathlib.Path.home() / ".codex" / "tmp" / "sub2api-settlement"
LOCAL_RUNTIME_MANIFEST = LOCAL_RUNTIME_ROOT / "manifest.json"
OLD_BASELINE_REF = "f94204d8a"
ADMIN_EMAIL = "settlement-admin@local.test"
ADMIN_PASSWORD = "Settlement-Test-Only-123!"
JWT_SECRET = "settlement-test-only-jwt-secret-32-bytes"

INFRA = {
    "old_postgres": "postgres://sub2api:settlement-test-only@127.0.0.1:55441/sub2api_old?sslmode=disable",
    "new_postgres": "postgres://sub2api:settlement-test-only@127.0.0.1:55442/sub2api_new?sslmode=disable",
    "old_redis": "redis://127.0.0.1:56381/0",
    "new_redis": "redis://127.0.0.1:56382/0",
    "fixture": "http://127.0.0.1:59000",
    "fixture_tls": "https://127.0.0.1:59001",
    "fixture_proxy": "http://127.0.0.1:59002",
}


SNAPSHOT_SQL = r"""
WITH marked AS (
    SELECT ul.*
    FROM usage_logs ul
    WHERE LEFT(COALESCE(ul.user_agent, ''), LENGTH({marker})) = {marker}
),
usage_rows AS (
    SELECT COALESCE(
        jsonb_agg(
            jsonb_strip_nulls(jsonb_build_object(
                'marker', m.user_agent,
                'model', m.model,
                'requested_model', to_jsonb(m)->>'requested_model',
                'upstream_model', to_jsonb(m)->>'upstream_model',
                'input_tokens', m.input_tokens,
                'output_tokens', m.output_tokens,
                'cache_creation_tokens', m.cache_creation_tokens,
                'cache_read_tokens', m.cache_read_tokens,
                'cache_creation_5m_tokens', to_jsonb(m)->>'cache_creation_5m_tokens',
                'cache_creation_1h_tokens', to_jsonb(m)->>'cache_creation_1h_tokens',
                'input_cost', m.input_cost::text,
                'output_cost', m.output_cost::text,
                'cache_creation_cost', m.cache_creation_cost::text,
                'cache_read_cost', m.cache_read_cost::text,
                'total_cost', m.total_cost::text,
                'actual_cost', m.actual_cost::text,
                'rate_multiplier', m.rate_multiplier::text,
                'account_stats_cost', to_jsonb(m)->>'account_stats_cost',
                'account_rate_multiplier', to_jsonb(m)->>'account_rate_multiplier',
                'billing_type', m.billing_type,
                'billing_mode', to_jsonb(m)->>'billing_mode',
                'stream', m.stream,
                'request_type', to_jsonb(m)->>'request_type',
                'openai_ws_mode', to_jsonb(m)->>'openai_ws_mode'
            )) ORDER BY m.user_agent, m.id
        ), '[]'::jsonb
    ) AS rows
    FROM marked m
),
user_state AS (
    SELECT COALESCE(jsonb_agg(
        jsonb_build_object(
            'email', u.email,
            'balance', u.balance::text,
            'frozen_balance', u.frozen_balance::text
        ) ORDER BY u.email
    ), '[]'::jsonb) AS rows
    FROM users u
    WHERE u.id IN (SELECT DISTINCT user_id FROM marked)
),
api_key_state AS (
    SELECT COALESCE(jsonb_agg(
        jsonb_build_object(
            'name', k.name,
            'quota', k.quota::text,
            'quota_used', k.quota_used::text,
            'usage_5h', to_jsonb(k)->>'usage_5h',
            'usage_1d', to_jsonb(k)->>'usage_1d',
            'usage_7d', to_jsonb(k)->>'usage_7d',
            'status', k.status
        ) ORDER BY k.name
    ), '[]'::jsonb) AS rows
    FROM api_keys k
    WHERE k.id IN (SELECT DISTINCT api_key_id FROM marked)
),
account_state AS (
    SELECT COALESCE(jsonb_agg(
        jsonb_strip_nulls(jsonb_build_object(
            'name', a.name,
            'platform', a.platform,
            'type', a.type,
            'quota_used', a.extra->>'quota_used',
            'quota_limit', a.extra->>'quota_limit',
            'quota_daily_used', a.extra->>'quota_daily_used',
            'quota_daily_limit', a.extra->>'quota_daily_limit',
            'quota_weekly_used', a.extra->>'quota_weekly_used',
            'quota_weekly_limit', a.extra->>'quota_weekly_limit'
        )) ORDER BY a.platform, a.name
    ), '[]'::jsonb) AS rows
    FROM accounts a
    WHERE a.id IN (SELECT DISTINCT account_id FROM marked)
),
profit_state AS (
    SELECT jsonb_build_object(
        'requests', COUNT(*),
        'revenue', COALESCE(SUM(m.actual_cost), 0)::text,
        'metered_cost', COALESCE(SUM(
            COALESCE(NULLIF(to_jsonb(m)->>'account_stats_cost', '')::numeric, m.total_cost)
            * COALESCE(NULLIF(to_jsonb(m)->>'account_rate_multiplier', '')::numeric, 1)
        ), 0)::text,
        'profit', (
            COALESCE(SUM(m.actual_cost), 0)
            - COALESCE(SUM(
                COALESCE(NULLIF(to_jsonb(m)->>'account_stats_cost', '')::numeric, m.total_cost)
                * COALESCE(NULLIF(to_jsonb(m)->>'account_rate_multiplier', '')::numeric, 1)
            ), 0)
        )::text
    ) AS value
    FROM marked m
)
SELECT jsonb_build_object(
    'marker_prefix', {marker},
    'usage_logs', usage_rows.rows,
    'users', user_state.rows,
    'api_keys', api_key_state.rows,
    'accounts', account_state.rows,
    'profit_contribution', profit_state.value
)::text
FROM usage_rows, user_state, api_key_state, account_state, profit_state;
"""


BILLING_STATE_SQL = r"""
SELECT jsonb_build_object(
    'user', COALESCE((
        SELECT jsonb_build_object('email', email, 'balance', balance::text, 'frozen_balance', frozen_balance::text)
        FROM users WHERE email = 'settlement-admin@local.test' LIMIT 1
    ), '{{}}'::jsonb),
    'api_key', COALESCE((
        SELECT jsonb_build_object('name', name, 'quota_used', quota_used::text, 'status', status)
        FROM api_keys WHERE name = {name} LIMIT 1
    ), '{{}}'::jsonb),
    'account', COALESCE((
        SELECT jsonb_strip_nulls(jsonb_build_object(
            'name', name,
            'quota_used', extra->>'quota_used',
            'quota_limit', extra->>'quota_limit'
        ))
        FROM accounts WHERE name = {name} LIMIT 1
    ), '{{}}'::jsonb),
    'usage_count', (SELECT COUNT(*) FROM usage_logs WHERE LEFT(COALESCE(user_agent, ''), LENGTH({marker})) = {marker}),
    'dedup_count', (SELECT COUNT(*) FROM usage_billing_dedup)
)::text;
"""


def sql_literal(value: str) -> str:
    """Return a PostgreSQL string literal without relying on shell interpolation."""
    if "\x00" in value:
        raise ValueError("PostgreSQL text cannot contain NUL")
    return "'" + value.replace("'", "''") + "'"


def parse_snapshot_output(raw: str) -> dict[str, Any]:
    lines = [line.strip() for line in raw.splitlines() if line.strip()]
    if len(lines) != 1:
        raise ValueError(f"expected one JSON row from snapshot query, got {len(lines)}")
    value = json.loads(lines[0])
    if not isinstance(value, dict):
        raise ValueError("snapshot query did not return a JSON object")
    return value


def query_snapshot(dsn: str, marker_prefix: str) -> dict[str, Any]:
    parsed_dsn = urllib.parse.urlparse(dsn)
    if parsed_dsn.scheme not in {"postgres", "postgresql"} or parsed_dsn.hostname not in {"127.0.0.1", "localhost", "::1"}:
        raise ValueError("snapshot DSN must be an explicit loopback PostgreSQL URI")
    if not marker_prefix.strip():
        raise ValueError("snapshot marker prefix must not be empty")
    psql = shutil.which("psql")
    if not psql:
        raise RuntimeError("psql is required for settlement snapshots")
    sql = SNAPSHOT_SQL.format(marker=sql_literal(marker_prefix))
    result = subprocess.run(
        [psql, "-X", "-A", "-t", "-q", "-v", "ON_ERROR_STOP=1", "--dbname", dsn, "--command", sql],
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or "snapshot query failed")
    return parse_snapshot_output(result.stdout)


def query_billing_state(dsn: str, platform: str, marker_prefix: str) -> dict[str, Any]:
    parsed_dsn = urllib.parse.urlparse(dsn)
    if parsed_dsn.scheme not in {"postgres", "postgresql"} or parsed_dsn.hostname not in {"127.0.0.1", "localhost", "::1"}:
        raise ValueError("billing state DSN must be an explicit loopback PostgreSQL URI")
    psql = shutil.which("psql")
    if not psql:
        raise RuntimeError("psql is required for settlement state capture")
    sql = BILLING_STATE_SQL.format(
        name=sql_literal("settlement-" + platform),
        marker=sql_literal(marker_prefix),
    )
    result = subprocess.run(
        [psql, "-X", "-A", "-t", "-q", "-v", "ON_ERROR_STOP=1", "--dbname", dsn, "--command", sql],
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or "billing state query failed")
    return parse_snapshot_output(result.stdout)


def execute_loopback_sql(dsn: str, sql: str) -> str:
    parsed_dsn = urllib.parse.urlparse(dsn)
    if parsed_dsn.scheme not in {"postgres", "postgresql"} or parsed_dsn.hostname not in {"127.0.0.1", "localhost", "::1"}:
        raise ValueError("SQL DSN must be an explicit loopback PostgreSQL URI")
    psql = shutil.which("psql")
    if not psql:
        raise RuntimeError("psql is required for settlement crash tests")
    result = subprocess.run(
        [psql, "-X", "-A", "-t", "-q", "-v", "ON_ERROR_STOP=1", "--dbname", dsn, "--command", sql],
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or "SQL command failed")
    return result.stdout.strip()


def query_settlement_attempt(dsn: str, marker: str) -> dict[str, Any]:
    sql = f"""
SELECT COALESCE((
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'id', id,
        'billing_identity', billing_identity,
        'provider_request_id', provider_request_id,
        'state', state,
        'retry_count', retry_count,
        'payload_hash', payload_hash,
        'lease_owner', lease_owner
    ))
    FROM usage_settlement_attempts
    WHERE payload->'usage_log'->>'user_agent' = {sql_literal(marker)}
    ORDER BY id DESC
    LIMIT 1
), '{{}}'::jsonb)::text;
"""
    return parse_snapshot_output(execute_loopback_sql(dsn, sql))


def query_settlement_attempt_after(dsn: str, minimum_id: int) -> dict[str, Any]:
    sql = f"""
SELECT COALESCE((
    SELECT jsonb_strip_nulls(jsonb_build_object(
        'id', id,
        'billing_identity', billing_identity,
        'provider_request_id', provider_request_id,
        'state', state,
        'retry_count', retry_count,
        'payload_hash', payload_hash,
        'last_error_code', last_error_code,
        'last_error_message', last_error_message,
        'reconciliation_deadline', reconciliation_deadline
    ))
    FROM usage_settlement_attempts
    WHERE id > {int(minimum_id)}
    ORDER BY id ASC
    LIMIT 1
), '{{}}'::jsonb)::text;
"""
    return parse_snapshot_output(execute_loopback_sql(dsn, sql))


def max_settlement_attempt_id(dsn: str) -> int:
    raw = execute_loopback_sql(dsn, "SELECT COALESCE(MAX(id), 0) FROM usage_settlement_attempts;")
    return int(raw or "0")


def capture_snapshot(dsn: str, marker_prefix: str, output: pathlib.Path | None) -> int:
    try:
        snapshot = query_snapshot(dsn, marker_prefix)
    except (ValueError, RuntimeError, json.JSONDecodeError) as error:
        print(str(error), file=sys.stderr)
        return 1
    if output:
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(snapshot, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(snapshot, ensure_ascii=False, indent=2, sort_keys=True))
    return 0


def disconnect_after_first_sse_event(base_url: str, case: dict[str, Any], run_id: str) -> dict[str, Any]:
    base_url = require_loopback_http(base_url)
    parsed = urllib.parse.urlparse(base_url)
    marker = f"sub2api-dual-diff/{run_id}/{case['name']}_disconnect"
    body = json.dumps(case["body"], separators=(",", ":")).encode()
    connection = http.client.HTTPConnection(parsed.hostname, parsed.port, timeout=20)
    started_at = time.monotonic()
    status = 0
    content_type = ""
    first_event_lines: list[str] = []
    try:
        connection.request(
            "POST",
            case["path"],
            body=body,
            headers={
                "Authorization": "Bearer " + os.environ[DIFF_KEY_ENV[str(case["platform"])]],
                "Content-Type": "application/json",
                "Accept": "text/event-stream",
                "User-Agent": marker,
            },
        )
        response = connection.getresponse()
        status = response.status
        content_type = response.getheader("Content-Type", "").split(";", 1)[0].strip().lower()
        saw_data = False
        for _ in range(200):
            raw = response.readline(65536)
            if not raw:
                break
            line = raw.decode("utf-8", errors="replace").rstrip("\r\n")
            first_event_lines.append(line[:500])
            if line.startswith("data:"):
                saw_data = True
            if saw_data and not line:
                break
        response.close()
    finally:
        connection.close()
    return {
        "marker": marker,
        "status": status,
        "content_type": content_type,
        "saw_first_event": any(line.startswith("data:") for line in first_event_lines),
        "first_event_line_count": len(first_event_lines),
        "closed_after_ms": round((time.monotonic() - started_at) * 1000, 3),
    }


def request_fixture_control(platform: str, method: str = "GET", body: dict[str, Any] | None = None) -> tuple[int, Any, str]:
    if platform != "grok":
        return request_json(INFRA["fixture"] + "/__control", method, body)
    data = None if body is None else json.dumps(body, separators=(",", ":")).encode()
    request = urllib.request.Request(
        INFRA["fixture_tls"] + "/__control",
        data=data,
        method=method,
        headers={"Content-Type": "application/json"},
    )
    context = ssl.create_default_context(cafile=str(NATIVE_ROOT / "tls" / "fixture-ca.crt"))
    with urllib.request.urlopen(request, timeout=5, context=context) as response:
        raw = response.read()
        return response.status, json.loads(raw), response.headers.get("Content-Type", "")


def wait_for_fixture_counter(platform: str, name: str, minimum: int, timeout_seconds: float = 15) -> dict[str, Any]:
    deadline = time.monotonic() + timeout_seconds
    state: dict[str, Any] = {}
    while time.monotonic() < deadline:
        _, payload, _ = request_fixture_control(platform)
        state = payload if isinstance(payload, dict) else {}
        if int(state.get(name, 0)) >= minimum:
            return state
        time.sleep(0.01)
    return state


def run_disconnect_side(base_url: str, dsn: str, case: dict[str, Any], run_id: str) -> dict[str, Any]:
    platform = str(case["platform"])
    request_fixture_control(platform, "POST", {"scenario": "hold_before_terminal"})
    client = disconnect_after_first_sse_event(base_url, case, run_id)
    before_release = wait_for_fixture_counter(platform, "partial_delivery_count", 1)
    # Let the gateway observe the downstream close before the deterministic
    # upstream publishes terminal usage.
    time.sleep(0.05)
    request_fixture_control(platform, "POST", {"action": "release_terminal"})
    after_release = wait_for_fixture_counter(platform, "terminal_delivery_count", 1)
    snapshot: dict[str, Any] = {}
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        snapshot = query_snapshot(dsn, str(client["marker"]))
        if len(snapshot.get("usage_logs", [])) >= 1:
            break
        time.sleep(0.05)
    return {
        "client": client,
        "fixture_before_release": before_release,
        "fixture_after_release": after_release,
        "snapshot": snapshot,
    }


def normalized_disconnect_snapshot(snapshot: dict[str, Any]) -> dict[str, Any]:
    value = json.loads(json.dumps(snapshot))
    value.pop("marker_prefix", None)
    for row in value.get("usage_logs", []):
        row.pop("marker", None)
    return value


def run_scheduler_fault_diff(
    old_url: str,
    new_url: str,
    old_dsn: str,
    new_dsn: str,
    old_secrets: pathlib.Path,
    new_secrets: pathlib.Path,
    scenario: str,
    output: pathlib.Path,
) -> int:
    fault_scenarios = {"http_401", "http_403", "http_429", "http_500"}
    try:
        old_url = require_loopback_http(old_url)
        new_url = require_loopback_http(new_url)
        if scenario not in fault_scenarios:
            raise ValueError(f"unsupported scheduler fault scenario: {scenario}")
        case = next(case for case in DIFF_CASES if case["name"] == "openai_json")
        run_id = f"scheduler-fault-{scenario}-{uuid.uuid4().hex[:12]}"

        def run_side(base_url: str, dsn: str, secrets: pathlib.Path) -> dict[str, Any]:
            load_fixture_secrets(secrets)
            request_fixture_control("openai", "POST", {"scenario": scenario})
            transport = request_one(base_url, case, run_id)
            _, fixture, _ = request_fixture_control("openai")
            snapshot: dict[str, Any] = {}
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                snapshot = query_snapshot(dsn, str(transport["marker"]))
                if len(snapshot.get("usage_logs", [])) >= 1:
                    break
                time.sleep(0.05)
            return {"transport": transport, "fixture": fixture, "snapshot": snapshot}

        old = run_side(old_url, old_dsn, old_secrets)
        new = run_side(new_url, new_dsn, new_secrets)
        comparison = compare_transport(old["transport"], new["transport"])

        def path_checks(side: dict[str, Any]) -> tuple[bool, bool]:
            paths = side.get("fixture", {}).get("request_paths", {})
            fault_hit = any("/fault/" in path and int(count) >= 1 for path, count in paths.items())
            fallback_hit = any("/openai/" in path and "/fault/" not in path and int(count) >= 1 for path, count in paths.items())
            return fault_hit, fallback_hit

        old_fault_hit, old_fallback_hit = path_checks(old)
        new_fault_hit, new_fallback_hit = path_checks(new)
        old_accounts = [row.get("name") for row in old["snapshot"].get("accounts", [])]
        new_accounts = [row.get("name") for row in new["snapshot"].get("accounts", [])]
        checks = {
            "old_transport_200": old["transport"]["status"] == 200,
            "new_transport_200": new["transport"]["status"] == 200,
            "transport_equal": all(value for key, value in comparison.items() if key.endswith("_equal")),
            "old_fault_path_hit": old_fault_hit,
            "new_fault_path_hit": new_fault_hit,
            "old_fallback_path_hit": old_fallback_hit,
            "new_fallback_path_hit": new_fallback_hit,
            "old_usage_exactly_once": len(old["snapshot"].get("usage_logs", [])) == 1,
            "new_usage_exactly_once": len(new["snapshot"].get("usage_logs", [])) == 1,
            "fallback_account_equal": old_accounts == new_accounts == ["settlement-openai-fallback"],
            "settlement_equal": normalized_disconnect_snapshot(old["snapshot"]) == normalized_disconnect_snapshot(new["snapshot"]),
        }
        report = {
            "run_id": run_id,
            "scenario": scenario,
            "old": old,
            "new": new,
            "comparison": comparison,
            "checks": checks,
            "passed": all(checks.values()),
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps({"scenario": scenario, "checks": checks, "passed": report["passed"]}, ensure_ascii=False, indent=2))
        return 0 if report["passed"] else 1
    except (OSError, ValueError, RuntimeError, KeyError, StopIteration, urllib.error.URLError) as error:
        print(f"scheduler fault diff failed: {error}", file=sys.stderr)
        return 1


def run_disconnect_diff(old_url: str, new_url: str, old_dsn: str, new_dsn: str, output: pathlib.Path) -> int:
    try:
        old_url = require_loopback_http(old_url)
        new_url = require_loopback_http(new_url)
        cases = [case for case in DIFF_CASES if str(case["name"]).endswith("_sse")]
        missing_keys = sorted({DIFF_KEY_ENV[str(case["platform"])] for case in cases} - os.environ.keys())
        if missing_keys:
            raise RuntimeError("missing API key environment variables: " + ", ".join(missing_keys))
        report: dict[str, Any] = {
            "run_id": "settlement-disconnect-" + uuid.uuid4().hex[:12],
            "old_url": old_url,
            "new_url": new_url,
            "cases": [],
        }
        for case in cases:
            case_run_id = f"{report['run_id']}-{case['platform']}"
            old = run_disconnect_side(old_url, old_dsn, case, case_run_id)
            new = run_disconnect_side(new_url, new_dsn, case, case_run_id)
            checks = {
                "old_client_received_partial": old["client"]["status"] == 200 and old["client"]["saw_first_event"],
                "new_client_received_partial": new["client"]["status"] == 200 and new["client"]["saw_first_event"],
                "old_terminal_drained": int(old["fixture_after_release"].get("terminal_delivery_count", 0)) == 1,
                "new_terminal_drained": int(new["fixture_after_release"].get("terminal_delivery_count", 0)) == 1,
                "old_usage_persisted": len(old["snapshot"].get("usage_logs", [])) == 1,
                "new_usage_persisted": len(new["snapshot"].get("usage_logs", [])) == 1,
                "snapshot_equal": normalized_disconnect_snapshot(old["snapshot"]) == normalized_disconnect_snapshot(new["snapshot"]),
            }
            report["cases"].append({
                "name": case["name"],
                "platform": case["platform"],
                "old": old,
                "new": new,
                "checks": checks,
                "passed": all(checks.values()),
            })
            print(f"{case['name']}_disconnect: {'passed' if all(checks.values()) else 'failed'}")
        report["summary"] = {
            "case_count": len(report["cases"]),
            "passed_count": sum(1 for case in report["cases"] if case["passed"]),
            "all_passed": all(case["passed"] for case in report["cases"]),
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps(report["summary"], ensure_ascii=False, indent=2))
        return 0 if report["summary"]["all_passed"] else 1
    except (OSError, ValueError, RuntimeError, KeyError, urllib.error.URLError, http.client.HTTPException) as error:
        print(f"disconnect diff failed: {error}", file=sys.stderr)
        return 1
    finally:
        for platform in ("openai", "grok"):
            try:
                request_fixture_control(platform, "POST", {"scenario": "normal"})
            except (OSError, ValueError, RuntimeError, urllib.error.URLError):
                pass


def run_short_path_diff(old_url: str, new_url: str, old_dsn: str, new_dsn: str, output: pathlib.Path) -> int:
    try:
        old_url = require_loopback_http(old_url)
        new_url = require_loopback_http(new_url)
        run_id = "settlement-short-path-" + uuid.uuid4().hex[:12]
        cases: list[dict[str, Any]] = []
        for case in SHORT_PATH_CASES:
            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as executor:
                old_future = executor.submit(request_one, old_url, case, run_id)
                new_future = executor.submit(request_one, new_url, case, run_id)
                old = old_future.result()
                new = new_future.result()
            comparison = compare_transport(old, new)
            passed = (
                old.get("status") == 200
                and new.get("status") == 200
                and all(value for name, value in comparison.items() if name.endswith("_equal"))
            )
            cases.append({
                "name": case["name"],
                "platform": case["platform"],
                "request": case["body"],
                "old": old,
                "new": new,
                "comparison": comparison,
                "passed": passed,
            })
            print(f"{case['name']}: {'passed' if passed else 'failed'}")
        marker_prefix = f"sub2api-dual-diff/{run_id}/"
        old_snapshot: dict[str, Any] = {}
        new_snapshot: dict[str, Any] = {}
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            old_snapshot = query_snapshot(old_dsn, marker_prefix)
            new_snapshot = query_snapshot(new_dsn, marker_prefix)
            if len(old_snapshot.get("usage_logs", [])) >= len(SHORT_PATH_CASES) and len(new_snapshot.get("usage_logs", [])) >= len(SHORT_PATH_CASES):
                break
            time.sleep(0.05)
        report = {
            "run_id": run_id,
            "old_url": old_url,
            "new_url": new_url,
            "cases": cases,
            "old_snapshot": old_snapshot,
            "new_snapshot": new_snapshot,
            "summary": {
                "case_count": len(cases),
                "transport_passed_count": sum(1 for case in cases if case["passed"]),
                "old_usage_count": len(old_snapshot.get("usage_logs", [])),
                "new_usage_count": len(new_snapshot.get("usage_logs", [])),
                "snapshot_equal": old_snapshot == new_snapshot,
            },
        }
        report["summary"]["all_passed"] = (
            report["summary"]["transport_passed_count"] == len(cases)
            and report["summary"]["old_usage_count"] == len(cases)
            and report["summary"]["new_usage_count"] == len(cases)
            and report["summary"]["snapshot_equal"]
        )
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps(report["summary"], ensure_ascii=False, indent=2))
        return 0 if report["summary"]["all_passed"] else 1
    except (OSError, ValueError, RuntimeError, KeyError, urllib.error.URLError) as error:
        print(f"short-path diff failed: {error}", file=sys.stderr)
        return 1


def run_single_baseline(base_url: str, dsn: str, output: pathlib.Path) -> int:
    try:
        base_url = require_loopback_http(base_url)
        run_id = "settlement-baseline-" + uuid.uuid4().hex[:12]
        results = [request_one(base_url, case, run_id) for case in DIFF_CASES]
        marker_prefix = f"sub2api-dual-diff/{run_id}/"
        snapshot: dict[str, Any] = {}
        successful = sum(1 for result in results if result.get("status") == 200)
        deadline = time.monotonic() + 15
        while True:
            snapshot = query_snapshot(dsn, marker_prefix)
            if len(snapshot.get("usage_logs", [])) >= successful or time.monotonic() >= deadline:
                break
            time.sleep(0.1)
        gemini_status, gemini_payload, gemini_type = request_json(base_url + "/v1beta/models")
        report = {
            "run_id": run_id,
            "base_url": base_url,
            "cases": [
                {
                    "name": case["name"],
                    "platform": case["platform"],
                    "result": result,
                }
                for case, result in zip(DIFF_CASES, results, strict=True)
            ],
            "gemini_retired_baseline": {
                "path": "/v1beta/models",
                "status": gemini_status,
                "content_type": gemini_type.split(";", 1)[0],
                "payload": gemini_payload,
            },
            "snapshot": snapshot,
            "summary": {
                "case_count": len(results),
                "http_200": successful,
                "usage_log_count": len(snapshot.get("usage_logs", [])),
                "all_active_cases_200": successful == len(results),
                "all_successes_persisted": len(snapshot.get("usage_logs", [])) == successful,
                "gemini_retired_explicit": gemini_status == 404,
            },
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps(report["summary"], ensure_ascii=False, indent=2))
        return 0 if all(report["summary"].values()) else 1
    except (ValueError, RuntimeError, KeyError, urllib.error.URLError) as error:
        print(f"baseline run failed: {error}", file=sys.stderr)
        return 1


def run_terminal_crash(base_url: str, dsn: str, case_name: str, container_name: str, output: pathlib.Path) -> int:
    try:
        base_url = require_loopback_http(base_url)
        if not container_name.startswith("sub2api-settlement-"):
            raise ValueError("refusing to kill a container outside the settlement test namespace")
        case = next((item for item in DIFF_CASES if item["name"] == case_name), None)
        if case is None:
            raise ValueError(f"unknown baseline case: {case_name}")
        run_id = "settlement-crash-" + uuid.uuid4().hex[:12]
        marker = f"sub2api-dual-diff/{run_id}/{case_name}"
        request_json(INFRA["fixture"] + "/__control", "POST", {"scenario": "hold_after_terminal"})
        result_holder: dict[str, Any] = {}

        def send_request() -> None:
            result_holder["result"] = request_one(base_url, case, run_id)

        client = threading.Thread(target=send_request)
        before = query_billing_state(dsn, str(case["platform"]), marker)
        client.start()
        terminal_count = 0
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            _, state, _ = request_json(INFRA["fixture"] + "/__control")
            terminal_count = int(state.get("terminal_delivery_count", 0)) if isinstance(state, dict) else 0
            if terminal_count > 0:
                break
            time.sleep(0.001)
        if terminal_count == 0:
            request_json(INFRA["fixture"] + "/__control", "POST", {"action": "release_terminal"})
            client.join(timeout=2)
            raise RuntimeError("terminal delivery barrier was not reached")
        killed = subprocess.run(
            ["docker", "kill", "--signal", "KILL", container_name],
            check=False,
            capture_output=True,
            text=True,
        )
        request_json(INFRA["fixture"] + "/__control", "POST", {"action": "release_terminal"})
        client.join(timeout=5)
        time.sleep(0.25)
        after = query_billing_state(dsn, str(case["platform"]), marker)
        report = {
            "run_id": run_id,
            "case": case_name,
            "platform": case["platform"],
            "marker": marker,
            "terminal_delivery_count": terminal_count,
            "kill_returncode": killed.returncode,
            "client_finished": not client.is_alive(),
            "client_result": result_holder.get("result"),
            "before": before,
            "after": after,
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps({
            "terminal_delivery_count": terminal_count,
            "kill_returncode": killed.returncode,
            "client_finished": not client.is_alive(),
            "usage_count_before": before.get("usage_count"),
            "usage_count_after": after.get("usage_count"),
            "balance_before": before.get("user", {}).get("balance"),
            "balance_after": after.get("user", {}).get("balance"),
            "dedup_before": before.get("dedup_count"),
            "dedup_after": after.get("dedup_count"),
        }, ensure_ascii=False, indent=2))
        return 0 if terminal_count == 1 and killed.returncode == 0 and not client.is_alive() else 1
    except (ValueError, RuntimeError, KeyError, urllib.error.URLError) as error:
        print(f"terminal crash run failed: {error}", file=sys.stderr)
        return 1


def verify_crash_restart(base_url: str, dsn: str, report_path: pathlib.Path) -> int:
    try:
        base_url = require_loopback_http(base_url)
        report = json.loads(report_path.read_text(encoding="utf-8"))
        if not isinstance(report, dict):
            raise ValueError("crash report must be a JSON object")
        deadline = time.monotonic() + 20
        health_status = 0
        while time.monotonic() < deadline:
            try:
                health_status, _, _ = request_json(base_url + "/health")
            except urllib.error.URLError:
                health_status = 0
            if health_status == 200:
                break
            time.sleep(0.1)
        if health_status != 200:
            raise RuntimeError("gateway did not become healthy after restart")
        time.sleep(1)
        after_restart = query_billing_state(dsn, str(report["platform"]), str(report["marker"]))
        report["after_restart"] = after_restart
        report["restart_observation"] = {
            "health_status": health_status,
            "usage_recovered": int(after_restart.get("usage_count", 0)) > int(report.get("after", {}).get("usage_count", 0)),
            "dedup_changed": int(after_restart.get("dedup_count", 0)) != int(report.get("after", {}).get("dedup_count", 0)),
            "balance_changed": after_restart.get("user", {}).get("balance") != report.get("after", {}).get("user", {}).get("balance"),
        }
        report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps(report["restart_observation"], ensure_ascii=False, indent=2))
        return 0
    except (OSError, ValueError, RuntimeError, KeyError, json.JSONDecodeError) as error:
        print(f"restart verification failed: {error}", file=sys.stderr)
        return 1


def compare_baseline_reports(old_path: pathlib.Path, new_path: pathlib.Path, output: pathlib.Path) -> int:
    try:
        old = json.loads(old_path.read_text(encoding="utf-8"))
        new = json.loads(new_path.read_text(encoding="utf-8"))
        old_cases = {item["name"]: item for item in old["cases"]}
        new_cases = {item["name"]: item for item in new["cases"]}
        names = sorted(set(old_cases) | set(new_cases))
        cases = []
        for name in names:
            if name not in old_cases or name not in new_cases:
                cases.append({"name": name, "present_both": False})
                continue
            comparison = compare_transport(old_cases[name]["result"], new_cases[name]["result"])
            cases.append({"name": name, "present_both": True, **comparison})

        def usage_by_suffix(report: dict[str, Any]) -> dict[str, dict[str, Any]]:
            result: dict[str, dict[str, Any]] = {}
            for row in report.get("snapshot", {}).get("usage_logs", []):
                normalized = dict(row)
                marker = str(normalized.pop("marker", ""))
                result[marker.rsplit("/", 1)[-1]] = normalized
            return result

        old_usage = usage_by_suffix(old)
        new_usage = usage_by_suffix(new)
        usage_names = sorted(set(old_usage) | set(new_usage))
        usage = [
            {
                "name": name,
                "present_both": name in old_usage and name in new_usage,
                "equal": old_usage.get(name) == new_usage.get(name),
                "old": old_usage.get(name),
                "new": new_usage.get(name),
            }
            for name in usage_names
        ]
        state_keys = ("users", "api_keys", "accounts", "profit_contribution")
        state_equal = {key: old.get("snapshot", {}).get(key) == new.get("snapshot", {}).get(key) for key in state_keys}
        protocol_equal = all(
            item.get("present_both")
            and item.get("status_equal")
            and item.get("content_type_equal")
            and item.get("format_equal")
            and item.get("schema_equal")
            and item.get("normalized_equal")
            and item.get("models_equal")
            and item.get("usage_equal")
            for item in cases
        )
        persisted_usage_equal = all(item["present_both"] and item["equal"] for item in usage)
        gemini_equal = old.get("gemini_retired_baseline", {}).get("status") == new.get("gemini_retired_baseline", {}).get("status") == 404
        report = {
            "old_report": str(old_path),
            "new_report": str(new_path),
            "cases": cases,
            "persisted_usage": usage,
            "state_equal": state_equal,
            "summary": {
                "protocol_equal": protocol_equal,
                "persisted_usage_equal": persisted_usage_equal,
                "state_equal": all(state_equal.values()),
                "gemini_retired_equal": gemini_equal,
            },
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps(report["summary"], ensure_ascii=False, indent=2))
        return 0 if all(report["summary"].values()) else 1
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as error:
        print(f"baseline comparison failed: {error}", file=sys.stderr)
        return 1


def request_json(url: str, method: str = "GET", body: dict[str, Any] | None = None, headers: dict[str, str] | None = None) -> tuple[int, Any, str]:
    data = None if body is None else json.dumps(body, separators=(",", ":")).encode()
    req = urllib.request.Request(url, data=data, method=method, headers={"Content-Type": "application/json", **(headers or {})})
    try:
        with urllib.request.urlopen(req, timeout=5) as response:
            status = response.status
            content_type = response.headers.get("Content-Type", "")
            raw = response.read()
    except urllib.error.HTTPError as error:
        status = error.code
        content_type = error.headers.get("Content-Type", "") if error.headers else ""
        raw = error.read()
        error.close()
    text = raw.decode("utf-8", errors="replace")
    if "application/json" in content_type:
        return status, json.loads(text), content_type
    return status, text, content_type


def require_loopback_http(base_url: str) -> str:
    parsed = urllib.parse.urlparse(base_url)
    if parsed.scheme != "http" or parsed.hostname not in {"127.0.0.1", "localhost", "::1"}:
        raise ValueError("gateway base URL must be an explicit loopback HTTP URL")
    return base_url.rstrip("/")


def response_data(status: int, payload: Any, operation: str) -> Any:
    if status < 200 or status >= 300:
        raise RuntimeError(f"{operation} failed with HTTP {status}: {payload}")
    if not isinstance(payload, dict):
        raise RuntimeError(f"{operation} returned a non-object response")
    if "data" in payload:
        return payload["data"]
    return payload


def seed_fixture_instance(base_url: str, admin_email: str, admin_password: str, secrets_output: pathlib.Path) -> int:
    try:
        base_url = require_loopback_http(base_url)
        status, payload, _ = request_json(
            base_url + "/api/v1/auth/login",
            "POST",
            {"email": admin_email, "password": admin_password},
        )
        login = response_data(status, payload, "admin login")
        if not isinstance(login, dict) or not isinstance(login.get("access_token"), str):
            raise RuntimeError("admin login response is missing access_token")
        token = login["access_token"]
        user = login.get("user") if isinstance(login.get("user"), dict) else {}
        user_id = int(user.get("id", 0))
        if user_id <= 0:
            raise RuntimeError("admin login response is missing user id")
        auth = {"Authorization": "Bearer " + token}

        status, payload, _ = request_json(base_url + "/api/v1/admin/compliance", headers=auth)
        compliance = response_data(status, payload, "read admin compliance status")
        if isinstance(compliance, dict) and not bool(compliance.get("acknowledged")):
            phrase = str(compliance.get("ack_phrase_en", "")).strip()
            if not phrase:
                raise RuntimeError("admin compliance response is missing English acknowledgement phrase")
            status, payload, _ = request_json(
                base_url + "/api/v1/admin/compliance/accept",
                "POST",
                {"phrase": phrase, "language": "en"},
                auth,
            )
            response_data(status, payload, "accept admin compliance")

        status, payload, _ = request_json(
            f"{base_url}/api/v1/admin/users/{user_id}/balance",
            "POST",
            {"balance": 100.0, "operation": "set", "notes": "isolated settlement fixture"},
            auth,
        )
        response_data(status, payload, "set fixture balance")

        fixture_host = os.environ.get("SUB2API_SETTLEMENT_FIXTURE_HOST", "127.0.0.1").strip()
        if fixture_host not in {"127.0.0.1", "localhost", "host.docker.internal"}:
            raise RuntimeError("fixture host must remain local to the test host")
        fixture_root = f"http://{fixture_host}:59000"
        proxy_host = os.environ.get("SUB2API_SETTLEMENT_PROXY_HOST", "127.0.0.1").strip()
        if proxy_host not in {"127.0.0.1", "localhost", "host.docker.internal"}:
            raise RuntimeError("fixture proxy host must remain local to the test host")
        status, payload, _ = request_json(
            base_url + "/api/v1/admin/proxies",
            "POST",
            {
                "name": "settlement-fixed-connect",
                "protocol": "http",
                "host": proxy_host,
                "port": 59002,
                "fallback_mode": "none",
            },
            auth,
        )
        proxy = response_data(status, payload, "create fixture CONNECT proxy")
        proxy_id = int(proxy.get("id", 0)) if isinstance(proxy, dict) else 0
        if proxy_id <= 0:
            raise RuntimeError("create fixture CONNECT proxy response is missing id")
        account_quota = {"quota_limit": 50.0, "quota_used": 0.0}
        definitions = (
            ("anthropic", "anthropic", "apikey", {"api_key": "fixture-anthropic", "base_url": fixture_root + "/anthropic"}, dict(account_quota), None),
            (
                "anthropic_passthrough",
                "anthropic",
                "apikey",
                {"api_key": "fixture-anthropic-passthrough", "base_url": fixture_root + "/anthropic"},
                {**account_quota, "anthropic_passthrough": True},
                None,
            ),
            (
                "openai",
                "openai",
                "apikey",
                {"api_key": "fixture-openai", "base_url": fixture_root + "/fault/openai/v1"},
                {
                    **account_quota,
                    "openai_apikey_responses_websockets_v2_enabled": True,
                    "openai_apikey_responses_websockets_v2_mode": "passthrough",
                },
                None,
            ),
            ("antigravity", "antigravity", "apikey", {"api_key": "fixture-antigravity", "base_url": fixture_root}, dict(account_quota), None),
            ("grok", "grok", "apikey", {"api_key": "fixture-grok", "base_url": "https://api.x.ai/v1", "using_api": True}, dict(account_quota), proxy_id),
            (
                "kimi",
                "kimi",
                "oauth",
                {
                    "access_token": "fixture-kimi",
                    "refresh_token": "fixture-kimi-refresh",
                    "expires_at": "2099-01-01T00:00:00Z",
                    "base_url": fixture_root + "/kimi/v1",
                },
                {},
                None,
            ),
        )
        keys: dict[str, str] = {}
        group_ids: dict[str, int] = {}
        account_ids: dict[str, int] = {}
        for fixture_name, platform, account_type, credentials, extra, account_proxy_id in definitions:
            status, payload, _ = request_json(
                base_url + "/api/v1/admin/groups",
                "POST",
                {
                    "name": f"settlement-{fixture_name}",
                    "description": "isolated settlement durability fixture",
                    "platform": platform,
                    "rate_multiplier": 1.0,
                    "subscription_type": "standard",
                },
                auth,
            )
            group = response_data(status, payload, f"create {fixture_name} group")
            group_id = int(group.get("id", 0)) if isinstance(group, dict) else 0
            if group_id <= 0:
                raise RuntimeError(f"create {fixture_name} group response is missing id")
            group_ids[fixture_name] = group_id

            status, payload, _ = request_json(
                base_url + "/api/v1/admin/accounts",
                "POST",
                {
                    "name": f"settlement-{fixture_name}",
                    "platform": platform,
                    "type": account_type,
                    "credentials": credentials,
                    "extra": extra,
                    "concurrency": 3,
                    "priority": 1,
                    "rate_multiplier": 2.0,
                    "group_ids": [group_id],
                    "proxy_id": account_proxy_id,
                    "confirm_mixed_channel_risk": True,
                },
                {**auth, "Idempotency-Key": f"settlement-account-{fixture_name}"},
            )
            account = response_data(status, payload, f"create {fixture_name} account")
            account_id = int(account.get("id", 0)) if isinstance(account, dict) else 0
            if account_id <= 0:
                raise RuntimeError(f"create {fixture_name} account response is missing id")
            account_ids[fixture_name] = account_id

            custom_key = f"sk-settlement-{fixture_name}-test-only"
            status, payload, _ = request_json(
                base_url + "/api/v1/keys",
                "POST",
                {"name": f"settlement-{fixture_name}", "group_id": group_id, "custom_key": custom_key, "quota": 50.0},
                {**auth, "Idempotency-Key": f"settlement-key-{fixture_name}"},
            )
            response_data(status, payload, f"create {fixture_name} API key")
            keys[fixture_name] = custom_key

        openai_group_id = group_ids["openai"]
        status, payload, _ = request_json(
            base_url + "/api/v1/admin/accounts",
            "POST",
            {
                "name": "settlement-openai-fallback",
                "platform": "openai",
                "type": "apikey",
                "credentials": {"api_key": "fixture-openai-fallback", "base_url": fixture_root + "/openai/v1"},
                "extra": {
                    **account_quota,
                    "openai_apikey_responses_websockets_v2_enabled": True,
                    "openai_apikey_responses_websockets_v2_mode": "passthrough",
                },
                "concurrency": 3,
                "priority": 2,
                "rate_multiplier": 2.0,
                "group_ids": [openai_group_id],
                "confirm_mixed_channel_risk": True,
            },
            {**auth, "Idempotency-Key": "settlement-account-openai-fallback"},
        )
        fallback_account = response_data(status, payload, "create openai fallback account")
        fallback_account_id = int(fallback_account.get("id", 0)) if isinstance(fallback_account, dict) else 0
        if fallback_account_id <= 0:
            raise RuntimeError("create openai fallback account response is missing id")
        account_ids["openai_fallback"] = fallback_account_id

        secrets_output.parent.mkdir(parents=True, exist_ok=True)
        secrets_output.write_text(
            "\n".join(f"SUB2API_DIFF_KEY_{name.upper()}={key}" for name, key in sorted(keys.items())) + "\n",
            encoding="utf-8",
        )
        secrets_output.chmod(0o600)
        print(json.dumps({
            "base_url": base_url,
            "platforms": sorted(keys),
            "group_ids": group_ids,
            "account_ids": account_ids,
            "secrets_output": str(secrets_output),
            "note": "Gemini supply is retired in this version and is tested as an explicit unavailable baseline, not seeded as an account.",
        }, ensure_ascii=False, indent=2))
        return 0
    except (ValueError, RuntimeError, urllib.error.URLError) as error:
        print(f"fixture seed failed: {error}", file=sys.stderr)
        return 1


def fixture_cases(base_url: str) -> list[dict[str, Any]]:
    return [
        {"provider": "anthropic", "path": "/anthropic/v1/messages", "body": {"model": "claude-fixture", "messages": [{"role": "user", "content": "hi"}], "stream": True}},
        {"provider": "openai", "path": "/openai/v1/responses", "body": {"model": "gpt-fixture", "input": "hi", "stream": True, "store": True}},
        {"provider": "gemini", "path": "/gemini/v1beta/models/gemini-fixture:streamGenerateContent?alt=sse", "body": {"contents": [{"role": "user", "parts": [{"text": "hi"}]}]}},
        {"provider": "antigravity", "path": "/antigravity/v1internal:streamGenerateContent?alt=sse", "body": {"request": {"contents": [{"role": "user", "parts": [{"text": "hi"}]}]}}},
        {"provider": "grok", "path": "/grok/v1/responses", "body": {"model": "grok-fixture", "input": "hi", "stream": True, "store": True}},
        {"provider": "kimi", "path": "/kimi/v1/chat/completions", "body": {"model": "kimi-fixture", "messages": [{"role": "user", "content": "hi"}], "stream": True}},
    ]


def fixture_smoke(output: pathlib.Path | None = None) -> int:
    server = make_server()
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base_url = f"http://127.0.0.1:{server.server_port}"
    report: dict[str, Any] = {"base_url": base_url, "scenarios": {}}
    try:
        for scenario in ("normal", "missing_terminal"):
            request_json(base_url + "/__control", "POST", {"scenario": scenario})
            results = []
            for case in fixture_cases(base_url):
                status, payload, content_type = request_json(
                    base_url + case["path"],
                    "POST",
                    case["body"],
                    {"X-Sub2API-Fixture-Provider": case["provider"]},
                )
                text = payload if isinstance(payload, str) else json.dumps(payload, separators=(",", ":"))
                terminal_markers = {
                    "anthropic": '"type":"message_stop"',
                    "openai": '"type":"response.completed"',
                    "gemini": '"usageMetadata":{',
                    "antigravity": '"usageMetadata":{',
                    "grok": '"type":"response.completed"',
                    "kimi": '"usage":{',
                }
                has_terminal = terminal_markers[case["provider"]] in text
                results.append({"provider": case["provider"], "status": status, "content_type": content_type.split(";", 1)[0], "has_terminal_marker": has_terminal})
            report["scenarios"][scenario] = results

        request_json(base_url + "/__control", "POST", {"scenario": "normal"})
        openai = fixture_cases(base_url)[1]
        status, payload, _ = request_json(base_url + openai["path"], "POST", {**openai["body"], "stream": False}, {"X-Sub2API-Fixture-Provider": "openai"})
        response_id = payload.get("id") if status == 200 and isinstance(payload, dict) else ""
        retrieve_status, retrieved, _ = request_json(base_url + f"/openai/v1/responses/{response_id}")
        report["stored_retrieve"] = {"create_status": status, "response_id": response_id, "retrieve_status": retrieve_status, "usage_present": isinstance(retrieved, dict) and isinstance(retrieved.get("usage"), dict)}

        request_json(base_url + "/__control", "POST", {"scenario": "retrieve_404"})
        missing_status, _, _ = request_json(base_url + f"/openai/v1/responses/{response_id}")
        report["retrieve_404_status"] = missing_status

        request_json(base_url + "/__control", "POST", {"scenario": "hold_after_terminal"})
        barrier_result: dict[str, Any] = {}

        def send_barrier_case() -> None:
            case = fixture_cases(base_url)[1]
            status, _, _ = request_json(
                base_url + case["path"],
                "POST",
                case["body"],
                {"X-Sub2API-Fixture-Provider": case["provider"]},
            )
            barrier_result["status"] = status

        barrier_client = threading.Thread(target=send_barrier_case)
        barrier_client.start()
        terminal_count = 0
        for _ in range(100):
            _, state, _ = request_json(base_url + "/__control")
            terminal_count = int(state.get("terminal_delivery_count", 0)) if isinstance(state, dict) else 0
            if terminal_count > 0:
                break
            threading.Event().wait(0.01)
        _, released_state, _ = request_json(base_url + "/__control", "POST", {"action": "release_terminal"})
        barrier_client.join(timeout=2)
        report["terminal_barrier"] = {
            "terminal_delivery_count": terminal_count,
            "released": isinstance(released_state, dict) and bool(released_state.get("terminal_released")),
            "client_status": barrier_result.get("status"),
            "client_finished": not barrier_client.is_alive(),
        }
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)

    normal_ok = all(item["status"] == 200 and item["has_terminal_marker"] for item in report["scenarios"]["normal"])
    missing_ok = all(item["status"] == 200 and not item["has_terminal_marker"] for item in report["scenarios"]["missing_terminal"])
    retrieve_ok = report["stored_retrieve"]["retrieve_status"] == 200 and report["stored_retrieve"]["usage_present"] and report["retrieve_404_status"] == 404
    barrier_ok = (
        report["terminal_barrier"]["terminal_delivery_count"] == 1
        and report["terminal_barrier"]["released"]
        and report["terminal_barrier"]["client_status"] == 200
        and report["terminal_barrier"]["client_finished"]
    )
    report["ok"] = normal_ok and missing_ok and retrieve_ok and barrier_ok
    if output:
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0 if report["ok"] else 1


def detect_compose_command() -> list[str]:
    docker = shutil.which("docker")
    if docker:
        probe = subprocess.run(
            [docker, "compose", "version"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
        )
        if probe.returncode == 0:
            return [docker, "compose"]
    legacy = shutil.which("docker-compose")
    if legacy:
        return [legacy]
    return []


def compose_command(*args: str) -> list[str]:
    base = detect_compose_command()
    if not base:
        return []
    return [*base, "-p", COMPOSE_PROJECT, "-f", str(COMPOSE_FILE), *args]


def run_checked(command: list[str]) -> int:
    if not command:
        print("docker compose is not available", file=sys.stderr)
        return 1
    print("+ " + " ".join(command))
    return subprocess.run(command, cwd=ROOT, check=False).returncode


def native_commands() -> dict[str, str]:
    return {name: shutil.which(name) or "" for name in ("initdb", "pg_ctl", "createdb", "psql", "postgres", "redis-server", "redis-cli")}


def native_available() -> bool:
    return all(native_commands().values())


def generate_fixture_tls_cert() -> int:
    openssl = shutil.which("openssl")
    if not openssl:
        print("openssl is required for the local Grok HTTPS fixture", file=sys.stderr)
        return 1
    tls_dir = NATIVE_ROOT / "tls"
    tls_dir.mkdir(parents=True, exist_ok=True)
    cert = tls_dir / "fixture-ca.crt"
    key = tls_dir / "fixture-ca.key"
    result = subprocess.run(
        [
            openssl,
            "req", "-x509", "-newkey", "rsa:2048", "-sha256", "-nodes", "-days", "2",
            "-keyout", str(key), "-out", str(cert), "-subj", "/CN=127.0.0.1",
            "-addext", "subjectAltName=IP:127.0.0.1,DNS:api.x.ai",
            "-addext", "basicConstraints=critical,CA:TRUE",
            "-addext", "keyUsage=critical,digitalSignature,keyEncipherment,keyCertSign",
        ],
        check=False,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.PIPE,
        text=True,
    )
    if result.returncode != 0:
        print(result.stderr.strip() or "openssl certificate generation failed", file=sys.stderr)
        return result.returncode
    key.chmod(0o600)
    cert.chmod(0o644)
    print(json.dumps({"cert": str(cert), "key": str(key), "url": INFRA["fixture_tls"]}, ensure_ascii=False, indent=2))
    return 0


def native_postgres_up(name: str, port: int, database: str) -> None:
    commands = native_commands()
    data_dir = NATIVE_ROOT / f"postgres-{name}"
    log_file = NATIVE_ROOT / f"postgres-{name}.log"
    if not (data_dir / "PG_VERSION").exists():
        data_dir.mkdir(parents=True, exist_ok=True)
        subprocess.run(
            [commands["initdb"], "-D", str(data_dir), "-U", "sub2api", "--auth=trust", "--no-locale"],
            check=True,
            stdout=subprocess.DEVNULL,
        )
    status = subprocess.run(
        [commands["pg_ctl"], "-D", str(data_dir), "status"],
        check=False,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    if status.returncode != 0:
        subprocess.run(
            [commands["pg_ctl"], "-D", str(data_dir), "-l", str(log_file), "-o", f"-p {port} -h 127.0.0.1", "-w", "start"],
            check=True,
            stdout=subprocess.DEVNULL,
        )
    exists = subprocess.run(
        [commands["psql"], "-h", "127.0.0.1", "-p", str(port), "-U", "sub2api", "-d", "postgres", "-tAc", f"SELECT 1 FROM pg_database WHERE datname='{database}'"],
        check=True,
        capture_output=True,
        text=True,
    ).stdout.strip()
    if exists != "1":
        subprocess.run(
            [commands["createdb"], "-h", "127.0.0.1", "-p", str(port), "-U", "sub2api", database],
            check=True,
            stdout=subprocess.DEVNULL,
        )


def native_redis_up(name: str, port: int) -> None:
    commands = native_commands()
    data_dir = NATIVE_ROOT / f"redis-{name}"
    data_dir.mkdir(parents=True, exist_ok=True)
    pid_file = data_dir / "redis.pid"
    ping = subprocess.run(
        [commands["redis-cli"], "-h", "127.0.0.1", "-p", str(port), "ping"],
        check=False,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    if ping.returncode == 0:
        return
    subprocess.run(
        [
            commands["redis-server"],
            "--bind", "127.0.0.1",
            "--port", str(port),
            "--daemonize", "yes",
            "--pidfile", str(pid_file),
            "--dir", str(data_dir),
            "--save", "",
            "--appendonly", "no",
        ],
        check=True,
        stdout=subprocess.DEVNULL,
    )


def native_infra_up() -> int:
    if not native_available():
        print("native PostgreSQL/Redis tools are incomplete", file=sys.stderr)
        return 1
    NATIVE_ROOT.mkdir(parents=True, exist_ok=True)
    try:
        native_postgres_up("old", 55441, "sub2api_old")
        native_postgres_up("new", 55442, "sub2api_new")
        native_redis_up("old", 56381)
        native_redis_up("new", 56382)
        NATIVE_MARKER.write_text("native\n", encoding="utf-8")
    except subprocess.CalledProcessError as error:
        print(f"native infrastructure start failed: {error}", file=sys.stderr)
        return 1
    print(json.dumps({"backend": "native", "root": str(NATIVE_ROOT), "infra": INFRA}, ensure_ascii=False, indent=2))
    return 0


def native_infra_down() -> int:
    commands = native_commands()
    for port in (56381, 56382):
        if commands["redis-cli"]:
            subprocess.run(
                [commands["redis-cli"], "-h", "127.0.0.1", "-p", str(port), "shutdown", "nosave"],
                check=False,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
    for name in ("old", "new"):
        data_dir = NATIVE_ROOT / f"postgres-{name}"
        if commands["pg_ctl"] and (data_dir / "PG_VERSION").exists():
            subprocess.run(
                [commands["pg_ctl"], "-D", str(data_dir), "-m", "fast", "-w", "stop"],
                check=False,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
    if NATIVE_ROOT.exists():
        shutil.rmtree(NATIVE_ROOT)
    print(json.dumps({"backend": "native", "removed": str(NATIVE_ROOT)}, ensure_ascii=False))
    return 0


def infra_up() -> int:
    compose = compose_command("up", "-d")
    if compose:
        if run_checked(compose) != 0:
            return 1
        import socket
        deadline = time.monotonic() + 45
        pending = {55441, 55442, 56381, 56382}
        while pending and time.monotonic() < deadline:
            for port in tuple(pending):
                with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
                    probe.settimeout(0.1)
                    if probe.connect_ex(("127.0.0.1", port)) == 0:
                        pending.remove(port)
            if pending:
                time.sleep(0.1)
        if pending:
            print(f"compose infrastructure ports did not become ready: {sorted(pending)}", file=sys.stderr)
            return 1
        compose_ps = compose_command("ps", "-q")
        container_ids = subprocess.run(
            compose_ps,
            cwd=ROOT,
            check=False,
            capture_output=True,
            text=True,
        ).stdout.split()
        if len(container_ids) != 4:
            print(f"expected four compose infrastructure containers, got {len(container_ids)}", file=sys.stderr)
            return 1
        docker = shutil.which("docker") or "docker"
        while time.monotonic() < deadline:
            health = []
            for container_id in container_ids:
                result = subprocess.run(
                    [docker, "inspect", "--format", "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}", container_id],
                    check=False,
                    capture_output=True,
                    text=True,
                )
                health.append(result.stdout.strip() if result.returncode == 0 else "missing")
            if all(status == "healthy" for status in health):
                return 0
            time.sleep(0.2)
        print(f"compose infrastructure did not become healthy: {health}", file=sys.stderr)
        return 1
    return native_infra_up()


def infra_down() -> int:
    if NATIVE_MARKER.exists() or not detect_compose_command():
        return native_infra_down()
    return run_checked(compose_command("down", "-v", "--remove-orphans"))


def wait_for_health(url: str, timeout: float = 30) -> None:
    url = require_loopback_http(url)
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            status, _, _ = request_json(url + "/health")
            if status == 200:
                return
        except (urllib.error.URLError, OSError):
            pass
        time.sleep(0.1)
    raise RuntimeError(f"gateway did not become healthy: {url}")


def docker_goarch() -> str:
    docker = shutil.which("docker")
    if not docker:
        raise RuntimeError("docker is required for dual gateway lifecycle")
    result = subprocess.run(
        [docker, "info", "--format", "{{.Architecture}}"],
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or "docker daemon is unavailable")
    architecture = result.stdout.strip().lower()
    if architecture in {"arm64", "aarch64"}:
        return "arm64"
    if architecture in {"amd64", "x86_64"}:
        return "amd64"
    raise RuntimeError(f"unsupported Docker architecture: {architecture}")


def ensure_dual_ports_free() -> None:
    import socket

    occupied = []
    for port in (55441, 55442, 56381, 56382, 58081, 58082, 59000, 59001, 59002):
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
            probe.settimeout(0.05)
            if probe.connect_ex(("127.0.0.1", port)) == 0:
                occupied.append(port)
    if occupied:
        raise RuntimeError(f"settlement test ports are already occupied: {occupied}; run dual-down first")


def start_fixture_process(command: list[str], log_path: pathlib.Path) -> int:
    log_path.parent.mkdir(parents=True, exist_ok=True)
    log = log_path.open("ab")
    try:
        process = subprocess.Popen(
            command,
            cwd=ROOT,
            stdout=log,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
    finally:
        log.close()
    return process.pid


def docker_gateway_command(
    name: str,
    binary_name: str,
    data_dir: pathlib.Path,
    port: int,
    db_port: int,
    database: str,
    redis_port: int,
    auto_setup: bool,
    server_args: list[str] | None = None,
    extra_env: dict[str, str] | None = None,
) -> list[str]:
    docker = shutil.which("docker") or "docker"
    linux_host_network = sys.platform.startswith("linux")
    infrastructure_host = "127.0.0.1" if linux_host_network else "host.docker.internal"
    command = [docker, "run", "-d", "--name", name]
    if linux_host_network:
        command.extend(["--network", "host"])
    else:
        command.extend([
            "--add-host", "host.docker.internal:host-gateway",
            "-p", f"127.0.0.1:{port}:{port}",
        ])
    command.extend([
        "-v", f"{LOCAL_RUNTIME_ROOT / 'bin'}:/fixture/bin:ro",
        "-v", f"{data_dir}:/data",
        "-v", f"{LOCAL_RUNTIME_ROOT / 'tls'}:/fixture/tls:ro",
        "-e", "DATA_DIR=/data",
        "-e", f"DATABASE_HOST={infrastructure_host}",
        "-e", f"DATABASE_PORT={db_port}",
        "-e", "DATABASE_USER=sub2api",
        "-e", "DATABASE_PASSWORD=settlement-test-only",
        "-e", f"DATABASE_DBNAME={database}",
        "-e", "DATABASE_SSLMODE=disable",
        "-e", f"REDIS_HOST={infrastructure_host}",
        "-e", f"REDIS_PORT={redis_port}",
        "-e", "REDIS_DB=0",
        "-e", f"SERVER_HOST={'127.0.0.1' if linux_host_network else '0.0.0.0'}",
        "-e", f"SERVER_PORT={port}",
        "-e", "SERVER_MODE=release",
        "-e", "SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=true",
    ])
    if auto_setup:
        command.extend([
            "-e", "AUTO_SETUP=true",
            "-e", f"ADMIN_EMAIL={ADMIN_EMAIL}",
            "-e", f"ADMIN_PASSWORD={ADMIN_PASSWORD}",
            "-e", f"JWT_SECRET={JWT_SECRET}",
        ])
    for key, value in sorted((extra_env or {}).items()):
        command.extend(["-e", f"{key}={value}"])
    quoted_server_args = ""
    if server_args:
        quoted_server_args = " " + " ".join(shlex.quote(value) for value in server_args)
    command.extend([
        "alpine:3.20", "sh", "-c",
        "apk add --no-cache ca-certificates tzdata >/dev/null"
        " && cp /fixture/tls/fixture-ca.crt /usr/local/share/ca-certificates/fixture-ca.crt"
        " && update-ca-certificates >/dev/null"
        f" && cp /fixture/bin/{binary_name} /tmp/server"
        " && chmod 755 /tmp/server && exec /tmp/server" + quoted_server_args,
    ])
    return command


def load_fixture_secrets(path: pathlib.Path) -> None:
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip() or line.lstrip().startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        os.environ[key.strip()] = value.strip()


def start_new_gateway_container(server_args: list[str] | None = None) -> None:
    data_new = LOCAL_RUNTIME_ROOT / "data-new"
    result = subprocess.run(
        docker_gateway_command(
            "sub2api-settlement-new-linux", "server-new", data_new,
            58082, 55442, "sub2api_new", 56382, False, server_args,
        ),
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or "failed to start new settlement gateway")
    wait_for_health("http://127.0.0.1:58082", 45)


def stop_new_gateway_container(signal_name: str = "TERM") -> None:
    docker = shutil.which("docker") or "docker"
    if signal_name == "KILL":
        result = subprocess.run([docker, "kill", "--signal", "KILL", "sub2api-settlement-new-linux"], check=False, capture_output=True, text=True)
    else:
        result = subprocess.run([docker, "stop", "-t", "3", "sub2api-settlement-new-linux"], check=False, capture_output=True, text=True)
    if result.returncode != 0 and "No such container" not in result.stderr:
        raise RuntimeError(result.stderr.strip() or "failed to stop new settlement gateway")
    remove = subprocess.run(
        [docker, "rm", "-f", "sub2api-settlement-new-linux"],
        check=False,
        capture_output=True,
        text=True,
    )
    if remove.returncode != 0 and "No such container" not in remove.stderr:
        raise RuntimeError(remove.stderr.strip() or "failed to remove new settlement gateway")
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        probe = subprocess.run([docker, "inspect", "sub2api-settlement-new-linux"], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if probe.returncode != 0:
            return
        time.sleep(0.05)
    raise RuntimeError("new settlement gateway container did not disappear")


def set_durable_mode(dsn: str, mode: str, group_scope: str = "*") -> None:
    execute_loopback_sql(dsn, f"""
INSERT INTO settings (key, value, updated_at)
VALUES ('usage_settlement_durability_mode', {sql_literal(mode)}, NOW())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
INSERT INTO settings (key, value, updated_at)
VALUES ('usage_settlement_durability_group_ids', {sql_literal(group_scope)}, NOW())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
""")


def fixture_group_id(dsn: str, api_key_name: str) -> int:
    raw = execute_loopback_sql(dsn, f"""
SELECT COALESCE(group_id, 0)
FROM api_keys
WHERE name = {sql_literal(api_key_name)}
ORDER BY id
LIMIT 1;
""")
    group_id = int(raw or "0")
    if group_id <= 0:
        raise RuntimeError(f"fixture API key has no positive group: {api_key_name}")
    return group_id


def run_group_rollout_gate(output: pathlib.Path) -> int:
    dsn = INFRA["new_postgres"]
    base_url = "http://127.0.0.1:58082"
    data_new = LOCAL_RUNTIME_ROOT / "data-new"
    selected_case = next(item for item in DIFF_CASES if item["name"] == "openai_json")
    legacy_case = next(item for item in DIFF_CASES if item["name"] == "anthropic_json")
    report: dict[str, Any] = {}
    try:
        if not LOCAL_RUNTIME_MANIFEST.exists() or not (LOCAL_RUNTIME_ROOT / "bin" / "server-new").exists():
            raise RuntimeError("dual runtime is not ready; run dual-up first")
        load_fixture_secrets(data_new / "diff.env")
        selected_group = fixture_group_id(dsn, "settlement-openai")
        legacy_group = fixture_group_id(dsn, "settlement-anthropic")
        if selected_group == legacy_group:
            raise RuntimeError("group rollout fixtures must use distinct groups")
        stop_new_gateway_container()
        set_durable_mode(dsn, "authoritative", json.dumps([selected_group]))
        start_new_gateway_container()

        cases: list[dict[str, Any]] = []
        for case, selected in ((selected_case, True), (legacy_case, False)):
            run_id = "settlement-group-rollout-" + uuid.uuid4().hex[:12]
            marker = f"sub2api-dual-diff/{run_id}/{case['name']}"
            before = query_billing_state(dsn, str(case["platform"]), marker)
            request_result = request_one(base_url, case, run_id)
            after: dict[str, Any] = {}
            attempt: dict[str, Any] = {}
            deadline = time.monotonic() + 20
            while time.monotonic() < deadline:
                after = query_billing_state(dsn, str(case["platform"]), marker)
                attempt = query_settlement_attempt(dsn, marker)
                billing_complete = (
                    int(after.get("usage_count", 0)) == int(before.get("usage_count", 0)) + 1
                    and int(after.get("dedup_count", 0)) == int(before.get("dedup_count", 0)) + 1
                )
                journal_complete = attempt.get("state") == "settled" if selected else not attempt
                if billing_complete and journal_complete:
                    break
                time.sleep(0.05)
            checks = {
                "request_succeeded": request_result.get("status") == 200,
                "exactly_one_usage_log": int(after.get("usage_count", 0)) == int(before.get("usage_count", 0)) + 1,
                "exactly_one_billing_dedup": int(after.get("dedup_count", 0)) == int(before.get("dedup_count", 0)) + 1,
                "selected_group_settled_by_journal": attempt.get("state") == "settled" if selected else True,
                "unselected_group_has_no_journal": not attempt if not selected else True,
            }
            cases.append({
                "name": case["name"],
                "platform": case["platform"],
                "group_id": selected_group if selected else legacy_group,
                "selected": selected,
                "request": request_result,
                "attempt": attempt,
                "before": before,
                "after": after,
                "checks": checks,
                "passed": all(checks.values()),
            })
            print(f"{case['name']}: {'passed' if all(checks.values()) else 'failed'}")
        summary = {
            "case_count": len(cases),
            "passed_count": sum(1 for item in cases if item["passed"]),
            "all_passed": all(item["passed"] for item in cases),
        }
        report = {
            "selected_group_id": selected_group,
            "legacy_group_id": legacy_group,
            "cases": cases,
            "summary": summary,
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps(summary, ensure_ascii=False, indent=2))
        return 0 if summary["all_passed"] else 1
    except (OSError, RuntimeError, ValueError, KeyError, json.JSONDecodeError, urllib.error.URLError) as error:
        print(f"group rollout gate failed: {error}", file=sys.stderr)
        return 1
    finally:
        try:
            stop_new_gateway_container()
        except RuntimeError:
            pass
        try:
            set_durable_mode(dsn, "off")
            start_new_gateway_container()
        except (OSError, RuntimeError, ValueError) as restore_error:
            print(f"warning: failed to restore new gateway after group rollout gate: {restore_error}", file=sys.stderr)


def run_durable_crash_matrix(output: pathlib.Path) -> int:
    dsn = INFRA["new_postgres"]
    base_url = "http://127.0.0.1:58082"
    stages = ("terminal_parsed", "event_persisted", "billing_before_commit", "commit_before_ack")
    data_new = LOCAL_RUNTIME_ROOT / "data-new"
    failpoint_host_dir = data_new / "sub2api-settlement-failpoint"
    case = next(item for item in DIFF_CASES if item["name"] == "openai_json")
    reports: list[dict[str, Any]] = []
    try:
        if not LOCAL_RUNTIME_MANIFEST.exists() or not (LOCAL_RUNTIME_ROOT / "bin" / "server-new").exists():
            raise RuntimeError("dual runtime is not ready; run dual-up first")
        load_fixture_secrets(data_new / "diff.env")
        selected_group = fixture_group_id(dsn, "settlement-openai")
        stop_new_gateway_container()
        set_durable_mode(dsn, "authoritative", json.dumps([selected_group]))

        for stage in stages:
            if failpoint_host_dir.exists():
                shutil.rmtree(failpoint_host_dir)
            failpoint_host_dir.mkdir(parents=True, mode=0o700)
            start_new_gateway_container([
                f"--settlement-test-failpoint={stage}",
                "--settlement-test-failpoint-dir=/data/sub2api-settlement-failpoint",
            ])
            run_id = "settlement-durable-crash-" + uuid.uuid4().hex[:12]
            marker = f"sub2api-dual-diff/{run_id}/openai_json"
            before = query_billing_state(dsn, "openai", marker)
            request_result: dict[str, Any] = {}

            def send_request() -> None:
                try:
                    request_result["value"] = request_one(base_url, case, run_id)
                except Exception as error:  # child is intentionally killed mid-request
                    request_result["error"] = f"{type(error).__name__}: {error}"

            client = threading.Thread(target=send_request)
            client.start()
            reached_path = failpoint_host_dir / f"{stage}.reached"
            deadline = time.monotonic() + 30
            while time.monotonic() < deadline and not reached_path.exists():
                time.sleep(0.01)
            if not reached_path.exists():
                raise RuntimeError(f"durable failpoint was not reached: {stage}")
            at_crash = query_billing_state(dsn, "openai", marker)
            attempt_at_crash = query_settlement_attempt(dsn, marker)
            stop_new_gateway_container("KILL")
            client.join(timeout=5)

            start_new_gateway_container()
            deadline = time.monotonic() + 50
            after_restart: dict[str, Any] = {}
            attempt_after_restart: dict[str, Any] = {}
            while time.monotonic() < deadline:
                after_restart = query_billing_state(dsn, "openai", marker)
                attempt_after_restart = query_settlement_attempt(dsn, marker)
                if (
                    int(after_restart.get("usage_count", 0)) == int(before.get("usage_count", 0)) + 1
                    and int(after_restart.get("dedup_count", 0)) == int(before.get("dedup_count", 0)) + 1
                    and attempt_after_restart.get("state") == "settled"
                ):
                    break
                time.sleep(0.1)
            passed = (
                int(after_restart.get("usage_count", 0)) == int(before.get("usage_count", 0)) + 1
                and int(after_restart.get("dedup_count", 0)) == int(before.get("dedup_count", 0)) + 1
                and attempt_after_restart.get("state") == "settled"
            )
            reports.append({
                "stage": stage,
                "run_id": run_id,
                "marker": marker,
                "request": request_result,
                "before": before,
                "at_crash": at_crash,
                "attempt_at_crash": attempt_at_crash,
                "after_restart": after_restart,
                "attempt_after_restart": attempt_after_restart,
                "passed": passed,
            })
            print(f"{stage}: {'passed' if passed else 'failed'}")
            stop_new_gateway_container()
            if not passed:
                raise RuntimeError(f"durable crash stage did not converge: {stage}")

        summary = {"stage_count": len(stages), "passed_count": sum(1 for item in reports if item["passed"]), "all_passed": all(item["passed"] for item in reports)}
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps({"selected_group_id": selected_group, "stages": reports, "summary": summary}, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps(summary, ensure_ascii=False, indent=2))
        return 0 if summary["all_passed"] else 1
    except (OSError, RuntimeError, ValueError, KeyError, json.JSONDecodeError, urllib.error.URLError) as error:
        print(f"durable crash matrix failed: {error}", file=sys.stderr)
        return 1
    finally:
        try:
            stop_new_gateway_container()
        except RuntimeError:
            pass
        try:
            set_durable_mode(dsn, "off")
            start_new_gateway_container()
        except (OSError, RuntimeError, ValueError) as restore_error:
            print(f"warning: failed to restore new gateway in off mode: {restore_error}", file=sys.stderr)


def run_missing_terminal_reconciliation(output: pathlib.Path) -> int:
    dsn = INFRA["new_postgres"]
    base_url = "http://127.0.0.1:58082"
    data_new = LOCAL_RUNTIME_ROOT / "data-new"
    base_case = next(item for item in DIFF_CASES if item["name"] == "openai_sse")
    case = {**base_case, "body": {**base_case["body"], "store": False}}
    report: dict[str, Any] = {}
    try:
        if not LOCAL_RUNTIME_MANIFEST.exists() or not (LOCAL_RUNTIME_ROOT / "bin" / "server-new").exists():
            raise RuntimeError("dual runtime is not ready; run dual-up first")
        load_fixture_secrets(data_new / "diff.env")
        stop_new_gateway_container()
        set_durable_mode(dsn, "authoritative")
        request_fixture_control("openai", "POST", {"scenario": "missing_terminal"})
        start_new_gateway_container()

        minimum_id = max_settlement_attempt_id(dsn)
        run_id = "settlement-missing-terminal-" + uuid.uuid4().hex[:12]
        marker = f"sub2api-dual-diff/{run_id}/{case['name']}"
        before = query_billing_state(dsn, "openai", marker)
        request_result = request_one(base_url, case, run_id)

        attempt: dict[str, Any] = {}
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            attempt = query_settlement_attempt_after(dsn, minimum_id)
            if attempt.get("state") == "started":
                break
            time.sleep(0.05)
        if attempt.get("state") != "started":
            raise RuntimeError(f"missing-terminal request did not remain started: {attempt}")
        attempt_id = int(attempt["id"])
        execute_loopback_sql(dsn, f"""
UPDATE usage_settlement_attempts
SET reconciliation_deadline = NOW() - INTERVAL '1 second', updated_at = NOW()
WHERE id = {attempt_id} AND state = 'started';
""")

        reconciled: dict[str, Any] = {}
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            reconciled = query_settlement_attempt_after(dsn, minimum_id)
            if reconciled.get("state") == "reconciliation_required":
                break
            time.sleep(0.05)
        after = query_billing_state(dsn, "openai", marker)
        checks = {
            "request_reached_upstream": request_result.get("status") == 200,
            "started_without_payload": attempt.get("state") == "started" and not attempt.get("payload_hash"),
            "store_false_has_no_retrieve_identity": not attempt.get("provider_request_id"),
            "reconciliation_required": reconciled.get("state") == "reconciliation_required",
            "missing_terminal_audited": reconciled.get("last_error_code") == "TERMINAL_USAGE_MISSING",
            "no_payload_invented": not reconciled.get("payload_hash"),
            "no_usage_log": int(after.get("usage_count", 0)) == int(before.get("usage_count", 0)),
            "no_billing_dedup": int(after.get("dedup_count", 0)) == int(before.get("dedup_count", 0)),
            "balance_unchanged": after.get("balance") == before.get("balance"),
            "api_key_quota_unchanged": after.get("api_key", {}).get("quota_used") == before.get("api_key", {}).get("quota_used"),
            "account_quota_unchanged": after.get("account", {}).get("quota_used") == before.get("account", {}).get("quota_used"),
        }
        report = {
            "run_id": run_id,
            "case": case["name"],
            "request": request_result,
            "attempt_before_deadline": attempt,
            "attempt_after_deadline": reconciled,
            "billing_before": before,
            "billing_after": after,
            "checks": checks,
            "passed": all(checks.values()),
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps({"passed": report["passed"], "checks": checks}, ensure_ascii=False, indent=2))
        return 0 if report["passed"] else 1
    except (OSError, RuntimeError, ValueError, KeyError, json.JSONDecodeError, urllib.error.URLError) as error:
        print(f"missing-terminal reconciliation failed: {error}", file=sys.stderr)
        return 1
    finally:
        try:
            request_fixture_control("openai", "POST", {"scenario": "normal"})
        except (OSError, ValueError, RuntimeError, urllib.error.URLError):
            pass
        try:
            stop_new_gateway_container()
        except RuntimeError:
            pass
        try:
            set_durable_mode(dsn, "off")
            start_new_gateway_container()
        except (OSError, RuntimeError, ValueError) as restore_error:
            print(f"warning: failed to restore new gateway in off mode: {restore_error}", file=sys.stderr)


def run_stored_response_recovery(output: pathlib.Path) -> int:
    dsn = INFRA["new_postgres"]
    base_url = "http://127.0.0.1:58082"
    data_new = LOCAL_RUNTIME_ROOT / "data-new"
    case_names = ("openai_sse", "grok_sse")
    reports: list[dict[str, Any]] = []
    try:
        if not LOCAL_RUNTIME_MANIFEST.exists() or not (LOCAL_RUNTIME_ROOT / "bin" / "server-new").exists():
            raise RuntimeError("dual runtime is not ready; run dual-up first")
        load_fixture_secrets(data_new / "diff.env")
        stop_new_gateway_container()
        set_durable_mode(dsn, "authoritative")
        start_new_gateway_container()

        for case_name in case_names:
            base_case = next(item for item in DIFF_CASES if item["name"] == case_name)
            case = {**base_case, "body": {**base_case["body"], "store": True}}
            platform = str(case["platform"])
            request_fixture_control(platform, "POST", {"scenario": "missing_terminal"})
            minimum_id = max_settlement_attempt_id(dsn)
            run_id = "settlement-stored-recovery-" + platform + "-" + uuid.uuid4().hex[:12]
            marker = f"sub2api-dual-diff/{run_id}/{case['name']}"
            before = query_billing_state(dsn, platform, marker)
            request_result = request_one(base_url, case, run_id)
            started: dict[str, Any] = {}
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                started = query_settlement_attempt_after(dsn, minimum_id)
                if started.get("state") == "started" and started.get("provider_request_id"):
                    break
                time.sleep(0.05)
            if started.get("state") != "started" or not started.get("provider_request_id"):
                raise RuntimeError(f"{platform} stored response did not persist recoverable identity: {started}")
            attempt_id = int(started["id"])
            execute_loopback_sql(dsn, f"""
UPDATE usage_settlement_attempts
SET reconciliation_deadline = NOW() - INTERVAL '1 second', updated_at = NOW()
WHERE id = {attempt_id} AND state = 'started';
""")

            settled: dict[str, Any] = {}
            after: dict[str, Any] = {}
            deadline = time.monotonic() + 30
            while time.monotonic() < deadline:
                settled = query_settlement_attempt_after(dsn, minimum_id)
                after = query_billing_state(dsn, platform, marker)
                if settled.get("state") == "settled" and int(after.get("usage_count", 0)) == int(before.get("usage_count", 0)) + 1:
                    break
                time.sleep(0.05)
            checks = {
                "request_reached_upstream": request_result.get("status") == 200,
                "stored_response_id_persisted": started.get("provider_request_id") == f"resp_fixture_{platform}",
                "recovered_payload_persisted": bool(settled.get("payload_hash")),
                "recovered_attempt_settled": settled.get("state") == "settled",
                "exactly_one_usage_log": int(after.get("usage_count", 0)) == int(before.get("usage_count", 0)) + 1,
                "exactly_one_billing_dedup": int(after.get("dedup_count", 0)) == int(before.get("dedup_count", 0)) + 1,
            }
            reports.append({
                "platform": platform,
                "run_id": run_id,
                "request": request_result,
                "attempt_before_recovery": started,
                "attempt_after_recovery": settled,
                "billing_before": before,
                "billing_after": after,
                "checks": checks,
                "passed": all(checks.values()),
            })
            request_fixture_control(platform, "POST", {"scenario": "normal"})
            print(f"{platform}_stored_response_recovery: {'passed' if all(checks.values()) else 'failed'}")

        report = {
            "cases": reports,
            "summary": {
                "case_count": len(reports),
                "passed_count": sum(1 for item in reports if item["passed"]),
                "all_passed": len(reports) == len(case_names) and all(item["passed"] for item in reports),
            },
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps(report["summary"], ensure_ascii=False, indent=2))
        return 0 if report["summary"]["all_passed"] else 1
    except (OSError, RuntimeError, ValueError, KeyError, json.JSONDecodeError, urllib.error.URLError) as error:
        print(f"stored response recovery failed: {error}", file=sys.stderr)
        return 1
    finally:
        try:
            for platform in ("openai", "grok"):
                request_fixture_control(platform, "POST", {"scenario": "normal"})
        except (OSError, ValueError, RuntimeError, urllib.error.URLError):
            pass
        try:
            stop_new_gateway_container()
        except RuntimeError:
            pass
        try:
            set_durable_mode(dsn, "off")
            start_new_gateway_container()
        except (OSError, RuntimeError, ValueError) as restore_error:
            print(f"warning: failed to restore new gateway in off mode: {restore_error}", file=sys.stderr)


def query_shadow_settlement_state(dsn: str, marker_prefix: str) -> dict[str, Any]:
    marker = sql_literal(marker_prefix)
    sql = f"""
WITH attempts AS (
    SELECT *
    FROM usage_settlement_attempts
    WHERE LEFT(COALESCE(payload->'usage_log'->>'user_agent', ''), LENGTH({marker})) = {marker}
), matched AS (
    SELECT a.id, a.state, a.payload_hash, ul.id AS usage_log_id,
        CASE WHEN ul.id IS NULL THEN TRUE ELSE
            (a.payload->'usage_log'->>'input_tokens')::bigint IS DISTINCT FROM ul.input_tokens::bigint OR
            (a.payload->'usage_log'->>'output_tokens')::bigint IS DISTINCT FROM ul.output_tokens::bigint OR
            (a.payload->'usage_log'->>'cache_creation_tokens')::bigint IS DISTINCT FROM ul.cache_creation_tokens::bigint OR
            (a.payload->'usage_log'->>'cache_read_tokens')::bigint IS DISTINCT FROM ul.cache_read_tokens::bigint OR
            ROUND((a.payload->'usage_log'->>'actual_cost')::numeric, 10) IS DISTINCT FROM ul.actual_cost::numeric OR
            ROUND((a.payload->'usage_log'->>'total_cost')::numeric, 10) IS DISTINCT FROM ul.total_cost::numeric
        END AS differs
    FROM attempts a
    LEFT JOIN usage_logs ul
      ON ul.request_id = a.payload->'usage_log'->>'request_id'
     AND ul.api_key_id = a.api_key_id
)
SELECT jsonb_build_object(
    'attempt_count', (SELECT COUNT(*) FROM attempts),
    'ready_count', (SELECT COUNT(*) FROM attempts WHERE state = 'ready'),
    'settled_count', (SELECT COUNT(*) FROM attempts WHERE state = 'settled'),
    'payload_count', (SELECT COUNT(*) FROM attempts WHERE payload_hash IS NOT NULL),
    'matched_usage_count', (SELECT COUNT(*) FROM matched WHERE usage_log_id IS NOT NULL),
    'mismatch_count', (SELECT COUNT(*) FROM matched WHERE differs)
)::text;
"""
    return parse_snapshot_output(execute_loopback_sql(dsn, sql))


def run_shadow_reconciliation(output: pathlib.Path) -> int:
    dsn = INFRA["new_postgres"]
    base_url = "http://127.0.0.1:58082"
    data_new = LOCAL_RUNTIME_ROOT / "data-new"
    try:
        if not LOCAL_RUNTIME_MANIFEST.exists() or not (LOCAL_RUNTIME_ROOT / "bin" / "server-new").exists():
            raise RuntimeError("dual runtime is not ready; run dual-up first")
        load_fixture_secrets(data_new / "diff.env")
        stop_new_gateway_container()
        set_durable_mode(dsn, "shadow")
        request_fixture_control("openai", "POST", {"scenario": "normal"})
        start_new_gateway_container()

        before_dedup = int(execute_loopback_sql(dsn, "SELECT COUNT(*) FROM usage_billing_dedup;") or "0")
        run_id = "settlement-shadow-" + uuid.uuid4().hex[:12]
        results = [request_one(base_url, case, run_id) for case in DIFF_CASES]
        marker_prefix = f"sub2api-dual-diff/{run_id}/"
        state: dict[str, Any] = {}
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            state = query_shadow_settlement_state(dsn, marker_prefix)
            if int(state.get("attempt_count", 0)) == len(DIFF_CASES) and int(state.get("matched_usage_count", 0)) == len(DIFF_CASES):
                break
            time.sleep(0.1)
        after_dedup = int(execute_loopback_sql(dsn, "SELECT COUNT(*) FROM usage_billing_dedup;") or "0")
        checks = {
            "all_requests_succeeded": all(item.get("status") == 200 for item in results),
            "one_attempt_per_request": int(state.get("attempt_count", 0)) == len(DIFF_CASES),
            "all_payloads_ready": int(state.get("ready_count", 0)) == len(DIFF_CASES) and int(state.get("payload_count", 0)) == len(DIFF_CASES),
            "shadow_worker_did_not_settle": int(state.get("settled_count", 0)) == 0,
            "one_legacy_usage_per_request": int(state.get("matched_usage_count", 0)) == len(DIFF_CASES),
            "journal_matches_legacy_usage": int(state.get("mismatch_count", 0)) == 0,
            "no_second_billing_application": after_dedup - before_dedup == len(DIFF_CASES),
        }
        report = {
            "run_id": run_id,
            "case_count": len(DIFF_CASES),
            "results": results,
            "state": state,
            "dedup_before": before_dedup,
            "dedup_after": after_dedup,
            "checks": checks,
            "passed": all(checks.values()),
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps({"passed": report["passed"], "checks": checks}, ensure_ascii=False, indent=2))
        return 0 if report["passed"] else 1
    except (OSError, RuntimeError, ValueError, KeyError, json.JSONDecodeError, urllib.error.URLError) as error:
        print(f"shadow reconciliation failed: {error}", file=sys.stderr)
        return 1
    finally:
        try:
            stop_new_gateway_container()
        except RuntimeError:
            pass
        try:
            set_durable_mode(dsn, "off")
            start_new_gateway_container()
        except (OSError, RuntimeError, ValueError) as restore_error:
            print(f"warning: failed to restore new gateway in off mode: {restore_error}", file=sys.stderr)


def run_additive_schema_rollback(output: pathlib.Path) -> int:
    dsn = INFRA["new_postgres"]
    base_url = "http://127.0.0.1:58082"
    data_new = LOCAL_RUNTIME_ROOT / "data-new"
    case = next(item for item in DIFF_CASES if item["name"] == "openai_json")
    report: dict[str, Any] = {}
    restored = False
    frontend_process: subprocess.Popen[bytes] | None = None
    frontend_log = None
    try:
        if not LOCAL_RUNTIME_MANIFEST.exists() or not (LOCAL_RUNTIME_ROOT / "bin" / "server-old").exists():
            raise RuntimeError("dual runtime is not ready; run dual-up first")
        manifest = json.loads(LOCAL_RUNTIME_MANIFEST.read_text(encoding="utf-8"))
        old_source = pathlib.Path(str(manifest.get("old_source", "")))
        old_frontend = old_source / "frontend"
        old_dist = old_source / "backend" / "internal" / "web" / "dist"
        pnpm = shutil.which("pnpm")
        if not pnpm or not (old_frontend / "package.json").is_file():
            raise RuntimeError("old frontend source or pnpm is unavailable")
        frontend_env = {
            **os.environ,
            "CI": "true",
            "PNPM_CONFIG_DANGEROUSLY_ALLOW_ALL_BUILDS": "true",
            "VITE_API_BASE_URL": base_url + "/api/v1",
        }
        for command in ([pnpm, "install", "--frozen-lockfile"], [pnpm, "run", "build"]):
            build = subprocess.run(
                command,
                cwd=old_frontend,
                env=frontend_env,
                check=False,
                capture_output=True,
                text=True,
            )
            if build.returncode != 0:
                detail = (build.stderr or build.stdout).strip()
                raise RuntimeError(f"old frontend build failed: {detail[-2000:]}")
        load_fixture_secrets(data_new / "diff.env")
        stop_new_gateway_container()
        set_durable_mode(dsn, "off")
        result = subprocess.run(
            docker_gateway_command(
                "sub2api-settlement-new-linux", "server-old", data_new,
                58082, 55442, "sub2api_new", 56382, False,
                extra_env={"CORS_ALLOWED_ORIGINS": "http://127.0.0.1:58171"},
            ),
            check=False,
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            raise RuntimeError(result.stderr.strip() or "failed to start old binary against additive schema")
        wait_for_health(base_url, 45)
        frontend_url = "http://127.0.0.1:58171"
        frontend_log_path = LOCAL_RUNTIME_ROOT / "logs" / "old-frontend-preview.log"
        frontend_log_path.parent.mkdir(parents=True, exist_ok=True)
        frontend_log = frontend_log_path.open("wb")
        frontend_process = subprocess.Popen(
            [pnpm, "exec", "vite", "preview", "--host", "127.0.0.1", "--port", "58171", "--strictPort"],
            cwd=old_frontend,
            stdout=frontend_log,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
        frontend_html = ""
        frontend_content_type = ""
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            if frontend_process.poll() is not None:
                raise RuntimeError("old frontend preview exited before becoming ready")
            try:
                request = urllib.request.Request(frontend_url + "/login", method="GET")
                with urllib.request.urlopen(request, timeout=2) as response:
                    frontend_content_type = response.headers.get("Content-Type", "")
                    frontend_html = response.read().decode("utf-8", errors="replace")
                break
            except urllib.error.URLError:
                time.sleep(0.05)
        else:
            raise RuntimeError("old frontend preview did not become ready")
        asset_path = ""
        for fragment in frontend_html.split('src="')[1:]:
            candidate = fragment.split('"', 1)[0]
            if candidate.startswith("/assets/"):
                asset_path = candidate
                break
        asset_status = 0
        if asset_path:
            with urllib.request.urlopen(frontend_url + asset_path, timeout=5) as asset_response:
                asset_status = asset_response.status
                asset_response.read(1)
        compiled_api_base = (base_url + "/api/v1").encode()
        compiled_api_base_present = any(
            compiled_api_base in path.read_bytes()
            for path in old_dist.glob("assets/*.js")
        )
        public_status, public_payload, _ = request_json(
            base_url + "/api/v1/settings/public",
            headers={"Origin": frontend_url},
        )
        login_status, login_payload, _ = request_json(
            base_url + "/api/v1/auth/login",
            "POST",
            {"email": ADMIN_EMAIL, "password": ADMIN_PASSWORD},
            {"Origin": frontend_url},
        )
        cors_request = urllib.request.Request(
            base_url + "/api/v1/settings/public",
            method="OPTIONS",
            headers={
                "Origin": frontend_url,
                "Access-Control-Request-Method": "GET",
            },
        )
        try:
            with urllib.request.urlopen(cors_request, timeout=5) as cors_response:
                cors_status = cors_response.status
                cors_origin = cors_response.headers.get("Access-Control-Allow-Origin", "")
                cors_response.read()
        except urllib.error.HTTPError as error:
            cors_status = error.code
            cors_origin = error.headers.get("Access-Control-Allow-Origin", "") if error.headers else ""
            error.read()
            error.close()
        run_id = "settlement-additive-rollback-" + uuid.uuid4().hex[:12]
        request_result = request_one(base_url, case, run_id)
        marker = f"sub2api-dual-diff/{run_id}/{case['name']}"
        snapshot: dict[str, Any] = {}
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            snapshot = query_billing_state(dsn, "openai", marker)
            if int(snapshot.get("usage_count", 0)) == 1:
                break
            time.sleep(0.05)
        checks = {
            "old_frontend_built": (old_dist / "index.html").is_file(),
            "old_frontend_login_route_served": "text/html" in frontend_content_type and bool(frontend_html),
            "old_frontend_asset_served": asset_status == 200,
            "old_frontend_targets_old_backend": compiled_api_base_present,
            "old_frontend_public_settings_succeeded": public_status == 200 and isinstance(public_payload, dict),
            "old_frontend_login_succeeded": login_status == 200 and isinstance(login_payload, dict),
            "old_frontend_cors_preflight_succeeded": cors_status in {200, 204} and cors_origin in {"*", frontend_url},
            "old_binary_started_with_additive_schema": True,
            "old_binary_request_succeeded": request_result.get("status") == 200,
            "old_binary_usage_persisted": int(snapshot.get("usage_count", 0)) == 1,
        }
        report = {
            "old_ref": OLD_BASELINE_REF,
            "frontend_url": frontend_url,
            "backend_url": base_url,
            "frontend_probes": {
                "login_route_content_type": frontend_content_type,
                "asset_status": asset_status,
                "public_settings_status": public_status,
                "public_settings_payload_type": type(public_payload).__name__,
                "login_status": login_status,
                "login_payload_type": type(login_payload).__name__,
                "cors_status": cors_status,
                "cors_origin": cors_origin,
            },
            "run_id": run_id,
            "request": request_result,
            "snapshot": snapshot,
            "checks": checks,
            "passed": all(checks.values()),
        }
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps({"passed": report["passed"], "checks": checks}, ensure_ascii=False, indent=2))
        return 0 if report["passed"] else 1
    except (OSError, RuntimeError, ValueError, KeyError, json.JSONDecodeError, urllib.error.URLError) as error:
        print(f"additive schema rollback failed: {error}", file=sys.stderr)
        return 1
    finally:
        if frontend_process is not None and frontend_process.poll() is None:
            try:
                os.killpg(frontend_process.pid, signal.SIGTERM)
                frontend_process.wait(timeout=5)
            except (OSError, subprocess.TimeoutExpired):
                try:
                    os.killpg(frontend_process.pid, signal.SIGKILL)
                except OSError:
                    pass
        if frontend_log is not None:
            frontend_log.close()
        try:
            stop_new_gateway_container()
        except RuntimeError:
            pass
        try:
            set_durable_mode(dsn, "off")
            start_new_gateway_container()
            restored = True
        except (OSError, RuntimeError, ValueError) as restore_error:
            print(f"warning: failed to restore new gateway after rollback probe: {restore_error}", file=sys.stderr)
        if report:
            report["new_binary_restored"] = restored
            try:
                output.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
            except OSError:
                pass


def dual_up(
    old_ref: str = OLD_BASELINE_REF,
    old_binary: pathlib.Path | None = None,
    new_binary: pathlib.Path | None = None,
) -> int:
    if LOCAL_RUNTIME_MANIFEST.exists():
        print(f"dual runtime already exists: {LOCAL_RUNTIME_MANIFEST}; run dual-down first", file=sys.stderr)
        return 1
    try:
        ensure_dual_ports_free()
        goarch = docker_goarch()
        LOCAL_RUNTIME_ROOT.mkdir(parents=True, exist_ok=False)
        if (old_binary is None) != (new_binary is None):
            raise ValueError("--old-binary and --new-binary must be provided together")
        use_prebuilt = old_binary is not None and new_binary is not None
        if use_prebuilt:
            old_binary = old_binary.resolve()
            new_binary = new_binary.resolve()
            if not old_binary.is_file() or not new_binary.is_file():
                raise ValueError("prebuilt old/new binary must both exist")
        old_source = LOCAL_RUNTIME_ROOT / "source-old"
        bin_dir = LOCAL_RUNTIME_ROOT / "bin"
        tls_dir = LOCAL_RUNTIME_ROOT / "tls"
        data_old = LOCAL_RUNTIME_ROOT / "data-old"
        data_new = LOCAL_RUNTIME_ROOT / "data-new"
        for directory in (bin_dir, tls_dir, data_old, data_new, LOCAL_RUNTIME_ROOT / "logs"):
            directory.mkdir(parents=True, exist_ok=True)
        manifest: dict[str, Any] = {
            "old_ref": old_ref,
            "old_source": "" if use_prebuilt else str(old_source),
            "fixture_pids": [],
            "containers": [],
            "goarch": goarch,
        }
        LOCAL_RUNTIME_MANIFEST.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")

        if use_prebuilt:
            shutil.copy2(old_binary, bin_dir / "server-old")
            shutil.copy2(new_binary, bin_dir / "server-new")
            (bin_dir / "server-old").chmod(0o755)
            (bin_dir / "server-new").chmod(0o755)
        else:
            subprocess.run(["git", "worktree", "add", "--detach", str(old_source), old_ref], cwd=ROOT, check=True)
            build_env = {**os.environ, "CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": goarch}
            subprocess.run(
                ["go", "build", "-trimpath", "-o", str(bin_dir / "server-old"), "./cmd/server"],
                cwd=old_source / "backend",
                env=build_env,
                check=True,
            )
            subprocess.run(
                ["go", "build", "-trimpath", "-o", str(bin_dir / "server-new"), "./cmd/server"],
                cwd=ROOT / "backend",
                env=build_env,
                check=True,
            )
        if infra_up() != 0:
            raise RuntimeError("isolated PostgreSQL/Redis startup failed")
        if generate_fixture_tls_cert() != 0:
            raise RuntimeError("fixture TLS certificate generation failed")
        shutil.copy2(NATIVE_ROOT / "tls" / "fixture-ca.crt", tls_dir / "fixture-ca.crt")
        shutil.copy2(NATIVE_ROOT / "tls" / "fixture-ca.key", tls_dir / "fixture-ca.key")
        (tls_dir / "fixture-ca.key").chmod(0o600)

        fake = ROOT / "tools" / "usage_settlement_fake_upstream.py"
        http_pid = start_fixture_process(
            [sys.executable, str(fake), "--host", "127.0.0.1", "--port", "59000"],
            LOCAL_RUNTIME_ROOT / "logs" / "fixture-http.log",
        )
        tls_pid = start_fixture_process(
            [
                sys.executable, str(fake), "--host", "127.0.0.1", "--port", "59001",
                "--tls-cert", str(tls_dir / "fixture-ca.crt"),
                "--tls-key", str(tls_dir / "fixture-ca.key"),
                "--connect-proxy-port", "59002",
            ],
            LOCAL_RUNTIME_ROOT / "logs" / "fixture-tls.log",
        )
        manifest["fixture_pids"] = [http_pid, tls_pid]
        LOCAL_RUNTIME_MANIFEST.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            try:
                if request_json(INFRA["fixture"] + "/health")[0] == 200:
                    break
            except urllib.error.URLError:
                pass
            time.sleep(0.05)
        else:
            raise RuntimeError("HTTP fixture did not become healthy")

        containers = (
            ("sub2api-settlement-old-linux", "server-old", data_old, 58081, 55441, "sub2api_old", 56381),
            ("sub2api-settlement-new-linux", "server-new", data_new, 58082, 55442, "sub2api_new", 56382),
        )
        for name, binary, data_dir, port, db_port, database, redis_port in containers:
            result = subprocess.run(
                docker_gateway_command(name, binary, data_dir, port, db_port, database, redis_port, True),
                check=False,
                capture_output=True,
                text=True,
            )
            if result.returncode != 0:
                raise RuntimeError(result.stderr.strip() or f"failed to start {name}")
            manifest["containers"].append(name)
            LOCAL_RUNTIME_MANIFEST.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")

        wait_for_health("http://127.0.0.1:58081", 45)
        wait_for_health("http://127.0.0.1:58082", 45)
        previous_fixture_host = os.environ.get("SUB2API_SETTLEMENT_FIXTURE_HOST")
        previous_proxy_host = os.environ.get("SUB2API_SETTLEMENT_PROXY_HOST")
        fixture_host = "127.0.0.1" if sys.platform.startswith("linux") else "host.docker.internal"
        os.environ["SUB2API_SETTLEMENT_FIXTURE_HOST"] = fixture_host
        os.environ["SUB2API_SETTLEMENT_PROXY_HOST"] = fixture_host
        try:
            if seed_fixture_instance("http://127.0.0.1:58081", ADMIN_EMAIL, ADMIN_PASSWORD, data_old / "diff.env") != 0:
                raise RuntimeError("old gateway fixture seed failed")
            if seed_fixture_instance("http://127.0.0.1:58082", ADMIN_EMAIL, ADMIN_PASSWORD, data_new / "diff.env") != 0:
                raise RuntimeError("new gateway fixture seed failed")
            for dsn in (INFRA["old_postgres"], INFRA["new_postgres"]):
                execute_loopback_sql(dsn, "UPDATE scheduled_test_plans SET enabled = FALSE, updated_at = NOW();")
        finally:
            if previous_fixture_host is None:
                os.environ.pop("SUB2API_SETTLEMENT_FIXTURE_HOST", None)
            else:
                os.environ["SUB2API_SETTLEMENT_FIXTURE_HOST"] = previous_fixture_host
            if previous_proxy_host is None:
                os.environ.pop("SUB2API_SETTLEMENT_PROXY_HOST", None)
            else:
                os.environ["SUB2API_SETTLEMENT_PROXY_HOST"] = previous_proxy_host
        print(json.dumps({
            "old_ref": old_ref,
            "old_url": "http://127.0.0.1:58081",
            "new_url": "http://127.0.0.1:58082",
            "runtime_root": str(LOCAL_RUNTIME_ROOT),
            "manifest": str(LOCAL_RUNTIME_MANIFEST),
        }, ensure_ascii=False, indent=2))
        return 0
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"dual-up failed: {error}", file=sys.stderr)
        docker = shutil.which("docker")
        if docker:
            for name in ("sub2api-settlement-old-linux", "sub2api-settlement-new-linux"):
                result = subprocess.run(
                    [docker, "logs", name],
                    check=False,
                    capture_output=True,
                    text=True,
                )
                output = (result.stdout + result.stderr).strip()
                if output:
                    print(f"--- {name} logs ---\n{output[-12000:]}", file=sys.stderr)
        dual_down()
        return 1


def stop_fixture_pid(pid: int) -> None:
    try:
        os.killpg(pid, signal.SIGTERM)
    except (ProcessLookupError, PermissionError):
        return
    deadline = time.monotonic() + 3
    while time.monotonic() < deadline:
        try:
            os.kill(pid, 0)
        except (ProcessLookupError, PermissionError):
            return
        time.sleep(0.05)
    try:
        os.killpg(pid, signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        pass


def dual_down() -> int:
    manifest: dict[str, Any] = {}
    if LOCAL_RUNTIME_MANIFEST.exists():
        try:
            value = json.loads(LOCAL_RUNTIME_MANIFEST.read_text(encoding="utf-8"))
            if isinstance(value, dict):
                manifest = value
        except (OSError, json.JSONDecodeError):
            pass
    docker = shutil.which("docker")
    container_names = set(manifest.get("containers", []))
    container_names.update({"sub2api-settlement-old-linux", "sub2api-settlement-new-linux"})
    if docker:
        for name in sorted(container_names):
            if not str(name).startswith("sub2api-settlement-"):
                continue
            subprocess.run([docker, "rm", "-f", str(name)], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for raw_pid in manifest.get("fixture_pids", []):
        try:
            stop_fixture_pid(int(raw_pid))
        except (TypeError, ValueError):
            continue
    infra_down()
    old_source_value = str(manifest.get("old_source", "")).strip()
    if old_source_value:
        old_source = pathlib.Path(old_source_value)
        if old_source.exists():
            subprocess.run(["git", "worktree", "remove", "--force", str(old_source)], cwd=ROOT, check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if LOCAL_RUNTIME_ROOT.exists():
        shutil.rmtree(LOCAL_RUNTIME_ROOT)
    print(json.dumps({"removed": str(LOCAL_RUNTIME_ROOT)}, ensure_ascii=False))
    return 0


def doctor() -> int:
    compose = detect_compose_command()
    native = native_commands()
    checks = {
        "python": sys.version.split()[0],
        "docker": shutil.which("docker") or "",
        "compose": " ".join(compose),
        "native_tools": native,
        "compose_file": str(COMPOSE_FILE) if COMPOSE_FILE.exists() else "",
        "diff_tool": str(DIFF_TOOL) if DIFF_TOOL.exists() else "",
    }
    checks["ok"] = bool((checks["compose"] or all(native.values())) and checks["compose_file"] and checks["diff_tool"])
    checks["infra"] = INFRA
    checks["fixture_scenarios"] = sorted(SCENARIOS)
    print(json.dumps(checks, ensure_ascii=False, indent=2))
    return 0 if checks["ok"] else 1


def main() -> int:
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("doctor")
    sub.add_parser("infra-up")
    sub.add_parser("infra-down")
    sub.add_parser("fixture-tls-cert")
    dual_start = sub.add_parser("dual-up")
    dual_start.add_argument("--old-ref", default=OLD_BASELINE_REF)
    dual_start.add_argument("--old-binary", type=pathlib.Path)
    dual_start.add_argument("--new-binary", type=pathlib.Path)
    sub.add_parser("dual-down")
    smoke = sub.add_parser("fixture-smoke")
    smoke.add_argument("--output")
    diff = sub.add_parser("run-diff")
    diff.add_argument("--old-url", required=True)
    diff.add_argument("--new-url", required=True)
    diff.add_argument("--output", required=True)
    diff.add_argument("--include-ws", action="store_true")
    diff.add_argument("--ws-turns", type=int, default=1)
    disconnect = sub.add_parser("run-disconnect-diff")
    disconnect.add_argument("--old-url", required=True)
    disconnect.add_argument("--new-url", required=True)
    disconnect.add_argument("--old-dsn", required=True)
    disconnect.add_argument("--new-dsn", required=True)
    disconnect.add_argument("--output", required=True)
    short_paths = sub.add_parser("run-short-path-diff")
    short_paths.add_argument("--old-url", required=True)
    short_paths.add_argument("--new-url", required=True)
    short_paths.add_argument("--old-dsn", required=True)
    short_paths.add_argument("--new-dsn", required=True)
    short_paths.add_argument("--output", required=True)
    scheduler_fault = sub.add_parser("run-scheduler-fault-diff")
    scheduler_fault.add_argument("--old-url", required=True)
    scheduler_fault.add_argument("--new-url", required=True)
    scheduler_fault.add_argument("--old-dsn", required=True)
    scheduler_fault.add_argument("--new-dsn", required=True)
    scheduler_fault.add_argument("--old-secrets", required=True)
    scheduler_fault.add_argument("--new-secrets", required=True)
    scheduler_fault.add_argument("--scenario", required=True, choices=("http_401", "http_403", "http_429", "http_500"))
    scheduler_fault.add_argument("--output", required=True)
    snapshot = sub.add_parser("snapshot")
    snapshot.add_argument("--dsn", required=True)
    snapshot.add_argument("--marker-prefix", required=True)
    snapshot.add_argument("--output")
    seed = sub.add_parser("seed-instance")
    seed.add_argument("--base-url", required=True)
    seed.add_argument("--admin-email", required=True)
    seed.add_argument("--admin-password-env", default="SUB2API_SETTLEMENT_ADMIN_PASSWORD")
    seed.add_argument("--secrets-output", required=True)
    baseline = sub.add_parser("run-baseline")
    baseline.add_argument("--base-url", required=True)
    baseline.add_argument("--dsn", required=True)
    baseline.add_argument("--output", required=True)
    crash = sub.add_parser("run-terminal-crash")
    crash.add_argument("--base-url", required=True)
    crash.add_argument("--dsn", required=True)
    crash.add_argument("--case", required=True)
    crash.add_argument("--container", required=True)
    crash.add_argument("--output", required=True)
    restart = sub.add_parser("verify-crash-restart")
    restart.add_argument("--base-url", required=True)
    restart.add_argument("--dsn", required=True)
    restart.add_argument("--report", required=True)
    durable_crash = sub.add_parser("run-durable-crash-matrix")
    durable_crash.add_argument("--output", required=True)
    missing_terminal = sub.add_parser("run-missing-terminal-reconciliation")
    missing_terminal.add_argument("--output", required=True)
    stored_recovery = sub.add_parser("run-stored-response-recovery")
    stored_recovery.add_argument("--output", required=True)
    shadow = sub.add_parser("run-shadow-reconciliation")
    shadow.add_argument("--output", required=True)
    group_rollout = sub.add_parser("run-group-rollout-gate")
    group_rollout.add_argument("--output", required=True)
    rollback = sub.add_parser("run-additive-schema-rollback")
    rollback.add_argument("--output", required=True)
    baseline_diff = sub.add_parser("compare-baselines")
    baseline_diff.add_argument("--old", required=True)
    baseline_diff.add_argument("--new", required=True)
    baseline_diff.add_argument("--output", required=True)
    args = parser.parse_args()

    if args.command == "doctor":
        return doctor()
    if args.command == "infra-up":
        return infra_up()
    if args.command == "infra-down":
        return infra_down()
    if args.command == "fixture-tls-cert":
        return generate_fixture_tls_cert()
    if args.command == "dual-up":
        return dual_up(args.old_ref, args.old_binary, args.new_binary)
    if args.command == "dual-down":
        return dual_down()
    if args.command == "fixture-smoke":
        return fixture_smoke(pathlib.Path(args.output) if args.output else None)
    if args.command == "run-diff":
        command = [sys.executable, str(DIFF_TOOL), "--old-url", args.old_url, "--new-url", args.new_url, "--output", args.output]
        if args.include_ws:
            command.append("--include-ws")
            command.extend(["--ws-turns", str(args.ws_turns)])
        return run_checked(command)
    if args.command == "run-disconnect-diff":
        return run_disconnect_diff(
            args.old_url,
            args.new_url,
            args.old_dsn,
            args.new_dsn,
            pathlib.Path(args.output),
        )
    if args.command == "run-short-path-diff":
        return run_short_path_diff(
            args.old_url,
            args.new_url,
            args.old_dsn,
            args.new_dsn,
            pathlib.Path(args.output),
        )
    if args.command == "run-scheduler-fault-diff":
        return run_scheduler_fault_diff(
            args.old_url,
            args.new_url,
            args.old_dsn,
            args.new_dsn,
            pathlib.Path(args.old_secrets),
            pathlib.Path(args.new_secrets),
            args.scenario,
            pathlib.Path(args.output),
        )
    if args.command == "snapshot":
        return capture_snapshot(args.dsn, args.marker_prefix, pathlib.Path(args.output) if args.output else None)
    if args.command == "seed-instance":
        password = os.environ.get(args.admin_password_env, "")
        if not password:
            print(f"missing admin password environment variable: {args.admin_password_env}", file=sys.stderr)
            return 1
        return seed_fixture_instance(args.base_url, args.admin_email, password, pathlib.Path(args.secrets_output))
    if args.command == "run-baseline":
        return run_single_baseline(args.base_url, args.dsn, pathlib.Path(args.output))
    if args.command == "run-terminal-crash":
        return run_terminal_crash(args.base_url, args.dsn, args.case, args.container, pathlib.Path(args.output))
    if args.command == "verify-crash-restart":
        return verify_crash_restart(args.base_url, args.dsn, pathlib.Path(args.report))
    if args.command == "run-durable-crash-matrix":
        return run_durable_crash_matrix(pathlib.Path(args.output))
    if args.command == "run-missing-terminal-reconciliation":
        return run_missing_terminal_reconciliation(pathlib.Path(args.output))
    if args.command == "run-stored-response-recovery":
        return run_stored_response_recovery(pathlib.Path(args.output))
    if args.command == "run-shadow-reconciliation":
        return run_shadow_reconciliation(pathlib.Path(args.output))
    if args.command == "run-group-rollout-gate":
        return run_group_rollout_gate(pathlib.Path(args.output))
    if args.command == "run-additive-schema-rollback":
        return run_additive_schema_rollback(pathlib.Path(args.output))
    if args.command == "compare-baselines":
        return compare_baseline_reports(pathlib.Path(args.old), pathlib.Path(args.new), pathlib.Path(args.output))
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
