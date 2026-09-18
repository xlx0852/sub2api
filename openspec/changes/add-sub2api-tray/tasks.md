## 1. Scaffold

- [x] 1.1 Create `tools/sub2api-tray` Go module and cmd entry
- [x] 1.2 Add local state store under `~/.sub2api-tray/`

## 2. Control plane

- [x] 2.1 Implement loopback HTTP control API with capability token
- [x] 2.2 Health/status/setup/model-set/repair/models endpoints
- [x] 2.3 Embed minimal HTML panel and open via system browser

## 3. Codex config

- [x] 3.1 Managed provider block writer with markers
- [x] 3.2 Snapshot + rollback on write failure / repair
- [x] 3.3 Unit tests for config mutate/repair

## 4. Tray shell

- [x] 4.1 System tray menu: status, models, panel, repair, quit
- [x] 4.2 Headless mode (`--headless`) for CI/servers without GUI
- [x] 4.3 README with build/run size notes
- [x] 4.4 Tray icon + default port `127.0.0.1:8787` + `panel.url`
- [x] 4.5 Login autostart (macOS/Linux/Windows) via menu/API/CLI
- [x] 4.6 Account password login → auto create/reuse API key → Codex

## 5. Validate

- [x] 5.1 `go test` for codexcfg/control/autostart/account
- [ ] 5.2 Manual macOS smoke (tray + Codex config write + autostart + password login)
