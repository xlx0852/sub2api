#!/usr/bin/env bash
# Build multi-OS / multi-CPU release packages for sicts-imagegen.
#
# Produces:
#   dist/sicts-imagegen-all.zip          — universal package (all platforms)
#   dist/sicts-imagegen-<platform>.zip   — thin single-platform packages
#
# Universal install auto-selects the host binary into scripts/sicts-image-gen.
set -euo pipefail

export PATH="${HOME}/.cargo/bin:${PATH}"

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
CRATE="$ROOT/tools/sicts-image-gen"
SKILL_SRC="$ROOT/skills/sicts-imagegen"
DIST="$ROOT/dist"
VERSION="$(grep -E '^version' "$CRATE/Cargo.toml" | head -1 | sed -E 's/.*"([^"]+)".*/\1/')"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"

mkdir -p "$DIST"

# platform-id|rust-target|artifact-name|bin-subdir
# platform-id is used in thin zip names and BUILD_INFO
TARGETS=(
  "macos-arm64|aarch64-apple-darwin|sicts-image-gen|darwin-arm64"
  "macos-x64|x86_64-apple-darwin|sicts-image-gen|darwin-x64"
  "linux-arm64|aarch64-unknown-linux-musl|sicts-image-gen|linux-arm64"
  "linux-x64|x86_64-unknown-linux-musl|sicts-image-gen|linux-x64"
  "windows-x64|x86_64-pc-windows-gnu|sicts-image-gen.exe|windows-x64"
  "windows-arm64|aarch64-pc-windows-gnullvm|sicts-image-gen.exe|windows-arm64"
)

# Map: platform-id -> absolute path of built binary
declare -A BUILT_BINS=()
declare -A BIN_SUBDIRS=()
declare -A ARTIFACT_NAMES=()

build_binary() {
  local label="$1"
  local triple="$2"
  local artifact="$3"
  local subdir="$4"

  echo ""
  echo "======== building ${label} (${triple}) ========"

  if ! rustup target list --installed | grep -qx "$triple"; then
    rustup target add "$triple"
  fi

  local host
  host="$(rustc -vV | awk '/^host:/{print $2}')"
  if [[ "$triple" == "$host" ]]; then
    (cd "$CRATE" && cargo build --release --target "$triple")
  else
    (cd "$CRATE" && cargo zigbuild --release --target "$triple")
  fi

  local built="$CRATE/target/${triple}/release/${artifact}"
  if [[ ! -f "$built" ]]; then
    echo "ERROR: missing binary: $built" >&2
    return 1
  fi

  if command -v strip >/dev/null 2>&1 && [[ "$artifact" != *.exe ]]; then
    strip -x "$built" 2>/dev/null || strip "$built" 2>/dev/null || true
  fi

  BUILT_BINS["$label"]="$built"
  BIN_SUBDIRS["$label"]="$subdir"
  ARTIFACT_NAMES["$label"]="$artifact"
  echo "OK  ${label}: $(ls -lh "$built" | awk '{print $5}')"
}

# Shared config.toml patcher (single source: tools/sicts-image-gen/patch_skills_config.py)
# Critical: Windows paths must use forward slashes in TOML (C:/Users/...), never raw \U.
write_config_patch_py() {
  local out="$1"
  cp "$CRATE/patch_skills_config.py" "$out"
  chmod +x "$out"
}

write_detect_platform_sh() {
  # Shared bash function body for platform detection
  cat <<'EOF'
detect_platform() {
  local os arch
  os="$(uname -s 2>/dev/null || echo unknown)"
  arch="$(uname -m 2>/dev/null || echo unknown)"
  case "$os" in
    Darwin)
      case "$arch" in
        arm64|aarch64) echo "darwin-arm64" ;;
        x86_64|amd64)  echo "darwin-x64" ;;
        *) echo "unsupported:darwin-$arch" ;;
      esac
      ;;
    Linux)
      case "$arch" in
        aarch64|arm64) echo "linux-arm64" ;;
        x86_64|amd64)  echo "linux-x64" ;;
        *) echo "unsupported:linux-$arch" ;;
      esac
      ;;
    MINGW*|MSYS*|CYGWIN*)
      case "$arch" in
        aarch64|arm64) echo "windows-arm64" ;;
        x86_64|amd64|i686|i386) echo "windows-x64" ;;
        *) echo "unsupported:windows-$arch" ;;
      esac
      ;;
    *)
      echo "unsupported:$os-$arch"
      ;;
  esac
}

platform_bin_name() {
  case "$1" in
    windows-*) echo "sicts-image-gen.exe" ;;
    *) echo "sicts-image-gen" ;;
  esac
}
EOF
}

write_universal_install_sh() {
  local out="$1"
  {
    cat <<'EOF'
#!/usr/bin/env bash
# Universal installer: auto-selects OS/CPU binary into scripts/ for Codex.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
DEST="$CODEX_HOME/skills/sicts-imagegen"
export SICTS_DISABLE_SYSTEM_IMAGEGEN="${SICTS_DISABLE_SYSTEM_IMAGEGEN:-1}"

EOF
    write_detect_platform_sh
    cat <<'EOF'

echo "==> sicts-imagegen universal install"
echo "    package root: $ROOT"
echo "    codex home:   $CODEX_HOME"

PLATFORM="$(detect_platform)"
if [[ "$PLATFORM" == unsupported:* ]]; then
  echo "ERROR: unsupported platform: ${PLATFORM#unsupported:}" >&2
  echo "Supported: darwin-arm64 darwin-x64 linux-arm64 linux-x64 windows-x64 windows-arm64" >&2
  echo "On Windows prefer: powershell -File install.ps1" >&2
  exit 1
fi

BIN_SRC_NAME="$(platform_bin_name "$PLATFORM")"
BIN_SRC="$ROOT/skills/sicts-imagegen/bin/$PLATFORM/$BIN_SRC_NAME"
if [[ ! -f "$BIN_SRC" ]]; then
  echo "ERROR: missing binary for $PLATFORM: $BIN_SRC" >&2
  echo "Available platforms:" >&2
  ls -1 "$ROOT/skills/sicts-imagegen/bin" 2>/dev/null || true
  exit 1
fi

BIN_DST_NAME="$BIN_SRC_NAME"
# On Unix always install as sicts-image-gen (skill expects this name)
if [[ "$PLATFORM" != windows-* ]]; then
  BIN_DST_NAME="sicts-image-gen"
fi

echo "==> detected platform: $PLATFORM"
echo "==> binary: $BIN_SRC"

mkdir -p "$CODEX_HOME/skills"
rm -rf "$DEST"
mkdir -p "$DEST"

# Copy skill tree (docs + all bins + scripts helpers)
cp -R "$ROOT/skills/sicts-imagegen/." "$DEST/"

# Activate host binary as the default CLI name Codex/skill invokes
mkdir -p "$DEST/scripts"
cp "$BIN_SRC" "$DEST/scripts/$BIN_DST_NAME"
chmod +x "$DEST/scripts/$BIN_DST_NAME" 2>/dev/null || true
# Also keep a stable non-.exe name on Windows Git-Bash if possible
if [[ "$BIN_DST_NAME" == *.exe ]]; then
  cp "$BIN_SRC" "$DEST/scripts/sicts-image-gen.exe"
fi

# Record which binary was selected
cat >"$DEST/scripts/SELECTED_PLATFORM.txt" <<SEL
platform=$PLATFORM
source=bin/$PLATFORM/$BIN_SRC_NAME
installed_as=scripts/$BIN_DST_NAME
selected_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)
SEL

# Optional: drop other platform bins to save space (keep by default for re-select)
if [[ "${SICTS_INSTALL_STRIP_OTHER_BINS:-0}" == "1" ]]; then
  echo "==> stripping other platform binaries (SICTS_INSTALL_STRIP_OTHER_BINS=1)"
  find "$DEST/bin" -mindepth 1 -maxdepth 1 ! -name "$PLATFORM" -exec rm -rf {} +
fi

CFG="$CODEX_HOME/config.toml"
python3 "$ROOT/patch_skills_config.py" \
  "$CFG" "$DEST/SKILL.md" "$CODEX_HOME" "$SICTS_DISABLE_SYSTEM_IMAGEGEN"

echo "==> self-check"
ACTIVE="$DEST/scripts/$BIN_DST_NAME"
if [[ -x "$ACTIVE" ]] || [[ -f "$ACTIVE" ]]; then
  "$ACTIVE" --version 2>/dev/null || true
  "$ACTIVE" --print-config --dry-run generate --prompt 'install-probe' --out /tmp/sicts-imagegen-install-probe.png || true
  if command -v python3 >/dev/null 2>&1 && [[ -f "$DEST/scripts/remove_chroma_key.py" ]]; then
    echo "==> chroma helper present: $DEST/scripts/remove_chroma_key.py"
  fi
else
  echo "WARN: active binary not executable: $ACTIVE" >&2
fi

echo ""
echo "Done. Platform selected: $PLATFORM"
echo "Binary: $ACTIVE"
echo "1) Restart ChatGPT.app / Codex Desktop"
echo "2) Open a NEW chat: 用 \$sicts-imagegen 生成一张测试图"
echo "Re-run this installer on another machine to auto-pick that host's binary."
echo "Disable system imagegen skip: SICTS_DISABLE_SYSTEM_IMAGEGEN=0 ./install.sh"
EOF
  } >"$out"
  chmod +x "$out"
}

write_universal_install_ps1() {
  local out="$1"
  cat >"$out" <<'EOF'
# Universal Windows installer for sicts-imagegen
# Auto-selects windows-x64 / windows-arm64 binary for Codex.
$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$CodexHome = if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $env:USERPROFILE ".codex" }
$Dest = Join-Path $CodexHome "skills\sicts-imagegen"
$DisableSystem = if ($env:SICTS_DISABLE_SYSTEM_IMAGEGEN) { $env:SICTS_DISABLE_SYSTEM_IMAGEGEN } else { "1" }

function Get-PlatformId {
  $arch = $env:PROCESSOR_ARCHITECTURE
  switch -Regex ($arch) {
    'ARM64' { return 'windows-arm64' }
    'AMD64|x86' { return 'windows-x64' }
    default { throw "Unsupported architecture: $arch" }
  }
}

$Platform = Get-PlatformId
$BinSrc = Join-Path $Root "skills\sicts-imagegen\bin\$Platform\sicts-image-gen.exe"
if (-not (Test-Path $BinSrc)) {
  throw "Missing binary for $Platform : $BinSrc"
}

Write-Host "==> sicts-imagegen universal install"
Write-Host "    platform: $Platform"
Write-Host "    dest:     $Dest"

New-Item -ItemType Directory -Force -Path (Join-Path $CodexHome "skills") | Out-Null
if (Test-Path $Dest) { Remove-Item -Recurse -Force $Dest }
New-Item -ItemType Directory -Force -Path $Dest | Out-Null
Copy-Item -Recurse -Force (Join-Path $Root "skills\sicts-imagegen\*") $Dest

$Scripts = Join-Path $Dest "scripts"
New-Item -ItemType Directory -Force -Path $Scripts | Out-Null
Copy-Item -Force $BinSrc (Join-Path $Scripts "sicts-image-gen.exe")
# Some shells look for no extension
Copy-Item -Force $BinSrc (Join-Path $Scripts "sicts-image-gen")

@"
platform=$Platform
source=bin/$Platform/sicts-image-gen.exe
installed_as=scripts/sicts-image-gen.exe
selected_utc=$((Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ"))
"@ | Set-Content -Encoding utf8 (Join-Path $Scripts "SELECTED_PLATFORM.txt")

$Cfg = Join-Path $CodexHome "config.toml"
$SkillMd = Join-Path $Dest "SKILL.md"
$Patch = Join-Path $Root "patch_skills_config.py"
$Py = if (Get-Command python -ErrorAction SilentlyContinue) { "python" } else { "python3" }
& $Py $Patch $Cfg $SkillMd $CodexHome $DisableSystem

$Active = Join-Path $Scripts "sicts-image-gen.exe"
Write-Host "==> self-check"
& $Active --version
Write-Host ""
Write-Host "Done. Platform: $Platform"
Write-Host "Binary: $Active"
Write-Host "Restart Codex / ChatGPT and open a NEW chat."
EOF
}

write_uninstall_sh() {
  local out="$1"
  cat >"$out" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
DEST="$CODEX_HOME/skills/sicts-imagegen"
if [ -d "$DEST" ]; then
  rm -rf "$DEST"
  echo "removed $DEST"
else
  echo "not installed: $DEST"
fi
echo "Note: [[skills.config]] entries in config.toml are left intact; remove manually if needed."
EOF
  chmod +x "$out"
}

write_universal_readme() {
  local out="$1"
  cat >"$out" <<EOF
# sicts-imagegen (universal / all platforms)

**One package for all OS/CPU.** Installer auto-selects the correct CLI binary for this machine.

Included platforms:

- macOS Apple Silicon (\`darwin-arm64\`)
- macOS Intel (\`darwin-x64\`)
- Linux arm64 / x64 (musl static)
- Windows arm64 / x64

## Recommended: attach zip in Codex

1. Attach this zip in **Codex Desktop / ChatGPT.app**
2. Send:

\`\`\`text
安装一下这个插件
\`\`\`

3. Fully quit and restart the app, open a **new** chat, then try:

\`\`\`text
用 \$sicts-imagegen 生成一张测试图
\`\`\`

## Alternate: terminal install (macOS / Linux)

\`\`\`bash
unzip sicts-imagegen-all.zip
cd sicts-imagegen-all
./install.sh
\`\`\`

## Alternate: terminal install (Windows PowerShell)

\`\`\`powershell
Expand-Archive sicts-imagegen-all.zip -DestinationPath .
cd sicts-imagegen-all
powershell -ExecutionPolicy Bypass -File .\\install.ps1
\`\`\`

### What install does

1. Detects OS + CPU
2. Copies skill into \`~/.codex/skills/sicts-imagegen\`
3. Installs the matching binary as \`scripts/sicts-image-gen\` (or \`.exe\`)
4. Keeps all platform binaries under \`bin/<platform>/\` (for re-install elsewhere)
5. Sets \`enabled = true\` for this skill; disables system \`imagegen\`
6. Does **not** rewrite your model providers / API keys

Optional:

\`\`\`bash
# do not disable system imagegen
SICTS_DISABLE_SYSTEM_IMAGEGEN=0 ./install.sh

# only keep the selected platform binary (smaller install)
SICTS_INSTALL_STRIP_OTHER_BINS=1 ./install.sh
\`\`\`

## Layout

\`\`\`
sicts-imagegen-all/
  install.sh
  install.ps1
  uninstall.sh
  README.md
  skills/sicts-imagegen/
    SKILL.md
    scripts/
      remove_chroma_key.py
      sicts-image-gen          # created at install time for this host
    bin/
      darwin-arm64/sicts-image-gen
      darwin-x64/sicts-image-gen
      linux-arm64/sicts-image-gen
      linux-x64/sicts-image-gen
      windows-x64/sicts-image-gen.exe
      windows-arm64/sicts-image-gen.exe
\`\`\`

## After install

Restart ChatGPT.app / Codex Desktop, open a **new** chat:

\`\`\`text
用 \$sicts-imagegen 生成一张测试图
\`\`\`

Chroma cutout (local, needs python3 + Pillow):

\`\`\`bash
~/.codex/skills/sicts-imagegen/scripts/sicts-image-gen remove-chroma \\
  --input in-chroma.png --out out.png --auto-key border --force
\`\`\`

## Version

- CLI: ${VERSION}
- Built: ${STAMP}
EOF
}

write_thin_install_sh() {
  local out="$1"
  local bin_name="$2"
  local platform_label="$3"
  cat >"$out" <<EOF
#!/usr/bin/env bash
set -euo pipefail
ROOT="\$(cd "\$(dirname "\$0")" && pwd)"
CODEX_HOME="\${CODEX_HOME:-\$HOME/.codex}"
DEST="\$CODEX_HOME/skills/sicts-imagegen"
BIN_NAME="${bin_name}"
export SICTS_DISABLE_SYSTEM_IMAGEGEN="\${SICTS_DISABLE_SYSTEM_IMAGEGEN:-1}"

echo "==> install sicts-imagegen (${platform_label}) into \$DEST"
mkdir -p "\$CODEX_HOME/skills"
rm -rf "\$DEST"
mkdir -p "\$DEST"
cp -R "\$ROOT/skills/sicts-imagegen/." "\$DEST/"
chmod +x "\$DEST/scripts/\$BIN_NAME" 2>/dev/null || true

python3 "\$ROOT/patch_skills_config.py" \\
  "\$CODEX_HOME/config.toml" "\$DEST/SKILL.md" "\$CODEX_HOME" "\$SICTS_DISABLE_SYSTEM_IMAGEGEN"

if [ -x "\$DEST/scripts/\$BIN_NAME" ] || [ -f "\$DEST/scripts/\$BIN_NAME" ]; then
  "\$DEST/scripts/\$BIN_NAME" --version 2>/dev/null || true
  "\$DEST/scripts/\$BIN_NAME" --print-config --dry-run generate --prompt 'install-probe' --out /tmp/sicts-imagegen-install-probe.png || true
fi
echo "Done. Restart Codex and open a new chat."
EOF
  chmod +x "$out"
}

stage_skill_docs() {
  local dest_skill="$1"
  mkdir -p "$dest_skill"
  rsync -a \
    --exclude 'scripts/sicts-image-gen' \
    --exclude 'scripts/sicts-image-gen.exe' \
    --exclude 'scripts/build.sh' \
    --exclude 'bin/' \
    "$SKILL_SRC/" "$dest_skill/"
  # ensure chroma helper is executable
  if [[ -f "$dest_skill/scripts/remove_chroma_key.py" ]]; then
    chmod +x "$dest_skill/scripts/remove_chroma_key.py"
  fi
}

build_thin_package() {
  local label="$1"
  local built="${BUILT_BINS[$label]}"
  local artifact="${ARTIFACT_NAMES[$label]}"
  local pkg_name="sicts-imagegen-${label}"
  local stage="$DIST/${pkg_name}"

  rm -rf "$stage"
  mkdir -p "$stage/skills/sicts-imagegen/scripts"
  stage_skill_docs "$stage/skills/sicts-imagegen"
  cp "$built" "$stage/skills/sicts-imagegen/scripts/${artifact}"
  chmod +x "$stage/skills/sicts-imagegen/scripts/${artifact}" 2>/dev/null || true

  write_config_patch_py "$stage/patch_skills_config.py"
  write_thin_install_sh "$stage/install.sh" "$artifact" "$label"
  write_uninstall_sh "$stage/uninstall.sh"
  cat >"$stage/README.md" <<EOF
# sicts-imagegen (${label})

Single-platform package. For auto platform selection, use \`sicts-imagegen-all.zip\`.

## Recommended: attach zip in Codex

1. Attach this zip in **Codex Desktop / ChatGPT.app**
2. Send:

\`\`\`text
安装一下这个插件
\`\`\`

3. Fully quit and restart the app, open a **new** chat.

## Alternate: terminal

\`\`\`bash
./install.sh
\`\`\`
EOF
  cat >"$stage/BUILD_INFO.txt" <<EOF
name: sicts-image-gen
version: ${VERSION}
label: ${label}
binary: skills/sicts-imagegen/scripts/${artifact}
binary_bytes: $(wc -c <"$built" | tr -d ' ')
built_utc: ${STAMP}
package_type: thin
EOF

  rm -f "$DIST/${pkg_name}.zip"
  (cd "$DIST" && zip -9 -r "${pkg_name}.zip" "$pkg_name" -x "*.DS_Store" -x "*__MACOSX*")
  echo "OK thin zip $(ls -lh "$DIST/${pkg_name}.zip" | awk '{print $5}') -> $DIST/${pkg_name}.zip"
}

build_universal_package() {
  local pkg_name="sicts-imagegen-all"
  local stage="$DIST/${pkg_name}"
  rm -rf "$stage"
  mkdir -p "$stage/skills/sicts-imagegen"
  stage_skill_docs "$stage/skills/sicts-imagegen"

  # Place all binaries under bin/<platform-id>/
  for label in "${!BUILT_BINS[@]}"; do
    local subdir="${BIN_SUBDIRS[$label]}"
    local artifact="${ARTIFACT_NAMES[$label]}"
    local built="${BUILT_BINS[$label]}"
    mkdir -p "$stage/skills/sicts-imagegen/bin/$subdir"
    cp "$built" "$stage/skills/sicts-imagegen/bin/$subdir/$artifact"
    chmod +x "$stage/skills/sicts-imagegen/bin/$subdir/$artifact" 2>/dev/null || true
  done

  # Convenience: on package host, pre-seed scripts/ with host binary so dry tests work
  # (installer always re-selects)
  local host_plat
  host_plat="$(uname -s)-$(uname -m)"
  case "$host_plat" in
    Darwin-arm64|Darwin-aarch64) host_plat=darwin-arm64 ;;
    Darwin-x86_64) host_plat=darwin-x64 ;;
    Linux-aarch64|Linux-arm64) host_plat=linux-arm64 ;;
    Linux-x86_64) host_plat=linux-x64 ;;
    *) host_plat="" ;;
  esac
  if [[ -n "$host_plat" && -f "$stage/skills/sicts-imagegen/bin/$host_plat/sicts-image-gen" ]]; then
    mkdir -p "$stage/skills/sicts-imagegen/scripts"
    cp "$stage/skills/sicts-imagegen/bin/$host_plat/sicts-image-gen" \
      "$stage/skills/sicts-imagegen/scripts/sicts-image-gen"
    chmod +x "$stage/skills/sicts-imagegen/scripts/sicts-image-gen"
  fi

  write_config_patch_py "$stage/patch_skills_config.py"
  write_universal_install_sh "$stage/install.sh"
  write_universal_install_ps1 "$stage/install.ps1"
  write_uninstall_sh "$stage/uninstall.sh"
  write_universal_readme "$stage/README.md"

  {
    echo "name: sicts-image-gen"
    echo "version: ${VERSION}"
    echo "package_type: universal"
    echo "built_utc: ${STAMP}"
    echo "platforms:"
    for label in $(printf '%s\n' "${!BUILT_BINS[@]}" | sort); do
      local subdir="${BIN_SUBDIRS[$label]}"
      local artifact="${ARTIFACT_NAMES[$label]}"
      local built="${BUILT_BINS[$label]}"
      echo "  - id: $label"
      echo "    dir: bin/$subdir/$artifact"
      echo "    bytes: $(wc -c <"$built" | tr -d ' ')"
    done
    echo "install: ./install.sh (Unix) or install.ps1 (Windows)"
    echo "note: installer auto-selects host binary into scripts/"
  } >"$stage/BUILD_INFO.txt"

  # Platform manifest for tools / Codex
  {
    echo "{"
    echo "  \"version\": \"${VERSION}\","
    echo "  \"built_utc\": \"${STAMP}\","
    echo "  \"binaries\": {"
    local first=1
    for label in $(printf '%s\n' "${!BIN_SUBDIRS[@]}" | sort); do
      local subdir="${BIN_SUBDIRS[$label]}"
      local artifact="${ARTIFACT_NAMES[$label]}"
      [[ $first -eq 1 ]] || echo ","
      first=0
      printf '    "%s": "bin/%s/%s"' "$subdir" "$subdir" "$artifact"
    done
    echo ""
    echo "  }"
    echo "}"
  } >"$stage/skills/sicts-imagegen/bin/manifest.json"

  rm -f "$DIST/${pkg_name}.zip"
  (cd "$DIST" && zip -9 -r "${pkg_name}.zip" "$pkg_name" -x "*.DS_Store" -x "*__MACOSX*")
  echo "OK universal zip $(ls -lh "$DIST/${pkg_name}.zip" | awk '{print $5}') -> $DIST/${pkg_name}.zip"
}

# --- main ---
FAILED=()
OK=()

for entry in "${TARGETS[@]}"; do
  IFS='|' read -r label triple artifact subdir <<<"$entry"
  if build_binary "$label" "$triple" "$artifact" "$subdir"; then
    OK+=("$label")
  else
    FAILED+=("$label")
  fi
done

if ((${#OK[@]} == 0)); then
  echo "ERROR: no binaries built" >&2
  exit 1
fi

echo ""
echo "======== packaging thin + universal ========"
for label in "${OK[@]}"; do
  build_thin_package "$label"
done
build_universal_package

echo ""
echo "======== summary ========"
echo "version: $VERSION  stamp: $STAMP"
echo "built: ${OK[*]}"
echo "failed: ${FAILED[*]:-none}"
ls -lh "$DIST"/sicts-imagegen-*.zip 2>/dev/null || true

if ((${#FAILED[@]} > 0)); then
  exit 1
fi
