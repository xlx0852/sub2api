# Change: Add OpenAI Responses access to Antigravity Gemini

## Why
Codex clients use the OpenAI Responses API, while the current Antigravity path accepts Gemini v1beta requests only. Users need to route Codex requests through `/v1/responses` to text Gemini Antigravity accounts.

## What Changes
- Add an explicit Antigravity Responses adapter that converts supported OpenAI Responses input into Gemini text requests.
- Convert Gemini JSON/SSE output back into OpenAI Responses JSON/SSE events.
- Preserve Codex turn semantics for ordinary turns, `previous_response_id`, and `function_call_output`.
- Keep the existing OpenAI Responses behavior for OpenAI, Grok, and other providers unchanged.
- Keep Antigravity text Gemini only; image and Claude upstream models remain unsupported.

## Impact
- Affected capability: Antigravity and OpenAI Responses routing
- Affected code: backend/internal/handler, backend/internal/service, backend/internal/pkg/antigravity, routing and protocol tests
- Breaking change: none for existing provider routes; new Antigravity Responses behavior is additive
