# Design

- 公共排序入口固定为 app/scheduling.OrderByPriorityAndLoad，所有合格候选都进入 acquire 顺序；没有历史数据的账号不受评分惩罚。负载缺失与已知零负载区分。
- 保留平台 adapter 的硬门槛和协议状态所有权，不把 OAuth/API Key 能力差异、模型路由和混合平台边界变成随机选号。
- 用户 pin/session 仅为有效候选中的优先尝试；忙则让路。previous_response 无法安全移动时保留原号等待或明确续链错误。请求级认证 class 隔离维持。
- compact 的显式 unsupported 仍拒绝，unknown 可尝试；以最终账号状态复核避免 stale metadata 错误永久过滤。全局模型并发预算满仍直接让路，不逐账号重试。
- 共享 scorer、TopK、加权 RNG/seed、调度 shadow 对比及旧 controller 开关均不再参与生产选号。RNG 仅保留同档 shuffle。
- 旧设置 API 的持久化字段为兼容层（无行为），UI 改成策略说明，不展示失真的总分。普通前端和 API 版本允许回滚。
- 不是切断真实用户已有 WS/SSE 连接：蓝绿后旧连接自然排空，旧容器留回滚。
