## ADDED Requirements
### Requirement: Strict client-supported Antigravity catalog
The Antigravity catalog and default mapping SHALL exclusively contain models currently supported by the Antigravity client: Gemini 3.8 Flash, Gemini 3.7 Flash, Gemini 3.6 Flash, and Gemini 3.1 Pro (and their reasoning effort variants). Deprecated models including `gemini-3-flash`, `gemini-3.1-flash-lite`, `gemini-2.5-*`, and `gemini-3-pro-*` SHALL NOT be published in the active Antigravity catalog or default presets.

#### Scenario: Client model catalog lookup
- **WHEN** a client queries the Antigravity default models
- **THEN** only Gemini 3.8 Flash, Gemini 3.7 Flash, Gemini 3.6 Flash, and Gemini 3.1 Pro (with effort variants) are returned, and deprecated models are absent

#### Scenario: Deprecated model rejection from catalog
- **WHEN** inspect the Antigravity model list
- **THEN** `gemini-3-flash`, `gemini-3.1-flash-lite`, and all `gemini-2.5-*` models are absent
