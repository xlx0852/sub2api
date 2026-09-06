## 1. Implementation
- [x] 1.1 Inventory and isolate Claude/image references in the Antigravity backend.
- [x] 1.2 Remove image-specific state and billing emission from the active Gemini Antigravity path while preserving shared OAuth/client/retry/quota code.
- [x] 1.2a Remove the legacy Antigravity Claude Messages route and its dedicated request, response, streaming, and upstream compatibility code.
- [x] 1.3 Add explicit Gemini-only model validation at the gateway boundary.
- [x] 1.4 Restrict the backend catalog and frontend Antigravity model choices to text Gemini models.
- [x] 1.5 Run targeted Antigravity Go tests and verify unsupported models are rejected before upstream execution.
- [x] 1.6 Align quota utilization with CPA `remainingFraction` semantics and omit unsupported Claude/image quota entries.
