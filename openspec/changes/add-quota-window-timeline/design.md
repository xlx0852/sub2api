## Context

The reference design (`Cli-Proxy-API-Management-Center`, `features/quota`) separates three concerns
cleanly: `quotaTimelineModel.ts` (pure projection), `QuotaTimeline.tsx` (layout only), and the provider
adapters that read each upstream payload structurally. We adopt the same split so the maths here is
testable without mounting anything and the component stays layout.

## Decisions

### Windows are projected by whole periods from the reported reset instant

A usage payload reports the *end* of the current window (`resetsAt`) plus a utilization for it. Every
other boundary on the chart is that anchor shifted by whole periods, backwards and forwards, which is
the only defensible projection: the upstream does not tell us when the next window opens, but it does
tell us the period length, and the boundaries are periodic by contract.

Consequences: a window is only ever drawn for an account whose payload carries a usable reset instant,
and the *live* bar (the one ending exactly at the anchor) is the only bar that may carry a utilization
— projected future windows have no usage yet and are drawn empty rather than inheriting a stale
percentage.

### The period comes from the window key, not from the label

Labels are localized and abbreviated (`7d`, `7d S`, `1d`, `Gemini 3 Pro`), so they are not a stable
input. The presentation layer already emits stable keys, and this change extends that mapping:

| window key                  | period |
| --------------------------- | ------ |
| `five-hour`                 | 5h     |
| `seven-day`, `seven-day-*`  | 168h   |
| `grok-weekly`               | 168h   |
| `gemini-shared`, `gemini-pro`, `gemini-flash` | 24h |
| `antigravity-<label>`       | parsed from the `5h` / `1d` / `7d` label, and buckets without a recognizable label fall back to the account's other windows |

An unknown key produces no lane bar (empty lane) rather than a guessed period.

### Two spans, and the lane picks the window that fits

`weekly` spans a fortnight from the containing Sunday (one cell per day, stepping a week at a time);
`session` spans three days from today (one cell per six hours, stepping a day). A credential usually has
both a 5-hour and a weekly window; drawing the soonest reset in a fortnight view turns every lane into a
row of slivers. The lane therefore picks the longest period that still fits the span, tie-broken by the
soonest reset, and the 5-hour zoom additionally requires a real 5-hour window — a longer window must not
be reinterpreted as a 5-hour one.

Span arithmetic uses date arithmetic (`setDate`) rather than adding fixed millisecond counts, so a DST
transition inside the span cannot shift every later day by an hour.

### The clock is injected

Bars are classified past / live / next against `now`, and the now marker is positioned by it, so the
component takes `now` as a prop that defaults to a shared ticking clock. Tests and screenshots pass a
fixed instant.

### Lanes are never dropped

An account with no usable reset instant renders as an explicitly empty row ("no window counting down")
and stays in the chart. A missing row reads as an oversight; an empty one reads as "nothing scheduled",
which is the truth. The whole panel is hidden only while *no* lane has anything to draw.

### Data agreement with the row cells

The timeline must never disagree with the bars in the table next to it, so it reads through the same
per-account usage loader and TTL cache the row cells use (extracted from `AccountUsageCell.vue` into
`useAccountUsageInfo`), instead of issuing its own parallel requests. Reset-credit ticks reuse the same
OpenAI quota payload the reset-credit cell already fetches; accounts whose payload has none simply have
no ticks.

## Alternatives considered

- **Extend the existing per-row bars with a reset timeline inline.** Rejected: the value is comparing
  lanes against each other, which needs one shared axis and a fixed label column.
- **A per-account timeline inside the usage drawer.** Useful, but it cannot answer the pool question —
  the drawer shows one account. Kept as a later extension; this change targets the list.
- **Deriving periods from `resetAt - startAt` when a payload exposes a window start.** We do not
  reliably receive a start instant for every platform; the key mapping is uniform and testable.
