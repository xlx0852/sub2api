# Change: Update Grok HTTP Upstream Routing

## Why

Grok OAuth credentials currently persist the official xAI API base URL, so all text HTTP traffic follows `api.x.ai` unless the account is manually rewritten to the CLI endpoint. CPA now distinguishes Grok Build OAuth traffic from official API traffic and routes each request type to the endpoint that actually supports it.

## What Changes

- Route Grok OAuth text HTTP requests to `cli-chat-proxy.grok.com` by default
- Route non-OAuth or explicitly `using_api=true` text HTTP requests to the official xAI API
- Preserve explicit custom gateways instead of rewriting them
- Keep image, video, and native WebSocket traffic on the official API path
- Attach Grok CLI identity headers only when the resolved target is the official CLI chat proxy
- Apply the same routing rules to account connection tests

## Impact

- Affected specs: `grok-http-upstream-routing`
- Affected code: Grok account credential resolution, Responses and Chat Completions request builders, account connection tests
