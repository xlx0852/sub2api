## 1. Preaggregation Simplification
- [x] 1.1 Stop and retire `ops_preaggregation_daily` background job and its leader lock in `OpsAggregationService`
- [x] 1.2 Retain `ops_preaggregation_hourly` and verify all dashboard queries continue to aggregate correctly

## 2. Error Log Payload Bounding
- [x] 2.1 Add string truncation helper to limit oversized error bodies and details in `OpsErrorLoggerMiddleware`
- [x] 2.2 Verify error logs record concise, actionable diagnostic summaries without bloating table storage

## 3. Alert Evaluation Simplification
- [x] 3.1 Verify and streamline alert rule evaluation in `OpsAlertEvaluatorService`
- [x] 3.2 Ensure health score and P0/P1 alerts function properly under threshold triggers

## 4. Verification and Regression Testing
- [x] 4.1 Run unit and integration tests for Ops service and dashboard
- [x] 4.2 Validate OpenSpec changes with `openspec validate --strict`
