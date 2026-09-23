# 二十四、OpenCode Go 请求不通：缺 `x-opencode-session` 与 UA 被 Cloudflare 拦

## 24.1 现象（用户实测）

账号测试返回：

```
API returned 400: {"type":"error","error":{"type":"MissingSessionID",
  "message":"Request is missing x-opencode-session and cannot be routed efficiently.
             Please see https://opencode.ai/docs/go/#where-can-i-use-it"}}
```

## 24.2 两个上游强制要求（读上游 v0.2.8 `openai_opencode_session.go` 核实）

1. **每个推理请求必须带 `x-opencode-session`**。自 **2026-09-05** 起缺失即 400 `MissingSessionID`。
   它是**客户端会话的名称**，上游按它做 prompt cache 路由。
2. **User-Agent 必须是 opencode 客户端形态**（`opencode/1.0.0`）。
   opencode.ai 前置 Cloudflare 按 UA 做 bot 拦截，编程库/其它客户端 UA 会被 **CF error code 1010**
   拒成 403；而本仓会把该 403 计成**账号凭据类故障** ⇒ **健康账号可能被自动禁用**。
   这是"我们可控的头导致账号被停"的一类问题，值得单独记住。

**判定与平台无关**：上游规则是「opencode 计划账号，**或任意账号目标为官方主机**」。
本仓当前接法（`openai` + `apikey` + `credentials.base_url` 指向官方基址）正落在后者，
所以**不需要先做平台**也必须遵守。

## 24.3 实现

`opencode_session.go`：

- `isOfficialOpenCodeHost`：**比主机名**（https + `opencode.ai`），不做前缀匹配
  ⇒ `opencode.ai.evil.example` 不会被当成官方主机。
- session 值优先级：客户端自带且通过字符/长度校验 → **原样沿用**；
  否则用稳定客户端标识（`session-id` / `thread-id` / `x-client-request-id` / 体里 `prompt_cache_key`）；
  都没有才退一次性 UUID。
  —— **不做哈希派生**：把客户端的会话名换成派生值等于凭空造一个新会话，反而丢失连续性；
  而每轮换随机值会让上游 prompt cache 持续 miss（上游注释明确警告）。
- UA 在官方主机上**强制**收敛为 `opencode/1.0.0`。

接线三处：**账号测试/探测**（用户失败的那条）、**Forward**、**passthrough**。

## 24.4 验证

| 断言 | 位置 |
|---|---|
| 官方主机带上 session 头 + opencode UA（且不含 codex 字样） | Go 单测 |
| 非官方主机（中继 / 伪装子域 / api.openai.com）**不改 UA、不加头** | Go 单测 |
| 同种子同值；不同会话不同值；无种子才随机 | Go 单测 |
| 客户端自带合法值原样沿用；含空格/换行/超长则拒绝并改用种子 | Go 单测 |
| 主机名判定：伪装子域与 http 均拒绝 | Go 单测 |
| seed 优先级：`session-id` > 体 `prompt_cache_key`；都无则空 | Go 单测 |

**上线**：`0.1.391-ocsession-5caa42c49` @18370；二进制含 `X-OpenCode-Session` 与 `opencode/1.0.0`；
公网 `/health` 200；回滚目标 `ocgcell-71fe371a8` @18369。

## 24.5 教训

这次是**"范围判断"漏掉的一环**：我按 release 标题只移植了"用量窗口"，
而同一个能力里还嵌着**协议强制项**（session 头）。判断一个上游能力能否"用现有平台接法接上"时，
除了转发是否通，还要问**它的协议是否比 OpenAI 兼容多要求了什么**。

## 24.6 第一次修复没生效：钩子挂错了代码路径

用户复测仍报 `MissingSessionID`。

**原因**：我把 `applyOpenCodeUpstreamHeaders` 挂到了 `ProbeOpenAIAPIKeyResponsesSupport`
（一个后台能力探测），而**「测试账号」按钮走的是 `account_test_service.go` 的
`testOpenAIAccountConnection`**。我误把上一次日志里出现的 `probe_done` 当成了按钮调用的路径——
那条日志记的是**探测模型**（`selectResponsesProbeModel` 选的），不是用户填的测试模型。

**改法**：在该文件**全部 4 处 OpenAI 测试路径**（`testOpenAIAccountConnection` /
`testOpenAIChatCompletionsConnection` / `testOpenAICompactConnection` / `testOpenAIImageAPIKey`）
统一挂钩，位置都在 `ApplyHeaderOverrides` **之前**——管理员显式配置的请求头仍拥有最终决定权。

## 24.7 补上"本该先写"的测试

这次错误的性质是**接线漏了**，而不是逻辑错。所以测试必须**走真实入口**，不能只测 `apply` 函数：

`account_test_service_opencode_session_test.go`：

| 断言 | 说明 |
|---|---|
| 走 `testOpenAIAccountConnection`（真实入口）后，出站请求带 `X-OpenCode-Session` 与 `opencode/1.0.0` | 这条断言**只有我的钩子能满足**——全仓 grep 确认该头**唯一写入点**在 `opencode_session.go`，因此它能真正锁住接线 |
| 普通 OpenAI 账号（`api.openai.com`）**不带该头、UA 不被改写** | 防止污染其它上游的指纹 |

**为什么自包含、不带 `unit` 构建标签**：带 `unit` 的那套 helper 当前在共享分支上**因其它测试文件
签名不匹配而无法编译**（与本次改动无关），而这条断言恰恰是最需要能跑起来的。

## 24.8 与上游的一致性核对

逐条比对上游 `resolveOpenCodeSessionID`：优先级（入站头 → 其它会话头 → 体 `prompt_cache_key`
→ `metadata.user_id.session_id` → 已存在的出站头 → **`uuid.NewString()`**）与 `sanitizeSessionID`
的字符/长度约束**均一致**。测试路径没有客户端会话，因此落到 UUID 兜底——与上游同路径行为相同。

## 24.9 第二个问题：DeepSeek Go 不能走 Responses

session 头修好后，用户复测 `deepseek-v4.1-flash` 的错误从 `MissingSessionID` 变成了：

```
API returned 400: {"type":"server_error","message":"Upstream request failed: This Go model requires Global regions..."}
```

配置 Global 后，错误进一步变成了**HTTP 200 但响应内容为空**；日志显示测试请求耗时从几百毫秒到 48 秒，
Scheduled Diagnostics 还因此把账号临时降级。

对照上游 v0.2.8 的 `DefaultOpenCodeGoProtocolRules` 后定位：

- `gpt-*` / `grok-*` / `muse-spark-*` → Responses；
- `minimax-*` / `qwen*` → Anthropic Messages（本仓最小接入暂未实现）；
- **其它 Go 模型（包括 `deepseek-*`）→ Chat Completions**。

本仓原来把所有 OpenAI API-key 账号默认当 Responses，DeepSeek 因此打错协议，返回空响应。

修复：

- 账号测试路径：OpenCode Go 的 DeepSeek 等默认模型改走 `/v1/chat/completions`；
- 正式转发路径：复用现有 `forwardResponsesViaRawChatCompletions`；
- `gpt/grok/muse` 仍走 Responses；
- session 头与 `opencode/1.0.0` UA 保持在两条路径上。

测试用例直接驱动真正的 `testOpenAIAccountConnection` 入口，断言
`deepseek-v4.1-flash → https://opencode.ai/zen/go/v1/chat/completions`，不是只测辅助函数。

上线后请再次点击账号测试；这次预期不会再是 MissingSessionID、Global region 或空响应。
