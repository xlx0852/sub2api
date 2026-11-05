from __future__ import annotations

import logging
import time
import uuid
from datetime import datetime, timedelta, timezone
from typing import Any

from hakimi_restock.config import AppConfig
from hakimi_restock.hakimi import HakimiClient, HakimiError, quote_inventory
from hakimi_restock.state import AgentState, InflightOrder, StateStore, utc_now
from hakimi_restock.sub2api import Sub2APIClient, count_schedulable, summarize_import

logger = logging.getLogger(__name__)

TERMINAL_ORDER_STATES = {"done", "completed", "delivered", "cancelled", "canceled", "failed", "expired"}
SUCCESS_ORDER_STATES = {"done", "completed", "delivered"}
ACTIVE_RECOVERY_STATES = {"queued", "running", "pending", "processing"}


class RestockAgent:
    def __init__(self, cfg: AppConfig) -> None:
        self.cfg = cfg
        self.store = StateStore(cfg.runtime.state_path)
        self.hakimi = HakimiClient(cfg.hakimi)
        self.sub2api = Sub2APIClient(cfg.sub2api)

    def close(self) -> None:
        self.hakimi.close()
        self.sub2api.close()

    def run_forever(self) -> None:
        logger.info(
            "agent started dry_run=%s min_schedulable=%s check=%ss",
            self.cfg.runtime.dry_run,
            self.cfg.pool.min_schedulable,
            self.cfg.runtime.check_seconds,
        )
        while True:
            try:
                self.run_once()
            except KeyboardInterrupt:
                raise
            except Exception as exc:
                state = self.store.load()
                state.last_error = str(exc)
                self.store.save(state)
                logger.exception("tick failed: %s", exc)
            time.sleep(self.cfg.runtime.check_seconds)

    def run_once(self) -> dict[str, Any]:
        state = self.store.load()
        report: dict[str, Any] = {"at": utc_now(), "dry_run": self.cfg.runtime.dry_run}

        if state.inflight and not state.inflight.imported:
            report["resumed"] = self._resume_inflight(state)

        pool = self._pool_snapshot()
        report["pool"] = pool
        state.last_schedulable = pool["schedulable"]
        gap = max(self.cfg.pool.min_schedulable - pool["schedulable"], 0)
        state.last_gap = gap
        report["gap"] = gap

        if gap > 0:
            if not self.cfg.buy.enabled:
                logger.info("buy disabled; gap=%s schedulable=%s", gap, pool["schedulable"])
                report["buy"] = {"skipped": "buy_disabled", "gap": gap}
            else:
                report["buy"] = self._maybe_buy(state, gap)
        else:
            logger.info("pool healthy schedulable=%s min=%s", pool["schedulable"], self.cfg.pool.min_schedulable)

        if self.cfg.recover.enabled:
            report["recover"] = self._maybe_recover(state)

        state.last_error = ""
        self.store.save(state)
        return report

    def _pool_snapshot(self) -> dict[str, int]:
        accounts = self.sub2api.list_accounts(
            platform=self.cfg.pool.platform,
            account_type=self.cfg.pool.type,
            group_id=self.cfg.pool.group_id,
        )
        stats = count_schedulable(accounts, self.cfg.pool)
        logger.info(
            "pool total=%s schedulable=%s temp_down=%s expired=%s",
            stats["total"],
            stats["schedulable"],
            stats["temp_down"],
            stats["expired"],
        )
        return stats

    def _maybe_buy(self, state: AgentState, gap: int) -> dict[str, Any]:
        if self._in_cooldown(state.last_buy_at, self.cfg.buy.cooldown_seconds):
            remain = self._cooldown_remaining(state.last_buy_at, self.cfg.buy.cooldown_seconds)
            logger.info("buy cooldown %ss remaining", remain)
            return {"skipped": "cooldown", "remaining_seconds": remain}

        quantity = min(max(gap, self.cfg.buy.minimum), self.cfg.buy.quantity, self.cfg.buy.maximum)
        wallet = self.hakimi.state()
        user = wallet.get("user") if isinstance(wallet.get("user"), dict) else wallet
        available_cents = int(user.get("available") or (int(user.get("balance") or 0) - int(user.get("held") or 0)))
        budget = min(self.cfg.buy.budget_cents, available_cents) if available_cents > 0 else self.cfg.buy.budget_cents
        if available_cents <= 0:
            logger.warning("hakimi wallet empty available=%s", available_cents)
            return {"skipped": "insufficient_balance", "available": available_cents}
        inventory = self.hakimi.inventory(
            quantity,
            fresh=self.cfg.buy.fresh,
            source=self.cfg.buy.source,
            cycle_id=self.cfg.buy.cycle_id,
        )
        try:
            buy_qty, max_total = quote_inventory(inventory, quantity, budget)
        except HakimiError as exc:
            unit = int(inventory.get("unit_hold") or 0)
            if unit > 0 and budget >= unit:
                affordable = min(quantity, budget // unit, int(inventory.get("available") or quantity))
                if affordable >= self.cfg.buy.minimum:
                    inventory = self.hakimi.inventory(
                        affordable,
                        fresh=self.cfg.buy.fresh,
                        source=self.cfg.buy.source,
                        cycle_id=self.cfg.buy.cycle_id,
                    )
                    buy_qty, max_total = quote_inventory(inventory, affordable, budget)
                else:
                    logger.warning("quote exceeds wallet/budget: %s", exc)
                    return {"skipped": "insufficient_balance", "error": str(exc), "available": available_cents}
            else:
                logger.warning("quote exceeds wallet/budget: %s", exc)
                return {"skipped": "insufficient_balance", "error": str(exc), "available": available_cents}
        body: dict[str, Any] = {
            "quantity": buy_qty,
            "fresh": self.cfg.buy.fresh,
            "max_total": max_total,
        }
        if self.cfg.buy.source is not None:
            body["source"] = self.cfg.buy.source
        if self.cfg.buy.cycle_id:
            body["cycle_id"] = self.cfg.buy.cycle_id

        logger.info("need buy quantity=%s max_total=%s gap=%s", buy_qty, max_total, gap)
        if not self.cfg.buy.enabled:
            return {"skipped": "buy_disabled", "quantity": buy_qty, "max_total": max_total}
        if self.cfg.runtime.dry_run:
            return {"skipped": "dry_run", "quantity": buy_qty, "max_total": max_total, "inventory": _public_inventory(inventory)}

        idempotency_key = f"hakimi-restock-{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%S')}-{uuid.uuid4().hex[:8]}"
        order = self.hakimi.create_order(body, idempotency_key=idempotency_key)
        order_id = str(order.get("id") or order.get("order_id") or "")
        if not order_id:
            raise HakimiError("create order returned no id", payload=order)

        state.inflight = InflightOrder(
            order_id=order_id,
            idempotency_key=idempotency_key,
            request_body=body,
            created_at=utc_now(),
            last_state=str(order.get("state") or ""),
        )
        self.store.remember_order(state, order_id)
        self.store.save(state)
        logger.info("order created id=%s state=%s", order_id, order.get("state"))
        imported = self._wait_and_import(state, state.inflight)
        state.last_buy_at = utc_now()
        return {"order_id": order_id, "quantity": buy_qty, "max_total": max_total, "import": imported}

    def _resume_inflight(self, state: AgentState) -> dict[str, Any]:
        inflight = state.inflight
        if inflight is None:
            return {}
        logger.info("resume inflight order=%s imported=%s", inflight.order_id, inflight.imported)
        if inflight.order_id:
            return {"order_id": inflight.order_id, "import": self._wait_and_import(state, inflight)}
        if inflight.idempotency_key and inflight.request_body:
            order = self.hakimi.create_order(inflight.request_body, idempotency_key=inflight.idempotency_key)
            inflight.order_id = str(order.get("id") or order.get("order_id") or "")
            inflight.last_state = str(order.get("state") or "")
            self.store.remember_order(state, inflight.order_id)
            self.store.save(state)
            return {"order_id": inflight.order_id, "import": self._wait_and_import(state, inflight)}
        raise HakimiError("inflight order missing id and idempotency key")

    def _wait_and_import(self, state: AgentState, inflight: InflightOrder) -> dict[str, Any]:
        order = self._poll_order(inflight)
        delivered = _delivered_count(order)
        if delivered <= 0 and _order_state(order) not in SUCCESS_ORDER_STATES:
            inflight.last_state = _order_state(order)
            self.store.save(state)
            logger.warning("order %s not ready for import state=%s delivered=%s", inflight.order_id, inflight.last_state, delivered)
            return {"skipped": inflight.last_state or "pending", "delivered": delivered}

        content = self.hakimi.download_order(inflight.order_id, "sub2")
        text = _as_text(content)
        if self.cfg.runtime.save_downloads:
            inflight.download_path = self._save_download(f"order-{inflight.order_id}.sub2.json", content)
        import_key = f"import-{inflight.order_id}"
        result = self.sub2api.import_codex_session(text, self.cfg.import_cfg, idempotency_key=import_key)
        inflight.imported = True
        inflight.last_state = _order_state(order) or "imported"
        state.inflight = None
        logger.info("imported order %s %s", inflight.order_id, summarize_import(result))
        return {"order_id": inflight.order_id, "delivered": delivered, **_import_public(result)}

    def _poll_order(self, inflight: InflightOrder) -> dict[str, Any]:
        deadline = time.time() + self.cfg.runtime.order_timeout_seconds
        last: dict[str, Any] = {}
        while time.time() < deadline:
            try:
                last = self.hakimi.get_order(inflight.order_id)
            except HakimiError as exc:
                if exc.status_code in {429, 503} or exc.status_code == 0:
                    logger.warning("order poll retryable: %s", exc)
                    time.sleep(self.cfg.runtime.order_poll_seconds)
                    continue
                raise
            state_name = _order_state(last)
            delivered = _delivered_count(last)
            quantity = int(last.get("quantity") or 0)
            inflight.last_state = state_name
            logger.info("order %s state=%s delivered=%s/%s", inflight.order_id, state_name, delivered, quantity)
            if delivered > 0 or state_name in TERMINAL_ORDER_STATES:
                return last
            time.sleep(self.cfg.runtime.order_poll_seconds)
        raise HakimiError(f"order {inflight.order_id} poll timeout")

    def _maybe_recover(self, state: AgentState) -> dict[str, Any]:
        interval = self.cfg.recover.interval_minutes * 60
        if self._in_cooldown(state.last_recover_at, interval):
            remain = self._cooldown_remaining(state.last_recover_at, interval)
            return {"skipped": "interval", "remaining_seconds": remain}

        order_ids = list(state.known_order_ids)
        if not order_ids:
            recent = self.hakimi.list_orders(page=1)
            items = recent.get("items") if isinstance(recent, dict) else recent
            for item in items or []:
                oid = str(item.get("id") or item.get("order_id") or "")
                if oid:
                    order_ids.append(oid)
                    self.store.remember_order(state, oid)

        if self.cfg.runtime.dry_run:
            state.last_recover_at = utc_now()
            return {"skipped": "dry_run", "orders": len(order_ids)}

        imported: list[dict[str, Any]] = []
        for order_id in order_ids[-50:]:
            result = self._recover_one(order_id)
            if result:
                imported.append(result)
        state.last_recover_at = utc_now()
        return {"orders": len(order_ids), "imported": imported}

    def _recover_one(self, order_id: str) -> dict[str, Any] | None:
        submitted = self.hakimi.recover_order(order_id)
        recovery_id = str(submitted.get("id") or submitted.get("recovery_id") or "")
        logger.info("recover submitted order=%s recovery=%s state=%s", order_id, recovery_id, submitted.get("state"))
        recovery = self._poll_recovery(order_id, recovery_id)
        if not recovery:
            return None
        if not recovery.get("download_ready") and int(recovery.get("revived") or 0) <= 0:
            logger.info("recover order=%s no new credentials", order_id)
            return None
        rid = str(recovery.get("id") or recovery_id)
        try:
            content = self.hakimi.download_recovery(rid, "sub2") if rid else self.hakimi.download_care(order_id, "sub2")
        except HakimiError:
            content = self.hakimi.download_care(order_id, "sub2")
        if self.cfg.runtime.save_downloads and rid:
            self._save_download(f"recovery-{rid}.sub2.json", content)
        result = self.sub2api.import_codex_session(
            _as_text(content),
            self.cfg.import_cfg,
            idempotency_key=f"recover-{order_id}-{rid or 'care'}",
        )
        logger.info("recovered order %s %s", order_id, summarize_import(result))
        return {"order_id": order_id, "recovery_id": rid, **_import_public(result)}

    def _poll_recovery(self, order_id: str, recovery_id: str) -> dict[str, Any] | None:
        deadline = time.time() + self.cfg.recover.timeout_seconds
        while time.time() < deadline:
            payload = self.hakimi.list_recoveries(order_id)
            items = payload if isinstance(payload, list) else (payload.get("items") or payload.get("recoveries") or [])
            match = None
            for item in items:
                if recovery_id and str(item.get("id")) == recovery_id:
                    match = item
                    break
            if match is None and items:
                match = items[0]
            if match:
                state_name = str(match.get("state") or "").lower()
                logger.info(
                    "recovery %s state=%s revived=%s download_ready=%s",
                    match.get("id"),
                    state_name,
                    match.get("revived"),
                    match.get("download_ready"),
                )
                if state_name not in ACTIVE_RECOVERY_STATES:
                    return match
            time.sleep(self.cfg.recover.poll_seconds)
        logger.warning("recovery poll timeout order=%s", order_id)
        return None

    def _save_download(self, name: str, content: bytes) -> str:
        directory = self.cfg.runtime.download_dir
        directory.mkdir(parents=True, exist_ok=True)
        path = directory / name
        path.write_bytes(content)
        logger.info("saved download %s bytes=%s", path.name, len(content))
        return str(path)

    def _in_cooldown(self, last_at: str, seconds: int) -> bool:
        if not last_at or seconds <= 0:
            return False
        try:
            last = datetime.fromisoformat(last_at)
        except ValueError:
            return False
        if last.tzinfo is None:
            last = last.replace(tzinfo=timezone.utc)
        return datetime.now(timezone.utc) < last + timedelta(seconds=seconds)

    def _cooldown_remaining(self, last_at: str, seconds: int) -> int:
        last = datetime.fromisoformat(last_at)
        if last.tzinfo is None:
            last = last.replace(tzinfo=timezone.utc)
        remain = (last + timedelta(seconds=seconds)) - datetime.now(timezone.utc)
        return max(int(remain.total_seconds()), 0)


def _order_state(order: dict[str, Any]) -> str:
    return str(order.get("state") or order.get("status") or "").lower()


def _delivered_count(order: dict[str, Any]) -> int:
    for key in ("delivered", "delivered_count", "fulfilled"):
        value = order.get(key)
        if value not in (None, ""):
            try:
                return int(value)
            except (TypeError, ValueError):
                pass
    items = order.get("items") or []
    if isinstance(items, list):
        ready = 0
        for item in items:
            item_state = str(item.get("state") or item.get("status") or "").lower()
            if item_state in SUCCESS_ORDER_STATES or item.get("delivered") is True:
                ready += 1
        return ready
    return 0


def _as_text(content: bytes) -> str:
    return content.decode("utf-8") if isinstance(content, (bytes, bytearray)) else str(content)


def _import_public(result: dict[str, Any]) -> dict[str, Any]:
    return {
        "total": result.get("total", 0),
        "created": result.get("created", 0),
        "updated": result.get("updated", 0),
        "skipped": result.get("skipped", 0),
        "failed": result.get("failed", 0),
    }


def _public_inventory(inventory: dict[str, Any]) -> dict[str, Any]:
    keys = (
        "available",
        "quantity",
        "stock",
        "unit_hold",
        "unit_price",
        "price",
        "price_cents",
        "hold_total",
        "estimated_total",
        "quoted_total",
        "total",
        "max_total",
        "source",
        "cycle_id",
    )
    return {key: inventory[key] for key in keys if key in inventory}


