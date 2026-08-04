## 1. Backend

- [x] 1.1 Add Kimi quota response parsing and upstream query service.
- [x] 1.2 Add 10-minute cache/singleflight integration to `AccountUsageService`.
- [x] 1.3 Wire the service and refresh dependencies.
- [x] 1.4 Add parser, request, retry, and cache tests.
- [x] 1.5 Attach one-minute-cached local window statistics to Kimi quota data.
- [x] 1.6 Persist normalized Kimi quota windows and synchronize temporary scheduling blocks with official reset times.
- [x] 1.7 Refresh official Kimi quota after a gateway 429 so recovery does not depend on opening the admin page.

## 2. Admin UI

- [x] 2.1 Render Kimi OAuth 5-hour and weekly usage windows in the shared usage cell.
- [x] 2.2 Add UI regression coverage for Kimi quota data and error states.
- [x] 2.3 Render Kimi request counts and full-utilization estimates through the shared progress component.
- [x] 2.4 Cover full-window pause, recovered-window clearing, and 429-triggered refresh behavior.

## 3. Verification

- [x] 3.1 Validate the OpenSpec change and run targeted Go/Vue tests.
