## 1. Ledger state machine

- [x] 1.1 Persist last-observed metadata required to distinguish an active window from an overdue window waiting for activation.
- [x] 1.2 Close overdue windows without opening a replacement and detect forward reset-at jumps even when theoretical windows overlap.
- [x] 1.3 Open a new row only after a real active-window observation, preserving reset-card exact events.
- [x] 1.4 Keep quota sweeps read-only and prevent inference probes from silently activating waiting windows.

## 2. Profit projection

- [x] 2.1 Return ledger source/close state and derived waiting-activation gaps from the Profit overview.
- [x] 2.2 Intersect real provider windows and gaps with recorded subscription-cycle spans.
- [x] 2.3 Merge real ledger rows with fallback projections without overwriting real gaps.

## 3. Frontend

- [x] 3.1 Render waiting-activation gaps separately from active, ended, upcoming, and uncertain windows.
- [x] 3.2 Keep drawer window-economics queries limited to real/effective window ranges.

## 4. Verification

- [x] 4.1 Cover early overlapping reset, scheduled expiry without activation, first-use activation, reset-card, and subscription-end clipping in backend tests.
- [x] 4.2 Cover mixed real/projected rows and waiting-activation rendering in frontend tests.
- [x] 4.3 Run targeted backend unit tests, frontend component tests, type-check, OpenSpec strict validation, and diff checks.
