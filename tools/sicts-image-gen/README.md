# sicts-image-gen

Lightweight Rust CLI for SicTs / Sub2API **Images API**.  
Intended as a Codex **built-in `image_gen` fallback** when the host tool is not mounted (API-key / custom `base_url` sessions).

## Build

```bash
cd tools/sicts-image-gen
cargo build --release
# binary: target/release/sicts-image-gen
```

Install into skill scripts path (optional):

```bash
cp target/release/sicts-image-gen ../../skills/sicts-imagegen/scripts/
```

## Auth / config resolution

Priority (high → low):

1. CLI flags: `--base-url`, `--api-key`
2. Env: `SICTS_BASE_URL` / `SICTS_API_KEY`, then `OPENAI_BASE_URL` / `OPENAI_API_KEY`
3. **Codex home** (`$CODEX_HOME` or `~/.codex`):
   - `config.toml` → active `model_provider` block:
     - `base_url`
     - `experimental_bearer_token` / `api_key` / `env_key`
   - `auth.json` → `OPENAI_API_KEY` (key fallback)
4. Default base: `https://code.sicts.shop/v1`

```bash
# zero-config if Codex already points at SicTs:
sicts-image-gen --print-config --dry-run generate --prompt 'test' --out /tmp/t.png

# explicit provider from config.toml
sicts-image-gen --provider OpenAI generate --prompt '...' --out out.png

# disable codex config
sicts-image-gen --no-codex-config --api-key sk-... --base-url https://code.sicts.shop/v1 ...
```

Bare host without `/v1` is auto-suffixed.

## Commands

```bash
# generate
sicts-image-gen generate \
  --prompt 'a soft translucent campus mascot' \
  --model gpt-image-2 \
  --size 1024x1024 \
  --quality high \
  --out brand/mascot.png

# dry-run
sicts-image-gen --dry-run generate --prompt 'test' --out /tmp/t.png

# edit
sicts-image-gen edit \
  --image ref.png \
  --prompt 'replace only the background with soft gradient' \
  --out brand/mascot-edit.png

# batch JSONL: {"prompt":"..."} per line (parallel, default concurrency=4)
sicts-image-gen generate-batch \
  --input tmp/prompts.jsonl \
  --out-dir output/imagegen/batch \
  --concurrency 4
```

## Notes

- Defaults to `gpt-image-2` and `response_format=b64_json`, then writes files.
- `gpt-image-2` rejects `--background transparent` (use `gpt-image-1.5`, or chroma-key + `remove-chroma`).
- Local cutout: `sicts-image-gen remove-chroma --input src.png --out out.png --auto-key border --force` (needs `python3` + Pillow).
- Pair with skill: `skills/sicts-imagegen/SKILL.md`.
