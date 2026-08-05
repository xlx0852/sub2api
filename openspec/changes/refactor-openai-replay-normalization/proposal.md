# Change: Refactor OpenAI Replay Normalization

## Why

OpenAI Responses 多轮工具调用目前在 HTTP bridge 出口对整个 `input` 做兜底改写，既可能误改客户端原始 payload，也无法解决 `response.output_item.done` 中间态抢先进入去重集合、导致 `response.completed` 完整项被丢弃的问题。连续出现的 `Missing arguments`、`Unknown parameter` 和 `Invalid id` 表明现有补丁式处理需要收敛为一条可验证的重放链路。

## What Changes

- 将 replay collector 改为以 `response.completed.output` / `response.done.output` 为权威快照，覆盖先前的中间态
- 仅对服务端采集并重新注入的 replay items 做标准 Responses 适配，客户端原始 input 保持不变
- 统一工具调用类型、参数和 item id 的兼容处理，并在发送前校验 call/output 关联完整性
- 为普通 Responses HTTP 路径增加上游明确拒绝 `input[N].arguments` / `input[N].id` 时的索引级单次修复重试
- 增加历史故障、事件乱序、重复 ID、长 input 和三条转发路径的回归测试
- 在新链路稳定前保留现有 bridge 出口兜底，但不再让它改写客户端所有 input

## Impact

- Affected specs: `openai-replay-compatibility`
- Affected code: OpenAI WS replay collector、WS/HTTP bridge replay 合并、Responses HTTP rejected-field retry、协议兼容测试

