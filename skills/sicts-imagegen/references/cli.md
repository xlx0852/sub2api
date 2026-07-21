# sicts-image-gen CLI reference

Binary crate: `tools/sicts-image-gen` (skill default for SicTs image traffic).

**Routing policy:** use this CLI for generate/edit under SicTs/Sub2API. Do not fall back to system `imagegen` / `scripts/image_gen.py` / `api.openai.com` unless the user explicitly asks for non-SicTs Platform access.

## Build / install

```bash
cargo build --release --manifest-path tools/sicts-image-gen/Cargo.toml
cp tools/sicts-image-gen/target/release/sicts-image-gen \
  skills/sicts-imagegen/scripts/sicts-image-gen
```

## Environment

| Variable | Purpose |
|----------|---------|
| `SICTS_API_KEY` | Preferred API key |
| `OPENAI_API_KEY` | Fallback key |
| `SICTS_BASE_URL` | Preferred base (`…/v1`) |
| `OPENAI_BASE_URL` | Fallback base |
| `CODEX_HOME` | Codex config dir (default `~/.codex`) |
| `SICTS_CODEX_PROVIDER` | Force `model_providers.<id>` from config.toml |
| `SICTS_IMAGE_GEN` | Optional absolute path to binary |

### Codex config auto-load

Unless `--no-codex-config` is set, the CLI reads:

- `$CODEX_HOME/config.toml`
  - top-level `model_provider`
  - `[model_providers.<id>].base_url`
  - `[model_providers.<id>].experimental_bearer_token` / `api_key` / `env_key`
- `$CODEX_HOME/auth.json` → `OPENAI_API_KEY` as key fallback

Resolution order: **CLI flag → env → Codex config → default base**.

Default base: `https://code.sicts.shop/v1`  
If base lacks `/v1`, the CLI appends it.

## Global flags

```text
--base-url <url>
--api-key <key>
--codex-home <dir>
--provider <id>          # e.g. OpenAI / sicts
--no-codex-config
--print-config           # stderr: resolved base/provider/key source
--dry-run
--timeout <seconds>      # default 900
```

## generate

```bash
sicts-image-gen generate \
  --prompt '...' \
  --prompt-file path.txt \
  --model gpt-image-2 \
  --size auto|1024x1024|1536x1024|... \
  --quality low|medium|high|auto \
  --n 1 \
  --output-format png|jpeg|webp \
  --background transparent|opaque|auto \
  --moderation auto|low \
  --out output/imagegen/output.png \
  --out-dir output/imagegen/
```

Notes:

- Uses `POST {base}/images/generations` with `response_format=b64_json`.
- `gpt-image-2` rejects `--background transparent`.
- Prompt may also be piped on stdin.

## edit

```bash
sicts-image-gen edit \
  --image in.png \
  --image extra.png \
  --mask mask.png \
  --prompt 'change only the background' \
  --model gpt-image-2 \
  --size auto \
  --quality medium \
  --input-fidelity high \
  --out edited.png
```

Notes:

- Multipart `POST {base}/images/edits`.
- Do not pass `--input-fidelity` with `gpt-image-2`.

## generate-batch

JSONL, one object per line:

```json
{"prompt":"red mug product shot","size":"1024x1024","quality":"low"}
{"prompt":"blue mug product shot"}
```

```bash
sicts-image-gen generate-batch \
  --input tmp/imagegen/prompts.jsonl \
  --out-dir output/imagegen/batch \
  --model gpt-image-2 \
  --concurrency 4
```

### Parallelism

- Jobs run in a worker pool (default **4** concurrent requests).
- `--concurrency 1` restores serial behavior.
- Max `--concurrency` is **16** (also via env `SICTS_BATCH_CONCURRENCY`).
- Output paths still print in **JSONL order** (`image_001`, `image_002`, …).
- If some jobs fail, completed paths still print; exit is non-zero with a summary.

Raise concurrency carefully: gateway rate limits / account concurrency may throttle or 429.

## remove-chroma

Local chroma-key → alpha (no API). Bundled `scripts/remove_chroma_key.py` (Pillow).

```bash
sicts-image-gen remove-chroma \
  --input brand/source/asset-chroma.png \
  --out brand/asset.png \
  --auto-key border \
  --soft-matte true \
  --transparent-threshold 12 \
  --opaque-threshold 220 \
  --despill true \
  --force
```

| Flag | Default | Notes |
|------|---------|--------|
| `--auto-key` | `border` | `none` / `corners` / `border` |
| `--key-color` | `#00ff00` | used when `--auto-key none` |
| `--soft-matte` | true | soft edge matte |
| `--despill` | true | remove green/magenta fringe |
| `--force` | false | overwrite `--out` |

Direct script path (same options as upstream Codex helper):

```bash
python3 "${CODEX_HOME:-$HOME/.codex}/skills/sicts-imagegen/scripts/remove_chroma_key.py" ...
```

## Exit behavior

- Success: prints one output path per line on stdout (generate/edit/batch), or helper logs for remove-chroma.
- Failure: message on stderr, non-zero exit.
