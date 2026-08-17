# OpenAI Gateway Operations

This runbook covers OpenAI upstream outcome classes, sticky-session re-evaluation,
legacy sticky-hash retirement, and stream timeout diagnosis. It documents the
current runtime behavior; it does not change defaults.

## Upstream outcome classes

OpenAI-compatible upstream attempts are classified before health and sticky
state is updated:

| Class | Typical result | Operational meaning |
|---|---|---|
| `success` | completed 2xx | Clears consecutive transient failures but preserves an unexpired cooldown. |
| `caller` | request 4xx other than 401/403/429 | Does not degrade account health. Check client payload and protocol compatibility. |
| `credential` | 401/403 or explicit auth failure | Clears affected sticky state and marks the credential for reauthentication/unschedulable handling. |
| `quota` | 429 or explicit quota exhaustion | Applies cooldown using `Retry-After`, reset hints, then the configured/default fallback. |
| `transient` | 5xx, connect/timeout, upstream stall | Updates transient failure health; inspect upstream availability and timeout stage. |

Structured `openai_upstream_outcome` logs include the account ID, HTTP status,
outcome class, and credential class. They do not include credentials.

## Sticky re-evaluation

The scheduler keeps the routing order `previous_response_id` -> `session_hash`
-> load balance. A valid sticky account is re-evaluated at most once per 60
seconds. If its visible usage score is at least 80 and a cooler eligible account
of the same credential class exists, the binding is cleared and selection falls
through. A switch emits `sticky_reeval_switch`.

Credential generation/class mismatch, an invalid sticky epoch, an unschedulable
account, or an incompatible transport also invalidates the binding and falls
through instead of failing the request.

## Legacy session-hash compatibility snapshot

Administrators can query one API instance through:

```text
GET /api/v1/admin/ops/runtime/openai-compatibility-fallbacks
```

The response is read-only and reports:

- `scope=process`
- effective legacy read-fallback and dual-write switches
- the 600-second maximum TTL applied to legacy keys
- monotonic legacy read-attempt, read-hit, and dual-write counters plus hit rate
- the existing request-metadata compatibility counters

The endpoint requires admin authentication and Ops monitoring to be enabled.
It does not reset counters. Query every API instance directly; a load-balanced
request to only one instance is not fleet-wide evidence.

Use counter differences over a defined observation window. A cumulative zero
immediately after restart is not sufficient evidence, and the hit-rate field is
not meaningful when there were no read attempts.

### Retirement sequence

1. Record a per-instance baseline snapshot and instance start/restart time.
2. Observe normal and peak windows while both compatibility switches remain on.
3. Set `gateway.openai_ws.session_hash_dual_write_old=false`; keep old-read
   fallback enabled and retain the paired rollback artifact.
4. Wait at least 600 seconds plus the chosen session safety window. Confirm each
   instance's legacy-read-hit delta remains zero.
5. Set `gateway.openai_ws.session_hash_read_old_fallback=false`; observe normal
   and peak traffic again. Roll back the config if affinity/failover regressions
   appear.
6. Remove compatibility code/config only in a separately reviewed cleanup after
   the zero-hit evidence and observation record are committed.

Do not disable old reads before dual writes have stopped and legacy keys have
expired. Do not compare counters across a process restart without starting a new
baseline.

## Stream timeout responsibilities

The relevant gateway settings are:

- `gateway.openai_first_output_timeout_seconds`: request start through first
  semantic upstream output; `0` disables it.
- `gateway.openai_high_effort_first_output_timeout_seconds`: optional high-effort
  override; `0` falls back to the normal first-output value.
- `gateway.openai_stream_stall_timeout_seconds`: silence between real upstream
  progress events; `0` disables it.

Client keepalive frames do not count as upstream progress. When stream-stall
enforcement fires after a Responses stream has started, the gateway emits
`response.incomplete` with reason `upstream_stall_timeout`, cancels upstream,
and follows the existing partial/incomplete usage path. It must not emit a fake
`response.completed` terminal.

For an apparent timeout, distinguish:

1. response-header wait,
2. first semantic output wait,
3. mid-stream upstream stall,
4. client disconnect while the gateway drains upstream for billing.

The stage and terminal reason determine whether to inspect upstream latency,
proxy idle settings, client cancellation, or settlement recovery.
