## ADDED Requirements
### Requirement: Antigravity model scope
The Antigravity gateway SHALL accept text Gemini model requests only. The supported catalog SHALL include modern Gemini models: `gemini-3.8-flash`, `gemini-3.8-flash-high`, `gemini-3.7-flash`, `gemini-3.7-flash-high`, `gemini-3.6-flash`, `gemini-3.6-flash-high`, `gemini-3.1-pro`, `gemini-3.1-pro-high`, `gemini-3.1-pro-low`, `gemini-3.1-flash-lite`, and `gemini-3-flash`. It SHALL preserve Gemini text generation, function calling, thinking configuration, OAuth authentication, project onboarding, quotas, retries, and streaming responses. Requests targeting Claude or image models SHALL be rejected with an explicit unsupported-model error.

#### Scenario: Modern Gemini 3.8 / 3.7 / 3.6 / 3.1 text request
- **WHEN** a request targets `gemini-3.8-flash`, `gemini-3.7-flash`, `gemini-3.6-flash`, or `gemini-3.1-pro` (or effort-based variants)
- **THEN** the gateway validates and forwards it through the Antigravity v1internal Gemini path and returns the requested JSON or SSE response

#### Scenario: Unsupported Claude or image request
- **WHEN** a request targets a Claude or image model
- **THEN** the gateway rejects it before upstream execution with an explicit unsupported-model error
