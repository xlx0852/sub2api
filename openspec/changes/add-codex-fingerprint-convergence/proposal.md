# Change: 增加 Codex OAuth 设备指纹收敛

## Why

多个客户端共享同一 OpenAI OAuth 账号时，Codex 会在 Responses 请求中携带 installation、session、thread 及其父子关系标识。直接透传会让上游看到大量设备和会话，且 HTTP、passthrough 和 WebSocket 路径若各自改写容易产生头体不一致。

## What Changes

- 为 OpenAI OAuth 账号增加 `off/device/session/full` 四档 Codex 指纹收敛策略。
- 使用账号级稳定 installation/session 标识，并按模式稳定派生 thread/window/父子线程标识。
- 统一改写 HTTP、OAuth passthrough 和 Responses WebSocket v2 路径的请求头、`client_metadata` 与内嵌 turn metadata。
- 覆盖最新 Codex `root_turn_id`、`parent_thread_id`、`forked_from_thread_id` 和 `parent_turn_id`，保留 sandbox、review 等非身份字段。
- 历史账号未配置时保持 `off`；新建 OAuth 账号默认选择 `session`，不对存量账号静默切换。
- 在账号创建、编辑和批量编辑界面暴露策略选择。

## Impact

- Affected specs: `codex-fingerprint-convergence`
- Affected code: OpenAI OAuth 账号 extra、Responses HTTP/passthrough/WS 出站改写、账号管理界面与测试
- Compatibility: 历史账号和非 OAuth 账号默认行为不变
