---
name: codex-desktop-gpt56-picker
description: Fix Codex Desktop (ChatGPT.app 26.x) model picker missing GPT-5.6 Sol/Terra/Luna when API-key or custom-provider mode is used. Seeds ~/.codex model cache/catalog, patches Desktop Statsig allowlist via CDP, and verifies model/list. Use when Codex App shows only GPT-5.5/5.4/5.2, marks gpt-5.6-* as 自定义/Custom, forced 5.6 calls work but the picker hides them, or the user asks to refresh Codex Desktop models.
compatibility: macOS with Codex Desktop ChatGPT.app; Python 3.10+; optional websockets package for CDP patch.
---

# Codex Desktop GPT-5.6 Model Picker

Make **Codex Desktop** (bundle `com.openai.codex`, app name often still `ChatGPT.app`) show and select:

- `gpt-5.6-sol`
- `gpt-5.6-terra`
- `gpt-5.6-luna`

This is **not** ChatGPT Classic (`com.openai.chat`). Classic will never use this path.

## Root Cause (read first)

Two independent layers:

| Layer | Symptom if broken | Fix |
|-------|-------------------|-----|
| **A. Model discovery** | `model/list` has no 5.6; App uses bundled old catalog | Seed `~/.codex/models_cache.json` + `model_catalog.json` |
| **B. Desktop UI filter** | `model/list` has 5.6, but picker shows 5.5/5.4/5.2 + 自定义 | Patch Statsig dynamic config `107580212` via CDP |

Forced inference with `model=gpt-5.6-sol` can work while the picker is still wrong. That is expected.

When Statsig is live with:

```json
{
  "use_hidden_models": true,
  "available_models": ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.2"]
}
```

the advanced picker hides every model not in `available_models`. A configured `gpt-5.6-sol` then appears as **自定义 / Custom**.

## Agent Workflow (do this in order)

### 0) Confirm target app

```bash
# Must be Codex Desktop, not Classic
/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "/Applications/ChatGPT.app/Contents/Info.plist"
# expected: com.openai.codex

codex --version
# prefer >= 0.144.0 (5.6 minimal_client_version)
```

If the user also has **ChatGPT Classic.app**, quit it to avoid confusion.

### 1) Refresh local model cache (persistent)

Run from this skill directory:

```bash
python3 scripts/refresh-models-cache.py
# optional explicit provider:
# python3 scripts/refresh-models-cache.py --base-url https://YOUR_GATEWAY --api-key sk-...
```

This writes:

- `~/.codex/models_cache.json`
- `~/.codex/model_catalog.json`

and best-effort updates `~/.codex/config.toml`:

- `model = "gpt-5.6-sol"` (only if missing or previously a 5.x default you choose to upgrade)
- `model_catalog_json = "~/.codex/model_catalog.json"` absolute path

Prefer online fetch from the user's provider (reads `config.toml` base_url + `auth.json` / env).
If online fetch fails, falls back to [assets/models_cache.example.json](assets/models_cache.example.json).

### 2) Verify app-server actually lists 5.6

```bash
python3 scripts/verify-model-list.py
```

Success means `model/list` includes `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`.

If this fails, stop and fix Layer A (cache / provider / client version). Do not claim UI is fixed yet.

### 3) Patch Desktop Statsig allowlist (in-memory; lost on quit)

```bash
# Quit existing Codex Desktop first if it was launched without CDP
pkill -x ChatGPT 2>/dev/null || true
sleep 1

open -na "/Applications/ChatGPT.app" --args --remote-debugging-port=9222 --remote-allow-origins=*
sleep 5

python3 scripts/patch-codex-model-picker.py
```

Or one-shot:

```bash
bash scripts/apply-codex-5.6-picker.sh
```

Patch success criteria:

- Statsig config `107580212` has `use_hidden_models: false`
- `available_models` includes the 5.6 trio
- Composer chip shows something like **`5.6 Sol 极高` / `5.6 Sol`**, not only 自定义

### 4) User verification checklist

Ask the user to confirm on **Codex Desktop** only:

1. Bottom model chip is `5.6 Sol` / `5.6 Terra` / `5.6 Luna` (not bare 自定义)
2. Opening the model menu shows the 5.6 family
3. A short task runs without falling back to 5.2/5.4

## One-shot for the user (prefer simplest)

If `~/.codex/bin/codex56` is installed (or linked as `~/.local/bin/codex56`):

```bash
codex56              # relaunch + turn off use_hidden_models
codex56 --refresh    # also refresh models cache first
codex56 --patch-only # when already launched with CDP :9222
```

Otherwise from this skill directory:

```bash
bash scripts/apply-codex-5.6-picker.sh
```

Double-click option (macOS): `~/Desktop/Codex5.6启动.command`

## Important limits

1. **Statsig CDP patch is temporary.** Quit/relaunch without the script → old allowlist returns.
2. **Layer A cache is persistent.** Keep `models_cache.json` refreshed when the provider adds models.
3. **Do not** tell the user to use ChatGPT Classic for this fix.
4. **Do not** claim gateway is broken if forced 5.6 inference already works.
5. Passwordless `/etc/hosts` Statsig blocking is optional and usually unnecessary; prefer CDP patch.
6. Remote host misconfig (`selected-remote-host-id` pointing at a dead SSH host) can make the UI look empty. If needed, set `selected-remote-host-id` to `null` in `~/.codex/.codex-global-state.json` after backup.

## Diagnosis cheat sheet

| Observation | Likely layer |
|-------------|--------------|
| `curl $BASE/v1/models` has 5.6, Desktop picker does not | B (Statsig) and/or A (cache) |
| `python3 scripts/verify-model-list.py` lacks 5.6 | A |
| `verify-model-list` has 5.6, UI still 5.2/自定义 | B |
| Chip is `5.6 Sol` but advanced list old | B not applied / page not reloaded |
| Only Classic shows wrong models | Wrong app |

## Config hints (API key / custom provider)

Typical `~/.codex/config.toml` fragment:

```toml
model_provider = "OpenAI"
model = "gpt-5.6-sol"
review_model = "gpt-5.5"
model_catalog_json = "/Users/YOU/.codex/model_catalog.json"
model_reasoning_effort = "ultra"

[model_providers.OpenAI]
name = "OpenAI"
base_url = "https://YOUR_GATEWAY"
wire_api = "responses"
requires_openai_auth = true
```

Auth is usually `~/.codex/auth.json` (`OPENAI_API_KEY` style) or env.

## Deeper references

- [references/root-cause.md](references/root-cause.md) — discovery vs Statsig filter internals
- [references/troubleshooting.md](references/troubleshooting.md) — recovery steps

## Safety

- Only patch the local Desktop renderer via CDP on the user's machine.
- Do not exfiltrate API keys.
- Backup before rewriting global state:
  - `cp ~/.codex/models_cache.json ~/.codex/models_cache.json.bak-$(date +%Y%m%d%H%M%S)`
  - `cp ~/.codex/.codex-global-state.json ~/.codex/.codex-global-state.json.bak-$(date +%Y%m%d%H%M%S)` when editing it
