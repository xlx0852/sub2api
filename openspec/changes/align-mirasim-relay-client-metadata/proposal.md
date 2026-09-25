# Change: 对齐 mirasim relay 的客户端元数据（session / agent / call）

## Why

对照官方客户端反推实现 `KIDA-MNESIA/cpa-plugin-mirasim`（README + 代码，MIT）后确认两处偏差：

1. **元数据 canonical 编码与我们不一致。** 我们用 `k\0v\0` 拼接后整体 sha256；官方 0.0.260 crypto core 用
   `key:value` 逐行、`\n` 连接、键排序、丢空值。用对方钉住向量实测：

   ```
   metadata = {x-mirasim-agent:claude, x-mirasim-call:1111…, x-mirasim-session:mirasim_0000…}
   官方算法 digest = 91bcb885e5b045a9f55f270bb0c6d633930407b6cca792839034702ed233be6b  ✅ 与向量一致
   我们算法 digest = 2227af34417f5293703bfee445b3d2966d32f53024cf7f3e9ac28d793def0077  ❌
   ```

   今天没暴露，只是因为我们**永远发空元数据**（空集在两种算法下都是空行）。

2. **推理请求缺少官方客户端必带的三个元数据头。** 官方客户端每次推理都带
   `x-mirasim-session`（稳定会话）、`x-mirasim-agent`（按模型/路径选上游 agent）、
   `x-mirasim-call`（每次尝试一个新 id），且三者都**密封在 `x-mirasim-enc` 内**并参与签名摘要。
   我们只密封 4 个签名头，完全没有这三个。

后果：relay 侧看到的是“没有会话、没有 agent、没有 call”的请求，与官方客户端形态可检测地不同；
这与本会话前面反复出现的「mirasim 缓存命中不稳定 / 风控差异」方向一致（**推断，未验证**）。

## What Changes

- **元数据 canonical 编码改为官方 crypto core 语义**（`key:value` 行、`\n` 连接、键小写、丢空值、键排序；
  空集 → 空行），并把官方钉住向量搬进我们的单测作为回归基准。
- **推理请求携带 `x-mirasim-session` / `x-mirasim-agent` / `x-mirasim-call`**：
  - 三者写入密封明文（与签名头同一个 blob），并计入签名元数据摘要；
  - `agent` 由最终上游模型与路径决定（`deepseek-*→dsh`、`glm-*→zcode`、`kimi-*→kimi`，
    `/v1/responses`、`/v1/alpha/search`、`/v1/images/*` → `codex`，其余 `/v1/messages` → `claude`）；
  - `call` 每次上游尝试都新生成；`session` 在同一会话内保持稳定。
- **密封明文改为“所有被密封头”的 JSON 对象**（键排序，含签名头与元数据头），排除
  `x-mirasim-client` 与 `x-mirasim-enc`；无元数据时退化为原来的 4 字段形态。
- **只读元数据端点与 device-session 申领保持不变**：`/v1/models`、`/v1/limits`、
  `/v1/model-roster` 仍为「空元数据 + 不密封」，`/v1/device/session` 仍只发明文签名头。
- **可选账号级开关**：`mirasim_locale`（写入 `x-mirasim-locale`）、`mirasim_collect=false`
  （写入 `x-mirasim-collect: off`）。默认都不发送，与官方客户端默认一致。
  `x-mirasim-account` 仅当 access token 自身声明子账号时才发送；我们的账号没有子账号，保持不发。
- **ticket 响应字段容错**：同时接受 `expiresIn`/`expiresAt`（camelCase，官方）与 `expires_in`（snake_case）。

## Impact

- Affected spec: `mirasim-relay-upstream`
- 代码：`internal/service/mirasim_protocol.go`（canonical + seal）、`mirasim_relay.go`（元数据构造与注入）、
  `internal/service/mirasim_protocol_test.go` / `mirasim_relay_test.go`（钉住向量与回归）
- 不改数据库、不改路由、不改前端；不改变只读端点的请求形态
- 风险：签名原文变化 → 若实现有误，relay 会以鉴权/签名错误拒绝。缓解：先用官方钉住向量做单测，
  再在单个账号上做一次真实请求核验（签名错误在上游推理之前就会被拒，几乎不消耗额度）
- 日志：只记录元数据**字段是否存在**与 agent 取值，绝不记录 session/call 明文或任何凭据
