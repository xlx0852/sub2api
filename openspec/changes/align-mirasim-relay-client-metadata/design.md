# Design: mirasim relay 客户端元数据对齐

## D1 为什么 canonical 必须改

`mrs-sig-v2` 的第 9 行是“元数据摘要”。官方 0.0.260 crypto core 的钉住向量（对方仓库
`internal/mirasim/protocol_test.go`）给出了确定答案，我们用 Python 独立复算：

| 编码 | digest | 与向量一致 |
|---|---|---|
| `key:value` 行，`\n` 连接，键排序 | `91bcb885…33be6b` | ✅ |
| 我们当前 `k\0v\0` 拼接 | `2227af34…ef0077` | ❌ |

空集合两侧都是「空行」，所以只发空元数据时两种算法不可区分 —— 这正是问题被隐藏的原因。

## D2 密封明文改为“全部被密封头”

官方把**所有** `x-mirasim-*`（除 `x-mirasim-client`、`x-mirasim-enc`）放进
`x-mirasim-enc`，因此元数据头不出现在明文线上。我们用 map（Go 会自动排序键）序列化，
无元数据时自然退化为原来的 4 字段形态（排序后 device/nonce/sig/ts）。

对应向量：`TestSealV1MatchesMirasimCryptoCoreVector` 的确定性密文可在实现后作为回归。

## D3 session 的稳定性来源

官方来源有二：宿主给出的 `execution_session_id`，或每个凭据一次性的随机 session。
我们当前没有把粘性会话 hash 传进上游构造器，因此：

1. 优先用请求体 `metadata.user_id`（客户端会话级稳定标识）派生：
   `mirasim_ + sha256hex(accountID + "\0" + seed)[:32]`（与官方派生式一致）；
2. 没有该字段时退回“每个账号客户端一份稳定随机 session”（官方 fallback）。

这样多轮请求共享同一 relay session，且同一账号不会每次换 session。
把粘性会话 hash 贯通进构造器是后续可做的增强，不在本变更范围内。

## D4 agent 派生必须用“最终上游模型”

`agent` 决定 relay 把请求交给哪个上游 agent（`dsh`/`zcode`/`kimi`/`claude`/`codex`）。
它必须基于**映射之后**的模型名（例如 `deepseek-v4.1-flash → deepseek-flash` 后取 `dsh`），
因此在写 body 之后的注入点计算。

## D5 只读端点与 mint 不动

官方 README 明确：`/v1/models`、`/v1/limits`、`/v1/model-roster` 用空元数据签名、不密封；
`/v1/device/session` 用 access token 签名、只发明文头。我们当前已与之一致，本次不改，
并在 spec 里写成约束，避免以后“顺手”加元数据导致端点形态漂移。

## D6 风险与验证顺序

签名原文改动会直接影响鉴权，验证顺序必须是：

1. 单测：canonical 向量 + 签名向量 + 密封向量（来自官方 core）；
2. 单测：注入后的出站请求解封后字段齐全、`x-mirasim-enc` 之外没有泄漏明文头；
3. 隔离构建 + `-tags unit` 包测试；
4. 真实核验：单个账号一次 `/v1/messages`，确认 200（签名错误会在推理前被拒，几乎不消耗额度）；
5. 观察窗口内 `forward_failed` 与 401/403 不上升。

回滚面：本变更只影响出站请求头，回滚即恢复旧实现；不需要数据迁移。
