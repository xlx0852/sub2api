-- 218: 下线"客户端限制"能力（Claude Code 分组限制 + Codex 官方客户端加固）。
--
-- 线上使用情况（2026-11-06）：
--   * groups.claude_code_only 全为 false，fallback_group_id 全为空 → 无人依赖；
--   * accounts.extra.codex_cli_only 仅 1 个已 disabled 的账号为 true；
--   * 仍配置了全局 codex_cli_only 名单/指纹（黑名单 1 条 Ubuntu UA），但没有任何活跃账号
--     开启 codex_cli_only，因此实际从未生效。
--
-- 本迁移只清数据，不删列（groups/accounts 的列由 ent schema 生成，删列需要重新生成代码）。
-- 代码侧已移除判定与拒绝：分组不再按客户端类型限制，网关不再按 codex 指纹/名单拦截。

-- 1) 分组级 Claude Code 限制
UPDATE groups
SET claude_code_only = FALSE,
    fallback_group_id = NULL,
    updated_at = NOW()
WHERE claude_code_only
   OR fallback_group_id IS NOT NULL;

-- 2) 全局 Codex 客户端加固配置
DELETE FROM settings
WHERE key IN (
    'codex_cli_only_blacklist',
    'codex_cli_only_whitelist',
    'codex_cli_only_allow_app_server_clients',
    'codex_cli_only_allow_body_engine_fingerprint',
    'codex_cli_only_engine_fingerprint_signals',
    'min_codex_version',
    'max_codex_version'
);

-- 3) 账号级 codex_cli_only 标记（含历史快捷字段）
UPDATE accounts
SET extra = extra - 'codex_cli_only' - 'codex_cli_only_allowed_clients' - 'codex_cli_only_allow_app_server',
    updated_at = NOW()
WHERE extra ? 'codex_cli_only'
   OR extra ? 'codex_cli_only_allowed_clients'
   OR extra ? 'codex_cli_only_allow_app_server';
