# 验证记录

## 范围

本次是显式分组启用的 OpenAI 普通请求 profile，不宣称已删除共享 scorer 或完成全平台迁移。
原始基线：`63f365071c7be6e8334008a84a5596fc05a44520`。
工作区原有的 item ID 长度修复和 OpenSpec 历史清理不包含在此候选中。

## 本地测试

- 测试先于实现：新增排序/profile 测试最初因缺少实现未通过；实现后全部通过。
- `go test ./internal/config ./internal/app/scheduling ./internal/server/middleware ./internal/archtest`：通过。
- 聚焦 `TestOrderByPriorityAndLoad|TestSimpleSelection|TestResolvePinnedCredentialClass|TestIgnoreUpstream401`：通过，race 同样通过。
- service/handler 的 `Scheduler|Schedul|Sticky|SelectAccount|Failover|Selection|Responses|WebSocket|HTTPBridge|ToolCall|Credential` 扩大回归：除下述两个基线失败外通过；显式 `-skip` 两项再运行 service 和 handler 均通过。
- `go vet`：scheduling/config/service/handler/middleware 通过。
- `go build ./internal/... ./cmd/server`：通过。
- `openspec validate simplify-openai-account-selection --strict` 与 scoped `git diff --check`：通过。

## 已知基线失败

在独立 detached worktree 的原始 HEAD 上执行以下两项，均出现同样失败，未修改断言掩盖问题：

- `TestOpenAIGatewayService_SelectAccountWithScheduler_LoadBalanceIgnoresHistoricalThreshold`
- `TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_CodexImageBridgeRespectsResponsesLite`

本机首次 race 链接失败是 macOS 27 SDK 与默认 cc 的 TAPI 不兼容；将 CC 指定为 `xcrun --find clang`、SDKROOT 指向 Xcode 的 MacOSX26.2.sdk 后重跑通过。该环境故障未作为功能测试通过证据。

## 覆盖的合同

- disabled/cooldown 来源不能锁住历史 apikey class。
- session 与 user pin 仅优先尝试可用来源，忙/无效时其他合格账号可接替。
- hard gates：组、平台、模型冷却、普通冷却、排除集、credential class、privacy、transport。
- 新号无历史样本与同档旧号均能获得首选机会；缺失/非法 load 不等同空闲。
- TopK=1 时仍能尝试完整的 10 个候选；第十个可用账号不被截断。
- 账号容量全忙时使用现有有界 WaitPlan；模型级全局预算满时仍立即 ModelLimited，不反复切号。
- DB 状态改变后最终复核拒绝候选。
- previous response 命中原号；原号忙时保留等待；不可移动且原号缺失时明确报错，不抢其他账号槽位。
- ignore_upstream_401 打开时仍保持本次请求认证 class 隔离；下一独立请求不继承限制；profile 回滚/高级调度关闭也不能绕过本次认证限制。
- 非认证失败以及其他平台不写入 OpenAI 请求级认证 class。
- 经新 profile 选号后，HTTP JSON、HTTP SSE、WS→HTTP bridge 的 mock upstream wire 验证 call_id 配对、完整工具输出和适用的 previous_response_id 保留。

## 发布状态

默认 `simple_selection_group_ids: []`，本记录不表示生产已启用。
生产灰度与观测需要按 operations.md 选择 scope；未执行全局切换或清理真实用户会话。

## 隔离候选交付

- 源码提交：`1817d36270384cc6f49c9ef89e9bac034dd238c3`（主体实现 `4a20f5e8c`，配置环境变量补全 `1817d3627`）。
- 使用 detached clean worktree 重跑 config/scheduling/middleware/archtest 全包，以及前述去除两项基线失败后的 service/handler 扩大回归，均通过。
- 候选：`/tmp/sub2api-simple-selection-candidate`，Linux amd64，CGO=0，embed；前端未改，复用现有已构建的 dist，不包含任何前端功能改动。
- SHA-256：`c8706411f738964adb7e7c2ccc88207df92684bbb63080c40cc258fa4b2fa45e`。
- 该环境 detached worktree 构建未自动记录 `vcs.revision`；已显式通过 `-X main.Commit=1817d36270384cc6f49c9ef89e9bac034dd238c3` 写入源码版本，并用构建信息与校验和核对。不把 release 标签当作版本证据。
- 已验证 `GATEWAY_OPENAI_SCHEDULER_SIMPLE_SELECTION_GROUP_IDS=4,42` 可加载为 `[4,42]`；未配置时仍为空。
- 生产状态：未上传、未切流、未变更分组 scope；本次完成的是实现、回归与候选交付。
