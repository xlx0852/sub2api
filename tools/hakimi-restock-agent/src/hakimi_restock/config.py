from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml


def _as_bool(value: Any, default: bool = False) -> bool:
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    return str(value).strip().lower() in {"1", "true", "yes", "on"}


def _as_int(value: Any, default: int = 0) -> int:
    if value is None or value == "":
        return default
    return int(value)


def _as_optional_int(value: Any) -> int | None:
    if value is None or value == "":
        return None
    return int(value)


def _as_int_list(value: Any) -> list[int]:
    if not value:
        return []
    if isinstance(value, list):
        return [int(item) for item in value]
    return [int(item) for item in str(value).split(",") if str(item).strip()]


@dataclass(slots=True)
class HakimiConfig:
    base_url: str
    token: str


@dataclass(slots=True)
class Sub2APIConfig:
    base_url: str
    admin_api_key: str = ""
    jwt: str = ""


@dataclass(slots=True)
class PoolConfig:
    platform: str = "openai"
    type: str = "oauth"
    group_id: int = 0
    min_schedulable: int = 20
    count_temp_unschedulable_as_down: bool = True


@dataclass(slots=True)
class BuyConfig:
    enabled: bool = False
    quantity: int = 10
    minimum: int = 1
    maximum: int = 20
    budget_cents: int = 35000
    fresh: bool = False
    source: int | None = None
    cycle_id: str | None = None
    cooldown_seconds: int = 300


@dataclass(slots=True)
class ImportConfig:
    group_ids: list[int] = field(default_factory=list)
    concurrency: int = 3
    priority: int = 50
    proxy_id: int | None = None
    update_existing: bool = True
    skip_default_group_bind: bool = True
    confirm_mixed_channel_risk: bool = True
    auto_pause_on_expired: bool = True


@dataclass(slots=True)
class RecoverConfig:
    enabled: bool = False
    interval_minutes: int = 30
    poll_seconds: int = 8
    timeout_seconds: int = 900


@dataclass(slots=True)
class RuntimeConfig:
    check_seconds: int = 60
    order_poll_seconds: int = 8
    order_timeout_seconds: int = 900
    dry_run: bool = False
    state_path: Path = Path("data/state.json")
    save_downloads: bool = False
    download_dir: Path = Path("data/downloads")
    log_level: str = "INFO"


@dataclass(slots=True)
class AppConfig:
    hakimi: HakimiConfig
    sub2api: Sub2APIConfig
    pool: PoolConfig
    buy: BuyConfig
    import_cfg: ImportConfig
    recover: RecoverConfig
    runtime: RuntimeConfig
    source_path: Path


def load_env_file(path: Path) -> None:
    if not path.exists():
        return
    for line in path.read_text(encoding="utf-8").splitlines():
        text = line.strip()
        if not text or text.startswith("#") or "=" not in text:
            continue
        key, _, value = text.partition("=")
        key = key.strip()
        value = value.strip().strip("'").strip('"')
        if key and key not in os.environ:
            os.environ[key] = value


def load_config(path: str | Path) -> AppConfig:
    config_path = Path(path).expanduser().resolve()
    load_env_file(config_path.parent / ".env")
    raw = yaml.safe_load(config_path.read_text(encoding="utf-8")) or {}
    hakimi_raw = raw.get("hakimi") or {}
    sub2api_raw = raw.get("sub2api") or {}
    pool_raw = raw.get("pool") or {}
    buy_raw = raw.get("buy") or {}
    import_raw = raw.get("import") or {}
    recover_raw = raw.get("recover") or {}
    runtime_raw = raw.get("runtime") or {}

    hakimi = HakimiConfig(
        base_url=(os.environ.get("HAKIMI_BASE_URL") or hakimi_raw.get("base_url") or "").rstrip("/"),
        token=os.environ.get("HAKIMI_TOKEN") or hakimi_raw.get("token") or "",
    )
    sub2api = Sub2APIConfig(
        base_url=(os.environ.get("SUB2API_BASE_URL") or sub2api_raw.get("base_url") or "").rstrip("/"),
        admin_api_key=os.environ.get("SUB2API_ADMIN_API_KEY") or sub2api_raw.get("admin_api_key") or "",
        jwt=os.environ.get("SUB2API_JWT") or sub2api_raw.get("jwt") or "",
    )
    pool = PoolConfig(
        platform=str(pool_raw.get("platform") or "openai"),
        type=str(pool_raw.get("type") or "oauth"),
        group_id=_as_int(pool_raw.get("group_id"), 0),
        min_schedulable=_as_int(pool_raw.get("min_schedulable"), 20),
        count_temp_unschedulable_as_down=_as_bool(pool_raw.get("count_temp_unschedulable_as_down"), True),
    )
    buy = BuyConfig(
        enabled=_as_bool(buy_raw.get("enabled"), False),
        quantity=_as_int(buy_raw.get("quantity"), 10),
        minimum=_as_int(buy_raw.get("minimum"), 1),
        maximum=_as_int(buy_raw.get("maximum"), 20),
        budget_cents=_as_int(buy_raw.get("budget_cents"), 35000),
        fresh=_as_bool(buy_raw.get("fresh"), False),
        source=_as_optional_int(buy_raw.get("source")),
        cycle_id=(str(buy_raw["cycle_id"]) if buy_raw.get("cycle_id") not in (None, "") else None),
        cooldown_seconds=_as_int(buy_raw.get("cooldown_seconds"), 300),
    )
    import_cfg = ImportConfig(
        group_ids=_as_int_list(import_raw.get("group_ids")),
        concurrency=_as_int(import_raw.get("concurrency"), 3),
        priority=_as_int(import_raw.get("priority"), 50),
        proxy_id=_as_optional_int(import_raw.get("proxy_id")),
        update_existing=_as_bool(import_raw.get("update_existing"), True),
        skip_default_group_bind=_as_bool(import_raw.get("skip_default_group_bind"), True),
        confirm_mixed_channel_risk=_as_bool(import_raw.get("confirm_mixed_channel_risk"), True),
        auto_pause_on_expired=_as_bool(import_raw.get("auto_pause_on_expired"), True),
    )
    recover = RecoverConfig(
        enabled=_as_bool(recover_raw.get("enabled"), True),
        interval_minutes=_as_int(recover_raw.get("interval_minutes"), 30),
        poll_seconds=_as_int(recover_raw.get("poll_seconds"), 8),
        timeout_seconds=_as_int(recover_raw.get("timeout_seconds"), 900),
    )
    runtime = RuntimeConfig(
        check_seconds=_as_int(runtime_raw.get("check_seconds"), 60),
        order_poll_seconds=_as_int(runtime_raw.get("order_poll_seconds"), 8),
        order_timeout_seconds=_as_int(runtime_raw.get("order_timeout_seconds"), 900),
        dry_run=_as_bool(os.environ.get("HAKIMI_RESTOCK_DRY_RUN"), _as_bool(runtime_raw.get("dry_run"), False)),
        state_path=Path(runtime_raw.get("state_path") or "data/state.json"),
        save_downloads=_as_bool(runtime_raw.get("save_downloads"), False),
        download_dir=Path(runtime_raw.get("download_dir") or "data/downloads"),
        log_level=str(runtime_raw.get("log_level") or "INFO").upper(),
    )

    if not hakimi.base_url:
        raise ValueError("hakimi.base_url is required")
    if not hakimi.token:
        raise ValueError("HAKIMI_TOKEN or hakimi.token is required")
    if not sub2api.base_url:
        raise ValueError("sub2api.base_url is required")
    if not sub2api.admin_api_key and not sub2api.jwt:
        raise ValueError("SUB2API_ADMIN_API_KEY or SUB2API_JWT is required")
    if buy.minimum > buy.maximum:
        raise ValueError("buy.minimum must be <= buy.maximum")
    if not (1 <= buy.quantity <= 3000):
        raise ValueError("buy.quantity must be 1-3000")
    if pool.min_schedulable < 0:
        raise ValueError("pool.min_schedulable must be >= 0")

    if not runtime.state_path.is_absolute():
        runtime.state_path = (config_path.parent / runtime.state_path).resolve()
    if not runtime.download_dir.is_absolute():
        runtime.download_dir = (config_path.parent / runtime.download_dir).resolve()

    return AppConfig(
        hakimi=hakimi,
        sub2api=sub2api,
        pool=pool,
        buy=buy,
        import_cfg=import_cfg,
        recover=recover,
        runtime=runtime,
        source_path=config_path,
    )
