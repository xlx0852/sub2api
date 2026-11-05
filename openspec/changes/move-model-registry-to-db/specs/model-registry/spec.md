## ADDED Requirements

### Requirement: Model identity lives in one place

The system SHALL treat the `models` table as the only definition of whether a model exists,
which platform it belongs to, and what its capabilities are. The compiled catalog file SHALL
remain only as a seed source and as the fallback when the registry source is `file`.

#### Scenario: Registered model is visible

- **WHEN** a model exists in `models` with `enabled = true`
- **THEN** it SHALL appear in `/v1/models` for its platform and in the admin model list, together with its display name, context window and media capabilities

#### Scenario: Unregistered model cannot be priced

- **WHEN** an operator tries to save a price row for a model id that is not in `models`
- **THEN** the request SHALL fail with `MODEL_NOT_REGISTERED` and no price row SHALL be written

#### Scenario: Retired model keeps resolving

- **WHEN** a model is marked `retired`
- **THEN** requests for it SHALL still resolve pricing and routing, and it SHALL NOT appear in model listings

### Requirement: Aliases map every spelling to one canonical model

Alias resolution SHALL come from `model_aliases` (database) and SHALL cover compatibility
aliases, upstream id overrides and retired ids. A request spelling that resolves through an
alias SHALL price and route exactly like the canonical id.

#### Scenario: Compatibility alias prices like the canonical model

- **WHEN** a request uses an alias such as `kimi3.1` for `kimi-k3.1-pro`
- **THEN** pricing resolution SHALL use the canonical model's rows and the upstream mapping SHALL use its canonical upstream id

#### Scenario: Conflicting alias is rejected

- **WHEN** an alias is saved for a model while it already points at a different model
- **THEN** the save SHALL be rejected instead of silently rewriting

#### Scenario: Unknown spelling fails closed

- **WHEN** a request spelling matches neither a model id nor an alias
- **THEN** pricing SHALL fail closed with the existing unavailable-pricing error

### Requirement: Prices reference the registry and support group overrides

Every price row SHALL reference `models.id`. Per-group rows SHALL override the model's
default row (`group_id IS NULL`); the resolution order SHALL be exact group row, then default
row, then fail closed. Price rows SHALL support `fx_mode` derivation and priority-tier prices.

#### Scenario: Group override wins

- **WHEN** a model has both a default row and a row for the requesting group
- **THEN** the group row SHALL be charged

#### Scenario: Missing group row falls back to the default

- **WHEN** a group has no row for a model but the model has a default row
- **THEN** the default row SHALL be charged and the group SHALL be marked as falling back in the admin

#### Scenario: No default row fails closed

- **WHEN** neither a group row nor a default row exists
- **THEN** the request SHALL be rejected with the unavailable-pricing error and the model SHALL be flagged in the admin list

#### Scenario: Priority price is honoured

- **WHEN** a price row defines `input_price_priority` / `output_price_priority` and the request uses the priority tier
- **THEN** those values SHALL be charged instead of the standard ones

#### Scenario: Empty price row is invalid

- **WHEN** a price row is saved with no token price, no per-request price and no image tier
- **THEN** the save SHALL be rejected, because a row that exists without a price would otherwise settle at zero

### Requirement: Registry and pricing sources are switchable and verifiable

The system SHALL provide `MODEL_REGISTRY_SOURCE` (`file|dual|db`) and
`MODEL_PRICING_SOURCE` (`legacy|dual|model_prices`) settings, switchable at runtime, where
`dual` computes both sources and only logs differences.

#### Scenario: Dual mode does not change charges

- **WHEN** either switch is `dual`
- **THEN** charging SHALL follow the legacy source and only differences SHALL be logged

#### Scenario: Seeded values reproduce current behaviour

- **WHEN** the seed migration imports the catalog and the existing sell rows
- **THEN** every model's resolved price per group SHALL equal the value produced before the migration

#### Scenario: Rollback is a switch

- **WHEN** the switches are set back to `file` and `legacy`
- **THEN** the previous resolution path SHALL be used again without a data rollback
