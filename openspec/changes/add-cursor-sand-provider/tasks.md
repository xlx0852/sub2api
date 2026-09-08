## 1. Platform and Protocol Fundamentals

- [x] 1.1 Register `PlatformCursor = "cursor"` across backend domain constants and platform lists
- [x] 1.2 Implement `internal/pkg/cursor/checksum.go` for `x-cursor-checksum` generation with unit tests
- [x] 1.3 Implement Go Protobuf message definitions and Connect-RPC transport client for `InferenceService` and `DashboardService`

## 2. Authentication and Token Lifecycle

- [x] 2.1 Implement `CursorOAuthService` for PKCE generation, state polling, and one-time ticket consumption
- [x] 2.2 Implement `CursorTokenRefresher` with distributed locking, proxy support, and credential rotation
- [x] 2.3 Implement `CursorQuotaFetcher` to sync profile, Fast Requests, and usage metrics

## 3. Gateway Pipeline Integration

- [x] 3.1 Implement message/tool converter from OpenAI schema to Cursor `InferenceStreamRequest`
- [x] 3.2 Implement streaming bridge converting `textPart`, `thinkingPart`, and `toolCallPart` to SSE chunks
- [x] 3.3 Integrate with gateway pipeline, proxy transport, and error mapping

## 4. Admin API and UI

- [x] 4.1 Implement admin endpoints for starting PKCE authorization, polling status, and saving accounts
- [x] 4.2 Add Cursor platform styling, icons, filters, and authorization modal to frontend

## 5. Validation

- [x] 5.1 Add unit tests for checksum generation, Connect framing, and message translation
- [x] 5.2 Validate end-to-end streaming and non-streaming calls with account credentials
- [x] 5.3 Run full backend test suite, linting, and OpenSpec strict validation
