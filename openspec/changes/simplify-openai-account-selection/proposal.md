# Change: 简化 OpenAI 账号选择与失效绑定处理

## Why

2026-09-20 group 4 在有健康 OAuth 供给时仍因历史 apikey 绑定派生的 credential_class 全部拒绝而返回 routing-503。来源账号 89 不可调度但模型映射存在；此前仅校验模型支持的修复不完整。多维评分、不同层的 eligibility 与粘性置顶重复也让决策难解释。

## What Changes

- 修复兼容路径的 class 来源校验：不可调度来源不参与推导。
- 在同一个 scheduling.Engine/OpenAI Strategy 内增加按 group 显式启用的简化 profile：统一硬门槛 → 有效绑定优先尝试 → priority/并发占用率/同档随机；完整候选集不做 TopK 截断。
- 普通 session/user pin 不再派生凭证类硬限制，不可用或忙时让其他合格账号尝试；真实 previous_response 续链保留账号约束。
- 将本次请求发生的认证失败凭证类显式保存在请求 context，而不是从失败账号的残留 sticky 猜测。不会把一次请求的限制传播到后续独立请求。
- 默认不启用；`gateway.openai_scheduler.simple_selection_group_ids` 是唯一灰度入口，空列表回滚。仅 OpenAI 普通请求适用，compact 和其他平台保留原策略。
- 保留并发原子获取、最终状态复核、等待上限、排除集、计费和协议改写边界。

## Impact

- Affected specs: 新增 `openai-account-selection`。
- Affected code: app/scheduling 纯排序、OpenAI Strategy/策略与请求级认证失败状态、API key context 装配、config、tests。
- Related changes: `promote-unified-scoring-scheduler` 的共享引擎和平台 adapter 不被替换，其未完成部署门禁不勾选；`harden-openai-pool-outcome-sticky-stream` 的请求级认证失败隔离保留，最低用量/评分仅在本次显式灰度 scope 被新顺序取代；`relax-credential-class-pin` 的失效来源缺口在此修复。
- 不包含 OpenSpec 历史清理、未提交 input item ID 修复、401 开关重设计或其他平台迁移。

## Approval and delivery boundary

用户在确认“先简化约束与粘性，再简化打分”的方案后回复“开工”，授权本范围实施。先交付代码、可复现回归和候选构建；生产启用必须显式选择 group 并完成检查，不能以短请求压测代替所有续链/流式回归。
