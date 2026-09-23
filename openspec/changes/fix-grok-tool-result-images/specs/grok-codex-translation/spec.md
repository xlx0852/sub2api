## ADDED Requirements

### Requirement: Tool results survive translation

The translation SHALL preserve what a tool returned, including images, in a shape the Grok upstream actually
reads.

#### Scenario: Tool result with text only

- **WHEN** a tool result contains text only
- **THEN** the text SHALL be carried in the translated `function_call_output` string
- **AND** no extra message SHALL be added

#### Scenario: Tool result with images

- **WHEN** a tool result contains image parts (`input_image` / `output_image`)
- **THEN** the text parts SHALL stay in the `function_call_output` string
- **AND** the images SHALL be re-sent as a follow-up user message carrying the original URLs, because the upstream
  accepts only a string for a tool result and would otherwise never receive the pixels
- **AND** the images SHALL NOT be replaced by a placeholder marker

#### Scenario: Image part without a usable URL

- **WHEN** an image part carries no URL that can be forwarded
- **THEN** the `[image]` marker SHALL remain in the tool output so the missing payload is visible

#### Scenario: Nested image URL objects

- **WHEN** an image part carries `image_url` as an object with a `url` field
- **THEN** the URL SHALL be extracted like the rest of the Grok payload path does
