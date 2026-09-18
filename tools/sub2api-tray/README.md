# sub2api-tray

Lightweight **Codex companion** for Sub2API.

- System tray + loopback control API
- Writes managed `~/.codex/config.toml` + `auth.json`
- Settings panel opens in a **native OS window** (system WebView / WKWebView; no Electron, no browser required)
- Login autostart (macOS LaunchAgent / Linux `.desktop` / Windows Startup)
- Target: single Go binary ≈ 6–8MB

Gateway protocol conversion stays in Sub2API. This tool only configures the client.

## Build

```bash
cd tools/sub2api-tray
make test
make build-all
ls -lh dist/
```

- `dist/sub2api-tray` — GUI tray (macOS needs CGO/AppKit)
- `dist/sub2api-tray-headless` — pure Go control server (`-tags headless`)

## Run

```bash
# tray + native panel window (default listen 127.0.0.1:8787)
./dist/sub2api-tray

# open panel window again (menu: 打开面板) or:
./dist/sub2api-tray --open

# headless control server only
./dist/sub2api-tray-headless --headless

# enable / disable login autostart
./dist/sub2api-tray --install-autostart
./dist/sub2api-tray --uninstall-autostart
```

Panel URL is printed on stderr and also written to:

```text
~/.sub2api-tray/panel.url
```

Capability token is required: `http://127.0.0.1:8787/c/<token>/panel`

## Panel / tray actions

### A. 账号密码（推荐）

1. 站点地址（`https://code.example` 或 `…/v1` 均可）
2. 邮箱 + 密码（如开启 2FA 再填验证码）
3. **登录并写入 Codex**
   - `POST /api/v1/auth/login`（+ 可选 `/login/2fa`）
   - 自动创建/复用名为 `sub2api-tray` 的 API Key
   - 写入 `~/.codex` managed 块
4. 完全退出并重启 Codex

> 密码只用于本次登录请求，**不会**写入 `settings.json`。

### B. 直接 API Key

1. Paste Sub2API Base URL (`https://host/v1`) and API Key
2. **测试连接** → `GET {base}/models`
3. **保存并写入 Codex** → managed provider block + `auth.json`
4. Fully quit & restart Codex
5. Optional: **开机启动**

## Managed config markers

```toml
# BEGIN sub2api-tray managed
model_provider = "sub2api"
model = "gpt-5.6-sol"
...
[model_providers.sub2api]
base_url = "https://.../v1"
wire_api = "responses"
requires_openai_auth = false
# END sub2api-tray managed
```

Tampering inside the managed block causes apply to refuse until repaired.

## State

`~/.sub2api-tray/`

- `capability` — local token
- `settings.json` — last base URL / key / model list
- `panel.url` — current panel URL
- `launchd.*.log` — macOS autostart logs (if enabled)

## Control API (loopback + token)

| Method | Path | Purpose |
|---|---|---|
| GET | `/health` | liveness |
| GET | `/status` | settings + codex + autostart |
| POST | `/setup` | write Codex config (API key mode) |
| POST | `/login` | email/password login → API key → Codex |
| POST | `/login/2fa` | complete TOTP then write Codex |
| POST | `/model-set` | switch model |
| POST | `/repair` | restore managed block |
| GET | `/models` | list models |
| POST | `/test` | probe upstream `/models` |
| GET/POST | `/autostart` | login autostart |

## Non-goals (MVP)

- Electron / embedded Chromium
- Local Responses↔Chat bridge
- Provider OAuth
- Usage charts
