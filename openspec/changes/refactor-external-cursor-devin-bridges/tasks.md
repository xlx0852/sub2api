## 1. External bridge packaging and deployment

- [x] 1.1 Pin and package the Cursor bridge from `grok_bot_2api_temp` without
  including local token files or `.git` metadata.
- [x] 1.2 Build and package `leookun/devin-2api` with generated protobuf
  bindings and runtime configuration supplied outside Git.
- [x] 1.3 Deploy both services privately with restart supervision, health
  checks, separate API keys where supported, and rollback markers.

## 2. Sub2API external HTTP integration

- [x] 2.1 Add generic sidecar account metadata and safe endpoint validation.
- [x] 2.2 Route Cursor and Devin migrated accounts through the sidecar HTTP
  APIs, preserving streaming, tools, model mapping, billing, and failover.
- [x] 2.3 Implement provider-agnostic health/model adapters plus Cursor and
  Devin usage projections from their documented HTTP surfaces.
- [x] 2.4 Add migration tooling for the existing Cursor and Devin records and
  verify scheduler/cache state after migration.

## 3. Remove native provider paths

- [ ] 3.1 Remove native Cursor Connect/Sand forwarding, OAuth refresh, and
  quota transport code after external routing is enabled.
- [ ] 3.2 Remove native Devin protobuf/Connect forwarding and replace its admin
  form with the external bridge account configuration.
- [ ] 3.3 Keep rollback-compatible data handling and document the migration.

## 4. Validation and rollout

- [x] 4.1 Add unit and integration tests for sidecar HTTP forwarding, SSE,
  model listing, quota mapping, auth failures, and failover.
- [x] 4.2 Run full backend/frontend validation and sidecar health/model probes.
- [x] 4.3 Execute staged blue-green deployment, traffic smoke tests, quota
  checks, and rollback verification before retiring `0.1.382`.
