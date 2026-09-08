## 1. Implementation
- [x] 1.1 Map OpenAI Responses input items into Gemini contents, including text, reasoning, function calls, and function_call_output.
- [x] 1.2 Reuse Antigravity project/token/proxy/retry/HTTP pipeline through an explicit Responses adapter.
- [x] 1.3 Map Gemini JSON and SSE chunks into OpenAI Responses output and terminal events.
- [x] 1.4 Route Antigravity accounts on `/v1/responses` without changing existing OpenAI/Grok routing.
- [x] 1.5 Add tests for ordinary turn, `previous_response_id`, function_call_output, stream and non-stream responses.
- [x] 1.6 Run targeted Go tests, frontend unaffected checks, OpenSpec validation, and production candidate build.

Validation: apicompat, routes and archtest package tests passed; targeted service/handler tests and Responses adapter race tests passed. This feature changes no frontend files. Linux amd64 candidate built at `/tmp/sub2api-antigravity-responses-linux-amd64`. Strict OpenSpec validation and diff whitespace checks passed. Existing unrelated workspace changes were preserved.

Deployment: backend 0.1.355 is live on 74.82.195.84:18286, with 0.1.354 retained at port 18285 for rollback. Public version, health, binary checksum and unauthenticated Responses 401 verified. No live authenticated Codex/Gemini round-trip has been performed. Continuation is bounded process-local storage; see design.md and docs/ANTIGRAVITY_RESPONSES.md for supported features and limits.
