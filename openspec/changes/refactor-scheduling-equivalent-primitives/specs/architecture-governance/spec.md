## ADDED Requirements

### Requirement: Scheduling primitive extraction preserves selection semantics

The system SHALL share behavior-equivalent scheduling primitives across Gateway and OpenAI without changing either Strategy's candidate thresholds, sticky-session policy, scoring, cooldown, quota pause, failure punishment, or credential-class behavior.

#### Scenario: A common primitive replaces duplicate code

- **WHEN** channel restriction, account listing, sorting, shuffling, or sticky candidate validation is migrated to a shared primitive
- **THEN** the old and shared implementations produce the same candidate set, ordering groups, exclusion result, and sticky decision for the same inputs

#### Scenario: Strategies differ after the common boundary

- **WHEN** Gateway requires session registration or OpenAI requires sticky epoch, quota re-evaluation, escape, runtime blocking, or transport compatibility checks
- **THEN** those checks remain in their existing Strategy and execute in the same order as before the extraction

#### Scenario: Advanced scheduler behavior is active

- **WHEN** OpenAI advanced scheduling, sticky weighted selection, lowest-usage selection, or TopK exploration is enabled
- **THEN** this refactor does not change its score inputs, weights, candidate gates, exploration set, or selected account
