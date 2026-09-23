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
