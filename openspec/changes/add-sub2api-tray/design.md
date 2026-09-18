## Context

Sub2API is the server gateway. The tray is a **local client companion** only:
configure Codex, show status, switch model labels. Protocol/WS/billing stay in
the gateway.

## Goals / Non-Goals

- Goals: ≤15MB binary class; loopback control API; safe Codex config mutation; tray + browser panel
- Non-Goals: Electron UI; local protocol bridge; provider OAuth; usage charts; multi-client surfaces

## Decisions

1. **Separate Go module** under `tools/sub2api-tray` so gateway builds never pull CGO/tray deps.
2. **System browser panel** instead of embedded webview to keep size small.
3. **Capability token** in path (`/c/<token>/...`) mirrors Codex Router’s local auth idea; bind `127.0.0.1` only.
4. **Managed config markers** in `config.toml` so repair/re-apply cannot clobber unrelated user config without detection.
5. **Headless mode** runs control server without systray for tests and Linux servers.

## Risks

- systray may need CGO/AppKit on macOS — document build requirements; headless always works
- Codex config schema drifts — keep writer minimal (`wire_api=responses`, base_url, model)
