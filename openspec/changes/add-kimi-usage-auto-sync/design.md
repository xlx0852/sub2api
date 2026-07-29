## Context

The OpenAI usage path already keeps upstream probes in the backend and exposes
cached `UsageInfo` to the frontend. Kimi's `/coding/v1/usages` response contains
one weekly detail and a list of rolling windows, including a 300-minute window.

## Decisions

- Keep quota acquisition in a dedicated `KimiQuotaService`; the service owns
  upstream request construction, token recovery, proxy routing, and response
  parsing.
- Reuse `UsageProgress` so existing admin API and drawer components need no new
  wire format. Map `usage` to `seven_day` and the 300-minute `limits` entry to
  `five_hour`.
- Store the result in an in-memory `UsageCache` with a 10-minute success TTL,
  one-minute negative TTL, and singleflight per account. The cache is process
  local just like the existing OpenAI probe throttle.
- Do not persist Kimi quota snapshots in account credentials or `Extra`; quota
  responses are refreshed and served through the existing usage endpoint.

## Error handling

401/403 responses are reported as reauthentication/forbidden states, 429 as a
rate-limit state, and other upstream or parsing failures as a degraded usage
response. Tokens and response bodies are never included in error messages.
