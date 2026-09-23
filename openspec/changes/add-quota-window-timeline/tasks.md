## 1. Projection model

- [ ] 1.1 Add `frontend/src/utils/quotaWindowTimeline.ts` with `windowPeriodHours(windowKey)`, `windowsIn`,
  `timelineSpan`, `pickLaneWindow`, `projectLane`, `projectResetCredits`, `laneHasWindow` and
  `buildTimelineLane` over our `AccountUsagePresentation` windows (utilization is percent used).
- [ ] 1.2 Keep the module pure: no Vue imports, no clock of its own, `now` passed in on every call.

## 2. Shared usage loader

- [ ] 2.1 Extract the per-account usage fetch plus TTL cache out of `AccountUsageCell.vue` into a
  composable (`useAccountUsageInfo`) and have the cell consume it, so the cell and the timeline read
  identical data.
- [ ] 2.2 Expose a batch read for a set of accounts with the same TTL and in-flight de-duplication, and
  keep the existing manual-refresh and cache-invalidation behaviour intact.

## 3. Timeline component

- [ ] 3.1 Add `frontend/src/components/account/QuotaWindowsTimeline.vue`: header (title, range,
  zoom/navigation controls), axis row, one lane per account, legend and note.
- [ ] 3.2 Draw lanes as a fixed label column (provider dot, name, period badge, per-window remaining
  chips) plus a track with the day/hour grid, the now marker, past/live/next bars and reset-credit ticks.
- [ ] 3.3 Hide the panel only when no lane can be drawn; render an explicit empty lane for accounts with
  no usable reset instant, and an empty state for the 5-hour zoom when no account reports a 5-hour window.
- [ ] 3.4 Horizontal scroll with a minimum chart width on narrow viewports; keep the label column aligned
  across rows.

## 4. Integration

- [ ] 4.1 Render the timeline as a collapsible section above the account table in `AccountsView.vue`,
  fed by the accounts currently on screen.
- [ ] 4.2 Keep every existing row summary, drawer and column affordance unchanged.

## 5. Internationalization

- [ ] 5.1 Add the timeline title, zoom, navigation, legend, empty-lane and note strings to the zh and en
  locale files; persist the open/closed state and the selected zoom locally per administrator.

## 6. Verification

- [ ] 6.1 Unit-test the model: whole-period projection in both directions, clipping at the span edges,
  live/next/past classification, anchor-only utilization, unusable period guard, span arithmetic across a
  DST boundary, lane selection preferring the longest fitting period, and empty-lane behaviour.
- [ ] 6.2 Component tests: a lane per account, the empty lane for a reset-less account, zoom switching
  (including an account without a 5-hour window being excluded from that zoom), navigation labels, the
  credit tick, and no render at all when no lane has data.
- [ ] 6.3 `vue-tsc --noEmit` clean and the usage/account component suites pass.
