## ADDED Requirements

### Requirement: Admin surface for the external bridge marker

The admin account form SHALL let an operator mark an OpenAI account as backed by an external bridge and
name the bridge kind, and SHALL persist that intent in the account's `extra` map, because the marker
switches a whole class of gateway behaviour that is otherwise only reachable through the admin API.

#### Scenario: Marking an account as an external bridge

- **WHEN** the operator enables the external bridge switch and enters a bridge kind
- **THEN** saving the form SHALL store `extra.external_bridge` as `true`
- **AND** SHALL store the kind lower-cased and trimmed in `extra.external_bridge_kind`

#### Scenario: Cleared switch leaves no stale kind

- **GIVEN** an account already carries `external_bridge` and `external_bridge_kind`
- **WHEN** the operator turns the switch off and saves
- **THEN** both keys SHALL be removed from `extra`
- **AND** the account SHALL be handled as a plain OpenAI-compatible upstream again

#### Scenario: Existing intent is visible when editing

- **WHEN** the operator opens an account that is already marked as an external bridge
- **THEN** the switch SHALL read as enabled and the kind field SHALL show the stored kind
