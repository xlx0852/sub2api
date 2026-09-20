## 1. 回归与契约
- [x] 1.1 补不可调度 class 来源、统一 eligibility、忙绑定让路和冷号同档轮转回归。
- [x] 1.2 补真实 previous-response 约束与请求级认证失败隔离回归。

## 2. 实现
- [x] 2.1 修复失效绑定来源校验，添加默认关闭的 OpenAI group 灰度配置。
- [x] 2.2 在 app/scheduling 实现 priority/load/equal-shuffle 纯排序，无 TopK 截断。
- [x] 2.3 同一 OpenAI Strategy 复用统一硬门槛、绑定偏好及 acquire/recheck/wait 出口。
- [x] 2.4 请求上下文显式保留认证失败 class；不由普通 sticky 派生该限制。
- [x] 2.5 文档说明配置、边界、观测和回滚方式。

## 3. 验证
- [x] 3.1 聚焦 scheduler/policy/affinity/failover 与 HTTP/SSE/WS/tool continuation 回归通过。
- [x] 3.2 race、编译、vet、diff check、OpenSpec strict；基线失败单列。
- [x] 3.3 交付 Linux 候选与验证记录；生产灰度状态明确，不提前声明线上完成。

## 4. 用户授权后的生产上线（2026-09-20）
- [x] 4.1 核对候选哈希和线上配置；仅给 group 4 启用简化 profile。
- [x] 4.2 独立 synthetic session 重现旧 503，对照新版本 JSON/SSE completed 与 132 用量。
- [x] 4.3 蓝绿切流、真实 origin 健康、sol/astra 冒烟、错误与用量窗口验证。
- [x] 4.4 记录回滚点与旧连接排空状态，保留其他账号设置和既有工作区改动。

## 5. 用户授权后的全量扩围（2026-09-20）
- [x] 5.1 核对全部当前 OpenAI 分组为 [4,42]，保持其他平台/compact/账号绑定不变。
- [x] 5.2 同一候选扩大环境变量 scope，健康验证后切到 18343；保留 18342 回滚点。
- [x] 5.3 验证 sol/astra JSON/SSE 和自然业务窗口，记录 group 42 无供给与存量连接排空边界。
