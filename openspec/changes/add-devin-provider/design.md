## Context

Devin uses a server-streaming Connect RPC at
`/exa.api_server_pb.ApiServerService/GetChatMessage` (the flattened service
path exposed by the upstream adapter). The request is protobuf, not JSON, and
contains the full conversation. The current gateway already owns protocol
decoding, account selection, response encoding, and billing, so the Devin
adapter must stop at a provider-neutral chat delta boundary.

## Decisions

- Keep the transport in `internal/pkg/devin` and use the existing Connect
  envelope framing pattern from the Cursor package.
- Generate bindings only for the Devin messages required by
  `GetChatMessage`; do not vendor the upstream service, dashboard, or its
  generated 11 MB all-descriptors package.
- Store Devin credentials in the existing account maps. Accept `token`,
  `access_token`, and `api_key` aliases, with optional `base_url` and `model`.
- Render Devin deltas through the existing Chat Completions to Responses and
  Responses to Anthropic compatibility helpers so all client-facing formats
  retain current gateway semantics.

## Risks / Trade-offs

- Devin may rotate client metadata or protobuf fields. The transport keeps the
  metadata version in one place and tests the wire-level fields we own.
- Devin session tokens do not expose a refresh flow in the referenced project;
  expired tokens fail normally and can be replaced from the account form.

## Migration Plan

Create a `devin` account with a Devin session token, optional base URL/model,
and attach it to a `devin` group. Existing accounts and groups are unchanged.
