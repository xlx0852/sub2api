## Context

当前 pipeline 已提供 Exchange 和阶段模型，但部分入口仍在 handler 中维护完整 provider 生命周期。目标是逐步迁移，保持行为可对拍。

## Goals / Non-Goals

- Goals: 缩短 handler 编排函数；显式管理 lease、错误和结算交接；降低 provider 分支漂移。
- Non-Goals: 不合并协议适配器；不改变调度算法、价格、倍率、配置或部署方式；不一次性重写全部平台。

## Decisions

- 每个迁移阶段只改变一个入口族，并保留旧/新响应、usage 和账务差分测试。
- 共享生命周期只传递 `pipeline.Exchange` 和显式阶段结果；provider adapter 不持有隐式全局状态。
- Lease 通过 `AccountLease` 传递，handler 只调用 `Release()`，兼容旧 `ReleaseFunc` 直到所有入口迁移完成。

## Risks / Trade-offs

- 分阶段迁移会暂时保留旧入口；通过差分门禁和独立提交控制风险。
- WS、SSE 和普通 HTTP 的 drain 语义不同，分别迁移，不强行共用响应解析器。

## Migration Plan

1. 选择一个 HTTP/SSE Responses 主路径，提取 parse/admission/relay/settle 阶段。
2. 运行定向测试与旧/新差分。
3. 迁移 WS、图片和兼容入口。
4. 所有入口完成后删除旧 handler 编排函数。

