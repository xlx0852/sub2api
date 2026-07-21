#!/usr/bin/env bash
# Install / upgrade sicts-imagegen into Codex home.
# Safe for existing config.toml:
#   - only appends/updates sicts-imagegen + optional system imagegen disable
#   - never rewrites unrelated keys
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SKILL_SRC="${SICTS_SKILL_SRC:-$ROOT/skills/sicts-imagegen}"
CRATE="$ROOT/tools/sicts-image-gen"
CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
DEST="$CODEX_HOME/skills/sicts-imagegen"
BIN_NAME="sicts-image-gen"
# Windows package may pass .exe via env
if [[ "${SICTS_BIN_NAME:-}" != "" ]]; then
  BIN_NAME="$SICTS_BIN_NAME"
fi

DISABLE_SYSTEM_IMAGEGEN="${SICTS_DISABLE_SYSTEM_IMAGEGEN:-1}"

echo "==> install sicts-imagegen into $DEST"
mkdir -p "$CODEX_HOME/skills"

# Prefer: package stage → cargo release → skill tree copy → existing install
BIN_SRC=""
if [[ -n "${SICTS_PACKAGE_ROOT:-}" && -x "$SICTS_PACKAGE_ROOT/skills/sicts-imagegen/scripts/$BIN_NAME" ]]; then
  BIN_SRC="$SICTS_PACKAGE_ROOT/skills/sicts-imagegen/scripts/$BIN_NAME"
elif [[ -x "$CRATE/target/release/${BIN_NAME%.exe}" ]]; then
  BIN_SRC="$CRATE/target/release/${BIN_NAME%.exe}"
elif [[ -x "$CRATE/target/release/$BIN_NAME" ]]; then
  BIN_SRC="$CRATE/target/release/$BIN_NAME"
elif [[ -x "$SKILL_SRC/scripts/$BIN_NAME" ]]; then
  BIN_SRC="$SKILL_SRC/scripts/$BIN_NAME"
fi

rsync -a --delete \
  --exclude 'scripts/sicts-image-gen' \
  --exclude 'scripts/sicts-image-gen.exe' \
  --exclude 'scripts/build.sh' \
  "$SKILL_SRC/" "$DEST/" 2>/dev/null || {
  # fallback without rsync
  rm -rf "$DEST"
  mkdir -p "$DEST"
  cp -R "$SKILL_SRC/." "$DEST/"
  rm -f "$DEST/scripts/build.sh" 2>/dev/null || true
}

mkdir -p "$DEST/scripts"
if [[ -n "$BIN_SRC" ]]; then
  cp "$BIN_SRC" "$DEST/scripts/$BIN_NAME"
  chmod +x "$DEST/scripts/$BIN_NAME" 2>/dev/null || true
elif [[ -x "$DEST/scripts/$BIN_NAME" ]]; then
  echo "==> keeping existing binary at $DEST/scripts/$BIN_NAME"
else
  echo "WARN: no binary found; skill docs installed without CLI" >&2
fi

CFG="$CODEX_HOME/config.toml"
PATCH_PY="$CRATE/patch_skills_config.py"
if [[ ! -f "$PATCH_PY" ]]; then
  echo "ERROR: missing $PATCH_PY" >&2
  exit 1
fi
python3 "$PATCH_PY" "$CFG" "$DEST/SKILL.md" "$CODEX_HOME" "$DISABLE_SYSTEM_IMAGEGEN"

echo "==> self-check"
if [[ -x "$DEST/scripts/$BIN_NAME" ]]; then
  "$DEST/scripts/$BIN_NAME" --version || true
  "$DEST/scripts/$BIN_NAME" --print-config --dry-run generate \
    --prompt 'install-probe' --out /tmp/sicts-imagegen-install-probe.png || true
else
  echo "WARN: binary missing, skipped self-check" >&2
fi

echo ""
echo "Done."
echo "1) Restart ChatGPT.app / Codex Desktop"
echo "2) Open a NEW chat and say: 用 \$sicts-imagegen 生成一张测试图"
echo "Binary: $DEST/scripts/$BIN_NAME"
echo "Note: system imagegen skill disabled in config (SICTS_DISABLE_SYSTEM_IMAGEGEN=0 to skip)."
