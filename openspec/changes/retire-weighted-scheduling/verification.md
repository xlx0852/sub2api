# 全平台调度退役验证

## 实现边界

- 原始基线：`5bd4508f4`（OpenAI group 4/42 简化 profile 已发布）。
- OpenAI/Grok/Kimi/compact 固定使用 priority/live-load/equal-shuffle；原高级开关、订阅优先、加权粘性、TopK 和分组 opt-in 均不参与执行。
- Anthropic/Gemini/Antigravity Gateway 使用同一排序；模型路由是显式优先池，RPM/window-cost/quota/session limit 继续作为平台约束；忙号让路，最后统一有界等待。
- Gemini Messages 兼容入口用同一排序。AI Studio 等专用端点保留账号类型的协议可用性分层，在合格层内使用统一排序。
- 真实 previous_response 与工具上下文不为选号而改写；认证失败 class 仍为请求级隔离。
- 旧评分 API 字段、历史设置和 YAML 字段仅留兼容读取/持久化，不提供恢复旧执行路径的开关。当前 UI 不再显示旧权重/TopK/rollout 或账号伪总分。
- 没有数据库 schema 迁移，不改模型映射、账号启停、额度或用户真实会话。

## 删除与替代

删除 app/scheduling 综合评分、TopK/加权抽样和 priority/LRU 第二排序实现；保留仅用于同档洗牌的 RNG。
删除 Gateway shadow/enforce 执行与对比器、OpenAI 旧粘性/评分/重算分支、无效灰度开关，以及仅测试已退役函数的断言。
原评分/LRU/强粘性断言替换为对应新合同；分组与 full-account 测试夹具补齐真实数据关系，没有为通过测试放松分组约束。

## 已执行验证

- 六平台 priority/load 选择回归，历史 advanced=false/rollout=off/shadow/enforce 均不能切换算法。
- 同档无历史账号参与竞争；全部候选（不截断 TopK）可获得槽位；未知负载不是测得零。
- Gateway session-limit 拒绝后槽位恰好释放；RPM/费用窗门槛；模型路由优先池及忙时回退；Gemini/Antigravity mixed 标记；DB group 变更后复核拒绝。
- compact unsupported 拒绝、unknown 可尝试；全局模型预算满直接 ModelLimited、不在账号间放大重试。
- 真实续链同号/不可恢复报错、HTTP JSON/SSE/WS bridge 工具配对；ignore-401 不绕过本次认证 class 隔离。
- Grok handler 同账号一次重试后失败转移；WS passthrough usage、reasoning-effort 与限额事件回归。
- `go test ./internal/config ./internal/app/scheduling ./internal/server/middleware ./internal/archtest ./internal/handler ./internal/handler/admin` 通过。
- service 聚焦调度/续链/compact 回归通过；`go test ./internal/service` 除下列六项既有失败外全部通过（显式 -skip 后再次全包执行）。
- app/scheduling 全包 race，service 跨平台/调度/续链/compact 聚焦 race 通过。macOS 使用匹配的 Xcode CC + MacOSX26.2.sdk。
- `go build ./internal/... ./cmd/server`、相关 `go vet`、前端 `vue-tsc --noEmit` 与改动组件 ESLint 通过。
- 前端 SettingsView/AccountsView scheduler 相关 Vitest 共 22 项通过，验证保存过旧布局也不会恢复总分列或调参控件。
- `openspec validate retire-weighted-scheduling --strict` 通过。

## 既有失败（独立 clean worktree 复现）

在原始 HEAD 对照执行下列六项，均同样失败，因此不修改这些协议/时间断言来掩盖失败：

- `TestIsModelRateLimited`
- `TestGetModelRateLimitRemainingTime`
- `TestGetRateLimitRemainingTime`
- `TestOpenAIGatewayServiceForward_CodexImageInjectionRespectsGroupCapability`
- `TestOpenAIGatewayService_OAuthPassthrough_CompactUsesJSONAndKeepsNonStreaming`
- `TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_CodexImageBridgeRespectsResponsesLite`

完整 `-tags unit` 历史套件另有既存 AuthService/RegisterWithVerification 签名漂移；本次主要回归均放在默认可执行测试集中，不把“no tests to run”当作通过证据。

## 工作区与发布

原有 item-ID 超长修复五个生产文件的 binary diff 与清理前逐字一致；OpenSpec 历史归档变更不加入本次提交。
候选须从独立 clean worktree 构建前端 dist 后再 embed 构建后端；外部 Nginx SPA 另行发布，并保留 API/SPA 成对回滚。
