-- 219: 下线"分组编辑 › 高级"整页能力。
--
-- 线上使用情况（2026-11-06，4 个分组 4/23/34/49）：
--   * require_oauth_only / require_privacy_set 全为 false；
--   * fallback_group_id_on_invalid_request 全为空；
--   * supported_model_scopes 全为 '[]'（走平台默认）；
--   * models_list_config 全为 enabled=false（配了名单但开关是关的）。
--   因此这一页没有任何实际生效的配置，直接删除界面与数据不会改变线上行为。
--
-- 本迁移只清数据，不删列（groups 由 ent schema 生成，删列需要重新生成代码）。
-- 代码侧：admin 分组接口不再接收这五个字段，前端分组弹窗已移除「高级」页签。

UPDATE groups
SET require_oauth_only = FALSE,
    require_privacy_set = FALSE,
    fallback_group_id_on_invalid_request = NULL,
    supported_model_scopes = '[]'::jsonb,
    models_list_config = '{}'::jsonb,
    updated_at = NOW()
WHERE require_oauth_only
   OR require_privacy_set
   OR fallback_group_id_on_invalid_request IS NOT NULL
   OR (supported_model_scopes IS NOT NULL AND supported_model_scopes::text NOT IN ('[]', 'null'))
   OR (models_list_config IS NOT NULL AND models_list_config::text NOT IN ('{}', 'null', ''));
