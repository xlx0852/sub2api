-- 217: 把 catalog.fallback_pricing 里的 alias_of 关系补进 model_aliases（kind=alias）。
--
-- 216 已经把这些名字播种成 models，但它们的"按目标模型收费"语义只存在于
-- fallback_pricing.alias_of 里。注册表来源切到 db 时，overlay 需要这张别名表才能把
-- 目标价物化到别名 key 上，否则 gpt-5.5 / kimi-k3 / claude-opus-5 这类名字会失去兜底价。
--
-- 幂等：ON CONFLICT DO NOTHING；平台留空表示"仅用于价格物化"。

INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('claude-opus-4.6', '', 'claude-opus-4.5', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('claude-opus-4.7', '', 'claude-opus-4.6', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('claude-opus-4.8', '', 'claude-opus-4.7', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('claude-opus-5', '', 'claude-opus-4.8', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('gemini-3.1-pro-high', 'antigravity', 'gemini-3.1-pro', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('gemini-3.1-pro-low', 'antigravity', 'gemini-3.1-pro', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('gemini-3.6-flash', 'antigravity', 'gemini-3.8-flash', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('gemini-3.6-flash-high', 'antigravity', 'gemini-3.8-flash', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('gemini-3.7-flash', 'antigravity', 'gemini-3.8-flash', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('gemini-3.7-flash-high', 'antigravity', 'gemini-3.8-flash', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('gemini-3.8-flash-high', 'antigravity', 'gemini-3.8-flash', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('gemini-pro-agent', 'antigravity', 'gemini-3.1-pro', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('gpt-5.5', '', 'gpt-5.4', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('gpt-5.5-pro', 'openai', 'gpt-5.5', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('grok-4.20-non-reasoning', 'grok', 'grok-4.20-0309-non-reasoning', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('grok-4.20-reasoning', 'grok', 'grok-4.20-0309-reasoning', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('k3-256k', 'kimi', 'k3', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('kimi-for-coding-highspeed', 'kimi', 'kimi-for-coding', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('kimi-k3', 'kimi', 'k3', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('swe-2-high', '', 'swe-2', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('swe-2-max', '', 'swe-2', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
INSERT INTO model_aliases (alias, platform, model_id, kind) VALUES ('swe-2-medium', '', 'swe-2', 'alias') ON CONFLICT (alias, platform) DO NOTHING;
