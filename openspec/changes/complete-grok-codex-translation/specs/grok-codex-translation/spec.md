## ADDED Requirements

### Requirement: Grok Requests Translate Codex Custom Tools

The system SHALL translate Codex `custom` tools into xAI-compatible `function` tools for Grok Responses requests while preserving each tool's name and free-form input contract. The system SHALL NOT drop `apply_patch` solely because it is a custom tool.

#### Scenario: Apply patch is forwarded as a function tool

- **GIVEN** a Grok Responses request declares a `custom` tool named `apply_patch`
- **WHEN** the gateway builds the xAI upstream request
- **THEN** the request contains a `function` tool named `apply_patch`
- **AND** its parameters accept the original free-form input through a string field

### Requirement: Grok Requests Translate Custom Tool History

The system SHALL translate Codex custom tool calls, custom tool outputs, and custom tool choices into their xAI function equivalents without changing `call_id`, tool name, ordering, or output content.

#### Scenario: Custom tool continuation remains executable

- **GIVEN** a Grok Responses request contains a prior `custom_tool_call` and matching `custom_tool_call_output`
- **WHEN** the gateway builds the xAI upstream request
- **THEN** it emits a matching `function_call` and `function_call_output`
- **AND** the original free-form input can be recovered from the function arguments
- **AND** both items retain the original `call_id`

#### Scenario: Custom tool choice matches translated definition

- **GIVEN** a request selects a named custom tool through `tool_choice`
- **WHEN** the custom tool is translated to a function tool
- **THEN** `tool_choice` is translated to select the same named function

### Requirement: Grok JSON Responses Restore Codex Custom Calls

The system SHALL restore xAI `function_call` output items to Codex `custom_tool_call` items only when the function name belongs to a custom tool declared by the same downstream request.

#### Scenario: Matching function call is restored

- **GIVEN** the downstream request declared a custom tool named `exec`
- **AND** Grok returns a `function_call` named `exec`
- **WHEN** the gateway returns a non-streaming Responses payload
- **THEN** the output item type is `custom_tool_call`
- **AND** it contains the original `call_id`, name, and recovered free-form `input`

#### Scenario: Ordinary function call is preserved

- **GIVEN** the downstream request declared an ordinary function tool named `lookup`
- **AND** Grok returns a `function_call` named `lookup`
- **WHEN** the gateway returns the response
- **THEN** the output item remains a `function_call`

### Requirement: Grok SSE Responses Restore the Complete Custom Tool Lifecycle

The system SHALL restore matching custom tool calls across the complete Responses SSE lifecycle, including item-added, input delta, input done, item-done, and terminal response payloads.

#### Scenario: Streaming custom call is executable by Codex

- **GIVEN** the downstream request declared a custom tool named `apply_patch`
- **WHEN** Grok streams that tool as function-call events
- **THEN** the gateway emits `custom_tool_call` items in added, done, and terminal response payloads
- **AND** function argument delta/done events are emitted as custom tool input delta/done events
- **AND** `call_id`, name, output index, item id, and free-form input remain consistent across the event sequence

### Requirement: Translation State Is Request Scoped

The system SHALL keep the original custom tool-name set scoped to one downstream request and SHALL NOT persist or reuse it across requests.

#### Scenario: Same function name in another request is not misclassified

- **GIVEN** one request declares `lookup` as a custom tool
- **AND** a later independent request declares `lookup` as an ordinary function tool
- **WHEN** each response is translated
- **THEN** only the first request's matching call is restored as `custom_tool_call`

