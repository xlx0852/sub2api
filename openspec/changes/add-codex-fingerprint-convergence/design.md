## Context

Codex 0.147.0 把 installation/session/thread/window/turn 同时投影到 HTTP 头、Responses `client_metadata` 和内嵌 `x-codex-turn-metadata`。最新 main 还增加 `root_turn_id`，并在多智能体请求中传递 parent/fork 关系。

## Goals / Non-Goals

- Goals: 同一账号同一请求在所有载体中使用一致的收敛标识；覆盖 HTTP、passthrough 与 WS；可显式回滚。
- Non-Goals: 不改写 User-Agent/originator 策略；不收敛 sandbox、tool inventory、review 状态或 routing hint。

## Decisions

- Decision: 收敛 ID 只在每个上游请求进入转换时计算一次，同一对象同时驱动头和体改写。
- Decision: installation/session 使用账号 ID 的加盐确定性 UUID；session 模式的 thread 使用客户端原 session 派生；full 模式把 thread/root/parent 关系收敛到账号线程图。
- Decision: `turn_id` 仍每请求新生成，避免并发 turn 冲突；parent turn 使用稳定派生值保持关系自洽。
- Decision: 账号 extra 缺失表示 `off`，前端只在新建 OAuth 账号时主动写入 `session`。

## Risks / Trade-offs

- Full 模式会把并发线程压到单线程指纹，可能增加上下文关联冲突 → UI 标记为激进选项，默认使用 session。
- 最新 main 字段可能在正式版前变化 → 改写器仅处理明确的身份字段，未知字段保留。

## Migration Plan

1. 上线代码和 UI，不改写存量账号 extra。
2. 新账号默认 session；存量账号由管理员显式选择。
3. 出现上游关联异常时将账号切回 off，无需数据库回滚。
