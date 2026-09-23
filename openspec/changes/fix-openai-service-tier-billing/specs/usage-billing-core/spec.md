## ADDED Requirements

### Requirement: Service-tier pricing follows the model's declared tier table

The system SHALL price OpenAI `service_tier` requests from the tier prices declared for that model,
and SHALL NOT infer a premium for a tier the model does not declare. Fast mode prices SHALL cover
input, cached input, cache writes, and output.

#### Scenario: Fast mode scales every input side including cache writes

- **WHEN** a request uses `service_tier: priority` for a model whose price entry declares `*_priority` prices
- **THEN** input, cached input, cache writes, and output are all billed at the declared Fast-tier prices

#### Scenario: A model without a Fast tier is not upscaled

- **WHEN** a request uses `service_tier: priority` for a model whose price entry declares no `*_priority` prices
- **THEN** the request is billed at the model's standard rates instead of an inferred premium

#### Scenario: Flex stays a uniform discount

- **WHEN** a request uses `service_tier: flex`
- **THEN** the request is billed at 0.5x the model's standard rates

#### Scenario: Fast mode composes with long-context pricing

- **WHEN** a Fast request exceeds the model's long-context input threshold
- **THEN** the Fast-tier prices are multiplied by the long-context multipliers, doubling the input side and scaling output by 1.5x

#### Scenario: Downgraded Fast requests bill at standard rates

- **WHEN** a request specifies `service_tier: priority` and the upstream response reports `service_tier: "default"`
- **THEN** the request is billed at the model's standard rates

#### Scenario: The reported tier is read from the upstream response

- **WHEN** an upstream Responses payload carries the effective tier
- **THEN** the tier is read from the terminal event of a stream or from the response body of a non-streaming response, taking the last frame value so an early `response.created` frame cannot mask it

#### Scenario: Requests without a reported tier keep prior behaviour

- **WHEN** the upstream response carries no usable `service_tier`
- **THEN** billing falls back to the tier requested in the request body

### Requirement: Long-context pricing only applies where declared

The system SHALL apply long-context multipliers only when the model's price entry declares a
long-context threshold together with at least one multiplier greater than 1.

#### Scenario: Declared long-context tier applies to the whole request

- **WHEN** total input tokens (input, cache creation, and cache read) exceed the declared threshold
- **THEN** the entire request is billed at the long-context rates, including cache writes scaled with the input multiplier

#### Scenario: Models without a long-context tier bill at standard rates

- **WHEN** a model's price entry declares no long-context threshold or multipliers
- **THEN** requests above 272,000 input tokens are still billed at the model's standard rates
