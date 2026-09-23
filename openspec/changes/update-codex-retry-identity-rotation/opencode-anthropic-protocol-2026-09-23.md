# 二十五、OpenCode Go 的 Anthropic Messages 原生协议：可用性实证与落地配置

## 25.1 结论

**可用，且本仓不需要改代码**——走的是已有的 Anthropic APIKey「自动透传」路径，
只需按 25.5 配置账号。之前判「不支持」源于一个**认证头误判**。

## 25.2 认证头误判：`Bearer` 401、`x-api-key` 200

同一个 `/zen/go/v1/messages`，仅换认证头：

| 请求头 | 结果 |
|---|---|
| `Authorization: Bearer <key>` | **401** `{"type":"error","error":{"type":"AuthError","message":"Missing API key."}}` |
| `x-api-key: <key>`（`anthropic-version` 可省） | **200** 标准 Anthropic 响应 |

OpenAI 系端点（`/chat/completions`）认 Bearer，Anthropic 系端点只认 `x-api-key`。
本仓 apikey 账号的 OpenAI 出站固定发 Bearer（`openai_gateway_cc_pipeline.go:203`、
`openai_apikey_responses_probe.go:149`），所以「用 OpenAI 路径打 `/messages`」必然 401。

**检查顺序**：上游先校验 `x-opencode-session`（缺则 400 `MissingSessionID`），再校验认证头，
所以缺 session 时看到的 400 会掩盖认证错误，排查时容易误判方向。

## 25.3 模型能力矩阵（33 个模型逐个实测 `/zen/go/v1/messages`）

**支持 Anthropic Messages：14/33**

- `deepseek-v4-flash` / `deepseek-v4-flash-vision-exp` / `deepseek-flash` / `deepseek-v4.1-flash` / `deepseek-v4-pro`
- `minimax-m2.5` / `minimax-m2.7` / `minimax-m3`
- `qwen3.6-plus` / `qwen3.7-max` / `qwen3.7-plus` / `qwen3.8-max` / `qwen3.8-flash`
- `kimi-k3`

**不支持**：`glm-5.1/5.2/5.3/5.3-flash`、`kimi-k2.6`、`kimi-k2.7-code`、`mimo-v2.5/v2.6-flash/v2.5-pro/v2.6-pro`、
`longcat-2.0`、`omen-alpha`、`hy3`、`hy4-preview`。

三类错误必须分清（同一条 `503 Endpoint is unavailable` 掩盖了两种完全不同的原因）：

| 现象 | 类型 | 判定依据 |
|---|---|---|
| `glm-*`/`kimi-k2.*`/`mimo-*` 在 `/messages` 503 | **协议不支持** | 对照：同模型同刻在 `/chat/completions` 上 **200** |
| `gpt-5.6-luna`/`grok-4.6`/`grok-4.7` | **容量不足** | 两个协议**都** 503 |
| `muse-spark-1.2/1.3-contributor` | **策略拒绝** | 400 `This Go model trains...` |

## 25.4 功能验证（不只是能返回）

- **流式为标准 Anthropic SSE**：`message_start` → `ping` → `content_block_start` → `content_block_delta`，
  首字节 **1.29s**，形态与 Claude 官方一致。
- **工具调用**：`stop_reason=tool_use`，返回规范 `tool_use` block。
- **多轮工具循环**：把 assistant 的 `thinking + tool_use` 原样回传 + `tool_result` → 200 / `end_turn` / 正确答案；
  剥掉 `thinking` 再试同样 200（签名被接受但不强制）。
- **推理以带 `signature` 的 `thinking` block 返回**，比 Chat Completions 的 `reasoning_content` 更规范，
  且对 Claude Code 类客户端无跨协议转换损耗——这是走原生协议的主要收益。
- `usage.cache_read_input_tokens` 生效，prompt cache 正常。

**伪装头要求（逐项剥离实测）**：`x-opencode-session` **必需**（缺 → 400 `MissingSessionID`）；
**UA 不要求**（curl 默认、python-requests、Chrome UA 全部 200）——与 `/chat/completions` 的结论不同，不要照搬。

## 25.5 本仓落地：配置即可，无需改码

| 环节 | 代码位置 | 结论 |
|---|---|---|
| 路由开关 | `gateway_forward.go:100` → `IsAnthropicAPIKeyPassthroughEnabled()` | 条件 `platform=anthropic` + `type=apikey` + `Extra["anthropic_passthrough"]=true` |
| 认证头 | `buildUpstreamRequestAnthropicAPIKeyPassthrough` → `setAnthropicAPIKeyAuthHeader` | 默认 **`x-api-key`**（`anthropic_apikey_auth.go:19`），正是上游要求 |
| 目标 URL | `base_url + "/v1/messages?beta=true"` | 上游实测 **200**（`?beta=true` 被忽略，无害） |
| session 头 | 该路径**未**挂 `applyOpenCodeUpstreamHeaders` | 但末尾有 `account.ApplyHeaderOverrides()` → 用账号 `header_overrides` 注入即可 |
| 后台测试 | `account_test_service.go::testClaudeAccountConnection` | 同 URL 约定、同 `x-api-key`、**同样调用 `ApplyHeaderOverrides`** ⇒ 测试按钮结论对生产有效 |

**账号配置**

| 字段 | 值 |
|---|---|
| `platform` / `type` | `anthropic` / `apikey` |
| `credentials.base_url` | `https://opencode.ai/zen/go` ← **结尾不带 `/v1`**（代码自己拼 `/v1/messages`；带 `/v1` 会变 `/v1/v1/messages` 404） |
| `extra.anthropic_passthrough` | `true` |
| `credentials.header_override_enabled` | `true` |
| `credentials.header_overrides` | `x-opencode-session: <稳定值>`（**必需**）；可选 `user-agent: opencode/1.0.0` |
| 模型白名单 | 仅 25.3 的 14 个模型 |

**两个坑**：
1. 不要设 `extra.anthropic_api_key_auth_scheme=authorization_bearer`，会切回 Bearer ⇒ 401。
2. session 值用**稳定值**，不要随机/派生（它决定上游 prompt cache 路由，随机值持续 miss）。

## 25.6 证据：网关真实构造函数的端到端探针

在独立 worktree 中直接调用 `buildUpstreamRequestAnthropicAPIKeyPassthrough` 打真实上游：

```
最终 URL            : https://opencode.ai/zen/go/v1/messages?beta=true
出站头键（原始大小写）: [User-Agent X-Api-Key anthropic-version content-type x-opencode-session]
x-api-key 已注入     : true
Authorization 残留   : ""
x-opencode-session  : "sub2api-gateway-probe"
上游 HTTP 200 ... {"type":"message","role":"assistant","stop_reason":"end_turn",
                   "content":[{"type":"text","text":"Hi there! How can I help you today?"}],
                   "usage":{"input_tokens":36,"output_tokens":11,"cache_read_input_tokens":128}}
```

`x-opencode-session` 确实到达上游（缺它会 400，此处 200 即证明）。

**排查记录**：`http.Header.Get("x-opencode-session")` 读不到值是假象——未知头由
`headerOverrideWireCasing` 按小写原始键写入 map，而 `Header.Get` 只做 MIME 规范化查找。
首轮断言因此误判，改用大小写不敏感查找后才看到真实值（`User-Agent` 有 wire casing 映射所以正常）。
**wire 上没问题**（Go 按 map 原样写键名，HTTP/1.1 头名不区分大小写），但**测试断言很容易因此误判**。

## 25.7 生产落地与安全收口（账号 139）

`2026-09-23 21:18` 建立 **id=139「OpenCode Go (Anthropic)」**：`platform=anthropic`、`type=apikey`、
`base_url=https://opencode.ai/zen/go`、`extra.anthropic_passthrough=true`、header_overrides 含 session 与 UA。

**安全收口（已核）**：`schedulable=false`、`account_groups` 绑定数 **0**、不在可调度 anthropic 集合内、
Redis 无 `sched:acc:139*` 残留 ⇒ **零流量影响**。138 未被改动（仍为用量展示账号）。

**注意**：139 里的 key 是**已在对话中暴露的旧 key**。

## 25.8 遗留

1. **后台账号测试未跑通**：`/api/v1/admin/accounts/:id/test` 需 admin 鉴权；
   生产 `/opt/sub2api/.env` 的 `ADMIN_PASSWORD` 为空（仅首次初始化用过），
   库里虽有管理类 API key 但**不提取**。需管理员提供 token，或由管理员在 UI 点「测试」。
2. **必须轮换**：OpenCode Go API key（轮换后更新 139 与 138）、OpenCode Console cookie、Admin API Key。
3. 若要 139 承接流量：先轮换 key，再绑独立分组并置 `schedulable=true`，先观察质量。
4. 未做：Anthropic 路径下的 OpenCode Go **一等平台化**（含 `minimax/qwen` 协议分流与按模型自动选协议）。

## 25.9 账号测试的两处修正（2026-09-23 追加）

### (1) 截断的推理流不再被报成「测试完成」

`processClaudeStream` 旧实现只转发 `delta.text`（忽略 `thinking_delta`），
收到 `message_stop` 就无条件发 `test_complete success=true`；
`emitReasoningProbeVerdict` 在 `answer == ""` 时静默 return。
⇒ 推理模型在预算不足时稳定产生「只有 thinking、答案为空、stop_reason=max_tokens」，
界面上与「测试通过」完全同形，看到的只是「测试完成！」+ 空白。

修正：跟踪 `sawText/sawThinking/stop_reason`；首个 thinking 增量用 `status` 事件
播报进度（判决只解析 `content`，不会被推理文本里的数字污染）；无正文时显式说明
（区分 max_tokens 截断 / 仅 thinking / 无内容块）；空答案改输出 `probeNoAnswerVerdict()`。
截断**不报 error**（不是账号故障，报 error 会牵连账号健康判定）。

### (2) 非 Claude 模型不再被注入 Claude Code 身份提示词

`createTestPayload` 原先无条件把 `claudeCodeSystemPrompt`
（`You are Claude Code, Anthropic's official CLI for Claude.`）放进 system 块。
Anthropic 协议账号指向非 Claude 上游时（OpenCode Go 上的 `deepseek-v4.1-flash`），
等于向第三方模型伪造身份，测到的不是真实模型行为。

修正：`testPayloadSystemPrompt(modelID)` 按模型区分，仅 `claude*`（含 `vendor/claude-*`）
保留 Claude Code 身份（真实 Claude 上游与 `claude_code_validator` 依赖它），
其余改用 `accountTestNeutralSystemPrompt`，system 块数保持 1。

**测量诚实性（防止后人误读）**：
单样本曾显示「cc 变体 thinking 13,038 字符截断、neutral 变体 3,407 字符答对」，
看起来像该提示词导致过度推理；但用 `max_tokens=2048`、每组 5 次复测：

| system | 结果 | thinking 中位 |
|---|---|---|
| `cc`（原状） | 5/5 截断 | 3,233 |
| `neutral` | 4/5 截断 + 1/5 答对 | 6,756 |
| 无 system | 5/5 截断 | 6,584 |

⇒ **该提示词不是截断主因**，主因是这题把推理预算吃光（15 次里 14 次截断）。
所以本节改动只按「身份是否分对」取舍，**不声称**性能收益。
`neutral` 提示词文本与测量时一致，改字会作废该测量。

**同源待办**：测试报文还会带 Claude Code 客户端头（`claude.DefaultHeaders`）与
`anthropic-beta`。OpenCode Go 对此容忍（经网关实测 200），但语义上同属对外伪造身份，
若要一并规整需另开改动。转发链路不受本节影响：Anthropic passthrough 原样转发客户端 body，
不做 system 注入。

## 26. 彻底移除 Anthropic 协议的 system 提示词注入（2026-09-23）

### 26.1 注入为什么存在

`gateway_claude_oauth_body.go` 的原注释自陈：**为了规避「Anthropic 基于 system 参数内容
检测第三方应用」**。手段是三步加码：前置追加 → 把非 Claude Code 的 system 迁到
messages、只留 CC 标识 → 再注入 `claudeCodeSystemPromptExpansion` 把 system 块数做到 3。
这是一整套伪装，不是顺手加的字段。

### 26.2 为什么可以安全删掉（先查证据再动手）

| 事实 | 证据 |
|---|---|
| 只对 **OAuth** 账号生效 | `applyClaudeCodeOAuthMimicryToBody` 开头 `!account.IsOAuth() → return` |
| 生产**已关闭** | `settings.enable_claude_oauth_system_prompt_injection = false`，`claude_oauth_system_prompt_blocks` 亦为 `enabled:false` 空文本 |
| 生产**无适用账号** | `platform=anthropic` 共 4 个账号，**全部是 apikey**，无一 OAuth |
| 其它注入函数无生产调用者 | `injectClaudeCodePrompt` / `rewriteSystemForNonClaudeCode*` 仅测试引用 |

⇒ 删除属于**零行为变化**：等价于长期生效的「注入关闭」取值。

### 26.3 删了什么

- `gateway_claude_oauth_body.go`：16 个声明、约 480 行注入机器（注入/重写/blocks 解析/
  默认块/模板展开/校验），以及 `applyClaudeCodeOAuthMimicryToBody` 里的注入步骤。
  保留的只是非提示词部分：`metadata.user_id`、`session_id`、指纹与缓存控制归一。
  `normalizeOpts.stripSystemCacheControl` 固定为 `true`，与历史关闭态一致。
- `claudeCodeSystemPromptExpansion` 常量（真实 CC 提示词段落，已无引用）。
- settings 三件套 `enable_claude_oauth_system_prompt_injection` /
  `claude_oauth_system_prompt` / `claude_oauth_system_prompt_blocks`：常量、解析、
  更新、审计、视图、DTO、admin 更新与回显，以及前端控制面（SettingsGatewayTab 的
  整块 block 编辑器 + SettingsView 约 370 行逻辑 + i18n + api 类型）。
- 后端 19 文件 `+143/-1126`，前端 6 文件 `-743`。

### 26.4 锁住不变量的测试

原「注入可被关闭」测试改名为
`TestGatewayService_AnthropicOAuth_SystemPromptIsNeverInjected`，并加强断言：
客户端 system 必须原样到上游，且出站 body 内不得出现 `claudeCodeSystemPrompt`、
`Claude Code`、`interactive agent that helps users` 任一字符串。

### 26.5 唯一保留的注入：账号测试对 Claude 模型

`createTestPayload` 仍对 `claude*` 模型注入 `claudeCodeSystemPrompt`。理由：该测试
发出的确实是 Claude Code 形态的请求（同一套 CC 客户端头），对真实 Claude 上游而言
这是**一致的客户端身份**、且可能是 OAuth 令牌被接受的前提；对非 Claude 模型
（`deepseek-v4.1-flash` / `minimax-m3` / `kimi-k3` …）已改为中性提示词。
若要求「连 Claude 模型也不注入」，是把 `testPayloadSystemPrompt` 改成恒定返回中性
提示词的一行改动，但会让 Claude OAuth 账号的测试不再代表真实客户端。

### 26.6 顺带修掉的既有问题（均可在更早提交上复现，非本节引入）

1. `opencode_go_usage_refresh`：后台刷新 goroutine 无 panic 保护。撞到只实现部分方法的
   repo stub 时 `ListByPlatform` nil 解引用 panic，**把整个进程/测试包打崩** →
   加 `recover` 兜底（旁路任务不该有能力杀进程）。
2. `backend/internal/server/testdata/routes.golden`：补上此前遗漏的
   `GET /api/v1/admin/accounts/:id/opencode-go-usage`。
3. 前端既有失败 3 项（`UserPlatformQuotaModal` 平台数 7→6、`AccountsView` Grok 徽章）
   已用 `git stash` 对照确认与本改动无关。

### 26.7 发布要求

**后端与前端必须同批发布**：只发后端会让管理界面继续把「Claude OAuth System 注入」
显示为开启（默认 true）而代码已无注入，属于比死开关更差的骗人界面。本次均未部署。

### 26.8 补充：注入不止一处（第二处是文本替换）

第一轮删除后全仓复扫，发现第二处同类注入：

`sanitizeSystemText` 会把客户端自带的身份句
`You are OpenCode, the best coding agent on the planet.` **静默替换成**
`claudeCodeSystemPrompt` 横幅（注释自陈理由：Anthropic 可能把它当非 Claude Code 指纹）。
它挂在 `normalizeClaudeOAuthSystemBody → normalizeClaudeOAuthRequestBody`，
仍在 OAuth 伪装路径内执行。已删除该函数与两个调用点，
`normalizeClaudeOAuthSystemBody` 只保留形态归一（按需剥离 `system.cache_control`），
并把 `gateway_body_order_test` 的断言**反转**为「客户端身份句必须原样保留、
`claudeCodeSystemPrompt` 不得出现」。

至此 `gateway_claude_oauth_body.go` 对 `claudeCodeSystemPrompt` **零引用**。

### 26.9 复扫后仍存在的三类\"Claude Code\"字样（均非注入，说明理由）

1. `claude_code_validator.go` 的 `claudeCodeSystemPrompts`：**入站检测**用的模板表，
   判断客户端是否 Claude Code（用于客户端门禁/旁路），不向任何出站请求写入内容。
2. ~~`migrations/129_seed_claude_code_template.sql`~~ **（更正：此条原先的判断是错的）**
   该迁移写的是 `channel_monitor_request_templates`（频道监控的探测模板，属另一特性），
   与本次删除的三个 setting key **无关**。全量检索确认：**没有任何迁移 seed 过**
   `enable_claude_oauth_system_prompt_injection` / `claude_oauth_system_prompt` /
   `claude_oauth_system_prompt_blocks`，这三行是运行期经设置 API 写入的。
3. 账号测试对 `claude*` 模型的 CC 身份（见 26.5）：测试本身就以 Claude Code 形态发请求，
   属一致的客户端身份，非对第三方模型伪造。

### 26.10 生产数据清理（2026-09-23 执行）

删除前现状：

| key | 值 |
|---|---|
| `enable_claude_oauth_system_prompt_injection` | `false`（len=5） |
| `claude_oauth_system_prompt` | 空 |
| `claude_oauth_system_prompt_blocks` | `[{"enabled": false, "type": "text", "text": ""}]`（len=68） |

均写入于 2026-09-21 14:45，非迁移产物；`redis-cli --scan '*setting*'` 无缓存键。
已用单事务删除三行（`DELETE 3`，复核剩余 0，再无 `%claude_oauth%` 键）。

**⚠️ 顺序风险（必须记住）**：生产当时仍运行**旧二进制**，而旧代码在该 key 缺失时
回落到默认 `true`——也就是说「先删数据、后发新二进制」会让旧进程的注入开关变成**开启**。
本次实际影响为零（生产 4 个 anthropic 账号全是 apikey，OAuth 伪装路径不可达），
但**在部署不再读取该 key 的新二进制之前，不要新增 anthropic OAuth 账号**。
