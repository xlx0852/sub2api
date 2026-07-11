# Change: Add Grok API Key Accounts

## Why

Sub2API already forwards Grok traffic to the official xAI API when an account is marked as API-key based, but administrators cannot create such an account from the UI and the model-sync and account-test paths still reject it. The capability is therefore incomplete and cannot be operated safely from the management center.

## What Changes

- Allow administrators to create and edit Grok API Key accounts using an xAI API key and base URL
- Fetch the live model list from the configured xAI-compatible `/v1/models` endpoint
- Test Grok API Key accounts through the official Responses API
- Reuse the existing Grok Responses, Chat Completions, image, and video forwarding paths
- Keep Grok OAuth endpoint routing, refresh, quota, and subscription behavior unchanged

## Impact

- Affected specs: `grok-api-key-accounts`
- Affected code: account management UI, Grok credential resolution, upstream model sync, account connection tests
