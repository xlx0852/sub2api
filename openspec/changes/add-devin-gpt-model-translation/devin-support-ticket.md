# Devin support ticket — draft (send from the Devin account owner's channel)

**Subject:** `GetChatMessage` rejects requests whose tool description exceeds ~4 KB with
`permission_denied: Unable to process request due to an MCP configuration issue`

## Summary

Our CLI proxy calls Devin's Connect `GetChatMessage` RPC with a Codex CLI-shaped payload. The RPC fails
with `permission_denied` / "Unable to process request due to an MCP configuration issue" whenever a tool
description in the request is longer than roughly 4 KB — the message points at MCP configuration, but the
request has no MCP servers at all and no MCP declaration. We bisected it to the description length:

| variant | result |
| --- | --- |
| single tool, description 4096 characters | 200 |
| single tool, description 5120 characters | 403 (MCP configuration issue) |
| same tools with every description replaced by `"tool"` | 200 |
| same tools with descriptions removed | 200 |
| same tools with **parameters** removed (descriptions kept) | 403 |

So `common_tools[].description` (or the equivalent field in the Connect payload) appears to be size
limited, and the rejection text is misleading.

Errors observed (trace IDs from the failing calls):

| model | error | trace ID |
| --- | --- | --- |
| `gpt-6-astra` | `permission_denied): Unable to process request due to an MCP configuration issue.` | `1900c7045eece3cbab60d5d6b86a017b` |
| `gpt-6-astra` | same | `32683f74a9ed8eb07d9ab7963e552d40` |
| `gpt-5.6-sol` | same | `b804e536ff39e000ce975d012d35311b` |
| `gpt-5.6-sol` | same | `bf5a31cdaaf22e135ceaab60f3017f0a` |
| `gpt-5.6-luna` | same | `7a2fff2921c80500749dd73ce5585be4` |
| `gpt-5.6-luna` | same | `dd04d5baebc489a956a61819c0775065` |
| `gpt-5.6-terra` | same | `5badadb146fa6b157adff452f6d4bd6b` |
| `gpt-5.6-terra` | same | `9b8dd1fd6f4ab0d088103a9e66d4c1d2` |

## What we send

Request: `POST https://server.codeium.com/exa.api_server_pb.ApiServerService/GetChatMessage`
(`Authorization: Basic <session>-<session>`, `Content-Type: application/proto`).

After translating the Codex payload, the tool section contains (names flattened from Codex's namespaces):

- `functions__exec` — **`type: "custom"`** (freeform, lark grammar) — this is Codex's unified exec tool
- `functions__wait`, `functions__request_user_input`, `functions__request_user_input_async` — `function`
- `clock__sleep` — `function`
- `collaboration__followup_task`, `collaboration__interrupt_agent`, `collaboration__list_agents`,
  `collaboration__send_message`, `collaboration__spawn_agent`, `collaboration__wait_agent` — `function`

## Isolation we already did

Replaying the exact payload against the same RPC:

| payload variant | `gpt-6-astra` | `gpt-5.5` |
| --- | --- | --- |
| full tool set (11 tools, incl. the `custom` exec) | 403 `permission_denied` | 403 |
| the same without the `custom` tool | **200** | **200** |
| only the `custom` `functions__exec` tool | 403 | 403 |
| no tools | 200 | 200 |
| a request with a single simple/`custom` tool with our own grammar | 200 | 200 |

So the failing factor is Codex's freeform (`type: "custom"`) exec tool as we declare it, not the model
ids themselves and not a blanket tool ban.

## Questions

1. What is the actual limit for a tool description in `GetChatMessage` (bytes? characters? per tool or
   for the whole tool list?). We measured 4096 accepted and 5120 rejected.
2. Why is a too-long description reported as `permission_denied` / "MCP configuration issue" instead of
   an `invalid_argument` size error? That wording cost us a lot of diagnosis time and makes logs
   misleading for every client.
3. Is there a supported way to pass long tool documentation (for example a reference to an external
   schema, or a size-agnostic field), so the model still sees the full description?

## What we do meanwhile

We truncate over-long tool descriptions (currently > 4000 characters, cut on a line boundary) for Devin
requests only, and keep the full description for every other upstream. That unblocks the four models
today, but the model loses the tail of Codex's `exec` documentation, which is the part it uses to call
nested tools. A proper size limit or an alternative documentation channel would remove that compromise.

## Repro detail

The failing request declares 11 tools; only `functions.exec` exceeds the limit (10,520 characters). The
eleven-tool request succeeds as soon as that one description is shortened or replaced.
