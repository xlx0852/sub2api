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
