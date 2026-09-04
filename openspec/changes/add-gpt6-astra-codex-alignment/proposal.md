# Change: Align GPT-6 Astra with the official Codex contract

## Why

Sub2API can forward and bill `gpt-6-astra`, but its Codex-facing catalog and runtime metadata are less expressive than the official Codex model contract. This causes capability loss during model discovery, reasoning selection, Responses Lite routing, and long-lived Codex sessions.

## What Changes

- Extend the model catalog to preserve Astra's Codex capability metadata and limits.
- Provide a validated local Astra fallback manifest when OAuth model discovery is unavailable.
- Add a GPT-6 Astra Codex instruction template and preserve the `ultra` reasoning level where supported.
- Make Responses Lite, tool mode, multi-agent v2, verbosity, and replay behavior explicit in Astra routing.
- Add regression coverage for normal turns, `previous_response_id`, `function_call_output`, compact, WebSocket rotation, and fallback discovery.

## Impact

- Affected capabilities: OpenAI model catalog, Codex Responses relay, Responses WebSocket ingress, billing metadata.
- Affected code: `backend/internal/pkg/modelcatalog`, `backend/internal/pkg/openai`, `backend/internal/service/openai_codex*`, `backend/internal/service/openai_ws*`, handler model discovery, and related tests.
- No database migration is expected.
