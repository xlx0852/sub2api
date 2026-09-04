## Context

The official `openai/codex` main branch defines Astra with a 272,000 token base context, an 872,000 token maximum context, `use_responses_lite`, code-mode-only tools, multi-agent v2, `ultra` reasoning, verbosity, and explicit replay/compaction metadata. Sub2API currently stores only a small public model shape and relies on an OAuth account manifest for rich Codex discovery.

## Goals / Non-Goals

- Goals: preserve official Astra metadata, keep discovery available during OAuth outages, avoid semantic loss in reasoning and Responses Lite routing, and retain safe WS recovery.
- Non-Goals: remove upstream WebSocket limits, invent unsupported provider capabilities, or change non-Astra model behavior.

## Decisions

- Keep public API pricing and OpenAI model listing compatible; add optional capability fields rather than changing existing required fields.
- Treat the official Astra row as the source of truth for Codex metadata. Use 272k as the default Codex window and 872k as the maximum provider ceiling; keep the API pricing resource's 1.05M API context separate.
- Use a local fallback only when live OAuth discovery is unavailable or stale. Live account entitlements remain authoritative for whether an account may route Astra.
- Preserve the existing 55-minute proactive upstream WS rotation and add tests around the official Astra replay contract.

## Risks / Trade-offs

- A richer fallback row can expose Astra in the picker before an account is entitled. Mitigation: keep account-gated availability separate from catalog metadata and fail closed during selection.
- A GPT-6 prompt may diverge as the official template evolves. Mitigation: keep it versioned and test model-specific selection.
- `ultra` may not be accepted by older upstream compact endpoints. Mitigation: constrain downgrade behavior to the exact endpoint/model compatibility boundary.

## Migration Plan

1. Add schema and embedded catalog metadata.
2. Add fallback manifest and model-specific prompt.
3. Update routing and reasoning guards.
4. Run targeted and full relevant tests.
5. Build and deploy through the existing artifact-only blue-green process after validation.

## Open Questions

- Whether all configured OpenAI OAuth accounts are entitled to Astra must remain an account discovery result, not a static catalog assumption.
