from __future__ import annotations

import json
import logging
from datetime import datetime, timezone
from typing import Any

import httpx

from hakimi_restock.config import ImportConfig, PoolConfig, Sub2APIConfig

logger = logging.getLogger(__name__)


class Sub2APIError(RuntimeError):
    def __init__(self, message: str, status_code: int = 0, payload: Any = None) -> None:
        super().__init__(message)
        self.status_code = status_code
        self.payload = payload


class Sub2APIClient:
    def __init__(self, cfg: Sub2APIConfig, timeout: float = 60.0) -> None:
        headers = {"Accept": "application/json"}
        if cfg.admin_api_key:
            headers["x-api-key"] = cfg.admin_api_key
        elif cfg.jwt:
            headers["Authorization"] = f"Bearer {cfg.jwt}"
        self._client = httpx.Client(
            base_url=cfg.base_url.rstrip("/") + "/",
            timeout=timeout,
            headers=headers,
        )

    def close(self) -> None:
        self._client.close()

    def _unwrap(self, response: httpx.Response) -> Any:
        try:
            payload = response.json()
        except json.JSONDecodeError as exc:
            raise Sub2APIError(
                f"Sub2API returned non-JSON ({response.status_code})",
                response.status_code,
                response.text[:200],
            ) from exc
        if response.status_code >= 400 or (isinstance(payload, dict) and payload.get("code") not in (0, None) and payload.get("code") != 0):
            message = ""
            if isinstance(payload, dict):
                message = str(payload.get("message") or payload.get("reason") or "")
            raise Sub2APIError(message or f"HTTP {response.status_code}", response.status_code, payload)
        if isinstance(payload, dict) and "data" in payload:
            return payload["data"]
        return payload

    def get_json(self, path: str, params: dict[str, Any] | None = None) -> Any:
        response = self._client.get(path.lstrip("/"), params=params)
        return self._unwrap(response)

    def post_json(self, path: str, body: dict[str, Any], *, idempotency_key: str | None = None) -> Any:
        headers = {"Content-Type": "application/json"}
        if idempotency_key:
            headers["Idempotency-Key"] = idempotency_key
        response = self._client.post(path.lstrip("/"), json=body, headers=headers)
        return self._unwrap(response)

    def list_accounts(
        self,
        *,
        platform: str,
        account_type: str,
        group_id: int = 0,
        status: str = "",
        page_size: int = 200,
    ) -> list[dict[str, Any]]:
        items: list[dict[str, Any]] = []
        page = 1
        while True:
            params: dict[str, Any] = {
                "page": page,
                "page_size": page_size,
                "platform": platform,
                "type": account_type,
                "lite": "true",
            }
            if status:
                params["status"] = status
            if group_id:
                params["group"] = group_id
            data = self.get_json("api/v1/admin/accounts", params)
            batch = data.get("items") if isinstance(data, dict) else data
            batch = batch or []
            items.extend(batch)
            total = int(data.get("total") or 0) if isinstance(data, dict) else len(items)
            if len(items) >= total or not batch:
                break
            page += 1
            if page > 100:
                break
        return items

    def import_codex_session(self, content: str, cfg: ImportConfig, *, idempotency_key: str) -> dict[str, Any]:
        body: dict[str, Any] = {
            "content": content,
            "concurrency": cfg.concurrency,
            "priority": cfg.priority,
            "update_existing": cfg.update_existing,
            "skip_default_group_bind": cfg.skip_default_group_bind,
            "confirm_mixed_channel_risk": cfg.confirm_mixed_channel_risk,
            "auto_pause_on_expired": cfg.auto_pause_on_expired,
        }
        if cfg.group_ids:
            body["group_ids"] = cfg.group_ids
        if cfg.proxy_id is not None:
            body["proxy_id"] = cfg.proxy_id
        return self.post_json(
            "api/v1/admin/accounts/import/codex-session",
            body,
            idempotency_key=idempotency_key,
        )


def count_schedulable(accounts: list[dict[str, Any]], pool: PoolConfig) -> dict[str, int]:
    now = datetime.now(timezone.utc)
    total = 0
    schedulable = 0
    temp_down = 0
    expired = 0
    for account in accounts:
        total += 1
        if account.get("status") not in ("", "active", None):
            continue
        expires_at = account.get("expires_at")
        if account.get("auto_pause_on_expired") and expires_at:
            try:
                if int(expires_at) > 0 and int(expires_at) < int(now.timestamp()):
                    expired += 1
                    continue
            except (TypeError, ValueError):
                pass
        until = account.get("temp_unschedulable_until")
        if pool.count_temp_unschedulable_as_down and _is_future(until, now):
            temp_down += 1
            continue
        if account.get("schedulable") is True:
            schedulable += 1
    return {
        "total": total,
        "schedulable": schedulable,
        "temp_down": temp_down,
        "expired": expired,
    }


def _is_future(value: Any, now: datetime) -> bool:
    if not value:
        return False
    if isinstance(value, datetime):
        ts = value if value.tzinfo else value.replace(tzinfo=timezone.utc)
        return ts > now
    if isinstance(value, (int, float)):
        stamp = float(value)
        if stamp > 1e12:
            stamp /= 1000
        return datetime.fromtimestamp(stamp, tz=timezone.utc) > now
    text = str(value).strip()
    if not text:
        return False
    try:
        parsed = datetime.fromisoformat(text.replace("Z", "+00:00"))
    except ValueError:
        return True
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed > now


def summarize_import(result: dict[str, Any]) -> str:
    return (
        f"total={result.get('total', 0)} created={result.get('created', 0)} "
        f"updated={result.get('updated', 0)} skipped={result.get('skipped', 0)} "
        f"failed={result.get('failed', 0)}"
    )
