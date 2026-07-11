## Context

The gateway treats Grok as OpenAI-compatible and can resolve an API Key account to `https://api.x.ai/v1`. Its shared credential resolver also reads `credentials.api_key`. The management UI, model discovery, connection-test path, and Responses forwarding gate must all accept Grok API Key accounts for the capability to work end to end.

## Goals / Non-Goals

- Goals:
  - Make official xAI API Key accounts operable end to end
  - Use the same administrator workflow as OpenAI API Key accounts
  - Preserve existing Grok OAuth behavior
  - Support official and explicitly configured xAI-compatible base URLs
- Non-Goals:
  - Emulate Grok OAuth subscription quotas for API Key accounts
  - Change Grok pricing or customer billing
  - Send CLI chat-proxy headers to official API Key upstreams

## Decisions

- Decision: store Grok API Key credentials as `api_key`, `base_url`, and `using_api=true`
  - This matches the existing gateway credential resolver and makes official routing explicit.
- Decision: use Bearer authentication for `/v1/models` and `/v1/responses`
  - This matches the official xAI API contract and the existing Grok forwarding path.
- Decision: use the configured base URL for model discovery and text/media requests
  - The URL is validated by the existing upstream URL validator before model discovery.
- Decision: keep OAuth-only quota probing and subscription display separate
  - API Key accounts use local request/token/cost statistics because OAuth billing snapshots do not apply.

## Risks / Trade-offs

- A compatible custom upstream may not implement `/v1/models`
  - The administrator can still enter a model whitelist manually; sync failure does not invalidate saved credentials.
- API Key and OAuth account behavior differ for quota and token refresh
  - Account type remains explicit, and API Key accounts never enter the OAuth refresh path.

## Migration Plan

1. Add Grok API Key selection and credential fields to account management
2. Add Grok API Key model discovery and connection testing
3. Add focused backend and frontend regression tests
4. Verify existing Grok OAuth tests and frontend build
