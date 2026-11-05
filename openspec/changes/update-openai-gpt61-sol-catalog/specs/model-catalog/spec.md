## ADDED Requirements

### Requirement: GPT-6.1 Sol catalog availability

The system SHALL expose `gpt-6.1-sol` as an OpenAI chat model in the first-party model catalog
without changing any existing entry, and SHALL publish it as a selectable preset.

#### Scenario: Model appears in the catalog

- **WHEN** the first-party model catalog is loaded
- **THEN** the OpenAI platform model list contains `gpt-6.1-sol` with display name "GPT-6.1 Sol", `object` `model`, `owned_by` `openai` and `use_responses_lite` `true`

#### Scenario: Existing GPT-6 entries are untouched

- **WHEN** the catalog is loaded after this change
- **THEN** `gpt-6-sol`, `gpt-6-astra` and `gpt-6-luna` keep their previous entries, and `gpt-6-sol` still resolves to a 0.2 USD per million token cache read rate

#### Scenario: Model is selectable in the client

- **WHEN** the frontend builds the OpenAI model presets from the catalog
- **THEN** `gpt-6.1-sol` is available as a preset labelled "GPT-6.1 Sol"

### Requirement: GPT-6.1 Sol pricing resolution

The system SHALL resolve official OpenAI pricing for `gpt-6.1-sol` and its date-suffixed variants
instead of inheriting the default test model price, and SHALL apply the official long-context and
priority tiers.

#### Scenario: Standard pricing resolution

- **WHEN** `gpt-6.1-sol` is queried for pricing
- **THEN** the input cost is 2 USD per million tokens, the output cost is 10 USD per million tokens, the cache write cost is 2.5 USD per million tokens, and the cache read cost is 0.1 USD per million tokens

#### Scenario: Priority tier resolution

- **WHEN** `gpt-6.1-sol` is billed on the priority (Fast) tier
- **THEN** the input cost is 4 USD per million tokens and the output cost is 20 USD per million tokens

#### Scenario: Long context tier applies

- **WHEN** a `gpt-6.1-sol` request exceeds 272,000 total input tokens
- **THEN** the whole request is billed with a 2x input multiplier and a 1.5x output multiplier

#### Scenario: Date-suffixed variants inherit the family price

- **WHEN** a model ID such as `gpt-6.1-sol-2026-10-01` is queried for pricing
- **THEN** it resolves to the `gpt-6.1-sol` family pricing, including the 0.1 USD per million token cache read rate, rather than the `gpt-6-sol` or default test model price

#### Scenario: Static fallback has no data dependency

- **WHEN** the pricing service has no pricing file loaded and `gpt-6.1-sol` is queried for pricing
- **THEN** the official GPT-6.1 Sol rates are returned instead of an error or the default test model price

### Requirement: Fallback pricing alias integrity

Every `fallback_pricing` entry in the model catalog SHALL resolve to a non-empty pricing entry
without selecting itself, so catalog validation accepts the document.

#### Scenario: No self-referential aliases

- **WHEN** the catalog fallback pricing block is validated
- **THEN** no entry's `alias_of` chain revisits the entry itself, and a multi-hop chain such as `claude-opus-4.8` resolves through `claude-opus-4.6` to `claude-opus-4.5`.

#### Scenario: Restored GPT-5.4 fallback entries keep effective prices

- **WHEN** `gpt-5.4-mini` is priced through the fallback path
- **THEN** the effective rates match the previously shipped embedded catalog (0.75 USD input and 4.5 USD output per million tokens).

#### Scenario: Catalog verifier passes

- **WHEN** the catalog verifier runs against the canonical and embedded copies
- **THEN** both copies are byte-identical and verification succeeds without an alias-cycle error
