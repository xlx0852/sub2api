# Change: Add a quota windows timeline to the account list

## Why

The accounts table already answers "how much quota is left" per account: each row shows up to two window
bars with utilization and a reset countdown (`account-usage-presentation`). It cannot answer the
question operators actually ask when a pool runs dry: **when does capacity come back, and does it come
back all at once?** Ten Codex accounts whose 5-hour windows all reset within the same twenty minutes is
a different situation from ten windows staggered across the evening, and no per-row percentage shows
that.

The reference implementation operators compared against (`router-for-me/Cli-Proxy-API-Management-Center`)
draws exactly that: one lane per credential, one bar per window occurrence on a shared time axis, the
current moment marked, and zoom between a two-week weekly view and a three-day 5-hour view. Its model
layers are notably disciplined — all projection maths lives in a pure module with the clock injected,
windows are projected by whole periods from the reported reset instant, and a credential with no usable
reset instant renders an *empty* lane instead of being dropped.

We already hold the data it needs. Every account usage presentation carries windows keyed
`five-hour` / `seven-day` / `grok-weekly` / `gemini-*` / `antigravity-*` with `utilization` and
`resetsAt` (`buildAccountUsagePresentation`). Nothing draws them on a timeline.

## What Changes

- Add a pure projection model `frontend/src/utils/quotaWindowTimeline.ts`: window-period resolution from
  the window key, whole-period window projection from a reset anchor, span construction for the two zoom
  modes, lane selection (longest period that fits the span), and reset-credit projection. No Vue, no
  clock of its own — `now` is always passed in, so every case is unit-testable.
- Add a `QuotaWindowsTimeline.vue` component under `components/account/`: a lane per account with a
  fixed label column (provider dot, name, period badge, per-window remaining chips) and a time axis that
  is one cell per day (weekly) or per six hours (5-hour zoom), with today and weekend shading, a
  vertical now marker, and bars drawn as past / live / next. Live bars fill by consumed share and carry
  a `remaining% · end time` label; next bars are dashed outlines; past bars recede. Manual reset credits
  appear as amber expiry ticks when the account payload exposes them.
- Zoom (`Weekly` / `5-hour`) and period navigation (`‹ Today ›`) controls, a legend, and an explanatory
  note; an empty state instead of a dropped lane when an account reports no usable reset instant.
- Extract the shared per-account usage info loading (API call + TTL cache) out of `AccountUsageCell.vue`
  into a composable so the timeline and the row cells read the same data and cannot disagree.
- Render the timeline as a collapsible section above the account table, fed by the accounts currently on
  screen, and keep every existing row/card affordance unchanged.

## Impact

- Affected specs: `account-usage-presentation` (modified: one added requirement).
- Affected code: new `frontend/src/utils/quotaWindowTimeline.ts`,
  `frontend/src/components/account/QuotaWindowsTimeline.vue` and tests; a targeted extraction in
  `frontend/src/components/account/AccountUsageCell.vue`; `frontend/src/features/admin/views/AccountsView.vue`;
  zh/en locale files.
- No backend change: the projection reads data the account usage payload already returns, and a window
  without a reset instant is shown as an empty lane rather than as invented capacity.
- Presentation only: no billing, scheduling, or quota-enforcement behaviour changes.
