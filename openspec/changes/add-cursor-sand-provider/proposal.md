# Change: Add Cursor Sand Provider

## Why

Sub2API aggregates multiple upstream AI platforms, but cannot natively ingest, authenticate, or forward traffic to Cursor / Sand (Grok Bot) cloud backends. Administrators need a first-class Cursor platform provider that authenticates via PKCE browser authorization, automatically refreshes OAuth credentials, computes client checksums (`x-cursor-checksum`), and streams Chat Completions through Connect-RPC.

## What Changes

- Add `cursor` as a first-class account and group platform constant (`PlatformCursor = "cursor"`)
- Implement native Go `x-cursor-checksum` obfuscation and client fingerprint generation
- Implement Cursor Connect-RPC client for streaming inference, tool calling, and thinking support
- Implement PKCE browser authorization flow with state polling, lease locks, and one-time login tickets
- Implement distributed token refresher (`CursorTokenRefresher`) with automated rotation before expiry
- Implement quota and account profile synchronization via Dashboard Connect-RPC service
- Route Chat Completions traffic to Cursor while preserving downstream OpenAI and streaming semantics
- Add administrator web surfaces for Cursor account authorization, quota metrics, and channel routing

## Impact

- Affected specs: `cursor-sand-provider`
- Affected code: platform constants, account scheduling, Connect-RPC client, token refresher, gateway pipeline, admin API and UI
