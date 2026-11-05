from __future__ import annotations

import argparse
import json
import logging
import sys
from pathlib import Path

from hakimi_restock.agent import RestockAgent
from hakimi_restock.config import load_config
from hakimi_restock.redact import RedactFilter


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="hakimi-restock",
        description="Buy Hakimi accounts and import them into a local Sub2API pool. Admin key never leaves this machine.",
    )
    parser.add_argument(
        "-c",
        "--config",
        default="config.yaml",
        help="Path to config.yaml (default: ./config.yaml)",
    )
    parser.add_argument("--dry-run", action="store_true", help="Inspect pool and quote inventory without buying")
    sub = parser.add_subparsers(dest="command")
    sub.add_parser("once", help="Run one restock/recover tick and exit")
    sub.add_parser("run", help="Run forever")
    sub.add_parser("status", help="Show local pool + Hakimi wallet snapshot")
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    config_path = Path(args.config)
    if not config_path.exists():
        parser.error(f"config not found: {config_path}. Copy config.example.yaml to config.yaml")

    cfg = load_config(config_path)
    if args.dry_run:
        cfg.runtime.dry_run = True
    _setup_logging(cfg.runtime.log_level)

    agent = RestockAgent(cfg)
    try:
        command = args.command or "run"
        if command == "status":
            return _status(agent)
        if command == "once":
            report = agent.run_once()
            print(json.dumps(report, ensure_ascii=False, indent=2, default=str))
            return 0
        agent.run_forever()
        return 0
    finally:
        agent.close()


def _status(agent: RestockAgent) -> int:
    pool = agent._pool_snapshot()
    wallet = agent.hakimi.state()
    user = wallet.get("user") if isinstance(wallet.get("user"), dict) else wallet
    public_wallet = {
        key: user.get(key)
        for key in ("balance", "held", "available")
        if key in user
    }
    payload = {
        "pool": pool,
        "min_schedulable": agent.cfg.pool.min_schedulable,
        "gap": max(agent.cfg.pool.min_schedulable - pool["schedulable"], 0),
        "hakimi": public_wallet,
        "dry_run": agent.cfg.runtime.dry_run,
        "state_path": str(agent.cfg.runtime.state_path),
    }
    print(json.dumps(payload, ensure_ascii=False, indent=2, default=str))
    return 0


def _setup_logging(level: str) -> None:
    logging.basicConfig(
        level=getattr(logging, level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        stream=sys.stderr,
    )
    filt = RedactFilter()
    for handler in logging.getLogger().handlers:
        handler.addFilter(filt)


if __name__ == "__main__":
    raise SystemExit(main())
