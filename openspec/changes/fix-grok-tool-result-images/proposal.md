# Change: Forward tool-result images on the Grok Responses path

## Why

An operator reported that Grok "cannot see images" when used through this gateway from pi. Live reproduction
narrowed it to one shape: the same screenshot read perfectly in a normal user message (row count and the first
row's order id and email all correct) but produced a **fabricated** answer when it arrived as a tool result
(`function_call_output.output[].input_image`) — "5 rows, nathan.k@example.net" for a 17-row payment table.

Root cause is in this repository, not in the client: xAI's ModelInput only accepts a string for
`function_call_output`, so the translation split content items and explicitly discarded every image part, leaving
the literal marker `[image]`:

```go
case "input_image", "output_image":
    // Drop image payloads; keep a short marker so tool continuity remains.
    parts = append(parts, "[image]")
```

The model therefore never received the pixels and answered about an image it could not see. The same converter
serves `custom_tool_call_output` and `local_shell_call_output`, so all three shapes were affected.

## What Changes

- When a tool result carries images, the translation keeps the text in the `function_call_output` string **and
  re-sends the images as a follow-up user message** (`input_image` parts), which is a shape the model does read.
- Image parts without a usable URL keep the existing `[image]` marker so tool continuity stays visible instead of
  silently vanishing.
- Nested `image_url: {url: ...}` objects are unwrapped like everywhere else in the Grok payload path.

## Impact

- Affected specs: `grok-codex-translation`.
- Affected code: `internal/service/openai_gateway_grok.go` (tool-output conversion) plus regression tests in
  `internal/service/grok_tool_image_test.go`.
- Every client that sends tool-result images through the Grok path benefits; no schema, config or UI change.
