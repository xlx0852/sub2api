## Context

Quota snapshots contain the current upstream reset but do not persist historical occurrences. The timeline projects occurrences from the current snapshot. Subscription cycles and confirmed ban settlements are the authoritative availability boundary for subscription accounts.

## Decisions

- Add an optional `recurring_until_at` boundary to each returned quota window.
- Derive the boundary from the controlling subscription cycle, using its end or active termination effective time, whichever comes first.
- Apply the boundary in both backend snapshot capping and frontend occurrence expansion so neither the current bar nor projected bars can cross the boundary.
- Leave metered/API-key accounts without a boundary.
