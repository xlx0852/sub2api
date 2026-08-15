#!/usr/bin/env python3
"""Component-scoped release installer for separated Sub2API deployments.

The command runs on the deployment host. Artifact transfer is deliberately a
separate, auditable step: the JSON manifest names the exact uploaded files and
their SHA-256 digests. Backend process verification/switching is delegated to
explicit operator commands because systemd, Docker, and nginx layouts differ.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tarfile
import tempfile
import zipfile
from pathlib import Path
from typing import Any

SCOPES = {"frontend", "backend", "both"}
SEMVER = re.compile(r"^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:[-+][0-9A-Za-z.-]+)?$")


class DeployError(RuntimeError):
    pass


def version_tuple(value: str) -> tuple[int, int, int]:
    match = SEMVER.fullmatch(value.strip())
    if not match:
        raise DeployError(f"invalid semantic version: {value!r}")
    return tuple(int(match.group(i)) for i in range(1, 4))


def compatible(current: str, minimum: str) -> bool:
    return version_tuple(current) >= version_tuple(minimum)


def load_json(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise DeployError(f"cannot read JSON {path}: {exc}") from exc
    if not isinstance(value, dict):
        raise DeployError(f"expected JSON object in {path}")
    return value


def require_text(value: dict[str, Any], key: str, where: str) -> str:
    result = value.get(key)
    if not isinstance(result, str) or not result.strip():
        raise DeployError(f"{where}.{key} must be a non-empty string")
    return result.strip()


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def validate_artifact(component: dict[str, Any], name: str) -> Path:
    path = Path(require_text(component, "artifact", name)).expanduser().resolve()
    if not path.is_file():
        raise DeployError(f"{name} artifact does not exist: {path}")
    expected = require_text(component, "sha256", name).lower()
    actual = sha256(path)
    if actual != expected:
        raise DeployError(f"{name} artifact SHA-256 mismatch: expected {expected}, got {actual}")
    return path


def validate_manifest(manifest: dict[str, Any]) -> str:
    scope = require_text(manifest, "scope", "manifest")
    if scope not in SCOPES:
        raise DeployError("manifest.scope must be frontend, backend, or both")
    if manifest.get("contract_change") is True and scope != "both":
        raise DeployError("cross-component contract changes require scope=both")
    for name in ("frontend", "backend"):
        if name in (scope, "both") or scope == "both":
            component = manifest.get(name)
            if not isinstance(component, dict):
                raise DeployError(f"manifest.{name} is required for scope={scope}")
            version_tuple(require_text(component, "version", name))
            validate_artifact(component, name)
    return scope


def current_frontend_metadata(root: Path, override: Path | None) -> dict[str, Any]:
    path = override or root / "frontend" / "current" / "sub2api-build.json"
    return load_json(path)


def current_backend_metadata(root: Path, override: Path | None) -> dict[str, Any]:
    path = override or root / "state" / ".codex-api-current"
    return load_json(path)


def preflight(
    manifest: dict[str, Any],
    root: Path,
    live_frontend: Path | None = None,
    live_backend: Path | None = None,
) -> None:
    scope = validate_manifest(manifest)
    if scope in {"frontend", "both"}:
        candidate = manifest["frontend"]
        if scope == "both":
            backend_version = require_text(manifest["backend"], "version", "backend")
        else:
            backend = current_backend_metadata(root, live_backend)
            backend_version = require_text(backend, "version", "live backend")
        minimum = require_text(candidate, "min_backend_version", "frontend")
        if not compatible(backend_version, minimum):
            raise DeployError(
                f"frontend requires backend >= {minimum}, live backend is {backend_version}"
            )
    if scope in {"backend", "both"}:
        candidate = manifest["backend"]
        frontend = current_frontend_metadata(root, live_frontend)
        frontend_version = require_text(frontend, "frontend_version", "live frontend")
        minimum = require_text(candidate, "min_frontend_version", "backend")
        if not compatible(frontend_version, minimum):
            raise DeployError(
                f"backend requires frontend >= {minimum}, live frontend is {frontend_version}"
            )


def safe_extract(artifact: Path, destination: Path) -> None:
    destination.mkdir(parents=True, exist_ok=False)
    destination_real = destination.resolve()

    def safe_member(name: str) -> bool:
        target = (destination / name).resolve()
        return target == destination_real or destination_real in target.parents

    if zipfile.is_zipfile(artifact):
        with zipfile.ZipFile(artifact) as archive:
            if not all(safe_member(info.filename) for info in archive.infolist()):
                raise DeployError(f"unsafe path in archive {artifact}")
            archive.extractall(destination)
    elif tarfile.is_tarfile(artifact):
        with tarfile.open(artifact) as archive:
            members = archive.getmembers()
            if not all(safe_member(member.name) and not member.issym() and not member.islnk() for member in members):
                raise DeployError(f"unsafe path in archive {artifact}")
            archive.extractall(destination)
    else:
        raise DeployError(f"unsupported artifact format: {artifact}")


def atomic_symlink(link: Path, target: Path) -> None:
    link.parent.mkdir(parents=True, exist_ok=True)
    temporary = link.with_name(f".{link.name}.next")
    temporary.unlink(missing_ok=True)
    temporary.symlink_to(os.path.relpath(target.resolve(), link.parent.resolve()))
    os.replace(temporary, link)


def write_marker(root: Path, name: str, value: dict[str, Any]) -> None:
    state = root / "state"
    state.mkdir(parents=True, exist_ok=True)
    target = state / name
    with tempfile.NamedTemporaryFile("w", dir=state, delete=False, encoding="utf-8") as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2, sort_keys=True)
        stream.write("\n")
        temporary = Path(stream.name)
    os.replace(temporary, target)


def switch_release(root: Path, name: str, release: Path, marker: dict[str, Any]) -> None:
    component_root = root / name
    current = component_root / "current"
    rollback = component_root / "rollback"
    previous = current.resolve() if current.is_symlink() and current.exists() else None
    if previous is not None:
        atomic_symlink(rollback, previous)
        current_marker = root / "state" / f".codex-{name if name == 'frontend' else 'api'}-current"
        if current_marker.exists():
            shutil.copy2(
                current_marker,
                root / "state" / f".codex-{name if name == 'frontend' else 'api'}-rollback",
            )
    atomic_symlink(current, release)
    write_marker(root, f".codex-{name if name == 'frontend' else 'api'}-current", marker)
    retain_only_current_and_rollback(component_root)


def retain_only_current_and_rollback(component_root: Path) -> None:
    releases = component_root / "releases"
    if not releases.exists():
        return
    retained = set()
    for name in ("current", "rollback"):
        link = component_root / name
        if link.is_symlink() and link.exists():
            retained.add(link.resolve())
    for release in releases.iterdir():
        if release.resolve() not in retained:
            if release.is_dir():
                shutil.rmtree(release)
            else:
                release.unlink()


def run_hook(command: str, release: Path) -> None:
    rendered = command.replace("{release}", shlex.quote(str(release)))
    subprocess.run(rendered, shell=True, check=True)


def install_frontend(manifest: dict[str, Any], root: Path) -> None:
    component = manifest["frontend"]
    artifact = validate_artifact(component, "frontend")
    version = require_text(component, "version", "frontend")
    release = root / "frontend" / "releases" / f"{version}-{sha256(artifact)[:12]}"
    if not release.exists():
        safe_extract(artifact, release)
    metadata = load_json(release / "sub2api-build.json")
    if require_text(metadata, "frontend_version", "frontend artifact") != version:
        raise DeployError("frontend artifact metadata version does not match manifest")
    if not (release / "index.html").is_file():
        raise DeployError("frontend artifact is missing index.html")
    switch_release(root, "frontend", release, metadata | {"sha256": sha256(artifact)})


def install_backend(
    manifest: dict[str, Any],
    root: Path,
    verify_command: str | None,
    switch_command: str | None,
    db_backup_command: str | None,
) -> None:
    if not verify_command or not switch_command:
        raise DeployError("backend deploy requires --backend-verify-command and --backend-switch-command")
    component = manifest["backend"]
    if component.get("db_change") is True:
        if not db_backup_command:
            raise DeployError("backend db_change=true requires --db-backup-command")
        run_hook(db_backup_command, root)
    artifact = validate_artifact(component, "backend")
    version = require_text(component, "version", "backend")
    release = root / "api" / "releases" / f"{version}-{sha256(artifact)[:12]}"
    if not release.exists():
        safe_extract(artifact, release)
    binary = release / "sub2api"
    if not binary.is_file():
        raise DeployError("backend artifact is missing sub2api")
    binary.chmod(binary.stat().st_mode | 0o111)
    run_hook(verify_command, release)
    marker = {
        "version": version,
        "min_frontend_version": require_text(component, "min_frontend_version", "backend"),
        "sha256": sha256(artifact),
    }
    run_hook(switch_command, release)
    switch_release(root, "api", release, marker)


def rollback(root: Path, component: str, switch_command: str | None) -> None:
    name = "api" if component == "backend" else "frontend"
    component_root = root / name
    current = component_root / "current"
    previous = component_root / "rollback"
    if not previous.is_symlink() or not previous.exists():
        raise DeployError(f"no {component} rollback release is available")
    current_target = current.resolve() if current.is_symlink() and current.exists() else None
    previous_target = previous.resolve()
    if component == "backend":
        if not switch_command:
            raise DeployError("backend rollback requires --backend-switch-command")
        run_hook(switch_command, previous_target)
    atomic_symlink(current, previous_target)
    if current_target:
        atomic_symlink(previous, current_target)
    current_marker = root / "state" / f".codex-{name}-current"
    rollback_marker = root / "state" / f".codex-{name}-rollback"
    if rollback_marker.exists():
        current_data = current_marker.read_bytes() if current_marker.exists() else None
        os.replace(rollback_marker, current_marker)
        if current_data is not None:
            rollback_marker.write_bytes(current_data)


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser()
    subcommands = result.add_subparsers(dest="command", required=True)
    for name in ("check", "deploy"):
        command = subcommands.add_parser(name)
        command.add_argument("--manifest", type=Path, required=True)
        command.add_argument("--root", type=Path, default=Path("/opt/sub2api"))
        command.add_argument("--live-frontend-metadata", type=Path)
        command.add_argument("--live-backend-metadata", type=Path)
        command.add_argument("--backend-verify-command")
        command.add_argument("--backend-switch-command")
        command.add_argument("--db-backup-command")
    command = subcommands.add_parser("rollback")
    command.add_argument("component", choices=("frontend", "backend"))
    command.add_argument("--root", type=Path, default=Path("/opt/sub2api"))
    command.add_argument("--backend-switch-command")
    return result


def main() -> int:
    args = parser().parse_args()
    try:
        if args.command == "rollback":
            rollback(args.root.resolve(), args.component, args.backend_switch_command)
            return 0
        manifest = load_json(args.manifest.resolve())
        root = args.root.resolve()
        preflight(manifest, root, args.live_frontend_metadata, args.live_backend_metadata)
        if args.command == "check":
            print("compatibility preflight passed")
            return 0
        scope = manifest["scope"]
        if scope in {"backend", "both"}:
            install_backend(
                manifest,
                root,
                args.backend_verify_command,
                args.backend_switch_command,
                args.db_backup_command,
            )
        if scope in {"frontend", "both"}:
            install_frontend(manifest, root)
        return 0
    except (DeployError, subprocess.CalledProcessError) as exc:
        print(f"component deploy failed: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
