# Change: Replace Native Cursor and Devin Transports with External Bridges

## Why

The native Cursor and Devin transports duplicate large upstream protocol
implementations inside Sub2API, while Devin quota lookup currently fails for
the account type used by the imported session token. The standalone projects
already provide HTTP-compatible bridge surfaces and provider-specific account
usage endpoints. Moving those transports out of Sub2API reduces gateway
coupling and makes upstream protocol updates independently deployable.

## What Changes

- **BREAKING** Remove the native Cursor Connect/Sand and Devin Connect transport
  paths from Sub2API after the sidecars pass traffic and rollback checks.
- Deploy the local `grok_bot_2api_temp` Cursor/Sand bridge and
  `leookun/devin-2api` as private, supervised services on the production host.
- Route migrated accounts through their OpenAI-compatible HTTP endpoints using
  Sub2API's generic upstream forwarding and authentication boundary.
- Add sidecar health/model/usage adapters so Cursor `/api/account` and Devin
  status/credit data can be queried without embedding provider protocols.
- Migrate the existing Cursor and Devin account records without exposing their
  provider tokens to client-facing responses.

## Impact

- Affected capabilities: Cursor provider, Devin provider, account usage and
  model discovery, separated deployment.
- Affected code: native provider packages, gateway platform branches, account
  migration/admin UI, sidecar deployment manifests and usage adapters.
- Rollback: retain the current `0.1.382` native release and account snapshots
  until the external bridge observation period completes.
