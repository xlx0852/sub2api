# Change: Add lightweight Sub2API Codex tray companion

## Why

Codex users currently must hand-edit `~/.codex/config.toml` to point at Sub2API.
We need a small local companion (not Electron) that configures Codex, switches
models, and shows connection status from the menu bar / system tray.

## What Changes

- Add `tools/sub2api-tray`: a Go single-binary local companion
  - loopback control API
  - managed Codex config writer (marker + snapshot + rollback)
  - minimal browser panel (embedded HTML, system browser)
  - system tray menu (optional; headless mode always available)
- Target install size ≤ ~15MB (no Electron/Chromium)
- Does **not** reimplement gateway protocol conversion; traffic still goes to Sub2API

## Impact

- Affected code: new tree under `tools/sub2api-tray/` only
- Specs: new capability `codex-tray`
- No backend API/DB migration required for MVP
