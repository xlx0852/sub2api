# Lifecycle migration baseline

The current forwarding entry points were checked on the `refactor-architecture-design` branch.

| Entry | Current lifecycle | Regression evidence |
| --- | --- | --- |
| OpenAI Responses HTTP/SSE | parse, admission, select, rewrite, transport, relay, settle | `go test ./internal/handler`, `go test ./internal/service` |
| OpenAI Chat/Messages compatibility | parse, admission, select, rewrite, transport, relay, settle | service pipeline tests |
| Gemini Messages/Native/Chat | parse, admission, select, rewrite, transport, relay, settle | Gemini pipeline tests |
| Claude forward | parse, admission, select, rewrite, transport, relay, settle | Claude pipeline stage test |
| Antigravity Gemini | parse, admission, select, rewrite, transport, relay, settle | Antigravity pipeline stage test |
| OpenAI WebSocket | connection/session lifecycle outside HTTP Exchange | WS fallback and ingress tests |
| OpenAI Images | dedicated forward function with failover and streaming branches | `openai_images_test.go` |

The WebSocket and Images entries remain intentionally outside the shared HTTP pipeline until response, usage, lease, and settlement differential tests are available.
