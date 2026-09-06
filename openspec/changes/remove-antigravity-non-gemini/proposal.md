# Change: Remove non-Gemini Antigravity paths

## Why
The deployment only needs text Gemini models through Antigravity. Claude and image paths add protocol, testing, and maintenance complexity.

## What Changes
- **BREAKING** Restrict Antigravity to text Gemini models, Gemini tools, thinking, JSON, and SSE.
- Remove Claude-specific Antigravity request/response pipelines and image-model handling.
- Preserve OAuth, token refresh, project onboarding, quota/credits, retry, account scheduling, schema cleaning, and Gemini streaming.
- Query `retrieveUserQuotaSummary`, preserve its group/bucket/window structure and CPA `remainingFraction` semantics, and expose only supported text Gemini quota groups.
- Keep the public Antigravity gateway entry point and reject unsupported model families explicitly.

## Impact
- Affected capability: antigravity
- Affected code: backend/internal/pkg/antigravity, backend/internal/service/antigravity_* and related model mappings/tests.
