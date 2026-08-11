## ADDED Requirements

### Requirement: Positive usage charges use a canonical six-decimal ceiling

The system SHALL convert every positive final usage charge to the smallest multiple of `0.000001 USD` that is greater than or equal to the unquantized charge. A zero charge SHALL remain zero.

#### Scenario: Positive charge has digits beyond the sixth decimal

- **WHEN** pricing and all applicable multipliers produce a final usage charge of `0.123456001 USD`
- **THEN** the canonical usage charge is `0.123457 USD`

#### Scenario: Positive charge is smaller than one quantum

- **WHEN** pricing and all applicable multipliers produce a positive final usage charge smaller than `0.000001 USD`
- **THEN** the canonical usage charge is `0.000001 USD`

#### Scenario: Charge is already on the canonical scale

- **WHEN** the final usage charge is exactly `0.123456 USD` or exactly zero
- **THEN** the canonical usage charge remains unchanged

### Requirement: All usage ledgers consume the same canonical charge

The system SHALL use the same canonical customer charge for customer balance deduction or subscription consumption, API Key quota and rate-limit consumption, user platform quota, billing caches, and the persisted `usage_logs.actual_cost` value. Account quota SHALL retain its existing account-cost basis and SHALL independently apply the same six-decimal positive ceiling. Cost component fields MAY retain their original calculation precision.

#### Scenario: Balance-billed request is settled

- **WHEN** a balance-billed request produces a positive canonical charge
- **THEN** the balance delta, applicable quota deltas, cache deltas and persisted actual cost equal that canonical charge exactly

#### Scenario: Subscription-billed request is settled

- **WHEN** a subscription-billed request produces a positive canonical charge
- **THEN** subscription consumption, applicable quota deltas, cache deltas and persisted actual cost equal that canonical charge exactly

### Requirement: Quantization preserves billing idempotency

The system SHALL derive the billing request fingerprint from the original unquantized amounts before applying the canonical charge, and SHALL settle a request identity at most once.

#### Scenario: Request retries across deployment

- **WHEN** the same request identity is retried after six-decimal ceiling quantization is deployed
- **THEN** it matches the original billing fingerprint and does not create a duplicate charge

### Requirement: Non-usage money flows are excluded

The system SHALL NOT apply positive usage-charge ceiling quantization to recharges, refunds, manual balance adjustments, upstream procurement costs, or historical records.

#### Scenario: Administrator issues a refund

- **WHEN** a refund or manual balance adjustment is recorded
- **THEN** its existing monetary precision and rounding contract remain unchanged
