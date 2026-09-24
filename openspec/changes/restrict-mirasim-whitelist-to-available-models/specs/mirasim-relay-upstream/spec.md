## ADDED Requirements

### Requirement: Mirasim Available Model Sync

`FetchUpstreamSupportedModels` 对 mirasim relay 账号 SHALL 通过账号维度的只读接口
`GET /v1/models` 获取该账号当前可用的模型集合，并 MUST NOT 再使用 `GET /v1/model-roster`
的 agent 选择结果作为白名单来源（roster 是全局目录，与「本账号能用什么」无关）。

请求 SHALL 沿用只读元数据通道的约定：空请求体、明文 Ed25519 签名头
（`x-mirasim-device` / `x-mirasim-ts` / `x-mirasim-nonce` / `x-mirasim-sig`，以及非空时的
`x-mirasim-client`）与 `Authorization: Bearer <credential>`，MUST NOT 发送 `x-mirasim-enc`
密封头；凭据选择顺序不变（缓存 device ticket → 缓存 access token → `/v1/device/session`），
只要缓存凭据可用 MUST NOT 触发 refresh token 轮换；SHALL 使用不大于 10 秒的独立调用超时。

响应 SHALL 按 OpenAI 风格结构解析：根为 JSON 对象且 `data` 为数组；每个条目取 `id`，
`id` 必须为非空 JSON 字符串，否则丢弃该条目；`id` SHALL 做 `trim`、去重并按稳定顺序排序后返回。
没有任何有效 `id` 时 SHALL 视为失败，MUST NOT 返回空白名单。

显式同步（管理员点击「同步上游模型」）SHALL 绕过进程内缓存重新请求上游。

失败映射 SHALL 保持既有语义：账号与凭据配置缺失映射为 `configuration` 且 MUST NOT 发起上游
请求；网络、超时、非 2xx 与解析失败映射为 `upstream`。返回给客户端的错误消息 MUST NOT 包含
凭据、ticket 或上游响应正文。

#### Scenario: mirasim 账号同步账号维度可用模型

- **WHEN** 管理员对 mirasim 账号点击「同步上游模型」
- **THEN** 后端请求 `GET /v1/models` 并返回该账号当前可用的模型 id 列表（去重、稳定排序）

#### Scenario: 不再使用全局 roster 的 agent 选择结果

- **WHEN** 某账号的 `/v1/models` 不包含 `claude-opus-5-5`，而全局 roster 的 `claude` agent 包含它且被标记为 default
- **THEN** 同步结果 MUST NOT 包含 `claude-opus-5-5`，且 MUST NOT 发起 `/v1/model-roster` 请求

#### Scenario: 上游声明可用即真实可调用

- **WHEN** 以真实凭据运行 E2E，枚举 `/v1/models` 返回的每个 id 并各发一次极小推理请求
- **THEN** 每个 id 都返回 2xx 且响应含内容（声明可用与实际可调用一致）

#### Scenario: 空列表视为失败

- **WHEN** `/v1/models` 返回 `{"object":"list","data":[]}` 或所有条目都没有合法 `id`
- **THEN** 同步返回 `upstream` 类错误，MUST NOT 把白名单静默清空

#### Scenario: 凭据不可用

- **WHEN** mirasim 账号的 refresh token 缺失或 device key 无法构造
- **THEN** 同步返回 `configuration` 类错误，且不发起任何上游请求

#### Scenario: 显式同步绕过缓存

- **WHEN** 同一账号在缓存有效期内被连续两次显式同步
- **THEN** 两次都请求上游（不命中缓存），白名单反映最新一次响应

#### Scenario: 日志不含凭据

- **WHEN** `/v1/models` 拉取或解析失败
- **THEN** 产生的日志只包含账号 ID 与错误类别，不含 token / ticket / device key / 上游响应正文
