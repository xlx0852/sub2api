## ADDED Requirements
### Requirement: Antigravity model scope
The Antigravity gateway SHALL accept text Gemini model requests only. It SHALL preserve Gemini text generation, function calling, thinking configuration, OAuth authentication, project onboarding, quotas, retries, and streaming responses. Requests targeting Claude or image models SHALL be rejected with an explicit unsupported-model error.

#### Scenario: Gemini text request
- **WHEN** a request targets a supported `gemini-*` text model
- **THEN** the gateway forwards it through the Antigravity v1internal Gemini path and returns the requested JSON or SSE response

#### Scenario: Unsupported Claude or image request
- **WHEN** a request targets a Claude or image model
- **THEN** the gateway rejects it before upstream execution with an explicit unsupported-model error
