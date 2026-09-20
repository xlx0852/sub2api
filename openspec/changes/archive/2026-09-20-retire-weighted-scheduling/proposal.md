# Change: 全平台统一简化选号，退役综合评分

## Why

用户在 OpenAI 全量切换后要求删除旧复杂逻辑，并进一步明确“所有平台都走简化后的选号逻辑”。继续保留分组开关、评分/TopK/灰度分支会让不同入口再次漂移。

## What Changes

- 所有平台默认且唯一使用共享 priority → live concurrent load → equal-tier shuffle 排序，无分组灰度、旧加权评分或 TopK 截断。
- OpenAI/Grok/Kimi 复用统一 eligibility/偏好/acquire/recheck/wait 出口；compact 保留能力硬门槛但不使用历史性能打分。
- Anthropic/Gemini/Antigravity 及混合平台保留显式模型路由、quota/RPM/window-cost/session limit，但排序改为同一共享函数；忙的普通粘性让路到其他合格账号。
- Gemini Messages 兼容入口也使用共享排序；专用端点账号类型能力仍作为约束保留。
- 删除旧 scoring/TopK/shadow 调度执行代码，移除已无意义的调参 UI 和伪综合分展示。
- 历史设置字段可为旧管理客户端保留兼容读写，但不能恢复旧执行路径；不删除用户已有数据库设置。

## Impact

用户本轮明确授权实施。本 change 替代 simplify-openai-account-selection 的 group/平台灰度限制，但不变更认证、计费、协议改写、凭据、账号启停或模型映射。已有未提交的 item-ID 修复及 OpenSpec 历史清理不混入。

完成跨平台与续链/工具/compact 回归后构建隔离候选并蓝绿上线；回滚依靠旧二进制/前端发布，不保留旧调度代码热切换。
