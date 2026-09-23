## 1. Form logic

- [x] 1.1 Add `applyOpenAIExternalBridge(extra, enabled, kind)` to the account form builder: writes
  `external_bridge = true` plus a normalized kind when enabled, and deletes both keys when disabled.
- [x] 1.2 Wire it into the create and edit submit paths and rehydrate the switch/kind from `account.extra`
  when the edit form opens.

## 2. Admin UI

- [x] 2.1 Add the switch and the kind field to the OpenAI advanced section of both modals, with the kind
  field revealed only while the switch is on and hinted with the known kinds.
- [x] 2.2 Add the zh/en strings (label, description, kind label/hint/placeholder).

## 3. Verification

- [x] 3.1 Unit tests for the builder: enable with whitespace/case normalization, enable without a kind,
  and disable clearing both keys while leaving unrelated keys alone.
- [x] 3.2 Modal tests: toggling on with a kind submits `external_bridge` + `external_bridge_kind`; an
  account that already has them shows the kind and clearing the switch removes both keys.
- [x] 3.3 `vue-tsc --noEmit` clean and the account component suites pass (148 tests).
- [x] 3.4 `openspec validate add-external-bridge-admin-toggle --strict`.

### Deploy record

- Frontend release `0.1.391-extbridge-564ca33a2` published; `current` repointed from
  `0.1.391-quotacontrol-e50b9d83b` (now `rollback`). Public index serves `assets/index-Bxk8Tfhu.js` (200)
  and the chunk carrying `external-bridge-toggle` answers 200 with the zh label.
- No backend change: account `extra` already round-trips verbatim, so the switch works against the
  running gateway binary.
