# Change: Add Devin Connect Provider

## Why

Sub2API currently has no first-class path for Devin session tokens. The
`leookun/devin-2api` adapter proves that Devin's Connect `GetChatMessage` RPC
can carry a complete, replayable conversation and stream text, reasoning,
tools, and usage. Bringing that transport into the existing gateway lets
administrators expose Devin accounts through the same scheduling, billing,
and OpenAI-compatible endpoints.

## What Changes

- Add `devin` as an OpenAI-compatible account and group platform.
- Add a native Go Devin Connect client with the required protobuf wire types,
  request conversion, streaming response decoding, and model discovery.
- Route `/v1/responses`, `/v1/chat/completions`, and `/v1/messages` for Devin
  accounts through the native adapter while retaining existing response and
  billing semantics.
- Add account-form metadata and platform labels for manually configured Devin
  session tokens.

## Impact

- Affected specs: `devin-provider`
- Affected code: backend platform constants, scheduling, OpenAI gateway
  forwarding, Devin transport package, frontend account registry and labels.
- No database migration is required; platform values are stored as strings.
