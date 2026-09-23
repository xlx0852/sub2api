#!/usr/bin/env python3
"""Package a pre-built backend and its reviewed runtime resources atomically.

No remote fetch and no pricing conversion: the repository's LiteLLM-compatible
price file is included byte-for-byte, alongside the alias-aware model catalog.
"""
from __future__ import annotations

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import tarfile
import tempfile

RESOURCE_PATHS = (
    "model-pricing/model_prices_and_context_window.json",
    "model-catalog/catalog.json",
)


def package(binary: Path, resources: Path, output: Path) -> None:
    entries = {"sub2api": (binary.read_bytes(), 0o755)}
    for name in RESOURCE_PATHS:
        path = resources / name
        data = path.read_bytes()  # A missing resource is a failed release, not a silent omission.
        parsed = json.loads(data)
        if not isinstance(parsed, dict) or not parsed:
            raise ValueError(f"empty/invalid resource: {name}")
        if name.startswith("model-pricing/"):
            priced = [entry for entry in parsed.values() if isinstance(entry, dict) and
                      any(key in entry for key in ("input_cost_per_token", "output_cost_per_token", "output_cost_per_image", "output_cost_per_image_token"))]
            if not priced:
                raise ValueError("pricing resource contains no parseable prices")
        entries["resources/" + name] = (data, 0o644)
    checksums = "".join(f"{hashlib.sha256(data).hexdigest()}  {name}\n"
                        for name, (data, _) in sorted(entries.items())).encode()
    entries["SHA256SUMS"] = (checksums, 0o644)
    output.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=output.name + ".", dir=output.parent)
    os.close(fd)
    try:
        with tarfile.open(temporary, "w:gz") as archive:
            for name, (data, mode) in sorted(entries.items()):
                info = tarfile.TarInfo(name)
                info.size = len(data)
                info.mode = mode
                archive.addfile(info, io.BytesIO(data))
        os.replace(temporary, output)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--resources", type=Path, default=Path(__file__).resolve().parents[1] / "resources")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    package(args.binary, args.resources, args.output)
