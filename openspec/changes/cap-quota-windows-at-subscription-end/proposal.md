# Change: Stop quota-window projection at subscription end

## Why

The profit quota timeline currently treats a rolling weekly snapshot as indefinitely recurring. For subscription accounts this projects quota windows after the paid cycle expires or after a confirmed upstream ban, even though no further quota is available.

## What Changes

- Cap subscription-account quota-window projections at the controlling subscription cycle end.
- Use the confirmed ban effective timestamp as the earlier cutoff when a cycle is terminated.
- Prevent the frontend timeline from rendering projected occurrences after that cutoff.
- Keep non-subscription account windows unchanged.

## Impact

- Affected code: profit quota-window aggregation and timeline rendering.
- Affected capability: account profit quota-window presentation.
