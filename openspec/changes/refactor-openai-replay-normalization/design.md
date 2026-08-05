## Context

当前 `openAIWSToolCallReplayCollector` 同时消费 `response.output_item.done` 和 `response.completed`，但按 `id/call_id` 首次写入后直接去重。上游若在 done 事件给出半成品、completed 给出完整项，完整项会被丢弃。随后 `prepareOpenAIWSHTTPBridgeBody` 对整个 input 补 `arguments`、改工具类型和 id，掩盖中间态问题并扩大了改写范围。

## Goals / Non-Goals

- Goals:
  - completed 快照成为重放数据的权威来源
  - 客户端原始 input 字节语义不被 replay 兼容层改写
  - WS、HTTP bridge 和普通 Responses HTTP 共用相同的 indexed repair 规则
  - 保持 `call_id` 不变，避免 call/output 断链
- Non-Goals:
  - 不改变账号调度、连接池或计费逻辑
  - 不主动探测每个上游的私有 item 类型能力
  - 不在本次改造中新增数据库字段

## Decisions

- Decision: collector 保存候选事件，但 terminal completed/done 中的完整 output 整体替换候选集合
  - Why: terminal output 同时提供完整字段和权威顺序，比分字段合并更容易证明正确
  - Alternative considered: 逐字段合并 done/completed
  - Rejected because: 不同 item 类型字段集合不同，逐字段合并容易保留过期状态

- Decision: 以内部数据流区分所有权，不向 JSON 注入 source 字段
  - Why: collector 输出天然是 `replay_owned`；客户端 payload 天然是 `client_owned`，不需要污染协议

- Decision: 私有工具 item 转标准 Responses 时保留 `call_id`，删除不合法的私有 item `id`
  - Why: `call_id` 是 call/output 关联键；简单前缀替换可能让 call 与 output 产生重复 item id
  - Alternative considered: 将所有私有 id 前缀改成 `fc_`
  - Rejected because: `ctc_x` 与 `ctco_x` 会映射到同一个 id，且伪造 id 可能形成无效引用

- Decision: 客户端原始 item 只在上游明确返回 indexed field rejection 后做对应索引的单次修复
  - Why: 首次请求保持原语义，同时为严格上游提供可控降级

## Risks / Trade-offs

- completed output 缺失时只能使用 done 候选
  - Mitigation: 发送前校验并丢弃不完整的 replay call 及其配对 output，不补造 arguments
- 某些第三方上游可能要求非标准 item id
  - Mitigation: indexed retry 只删除被明确拒绝的 id；不全量猜测上游方言
- 新旧路径短期并存增加排查维度
  - Mitigation: 测试中对三条路径使用同一组 fixtures，并用账号 `extra.openai_responses_item_dialect` 做方言级灰度和回退

## Migration Plan

1. 上线 completed 权威覆盖与 replay-only adapter
2. 上线 indexed repair retry，并记录触发原因
3. 对测试账号和少量流量灰度，比较错误率与请求改写差异
4. 稳定后删除 bridge 全量 input 归一化旧逻辑
