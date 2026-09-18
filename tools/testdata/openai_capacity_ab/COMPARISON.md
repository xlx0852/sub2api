# Codex /responses capacity vs ok (gpt-6-astra)

Artifacts:
- `tools/testdata/openai_capacity_ab/capacity.json`
- `tools/testdata/openai_capacity_ab/ok.json`

Same Codex CLI ChatGPT account, same model `gpt-6-astra`, same `POST /responses` HTTP SSE path (`RUST_LOG=codex_api=trace`). Both map upstream `response.model=gpt-5.6-luna`. HTTP status is 200 in both cases.

## Named wire-level difference

Capacity (`capacity.json`, 13:22Z):
- stream: `response.created` → `in_progress` → **`error`** → **`response.failed`**
- `error.type=service_unavailable_error`, `error.code=server_is_overloaded`
- message: "Our servers are currently overloaded. Please try again later."
- CLI maps to "Selected model is at capacity. Please try a different model."
- `usage` absent, `rate_limit` object absent

OK (`ok.json`, 13:41Z, first successful astra retry):
- stream: `response.created` → `in_progress` → **output_text deltas** → **`response.completed`**
- no `error` event, `error_code` empty
- `response.status=completed`, `usage.total_tokens=19701` (`input=19696`, `output=5`)
- `rate_limit` object still absent; no `x-codex-*` in CLI SSE logs

Difference is **stream terminal event + error.code**, not quota headers and not HTTP status. Capacity is model-supply shed on the same Luna backend that later completed.
