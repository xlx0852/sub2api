# 启用与回滚

## 默认行为

新增配置 `gateway.openai_scheduler.simple_selection_group_ids` 默认为空。
本 change 不改变 `scheduler_scoring_rollout` 的 off/shadow/enforce 含义，不自动启用任何生产分组。

需要对某个 OpenAI 分组灰度时，在该 API 实例配置中设置：

```yaml
gateway:
  openai_scheduler:
    simple_selection_group_ids: [4]
```

按现有发布流程加载新配置后，group 4 的普通 OpenAI 请求使用新 profile。
其他 group、Grok/Kimi/Anthropic/Gemini/Antigravity、compact 请求保留原策略。
旧 `scheduler_score_weights`/TopK/sticky-weight/subscription-priority 参数保留，
但不参与简化 profile 的普通排序。已有真实会话仍优先复用健康原号，不承诺均匀分流。

## 观测

- `openai_simple_selection`：模型、group、最终 account、layer、candidate_count、required_credential_class。
- `openai_simple_affinity_invalid`：被失效的绑定类型与账号，不记录 session 内容。
- `openai_scheduler_no_candidates_reject_trace`：现有空池 policy 明细。
- 区分成功 usage 的 account_id 与 ops_error 的失败尝试；HTTP 200 不能代替 response.completed。
- 首轮重点验证 disabled apikey 绑定不排除健康 OAuth、并发满可让路、新号无历史仍能参与。
- `previous_response_id` 不可移动且原号不可用时，HTTP 返回明确的 400 续链错误、WS 明确关闭原因；不得为获得 200 删除工具上下文。
- 401/403 后本次请求 class 隔离独立于 ignore_upstream_401（该开关仅影响账号摘除）。

## 发布门禁

从隔离 worktree 构建候选，避免把未提交的 input item ID 修复或文档清理带入发布。
候选健康检查只是控制面验证，不能替代已记录的 HTTP/SSE/WS/tool continuation 回归。
未得到实际生产启用证据前，本 change 不声称已经灰度成功。

## 回滚

把 group ID 从列表移除并按现有方式重新加载配置，即退回兼容 profile；
必要时回到前一 API 二进制和配置。账号优先级、凭据、真实会话绑定和旧权重不批量改写。
不要通过清空所有 Redis 会话或停用健康账号来制造“分流成功”。
