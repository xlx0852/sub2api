## ADDED Requirements

### Requirement: Each account owns exactly one diagnostics plan

The system SHALL allow at most one scheduled diagnostics plan per account, so schedule, model and recovery settings
have a single owner.

#### Scenario: Creating a second plan is rejected

- **WHEN** a client creates a plan for an account that already has one
- **THEN** the system SHALL reject the request with a conflict response naming the existing-plan rule
- **AND** SHALL NOT insert a second row

#### Scenario: The rule survives concurrency and old clients

- **WHEN** two creates race, or an older client that does not know the rule calls the create endpoint
- **THEN** the database uniqueness constraint SHALL still leave exactly one plan per account

#### Scenario: Existing duplicates are collapsed

- **WHEN** a deployment already holds several plans for one account
- **THEN** the migration SHALL keep the enabled, most recently updated plan and delete the rest

#### Scenario: The interface does not invite duplicates

- **WHEN** an account already has its plan
- **THEN** the diagnostics panel SHALL hide the create action and explain that the existing plan is edited instead

### Requirement: The next execution is shown as a live countdown

Every diagnostics view SHALL count down to the plan's next execution instead of showing a frozen relative time.

#### Scenario: Countdown ticks

- **WHEN** a plan has a next run time
- **THEN** the view SHALL show the remaining time in minutes and seconds (hours for longer intervals)
- **AND** SHALL update every second without a manual refresh

#### Scenario: Execution is due

- **WHEN** the countdown reaches zero
- **THEN** the view SHALL show that the run is executing now rather than a negative value
- **AND** SHALL reload once, a few seconds later, so the new result appears without a manual refresh

#### Scenario: No background timer survives the view

- **WHEN** the panel or drawer is closed, closed while its data is still loading, or unmounted
- **THEN** the ticking SHALL stop entirely

#### Scenario: No next run time

- **WHEN** a plan is disabled or has no next run time
- **THEN** no countdown SHALL be rendered

### Requirement: Diagnostics performs no model-fingerprint challenge

The diagnostics execution SHALL only send a lightweight connectivity probe. The system SHALL NOT send model-purity
challenges, and SHALL NOT store, display, or expose any attribution result.

#### Scenario: Connectivity probe only

- **WHEN** a diagnostics plan is due
- **THEN** the runner SHALL send the connectivity probe to the account's models
- **AND** SHALL NOT send a fingerprint challenge, whatever historical plan settings exist

#### Scenario: The removed capability is unreachable

- **WHEN** a client calls a model-purity endpoint
- **THEN** the request SHALL fail as an unknown route

#### Scenario: No attribution state remains

- **WHEN** the schema is inspected after the removal
- **THEN** no attribution table, column, index, resource file, config key or interface element SHALL remain

### Requirement: Diagnostics detects reasoning degradation

The diagnostics probe SHALL send a reasoning problem with a single integer answer instead of a trivial greeting, and
SHALL judge the answer server side, so an account that still answers but no longer reasons like the model it claims
can be spotted.

#### Scenario: The probe replaces the greeting

- **WHEN** any text account test runs (interactive test, or diagnostics)
- **THEN** the request SHALL carry the reasoning problem
- **AND** image-generation tests SHALL keep their image prompt

#### Scenario: The expected answer is never sent

- **WHEN** the request body is inspected
- **THEN** it SHALL NOT contain the expected answer or any wrong-answer hint
- **AND** the expected value SHALL live only in server-side code

#### Scenario: The verdict is recorded once

- **WHEN** a test finishes with a text answer
- **THEN** the result SHALL carry exactly one judgement line (expected, observed, correct/incorrect)
- **AND** the same line SHALL be visible in the interactive test stream and in the diagnostics history

#### Scenario: The answer key is configurable and disclosed

- **WHEN** an operator sets a different expected answer (config `diagnostics.reasoning_probe_expected_answer`
  or the matching environment variable)
- **THEN** the judgement SHALL use that key
- **AND** the report SHALL name the key actually used
- **AND** when the key differs from the strictly derived answer and the model produced that derived answer, the
  report SHALL disclose the discrepancy instead of hiding it behind the key

#### Scenario: A slow answer is not a broken account

- **WHEN** the upstream delivers part of the answer but exceeds the test budget
- **THEN** the execution SHALL be recorded as connected
- **AND** the answer SHALL be reported as unjudged rather than wrong
- **AND** quota-wall and demote decisions SHALL NOT treat the timeout as an upstream failure

#### Scenario: One sample is not an accusation

- **WHEN** the answer is wrong
- **THEN** the report SHALL describe it as a lead that needs repeat samples rather than as proof of degradation

#### Scenario: Non-text answers are not judged

- **WHEN** the answer is an image payload, a URL or an error
- **THEN** no judgement SHALL be produced
