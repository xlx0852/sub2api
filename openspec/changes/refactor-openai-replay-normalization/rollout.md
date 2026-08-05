## 灰度观察

- 关注 `ingress_ws_indexed_compatibility_retry` 与 `ingress_ws_http_bridge_indexed_retry` 日志数量；它们只应出现在上游明确拒绝 `input[N].arguments`、`input[N].id` 或明确报告 `No tool call found ... call_id` 后。
- 对比灰度账号改造前后的 `Unknown parameter: input[N].arguments`、`Invalid input[N].id`、`Missing required parameter: input[N].arguments`、`No tool call found` 和 `INTERNAL_ERROR` 数量。
- 抽样核对同一请求的首次与重试 body：首次必须保持客户端 item 类型；重试只允许改变被拒绝索引及相同 `call_id` 的配对项。
- 观察重试后的成功率和额外上游请求量；indexed compatibility repair 每个 turn 最多增加一次请求。

## 方言开关

- 默认 API Key 账号使用 `openai_standard` replay adapter。
- 默认 OpenAI OAuth 账号使用 `codex_native`，只校验完整性，不改私有类型。
- 如兼容上游原生支持 Codex 私有 item，可在账号 `extra.openai_responses_item_dialect` 设置 `codex_native`。
- 如 OAuth 中转只接受标准 Responses item，可设置为 `openai_standard`。

## 回滚

1. 单账号异常时，先通过 `extra.openai_responses_item_dialect` 切换该账号方言，避免全局回滚。
2. indexed repair 仅由上游结构化 400 拒绝触发；若出现误判，回滚本变更提交即可恢复旧行为。
3. 回滚不涉及数据库迁移或持久化数据修正。
