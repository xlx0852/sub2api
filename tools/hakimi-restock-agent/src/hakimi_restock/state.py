from __future__ import annotations

import json
import os
import tempfile
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat()


@dataclass
class InflightOrder:
    order_id: str = ""
    idempotency_key: str = ""
    request_body: dict[str, Any] = field(default_factory=dict)
    created_at: str = ""
    last_state: str = ""
    imported: bool = False
    download_path: str = ""


@dataclass
class AgentState:
    last_buy_at: str = ""
    last_recover_at: str = ""
    last_gap: int = 0
    last_schedulable: int = 0
    last_error: str = ""
    known_order_ids: list[str] = field(default_factory=list)
    inflight: InflightOrder | None = None

    def to_dict(self) -> dict[str, Any]:
        data = asdict(self)
        if self.inflight is None:
            data["inflight"] = None
        return data

    @classmethod
    def from_dict(cls, raw: dict[str, Any] | None) -> "AgentState":
        raw = raw or {}
        inflight_raw = raw.get("inflight") or None
        inflight = InflightOrder(**inflight_raw) if isinstance(inflight_raw, dict) else None
        return cls(
            last_buy_at=str(raw.get("last_buy_at") or ""),
            last_recover_at=str(raw.get("last_recover_at") or ""),
            last_gap=int(raw.get("last_gap") or 0),
            last_schedulable=int(raw.get("last_schedulable") or 0),
            last_error=str(raw.get("last_error") or ""),
            known_order_ids=[str(item) for item in (raw.get("known_order_ids") or [])],
            inflight=inflight,
        )


class StateStore:
    def __init__(self, path: Path) -> None:
        self.path = path

    def load(self) -> AgentState:
        if not self.path.exists():
            return AgentState()
        return AgentState.from_dict(json.loads(self.path.read_text(encoding="utf-8")))

    def save(self, state: AgentState) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True)
        payload = json.dumps(state.to_dict(), ensure_ascii=False, indent=2)
        fd, tmp_name = tempfile.mkstemp(prefix=".state.", suffix=".json", dir=self.path.parent)
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as handle:
                handle.write(payload)
                handle.write("\n")
            os.replace(tmp_name, self.path)
        except Exception:
            try:
                os.unlink(tmp_name)
            except OSError:
                pass
            raise

    def remember_order(self, state: AgentState, order_id: str) -> None:
        if order_id and order_id not in state.known_order_ids:
            state.known_order_ids.append(order_id)
            if len(state.known_order_ids) > 200:
                state.known_order_ids = state.known_order_ids[-200:]
