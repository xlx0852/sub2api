# Change: Add Kimi OAuth Accounts

## Why

Sub2API can price and forward Kimi-compatible API-key traffic, but it cannot sign in to Kimi Coding subscriptions or keep their OAuth credentials refreshed. Administrators need a first-class Kimi account type that can be authenticated with Kimi's device authorization flow and scheduled safely by the gateway.

## What Changes

- Add Kimi as a first-class account and group platform
- Add Kimi OAuth device authorization with start, status, and cancellation operations
- Persist Kimi access tokens, refresh tokens, expiry, scope, and stable device identity
- Refresh Kimi OAuth credentials through the shared distributed refresh framework
- Forward Kimi OAuth traffic to Kimi Coding Chat Completions while preserving Chat, Responses, and Messages-compatible downstream semantics
- Normalize Kimi model aliases, device headers, thinking fields, tool links, and streaming usage
- Add an administrator device-login workflow and Kimi account management surfaces

## Impact

- Affected specs: `kimi-oauth-accounts`
- Affected code: platform constants, account scheduling, OAuth services, token refresh, gateway routing, account management API and UI
