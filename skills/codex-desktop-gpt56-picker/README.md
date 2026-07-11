# codex-desktop-gpt56-picker

Skill package that makes **Codex Desktop** show GPT-5.6 Sol / Terra / Luna in the model picker when using API-key or custom-provider mode.

## For humans (simplest)

After install, just:

```bash
codex56
```

Or double-click Desktop `Codex5.6启动.command`.

This relaunches Codex Desktop with debugging, turns off Statsig `use_hidden_models`, and reloads the picker.

Full skill path (also fine):

```bash
cd skills/codex-desktop-gpt56-picker
bash scripts/apply-codex-5.6-picker.sh
```

Then reopen the model picker in Codex Desktop (`ChatGPT.app` with bundle id `com.openai.codex`).

## For AI agents

Read [SKILL.md](./SKILL.md) and follow the workflow exactly (cache → verify → CDP Statsig patch).

## Install locations

Copy or symlink this directory to any Agent Skills path, for example:

- `~/.codex/skills/codex-desktop-gpt56-picker`
- `~/.pi/agent/skills/codex-desktop-gpt56-picker`
- `~/.agents/skills/codex-desktop-gpt56-picker`
- project `skills/codex-desktop-gpt56-picker`

## Persistence reminder

| Step | Persistent? |
|------|-------------|
| Refresh `~/.codex/models_cache.json` | Yes |
| Statsig CDP patch | No — re-run after app restart |
