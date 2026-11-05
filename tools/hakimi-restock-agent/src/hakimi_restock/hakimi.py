from __future__ import annotations

import json
import logging
from typing import Any

import httpx

from hakimi_restock.config import HakimiConfig

logger = logging.getLogger(__name__)


class HakimiError(RuntimeError):
    def __init__(self, message: str, status_code: int = 0, payload: Any = None) -> None:
        super().__init__(message)
        self.status_code = status_code
        self.payload = payload


class HakimiClient:
    def __init__(self, cfg: HakimiConfig, timeout: float = 30.0) -> None:
        self.cfg = cfg
        self._client = httpx.Client(
            base_url=cfg.base_url.rstrip("/") + "/",
            timeout=timeout,
            headers={
                "X-Customer-Token": cfg.token,
                "Accept": "application/json",
            },
        )

    def close(self) -> None:
        self._client.close()

    def _unwrap(self, response: httpx.Response) -> Any:
        try:
            payload = response.json()
        except json.JSONDecodeError as exc:
            raise HakimiError(
                f"Hakimi returned non-JSON ({response.status_code})",
                response.status_code,
                response.text[:200],
            ) from exc

        if response.status_code >= 400:
            error = _extract_error(payload) or f"HTTP {response.status_code}"
            raise HakimiError(error, response.status_code, payload)

        if isinstance(payload, dict) and payload.get("success") is False:
            raise HakimiError(_extract_error(payload) or "Hakimi business error", response.status_code, payload)

        if isinstance(payload, dict) and "data" in payload:
            return payload["data"]
        return payload

    def get_json(self, path: str, params: dict[str, Any] | None = None) -> Any:
        response = self._client.get(path.lstrip("/"), params=_clean_params(params))
        return self._unwrap(response)

    def post_json(
        self,
        path: str,
        body: dict[str, Any] | None = None,
        *,
        idempotency_key: str | None = None,
    ) -> Any:
        headers = {"Content-Type": "application/json"}
        if idempotency_key:
            headers["Idempotency-Key"] = idempotency_key
        response = self._client.post(path.lstrip("/"), json=body or {}, headers=headers)
        return self._unwrap(response)

    def state(self) -> dict[str, Any]:
        return self.get_json("state")

    def inventory(
        self,
        quantity: int,
        *,
        fresh: bool = False,
        source: int | None = None,
        cycle_id: str | None = None,
    ) -> dict[str, Any]:
        params: dict[str, Any] = {
            "quantity": quantity,
            "fresh": str(fresh).lower(),
        }
        if source is not None:
            params["source"] = source
        if cycle_id:
            params["cycle_id"] = cycle_id
        return self.get_json("inventory", params)

    def create_order(
        self,
        body: dict[str, Any],
        *,
        idempotency_key: str,
    ) -> dict[str, Any]:
        return self.post_json("orders", body, idempotency_key=idempotency_key)

    def get_order(self, order_id: str) -> dict[str, Any]:
        return self.get_json(f"orders/{order_id}")

    def list_orders(self, page: int = 1, state: str | None = None) -> dict[str, Any]:
        params: dict[str, Any] = {"page": page}
        if state:
            params["state"] = state
        return self.get_json("orders", params)

    def download_order(self, order_id: str, fmt: str = "sub2") -> bytes:
        response = self._client.get(
            f"orders/{order_id}/download",
            params={"format": fmt},
            headers={"Accept": "*/*"},
        )
        if response.status_code >= 400:
            payload: Any
            try:
                payload = response.json()
            except json.JSONDecodeError:
                payload = response.text[:200]
            raise HakimiError(
                _extract_error(payload) or f"download failed HTTP {response.status_code}",
                response.status_code,
                payload,
            )
        logger.info("downloaded order %s bytes=%s", order_id, len(response.content))
        return response.content

    def recover_order(self, order_id: str) -> dict[str, Any]:
        return self.post_json(f"orders/{order_id}/recover", {})

    def list_recoveries(self, order_id: str | None = None) -> Any:
        params = {"order_id": order_id} if order_id else None
        return self.get_json("recoveries", params)

    def download_recovery(self, recovery_id: str, fmt: str = "sub2") -> bytes:
        response = self._client.get(
            f"recoveries/{recovery_id}/download",
            params={"format": fmt},
            headers={"Accept": "*/*"},
        )
        if response.status_code >= 400:
            payload: Any
            try:
                payload = response.json()
            except json.JSONDecodeError:
                payload = response.text[:200]
            raise HakimiError(
                _extract_error(payload) or f"recovery download failed HTTP {response.status_code}",
                response.status_code,
                payload,
            )
        return response.content

    def download_care(self, order_id: str, fmt: str = "sub2") -> bytes:
        response = self._client.get(
            f"orders/{order_id}/care-download",
            params={"format": fmt},
            headers={"Accept": "*/*"},
        )
        if response.status_code >= 400:
            payload: Any
            try:
                payload = response.json()
            except json.JSONDecodeError:
                payload = response.text[:200]
            raise HakimiError(
                _extract_error(payload) or f"care download failed HTTP {response.status_code}",
                response.status_code,
                payload,
            )
        return response.content


def quote_inventory(inventory: dict[str, Any], quantity: int, budget_cents: int) -> tuple[int, int]:
    """Return (buy_quantity, max_total_cents)."""
    quote = inventory.get("quote") if isinstance(inventory.get("quote"), dict) else {}
    quoted = _as_int(
        inventory.get("hold_total")
        or inventory.get("estimated_total")
        or inventory.get("quoted_total")
        or inventory.get("total")
        or inventory.get("max_total")
        or inventory.get("total_cents")
        or quote.get("total")
        or quote.get("max_total")
    )
    unit = _as_int(
        inventory.get("unit_hold")
        or inventory.get("unit_price")
        or inventory.get("price")
        or inventory.get("price_cents")
        or quote.get("unit_price")
        or quote.get("price")
    )
    available = _as_int(
        inventory.get("available")
        or inventory.get("sellable")
        or inventory.get("quantity")
        or inventory.get("stock"),
        quantity,
    )
    buy_qty = min(quantity, available if available > 0 else quantity)
    if quoted > 0 and available > 0 and buy_qty != quantity:
        quoted = int(round(quoted * buy_qty / max(quantity, 1)))
    if quoted <= 0 and unit > 0:
        quoted = unit * buy_qty
    if quoted <= 0:
        raise HakimiError("inventory quote missing total/unit price")
    if quoted > budget_cents:
        raise HakimiError(f"quote {quoted} exceeds budget {budget_cents}")
    return buy_qty, quoted


def _extract_error(payload: Any) -> str:
    if isinstance(payload, dict):
        for key in ("error", "message", "detail"):
            value = payload.get(key)
            if isinstance(value, str) and value.strip():
                return value
            if isinstance(value, list) and value:
                return json.dumps(value, ensure_ascii=False)
    if isinstance(payload, str):
        return payload
    return ""


def _clean_params(params: dict[str, Any] | None) -> dict[str, Any] | None:
    if not params:
        return None
    return {key: value for key, value in params.items() if value is not None}


def _as_int(value: Any, default: int = 0) -> int:
    try:
        if value is None or value == "":
            return default
        return int(value)
    except (TypeError, ValueError):
        return default


