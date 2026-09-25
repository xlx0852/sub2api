## ADDED Requirements

### Requirement: Mirasim metadata canonicalization matches the client crypto core

The relay client SHALL canonicalize signed metadata the way the official client's crypto core does: keys lower-cased, entries with empty values dropped, keys sorted, each entry written as `key:value`, entries joined with `\n`, and the whole set digested with SHA-256. An empty metadata set MUST contribute an empty line rather than the digest of the empty string. The canonical encoding MUST be pinned by the crypto-core regression vector.

#### Scenario: Signed request carries relay metadata

- **WHEN** a request is signed with the metadata set `{x-mirasim-agent: claude, x-mirasim-call: 11111111-2222-4333-8444-555555555555, x-mirasim-session: mirasim_00000000-0000-4000-8000-000000000000}`
- **THEN** the metadata line of the signature payload SHALL equal `91bcb885e5b045a9f55f270bb0c6d633930407b6cca792839034702ed233be6b`
- **AND** the produced signature SHALL match the crypto-core vector `zUYTEKW17Gzn7TEdEzWZ2aEOpO4oW9YFFpdsyzJaUyS4A_byq3DUNYzNOL96D24MExQ0mVbot75TkvkJw3vVAQ`

#### Scenario: Signed request carries no metadata

- **WHEN** a request is signed with an empty metadata set
- **THEN** the metadata line SHALL be empty and the payload line count SHALL stay ten

### Requirement: Inference requests carry the official relay metadata

Inference requests to the relay SHALL carry `x-mirasim-session`, `x-mirasim-agent`, and `x-mirasim-call` inside the sealed metadata blob, and those entries MUST be included in the signed metadata digest. `x-mirasim-call` MUST be fresh for every upstream attempt, `x-mirasim-session` MUST stay stable within one conversation, and `x-mirasim-agent` MUST be derived from the final upstream model and request path.

#### Scenario: DeepSeek model on the messages path

- **WHEN** an inference request sends the model `deepseek-flash` to `/v1/messages`
- **THEN** the sealed metadata SHALL contain `x-mirasim-agent: dsh`
- **AND** the plaintext `x-mirasim-device`/`-ts`/`-nonce`/`-sig` headers SHALL NOT appear on the wire outside the sealed blob

#### Scenario: Retry on another account

- **WHEN** one request is attempted against two accounts
- **THEN** each attempt SHALL carry a different `x-mirasim-call` value

#### Scenario: Metadata fields never leak into logs

- **WHEN** the relay metadata is built
- **THEN** logs MAY record only presence booleans and the agent value
- **AND** no log line SHALL contain the session or call value

### Requirement: Non-inference relay calls keep their current wire shape

The device-ticket mint and the read-only metadata endpoints SHALL keep signing with an empty metadata set, SHALL keep sending plaintext signature headers where they do today, and MUST NOT carry the sealed blob.

#### Scenario: Available-models request

- **WHEN** the client fetches `/v1/models` or `/v1/limits`
- **THEN** the request SHALL carry only the plaintext signature headers (plus `x-mirasim-probe` where required)
- **AND** the request SHALL NOT carry `x-mirasim-enc`, `x-mirasim-session`, `x-mirasim-agent`, or `x-mirasim-call`

#### Scenario: Device ticket mint

- **WHEN** the client mints a device ticket at `/v1/device/session`
- **THEN** the request SHALL be signed with the access token and an empty metadata set
- **AND** the request SHALL NOT carry the sealed blob

### Requirement: Optional relay metadata stays operator-controlled

`x-mirasim-locale` SHALL be sent only when the account configures a locale, and `x-mirasim-collect: off` SHALL be sent only when the account explicitly disables collection. `x-mirasim-account` SHALL be sent only when the access token itself names a sub-account. Absent configuration, none of these three headers may be sent.

#### Scenario: Account has no locale and no collection override

- **WHEN** an inference request is built for an account without `mirasim_locale` and without `mirasim_collect=false`
- **THEN** the sealed metadata SHALL contain neither `x-mirasim-locale` nor `x-mirasim-collect`
- **AND** `x-mirasim-account` SHALL be absent

#### Scenario: Operator disables collection

- **WHEN** the account sets `mirasim_collect=false`
- **THEN** the sealed metadata SHALL contain `x-mirasim-collect: off`

### Requirement: Device ticket expiry parsing tolerates both field spellings

The device-ticket response parser SHALL accept `expiresIn`/`expiresAt` (camelCase, as the relay sends them) and `expires_in` (snake_case) and SHALL fall back to the documented default lifetime when neither is usable or the value is non-positive.

#### Scenario: Relay returns camelCase expiry

- **WHEN** the mint response contains `{"ticket":"t","expiresIn":600}`
- **THEN** the cached ticket SHALL expire 600 seconds after the mint
- **AND** the client SHALL NOT fall back to the default lifetime

#### Scenario: Relay returns no usable expiry

- **WHEN** the mint response contains a ticket without a positive expiry field
- **THEN** the cached ticket SHALL use the default lifetime
