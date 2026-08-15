#!/usr/bin/env python3
"""Build a staging candidate from an explicit runtime-file allowlist."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
from typing import Any


ROOT = pathlib.Path(__file__).resolve().parents[1]
DEFAULT_MANIFEST = (
    ROOT
    / "openspec"
    / "changes"
    / "harden-usage-settlement-durability"
    / "staging-candidate.json"
)


def run(command: list[str], *, cwd: pathlib.Path = ROOT, env: dict[str, str] | None = None) -> str:
    result = subprocess.run(
        command,
        cwd=cwd,
        env=env,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    if result.stdout:
        print(result.stdout, end="")
    if result.returncode != 0:
        raise RuntimeError(f"command failed ({result.returncode}): {' '.join(command)}")
    return result.stdout.strip()


def git_output(*args: str, cwd: pathlib.Path = ROOT) -> str:
    return subprocess.check_output(["git", *args], cwd=cwd, text=True).strip()


def dirty_paths(cwd: pathlib.Path = ROOT) -> set[str]:
    output = subprocess.check_output(
        ["git", "status", "--porcelain=v1", "--untracked-files=all"],
        cwd=cwd,
        text=True,
    )
    paths: set[str] = set()
    for line in output.splitlines():
        if len(line) < 4:
            continue
        path = line[3:]
        if " -> " in path:
            raise RuntimeError(f"renamed paths are not supported in a staging candidate: {path}")
        paths.add(path)
    return paths


def sha256_file(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def sha256_tree(root: pathlib.Path) -> tuple[str, dict[str, str]]:
    digest = hashlib.sha256()
    files: dict[str, str] = {}
    for path in sorted(item for item in root.rglob("*") if item.is_file()):
        relative = path.relative_to(root).as_posix()
        file_hash = sha256_file(path)
        files[relative] = file_hash
        digest.update(relative.encode("utf-8"))
        digest.update(b"\0")
        digest.update(file_hash.encode("ascii"))
        digest.update(b"\n")
    return digest.hexdigest(), files


def load_manifest(path: pathlib.Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if value.get("schema_version") != 1:
        raise RuntimeError("unsupported staging candidate manifest schema")
    runtime_files = value.get("runtime_files")
    if not isinstance(runtime_files, list) or not runtime_files:
        raise RuntimeError("staging candidate runtime_files must be a non-empty list")
    normalized = [str(pathlib.PurePosixPath(str(item))) for item in runtime_files]
    if len(normalized) != len(set(normalized)):
        raise RuntimeError("staging candidate runtime_files contains duplicates")
    if normalized != sorted(normalized):
        raise RuntimeError("staging candidate runtime_files must be sorted")
    value["runtime_files"] = normalized
    return value


def validate_source(manifest: dict[str, Any]) -> tuple[str, set[str]]:
    head = git_output("rev-parse", "HEAD")
    base_ref = str(manifest.get("base_ref", ""))
    if head != base_ref:
        raise RuntimeError(f"manifest base_ref {base_ref!r} does not match HEAD {head!r}")

    runtime_files = set(manifest["runtime_files"])
    current_dirty = dirty_paths()
    missing_dirty = sorted(runtime_files - current_dirty)
    if missing_dirty:
        raise RuntimeError("manifest runtime files are not dirty in the source tree: " + ", ".join(missing_dirty))

    hidden_backend_runtime = sorted(
        path
        for path in current_dirty
        if path.startswith("backend/") and not path.endswith("_test.go") and path not in runtime_files
    )
    release_sensitive = sorted(
        path
        for path in current_dirty
        if (
            path.startswith("frontend/")
            or path.startswith("deploy/")
            or path in {"Makefile", "Dockerfile", "docker-compose.yml", "install.sh"}
        )
        and path not in runtime_files
    )
    unexpected = hidden_backend_runtime + release_sensitive
    if unexpected:
        raise RuntimeError("dirty release-sensitive files are outside the allowlist: " + ", ".join(unexpected))

    for relative in sorted(runtime_files):
        source = ROOT / relative
        if not source.is_file():
            raise RuntimeError(f"allowlisted runtime file is missing: {relative}")
    return head, current_dirty


def build_candidate(
    manifest_path: pathlib.Path,
    report_path: pathlib.Path,
    artifact_dir: pathlib.Path | None,
) -> dict[str, Any]:
    manifest = load_manifest(manifest_path)
    head, current_dirty = validate_source(manifest)
    runtime_files = list(manifest["runtime_files"])

    temp_root = pathlib.Path(tempfile.mkdtemp(prefix="sub2api-settlement-candidate-"))
    candidate = temp_root / "source"
    worktree_added = False
    try:
        run(["git", "worktree", "add", "--detach", str(candidate), head])
        worktree_added = True
        for relative in runtime_files:
            source = ROOT / relative
            target = candidate / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, target)

        candidate_dirty = dirty_paths(candidate)
        if candidate_dirty != set(runtime_files):
            missing = sorted(set(runtime_files) - candidate_dirty)
            extra = sorted(candidate_dirty - set(runtime_files))
            raise RuntimeError(f"candidate source mismatch: missing={missing}, extra={extra}")

        build_env = {
            **os.environ,
            "CI": "true",
            "PNPM_CONFIG_DANGEROUSLY_ALLOW_ALL_BUILDS": "true",
        }
        run(["pnpm", "--dir", "frontend", "install", "--frozen-lockfile"], cwd=candidate, env=build_env)
        run(["make", "build"], cwd=candidate, env=build_env)

        host_server = candidate / "backend" / "bin" / "server"
        frontend = candidate / "backend" / "internal" / "web" / "dist"
        if not host_server.is_file() or not frontend.is_dir():
            raise RuntimeError("candidate build did not produce the expected backend/frontend artifacts")
        version = subprocess.check_output(
            ["./scripts/resolve-version.sh"],
            cwd=candidate / "backend",
            text=True,
        ).strip()
        linux_server = candidate / "backend" / "bin" / "server-linux-amd64"
        linux_env = {**build_env, "CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64"}
        run(
            [
                "go", "build",
                "-buildvcs=false",
                f"-ldflags=-s -w -X main.Version={version}",
                "-trimpath",
                "-o", str(linux_server),
                "./cmd/server",
            ],
            cwd=candidate / "backend",
            env=linux_env,
        )
        linux_metadata = subprocess.check_output(
            ["go", "version", "-m", str(linux_server)],
            cwd=candidate,
            text=True,
        )
        linux_build_metadata = [line.strip() for line in linux_metadata.splitlines() if "build\t" in line]

        frontend_hash, frontend_files = sha256_tree(frontend)
        runtime_hashes = {relative: sha256_file(ROOT / relative) for relative in runtime_files}
        artifact_payload_hash = ""
        artifact_file_count = 0
        if artifact_dir is not None:
            if artifact_dir.exists():
                raise RuntimeError(f"artifact directory already exists: {artifact_dir}")
            artifact_dir.mkdir(parents=True)
            shutil.copy2(linux_server, artifact_dir / "server-linux-amd64")
            shutil.copytree(frontend, artifact_dir / "frontend")
            migrations_dir = artifact_dir / "migrations"
            migrations_dir.mkdir()
            for relative in runtime_files:
                if relative.startswith("backend/migrations/"):
                    shutil.copy2(ROOT / relative, migrations_dir / pathlib.Path(relative).name)
            artifact_payload_hash, artifact_files = sha256_tree(artifact_dir)
            artifact_file_count = len(artifact_files)
            checksum_lines = [f"{digest}  {relative}" for relative, digest in sorted(artifact_files.items())]
            (artifact_dir / "SHA256SUMS").write_text("\n".join(checksum_lines) + "\n", encoding="utf-8")

        report: dict[str, Any] = {
            "schema_version": 1,
            "change_id": manifest["change_id"],
            "base_ref": head,
            "runtime_file_count": len(runtime_files),
            "runtime_files": runtime_files,
            "runtime_file_sha256": runtime_hashes,
            "source_dirty_path_count": len(current_dirty),
            "candidate_dirty_paths": sorted(candidate_dirty),
            "artifacts": {
                "build_version": version,
                "host_backend_server_sha256": sha256_file(host_server),
                "host_backend_server_size": host_server.stat().st_size,
                "linux_amd64_server_sha256": sha256_file(linux_server),
                "linux_amd64_server_size": linux_server.stat().st_size,
                "linux_amd64_build_metadata": linux_build_metadata,
                "frontend_tree_sha256": frontend_hash,
                "frontend_file_count": len(frontend_files),
                "artifact_directory": str(artifact_dir) if artifact_dir is not None else "",
                "artifact_payload_tree_sha256": artifact_payload_hash,
                "artifact_payload_file_count": artifact_file_count,
            },
            "checks": {
                "head_matches_manifest_base": True,
                "all_dirty_backend_runtime_allowlisted": True,
                "no_dirty_frontend_or_deploy_input": True,
                "candidate_contains_only_allowlisted_dirty_paths": True,
                "clean_candidate_build_passed": True,
            },
        }
        report_path.parent.mkdir(parents=True, exist_ok=True)
        report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print(json.dumps({"report": str(report_path), "artifacts": report["artifacts"], "checks": report["checks"]}, ensure_ascii=False, indent=2))
        return report
    finally:
        if worktree_added:
            subprocess.run(
                ["git", "worktree", "remove", "--force", str(candidate)],
                cwd=ROOT,
                check=False,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
        shutil.rmtree(temp_root, ignore_errors=True)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", type=pathlib.Path, default=DEFAULT_MANIFEST)
    parser.add_argument("--report", type=pathlib.Path, required=True)
    parser.add_argument("--artifact-dir", type=pathlib.Path)
    args = parser.parse_args()
    try:
        artifact_dir = args.artifact_dir.resolve() if args.artifact_dir is not None else None
        build_candidate(args.manifest.resolve(), args.report.resolve(), artifact_dir)
        return 0
    except (OSError, RuntimeError, ValueError, json.JSONDecodeError, subprocess.CalledProcessError) as error:
        print(f"staging candidate build failed: {error}", file=os.sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
