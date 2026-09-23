# Change: Expose the external-bridge switches in the admin account form

## Why

External bridges (Devin, Cursor) are opted into with the account-level marker
`extra.external_bridge` plus `extra.external_bridge_kind`. The flag decides a whole class of gateway
behaviour — namespace flattening, freeform-tool downgrade, over-long tool-description truncation, and
which status endpoint the usage projection reads — yet it had no admin surface at all. Operators had to
call the admin API by hand (or edit `accounts.extra` directly), and a wrong or missing kind silently
degrades the account to generic-bridge behaviour because the backend does not validate it.

## What Changes

- Add an **External bridge** switch to the OpenAI advanced section of both the create and the edit
  account forms, plus a free-text **bridge kind** field that appears while the switch is on (hinted with
  the known kinds `devin` / `cursor`).
- Persist the switch as `extra.external_bridge: true` and the kind as `extra.external_bridge_kind`
  (lower-cased, trimmed). Turning the switch off removes **both** keys, so a stale kind cannot keep
  leaking into behaviour that reads the kind without checking the flag.
- Rehydrate both fields from `account.extra` when the edit form opens.
- Keep everything else unchanged: the backend already stores account `extra` verbatim (only `base_rpm` is
  sanitized), so no API or schema change is needed.

## Impact

- Affected specs: `external-provider-bridges` (added: the admin surface for the bridge marker).
- Affected code: `frontend/src/features/admin-accounts/form/credentialsBuilder.ts` (new
  `applyOpenAIExternalBridge`), both account modals, and the zh/en admin-account locale files.
- Operators can now create, flip, and roll back a bridge account from the UI; disabling the switch
  returns the account to plain OpenAI-compatible upstream behaviour without touching its base URL or key.
