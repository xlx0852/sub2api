# Troubleshooting

## CDP unavailable

```
CDP unavailable: ...
```

Cause: Desktop was not launched with remote debugging.

Fix:

```bash
pkill -f "/Applications/ChatGPT.app/Contents/MacOS/ChatGPT" || true
open -na "/Applications/ChatGPT.app" --args --remote-debugging-port=9222 --remote-allow-origins=*
sleep 5
python3 scripts/patch-codex-model-picker.py
```

## Patch OK but UI still old

1. Confirm you are on **Codex Desktop** (`com.openai.codex`), not Classic.
2. Fully close the model menu and reopen it.
3. Re-run patch (it reloads the page).
4. Check chip text via CDP / visually — expect `5.6 Sol`, not only `自定义`.
5. Verify Layer A first:

```bash
python3 scripts/verify-model-list.py
```

## model/list missing 5.6

```bash
python3 scripts/refresh-models-cache.py --force-example
python3 scripts/verify-model-list.py
```

If still missing:

- Upgrade `codex` CLI / Desktop so client ≥ 0.144.0
- Confirm `~/.codex/config.toml` `model_catalog_json` points to the file you wrote
- Restart app-server / Desktop after cache write

## Provider online fetch returns sparse models only

Sparse `{id, object}` lists cannot power Desktop reasoning levels. Use:

- gateway Codex manifest endpoints with `client_version`
- or bundled `assets/models_cache.example.json`

The model still needs to exist on the gateway for inference; the rich cache is for UI/metadata.

## selected remote host is empty / wrong

If UI binds to a dead remote:

1. Backup `~/.codex/.codex-global-state.json`
2. Set `selected-remote-host-id` to `null`
3. Disable remote auto-connect if present
4. Relaunch Desktop

## After every reboot / app quit

Statsig patch is gone. Re-run:

```bash
bash scripts/apply-codex-5.6-picker.sh
```

Optional: create a shell alias:

```bash
alias codex56='bash /path/to/codex-desktop-gpt56-picker/scripts/apply-codex-5.6-picker.sh'
```

## websockets missing

```bash
python3 -m pip install websockets
```

The patch script auto-installs if pip works.

## Security notes for agents

- Do not print full API keys.
- Only connect CDP to localhost.
- Do not ship user `auth.json` inside the skill.
