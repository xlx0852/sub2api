#!/usr/bin/env python3
"""Run identical gateway requests against two isolated Sub2API instances.

API keys are read from environment variables and are never written to the report.
The report compares transport status, JSON/SSE schema, terminal event, model, and
canonical usage fields. Persisted usage rows are intentionally reconciled by a
separate database query keyed by the emitted User-Agent marker.
"""

from __future__ import annotations

import argparse
import asyncio
import concurrent.futures
import datetime as dt
import hashlib
import json
import os
import pathlib
import sys
import urllib.error
import urllib.request
import uuid
from typing import Any


CASES = (
    {
        "name": "anthropic_json",
        "platform": "anthropic",
        "path": "/v1/messages",
        "body": {
            "model": "deepseek-v4-flash",
            "max_tokens": 1,
            "messages": [{"role": "user", "content": "Reply only OK"}],
            "stream": False,
        },
    },
    {
        "name": "anthropic_sse",
        "platform": "anthropic",
        "path": "/v1/messages",
        "body": {
            "model": "deepseek-v4-flash",
            "max_tokens": 2,
            "messages": [{"role": "user", "content": "Reply only OK"}],
            "stream": True,
        },
    },
    {
        "name": "openai_json",
        "platform": "openai",
        "path": "/v1/responses",
        "body": {
            "model": "gpt-4o-mini",
            "input": "Reply only OK",
            "max_output_tokens": 1,
            "stream": False,
        },
    },
    {
        "name": "openai_sse",
        "platform": "openai",
        "path": "/v1/responses",
        "body": {
            "model": "gpt-4o-mini",
            "input": "Reply only OK",
            "max_output_tokens": 2,
            "stream": True,
        },
    },
    {
        "name": "antigravity_json",
        "platform": "antigravity",
        "path": "/antigravity/v1/messages",
        "body": {
            "model": "gemini-2.5-pro",
            "max_tokens": 1,
            "messages": [{"role": "user", "content": "Reply only OK"}],
            "stream": False,
        },
    },
    {
        "name": "antigravity_sse",
        "platform": "antigravity",
        "path": "/antigravity/v1/messages",
        "body": {
            "model": "gemini-2.5-pro",
            "max_tokens": 2,
            "messages": [{"role": "user", "content": "Reply only OK"}],
            "stream": True,
        },
    },
    {
        "name": "grok_json",
        "platform": "grok",
        "path": "/v1/responses",
        "body": {
            "model": "grok-4.5",
            "input": "Reply only OK",
            "max_output_tokens": 1,
            "stream": False,
        },
    },
    {
        "name": "grok_sse",
        "platform": "grok",
        "path": "/v1/responses",
        "body": {
            "model": "grok-4.5",
            "input": "Reply only OK",
            "max_output_tokens": 2,
            "stream": True,
        },
    },
    {
        "name": "kimi_json",
        "platform": "kimi",
        "path": "/v1/chat/completions",
        "body": {
            "model": "k3",
            "messages": [{"role": "user", "content": "Reply only OK"}],
            "max_tokens": 1,
            "stream": False,
        },
    },
    {
        "name": "kimi_sse",
        "platform": "kimi",
        "path": "/v1/chat/completions",
        "body": {
            "model": "k3",
            "messages": [{"role": "user", "content": "Reply only OK"}],
            "max_tokens": 2,
            "stream": True,
        },
    },
)

SHORT_PATH_CASES = (
    {
        "name": "anthropic_passthrough_json",
        "platform": "anthropic_passthrough",
        "path": "/v1/messages",
        "body": {
            "model": "claude-sonnet-4-5",
            "max_tokens": 1,
            "messages": [{"role": "user", "content": "Reply only OK"}],
            "stream": False,
        },
    },
    {
        "name": "anthropic_passthrough_sse",
        "platform": "anthropic_passthrough",
        "path": "/v1/messages",
        "body": {
            "model": "claude-sonnet-4-5",
            "max_tokens": 2,
            "messages": [{"role": "user", "content": "Reply only OK"}],
            "stream": True,
        },
    },
    {
        "name": "raw_cc_json",
        "platform": "kimi",
        "path": "/v1/chat/completions",
        "body": {
            "model": "k3",
            "messages": [{"role": "user", "content": "Reply only OK"}],
            "max_tokens": 1,
            "stream": False,
        },
    },
    {
        "name": "raw_cc_sse",
        "platform": "kimi",
        "path": "/v1/chat/completions",
        "body": {
            "model": "k3",
            "messages": [{"role": "user", "content": "Reply only OK"}],
            "max_tokens": 2,
            "stream": True,
        },
    },
)

KEY_ENV = {
    "anthropic": "SUB2API_DIFF_KEY_ANTHROPIC",
    "openai": "SUB2API_DIFF_KEY_OPENAI",
    "antigravity": "SUB2API_DIFF_KEY_ANTIGRAVITY",
    "grok": "SUB2API_DIFF_KEY_GROK",
    "kimi": "SUB2API_DIFF_KEY_KIMI",
    "anthropic_passthrough": "SUB2API_DIFF_KEY_ANTHROPIC_PASSTHROUGH",
}


def utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat()


def schema_paths(value: Any, prefix: str = "$") -> set[str]:
    if isinstance(value, dict):
        if not value:
            return {prefix + "{}"}
        result: set[str] = set()
        for key, child in value.items():
            result.update(schema_paths(child, f"{prefix}.{key}"))
        return result
    if isinstance(value, list):
        if not value:
            return {prefix + "[]"}
        result = set()
        for child in value:
            result.update(schema_paths(child, prefix + "[]"))
        return result
    return {prefix}


def normalized_protocol_value(value: Any, parent_key: str = "") -> Any:
    """Keep protocol/control values while masking upstream-generated volatility."""
    if parent_key == "usage":
        return "<usage>"
    if isinstance(value, dict):
        return {key: normalized_protocol_value(child, key) for key, child in sorted(value.items())}
    if isinstance(value, list):
        return [normalized_protocol_value(child, parent_key) for child in value]
    if isinstance(value, str):
        if parent_key in {
            "id",
            "request_id",
            "response_id",
            "item_id",
            "system_fingerprint",
            "created_at",
            "completed_at",
            "timestamp",
        }:
            return f"<{parent_key}>"
        if parent_key in {
            "text",
            "content",
            "delta",
            "output_text",
            "reasoning_content",
            "encrypted_content",
        }:
            return "<generated_text>"
    if isinstance(value, (int, float)) and parent_key in {"created", "sequence_number"}:
        return f"<{parent_key}>"
    return value


def normalized_digest(value: Any) -> str:
    normalized = normalized_protocol_value(value)
    encoded = json.dumps(normalized, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(encoded).hexdigest()


def find_usage(value: Any) -> dict[str, Any] | None:
    candidates: list[dict[str, Any]] = []

    def walk(node: Any) -> None:
        if isinstance(node, dict):
            usage = node.get("usage")
            if isinstance(usage, dict):
                candidates.append(usage)
            for child in node.values():
                walk(child)
        elif isinstance(node, list):
            for child in node:
                walk(child)

    walk(value)
    if not candidates:
        return None

    # Streaming protocols can emit an initial usage object with output_tokens=0
    # and a final object with the completed count. Prefer the most complete/latest
    # candidate instead of the first object with the largest number of fields.
    def score(indexed: tuple[int, dict[str, Any]]) -> tuple[float, float, int, int]:
        index, item = indexed
        output_tokens = item.get("output_tokens", item.get("completion_tokens", 0))
        total_tokens = item.get("total_tokens", 0)
        numeric_fields = sum(isinstance(value, (int, float)) for value in item.values())
        return (
            float(output_tokens or 0),
            float(total_tokens or 0),
            numeric_fields,
            index,
        )

    return max(enumerate(candidates), key=score)[1]


def canonical_usage(raw: dict[str, Any] | None) -> dict[str, Any] | None:
    if raw is None:
        return None
    input_details = raw.get("input_tokens_details") or raw.get("prompt_tokens_details") or {}
    output_details = raw.get("output_tokens_details") or raw.get("completion_tokens_details") or {}
    return {
        "input_tokens": raw.get("input_tokens", raw.get("prompt_tokens")),
        "output_tokens": raw.get("output_tokens", raw.get("completion_tokens")),
        "total_tokens": raw.get("total_tokens"),
        "cache_read_tokens": raw.get(
            "cache_read_input_tokens",
            raw.get("cache_read_tokens", input_details.get("cached_tokens")),
        ),
        "cache_creation_tokens": raw.get("cache_creation_input_tokens", raw.get("cache_creation_tokens")),
        "reasoning_tokens": output_details.get("reasoning_tokens"),
    }


def parse_payload(body: bytes, content_type: str) -> tuple[Any, str, int]:
    text = body.decode("utf-8", errors="replace")
    if "text/event-stream" not in content_type.lower():
        try:
            return json.loads(text), "json", 1
        except json.JSONDecodeError:
            return {"_non_json": text[:500]}, "non_json", 0

    events: list[Any] = []
    for line in text.splitlines():
        if not line.startswith("data:"):
            continue
        payload = line[5:].strip()
        if not payload or payload == "[DONE]":
            continue
        try:
            events.append(json.loads(payload))
        except json.JSONDecodeError:
            events.append({"_non_json_event": payload[:500]})
    return events, "sse", len(events)


def semantic_summary(parsed: Any) -> dict[str, Any]:
    nodes = parsed if isinstance(parsed, list) else [parsed]
    event_types: list[str] = []
    models: list[str] = []
    for node in nodes:
        if not isinstance(node, dict):
            continue
        event_type = node.get("type") or node.get("object")
        if isinstance(event_type, str):
            event_types.append(event_type)
        model = node.get("model")
        if isinstance(model, str):
            models.append(model)
        response = node.get("response")
        if isinstance(response, dict):
            if isinstance(response.get("model"), str):
                models.append(response["model"])
            if isinstance(response.get("status"), str):
                event_types.append("response.status:" + response["status"])
    usage = find_usage(parsed)
    return {
        "event_types": sorted(set(event_types)),
        "models": sorted(set(models)),
        "usage": canonical_usage(usage),
    }


def request_one(base_url: str, case: dict[str, Any], run_id: str) -> dict[str, Any]:
    key = os.environ[KEY_ENV[case["platform"]]]
    marker = f"sub2api-dual-diff/{run_id}/{case['name']}"
    request = urllib.request.Request(
        base_url.rstrip("/") + case["path"],
        data=json.dumps(case["body"], separators=(",", ":")).encode(),
        method="POST",
        headers={
            "Authorization": "Bearer " + key,
            "Content-Type": "application/json",
            "Accept": "text/event-stream, application/json",
            "User-Agent": marker,
        },
    )
    started = utc_now()
    try:
        with urllib.request.urlopen(request, timeout=120) as response:
            status = response.status
            content_type = response.headers.get("Content-Type", "")
            body = response.read()
    except urllib.error.HTTPError as error:
        status = error.code
        content_type = error.headers.get("Content-Type", "") if error.headers else ""
        body = error.read()
    except Exception as error:  # noqa: BLE001 - report transport failures verbatim by type only
        return {
            "started_at": started,
            "finished_at": utc_now(),
            "marker": marker,
            "status": 0,
            "content_type": "",
            "format": "transport_error",
            "event_count": 0,
            "schema": [],
            "normalized_sha256": "",
            "semantic": {"event_types": [], "models": [], "usage": None},
            "error": f"{type(error).__name__}: {str(error)[:300]}",
        }

    parsed, payload_format, event_count = parse_payload(body, content_type)
    error_message = ""
    if status >= 400 and isinstance(parsed, dict):
        error = parsed.get("error")
        if isinstance(error, dict):
            error_message = str(error.get("message", ""))
        else:
            error_message = str(parsed.get("message", ""))
    return {
        "started_at": started,
        "finished_at": utc_now(),
        "marker": marker,
        "status": status,
        "content_type": content_type.split(";", 1)[0].strip().lower(),
        "format": payload_format,
        "event_count": event_count,
        "schema": sorted(schema_paths(parsed)),
        "normalized_sha256": normalized_digest(parsed),
        "semantic": semantic_summary(parsed),
        "error": error_message,
    }


async def request_ws_one(base_url: str, run_id: str, turns: int = 1) -> dict[str, Any]:
    try:
        import websockets
    except ImportError as error:
        return {
            "started_at": utc_now(),
            "finished_at": utc_now(),
            "marker": f"sub2api-dual-diff/{run_id}/openai_ws",
            "status": 0,
            "content_type": "",
            "format": "transport_error",
            "event_count": 0,
            "schema": [],
            "normalized_sha256": "",
            "semantic": {"event_types": [], "models": [], "usage": None},
            "error": f"ImportError: {error}",
        }

    marker = f"sub2api-dual-diff/{run_id}/openai_ws"
    ws_url = base_url.rstrip("/").replace("https://", "wss://", 1).replace("http://", "ws://", 1)
    events: list[Any] = []
    completed_ids: list[str] = []
    started = utc_now()
    try:
        async with websockets.connect(
            ws_url + "/v1/responses",
            extra_headers={
                "Authorization": "Bearer " + os.environ[KEY_ENV["openai"]],
                "OpenAI-Beta": "responses_websockets_v2",
            },
            user_agent_header=marker,
            open_timeout=20,
            close_timeout=5,
            max_size=2**20,
        ) as socket:
            previous_response_id = ""
            for turn in range(turns):
                payload = {
                    "type": "response.create",
                    "model": "gpt-4o-mini",
                    "input": f"Reply only OK turn {turn + 1}",
                    "max_output_tokens": 16,
                }
                if previous_response_id:
                    payload["previous_response_id"] = previous_response_id
                await socket.send(json.dumps(payload, separators=(",", ":")))
                terminal = ""
                for _ in range(200):
                    raw = await asyncio.wait_for(socket.recv(), timeout=60)
                    event = json.loads(raw)
                    events.append(event)
                    terminal = str(event.get("type", ""))
                    if terminal in {"response.completed", "response.failed", "error"}:
                        break
                if terminal != "response.completed":
                    break
                response = event.get("response") if isinstance(event.get("response"), dict) else {}
                previous_response_id = str(response.get("id", ""))
                completed_ids.append(previous_response_id)
    except Exception as error:  # noqa: BLE001 - transport errors belong in the report
        return {
            "started_at": started,
            "finished_at": utc_now(),
            "marker": marker,
            "status": getattr(error, "status_code", 0) or 0,
            "content_type": "application/websocket",
            "format": "ws_error",
            "event_count": len(events),
            "schema": sorted(schema_paths(events)),
            "normalized_sha256": normalized_digest(events),
            "semantic": semantic_summary(events),
            "error": f"{type(error).__name__}: {str(error)[:300]}",
        }

    terminal = events[-1].get("type") if events and isinstance(events[-1], dict) else ""
    return {
        "started_at": started,
        "finished_at": utc_now(),
        "marker": marker,
        "status": 101,
        "content_type": "application/websocket",
        "format": "ws",
        "event_count": len(events),
        "schema": sorted(schema_paths(events)),
        "normalized_sha256": normalized_digest(events),
        "semantic": semantic_summary(events),
        "turns_requested": turns,
        "turns_completed": len(completed_ids),
        "completed_response_ids": completed_ids,
        "error": "" if terminal == "response.completed" else f"terminal event: {terminal or 'missing'}",
    }


async def request_ws_pair(old_url: str, new_url: str, run_id: str, turns: int = 1) -> tuple[dict[str, Any], dict[str, Any]]:
    old_result, new_result = await asyncio.gather(
        request_ws_one(old_url, run_id, turns),
        request_ws_one(new_url, run_id, turns),
    )
    return old_result, new_result


def compare(old: dict[str, Any], new: dict[str, Any]) -> dict[str, Any]:
    old_schema = set(old["schema"])
    new_schema = set(new["schema"])
    return {
        "status_equal": old["status"] == new["status"],
        "content_type_equal": old["content_type"] == new["content_type"],
        "format_equal": old["format"] == new["format"],
        "schema_equal": old_schema == new_schema,
        "schema_only_old": sorted(old_schema - new_schema),
        "schema_only_new": sorted(new_schema - old_schema),
        "normalized_equal": old["normalized_sha256"] == new["normalized_sha256"],
        "models_equal": old["semantic"]["models"] == new["semantic"]["models"],
        "usage_equal": old["semantic"]["usage"] == new["semantic"]["usage"],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--old-url", required=True)
    parser.add_argument("--new-url", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--run-id", default="dual-" + uuid.uuid4().hex[:12])
    parser.add_argument("--include-ws", action="store_true")
    parser.add_argument("--only-ws", action="store_true")
    parser.add_argument("--include-short-paths", action="store_true")
    parser.add_argument("--only-short-paths", action="store_true")
    parser.add_argument("--ws-turns", type=int, default=1)
    args = parser.parse_args()

    if args.only_ws and not args.include_ws:
        parser.error("--only-ws requires --include-ws")
    if args.only_short_paths and not args.include_short_paths:
        parser.error("--only-short-paths requires --include-short-paths")
    if args.only_ws and args.only_short_paths:
        parser.error("--only-ws and --only-short-paths are mutually exclusive")
    if args.ws_turns < 1:
        parser.error("--ws-turns must be positive")

    selected_cases = [] if args.only_ws or args.only_short_paths else list(CASES)
    if args.include_short_paths:
        selected_cases.extend(SHORT_PATH_CASES)
    required_platforms = {str(case["platform"]) for case in selected_cases}
    if args.include_ws:
        required_platforms.add("openai")
    missing = [KEY_ENV[platform] for platform in sorted(required_platforms) if not os.environ.get(KEY_ENV[platform])]
    if missing:
        parser.error("missing API key environment variables: " + ", ".join(missing))

    report: dict[str, Any] = {
        "run_id": args.run_id,
        "started_at": utc_now(),
        "old_url": args.old_url,
        "new_url": args.new_url,
        "cases": [],
    }
    for case in selected_cases:
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as executor:
            old_future = executor.submit(request_one, args.old_url, case, args.run_id)
            new_future = executor.submit(request_one, args.new_url, case, args.run_id)
            old_result = old_future.result()
            new_result = new_future.result()
        comparison = compare(old_result, new_result)
        report["cases"].append(
            {
                "name": case["name"],
                "platform": case["platform"],
                "request": case["body"],
                "old": old_result,
                "new": new_result,
                "comparison": comparison,
            }
        )
        flags = [name for name, value in comparison.items() if name.endswith("_equal") and not value]
        print(
            f"{case['name']}: old={old_result['status']} new={new_result['status']} "
            f"diff={','.join(flags) if flags else 'none'}"
        )

    if args.include_ws:
        old_result, new_result = asyncio.run(request_ws_pair(args.old_url, args.new_url, args.run_id, args.ws_turns))
        comparison = compare(old_result, new_result)
        report["cases"].append(
            {
                "name": "openai_ws",
                "platform": "openai",
                "request": {
                    "type": "response.create",
                    "model": "gpt-4o-mini",
                    "input": "Reply only OK",
                    "max_output_tokens": 16,
                },
                "old": old_result,
                "new": new_result,
                "comparison": comparison,
            }
        )
        flags = [name for name, value in comparison.items() if name.endswith("_equal") and not value]
        print(
            f"openai_ws: old={old_result['status']} new={new_result['status']} "
            f"diff={','.join(flags) if flags else 'none'}"
        )

    report["finished_at"] = utc_now()
    transport_success_count = sum(
        1
        for case in report["cases"]
        if case["old"]["status"] in {101, 200} and case["new"]["status"] in {101, 200}
    )
    all_equal = all(
        value
        for case in report["cases"]
        for name, value in case["comparison"].items()
        if name.endswith("_equal")
    )
    report["summary"] = {
        "case_count": len(report["cases"]),
        "transport_success_count": transport_success_count,
        "schema_equal_count": sum(1 for case in report["cases"] if case["comparison"]["schema_equal"]),
        "usage_equal_count": sum(1 for case in report["cases"] if case["comparison"]["usage_equal"]),
        "passed": transport_success_count == len(report["cases"]) and all_equal,
    }
    output = pathlib.Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report["summary"], ensure_ascii=False))
    return 0 if report["summary"]["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
