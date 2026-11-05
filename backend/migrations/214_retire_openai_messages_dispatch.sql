-- 214: 下线「OpenAI 分组的 /v1/messages 调度 + Claude 系列 → GPT 族映射」能力。
--
-- 理由：线上 30 天只有 10 次请求（且仅 GPT Pro 组开启），却带来三套配置项与一条容易误配的
-- 映射路径；OpenAI 平台的 /v1/messages 现在直接拒绝（请用 Anthropic 分组承接）。
-- grok / kimi 的原生兼容入口不受影响。
--
-- 列保留（ent schema 未改），但数据清空；以后再决定是否删列。

UPDATE groups
SET allow_messages_dispatch = FALSE,
    messages_dispatch_model_config = '{}'::JSONB,
    default_mapped_model = '',
    updated_at = NOW()
WHERE allow_messages_dispatch
   OR messages_dispatch_model_config <> '{}'::JSONB
   OR default_mapped_model <> '';
