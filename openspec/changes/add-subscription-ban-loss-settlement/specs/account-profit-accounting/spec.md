## ADDED Requirements

### Requirement: Subscription ban settlement explicitly terminates account supply

The system SHALL let an administrator explicitly terminate a recorded subscription cycle because the upstream account was banned. A successful termination SHALL atomically record the effective timestamp, reason, and notes; set the account and any credential-sharing shadow accounts to `disabled` and `schedulable=false`; and enqueue the corresponding scheduler changes. The system SHALL NOT infer a financial termination from generic errors, rate limits, token expiry, connectivity tests, or an existing disabled status.

#### Scenario: Administrator confirms an upstream ban

- **WHEN** an administrator confirms that a subscription account was banned during a recorded cycle
- **THEN** the system SHALL record one active termination for that cycle at the administrator-confirmed effective timestamp
- **AND THEN** it SHALL disable scheduling for the account and any credential-sharing shadow accounts in the same database transaction
- **AND THEN** subsequent routing SHALL not assign new work to those accounts

#### Scenario: Account has a temporary upstream error

- **WHEN** an account receives a 401, 403, rate limit, token expiry, connectivity failure, or generic disabled state without an administrator-confirmed ban settlement
- **THEN** the system SHALL NOT create a subscription termination or recognize a ban loss automatically

#### Scenario: Termination transaction fails

- **WHEN** any termination-ledger, account-state, shadow-account, or scheduler-outbox database write fails
- **THEN** the system SHALL roll back the whole termination transaction
- **AND THEN** it SHALL not report the cycle as financially terminated

### Requirement: Confirmed refunds reduce ban loss when received

The system SHALL record actual upstream refunds against a terminated subscription cycle with the amount and receipt timestamp. It SHALL support multiple partial refunds, SHALL exclude voided refund records, SHALL NOT count expected or pending refunds, and SHALL reject a non-voided refund total greater than the cycle purchase fee.

#### Scenario: Ban receives no refund

- **WHEN** a cycle purchased for `$865` is terminated and no refund has been received
- **THEN** its refund total SHALL be `$0`
- **AND THEN** its net purchase cost SHALL remain `$865`

#### Scenario: Partial refund arrives after the ban

- **WHEN** a terminated `$865` cycle later receives a confirmed `$200` refund
- **THEN** the system SHALL record the refund on its actual receipt timestamp
- **AND THEN** the cycle net purchase cost SHALL become `$665`
- **AND THEN** the realized loss SHALL decrease by `$200`, subject to the cycle's realized revenue

#### Scenario: Refund is pending but not received

- **WHEN** an upstream refund has been requested or promised but has not reached the actual account
- **THEN** the system SHALL NOT include it in refund total, net purchase cost, or realized loss

#### Scenario: Refund total exceeds purchase fee

- **WHEN** a new refund would make non-voided refunds exceed the cycle purchase fee
- **THEN** the system SHALL reject the refund without changing financial reports

### Requirement: Ban loss is derived from purchase cost, revenue, and refunds

For a terminated cycle, the system SHALL calculate pre-ban revenue from actual user charges inside the cycle before the ban timestamp. It SHALL derive net purchase cost, recovered amount, recovery progress, realized profit, and realized loss from the original cycle and confirmed refunds; it SHALL NOT store an independently editable loss amount.

#### Scenario: Account is banned before recovering its purchase cost

- **WHEN** a cycle costs `$865`, earns `$300` before the ban, and receives no refund
- **THEN** the system SHALL report `$565` as realized loss
- **AND THEN** recovery progress SHALL equal `$300 / $865`
- **AND THEN** the values SHALL stop changing from future elapsed time because the cycle can no longer work

#### Scenario: Partial refund offsets the unrecovered amount

- **WHEN** a cycle costs `$865`, earns `$300` before the ban, and has `$200` in received refunds
- **THEN** the system SHALL report `$665` as net purchase cost
- **AND THEN** it SHALL report `$365` as realized loss
- **AND THEN** recovery progress SHALL equal `($300 + $200) / $865`

#### Scenario: Account recovered its cost before the ban

- **WHEN** a cycle costs `$865`, earns `$900` before the ban, and receives no refund
- **THEN** the system SHALL report `$35` as realized profit
- **AND THEN** it SHALL report `$0` as realized loss and `100%` recovery progress

#### Scenario: Zero-cost cycle is terminated

- **WHEN** a confirmed `$0` cycle is terminated
- **THEN** the system SHALL report `$0` net purchase cost and `$0` realized loss
- **AND THEN** it SHALL report `100%` recovery progress without division by zero

### Requirement: Range profit recognizes ban impairment and received refunds

For a cycle with an active termination, the system SHALL straight-line amortize gross purchase cost only before the ban timestamp, SHALL recognize the remaining unamortized gross purchase cost at the ban timestamp, SHALL stop regular amortization after the ban, and SHALL recognize each non-voided refund as negative cost at its actual receipt timestamp. Across the full lifecycle, cumulative cycle cost SHALL equal purchase fee minus confirmed refunds.

#### Scenario: Ban occurs before the planned cycle end

- **WHEN** a 30-day `$900` cycle is banned after 10 elapsed days with no refund
- **THEN** the system SHALL amortize `$300` before the ban
- **AND THEN** it SHALL recognize the remaining `$600` at the ban timestamp
- **AND THEN** it SHALL attribute no further regular amortization after the ban

#### Scenario: Refund arrives in a later reporting range

- **WHEN** the same cycle receives a `$200` refund in a later reporting range
- **THEN** the later range SHALL contain a `-$200` cost adjustment on the receipt timestamp
- **AND THEN** full-lifecycle cumulative cost SHALL equal `$700`

#### Scenario: Reporting range ends before the ban

- **WHEN** a queried range ends before the confirmed ban timestamp
- **THEN** the range SHALL retain the existing straight-line amortization result
- **AND THEN** it SHALL not include the later ban impairment or refund events

### Requirement: Terminated cycle details remain visible and correctable

The system SHALL display a terminated cycle's original purchase period together with its ban timestamp, pre-ban revenue, received refunds, net purchase cost, recovery progress, realized profit, and realized loss. It SHALL allow an audited termination reversal for an erroneous settlement and audited refund voiding for an erroneous refund. A termination reversal SHALL NOT automatically make an account active or schedulable again.

#### Scenario: Administrator reviews a terminated cycle

- **WHEN** an administrator opens cost configuration or account financial statistics for a terminated cycle
- **THEN** the interface SHALL identify the cycle as terminated by upstream ban
- **AND THEN** it SHALL show the original cycle boundary and the frozen ban timestamp separately
- **AND THEN** it SHALL show the derived loss inputs and result rather than an editable loss field

#### Scenario: Administrator reverses an erroneous termination

- **WHEN** an administrator confirms a reversal reason for an erroneous termination
- **THEN** the system SHALL retain the original termination and reversal audit data
- **AND THEN** subsequent financial calculations SHALL treat the cycle as not actively terminated
- **AND THEN** the account SHALL remain disabled and unschedulable until an administrator performs a separate account recovery action

#### Scenario: Administrator tries to delete a settled cycle

- **WHEN** a cycle has termination or refund history
- **THEN** the system SHALL reject direct cycle deletion
- **AND THEN** it SHALL require audited correction actions so the financial history remains explainable

### Requirement: Ban accounting preserves bounded profit queries and cache correctness

The system SHALL batch-load cycle termination and refund data for account summaries, global summaries, trends, and overviews without database queries per account or per cycle. Successful termination, refund, refund void, or termination reversal writes SHALL invalidate cached profit overviews.

#### Scenario: Global range contains many terminated accounts

- **WHEN** an administrator loads a global profit range containing many terminated subscription accounts
- **THEN** the database query count SHALL remain constant as the number of accounts and cycles increases
- **AND THEN** the returned account totals and daily trend SHALL include ban impairment and refund adjustments

#### Scenario: Financial event changes a cached range

- **WHEN** an administrator records or corrects a termination or refund after an overview has been cached
- **THEN** the system SHALL invalidate cached profit overviews after the write succeeds
- **AND THEN** the next overview request SHALL recompute the financial result
