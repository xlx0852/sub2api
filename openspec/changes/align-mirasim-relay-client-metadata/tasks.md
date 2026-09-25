# Tasks

## 1. 协议层（mirasim_protocol.go）

- [x] 1.1 `mirasimCanonicalString` 元数据行改为官方 crypto core 语义（`key:value` 行、`\n` 连接、键小写、丢空值、键排序；空集空行）
- [x] 1.2 `mirasimSealHeaders` 改为密封“全部被密封头”（map → 键排序），排除 `x-mirasim-client`/`x-mirasim-enc`
- [x] 1.3 保持只读端点与 device-session 的现有形态（空元数据、不密封）

## 2. 注入层（mirasim_relay.go）

- [x] 2.1 新增元数据构造：`x-mirasim-session`（会话稳定）、`x-mirasim-agent`（按最终上游模型/路径）、`x-mirasim-call`（每次尝试新 id）
- [x] 2.2 可选 `x-mirasim-locale`、`x-mirasim-collect: off`（账号级开关，默认不发送）；`x-mirasim-account` 保持不发送
- [x] 2.3 ticket 响应解析同时接受 `expiresIn`/`expiresAt` 与 `expires_in`
- [x] 2.4 日志只记录 agent 与字段存在性布尔，不记录 session/call 明文

## 3. 测试

- [x] 3.1 canonical 向量 + 签名向量（搬自官方 0.0.260 core）
- [x] 3.2 密封向量回归（确定性密文）
- [x] 3.3 注入后出站请求：解封含元数据、明文只余 client/enc、每次尝试 call 不同、session 稳定
- [x] 3.4 只读端点回归：无 `x-mirasim-enc`、无元数据头
- [x] 3.5 ticket 过期字段：camelCase 生效、缺失时回落默认 TTL

## 4. 验证

- [x] 4.1 `go build ./...`；隔离环境 `go vet -tags unit ./internal/service/` 与 `go test -tags unit ./internal/service/ -count=1`（含官方 0.0.260 签名/密封钉住向量）全绿
- [x] 4.2 `gofmt -l` 干净；`go test ./internal/service/ -count=1` 失败集合与 HEAD 基线一致（6 条 pre-existing）；`vue-tsc --noEmit` 干净
- [x] 4.3 `openspec validate align-mirasim-relay-client-metadata --strict`
- [x] 4.4 真实核验（已批准执行 A 方案）：新二进制热替换生产候选容器；账号 140 两次 relay 请求均 200 且有真实推理增量；`forward_failed`=0、上游 401/403=0；记录见 `deploy-2026-09-25-mirasim-client-metadata.md`
- [x] 4.5 客户端版本对齐：`mirasim_default_client_version` 0.0.322 → 0.0.354（账号级 `mirasim_client_version` 覆写仍优先）
- [x] 4.6 access token 刷新窗口对齐官方：15m lead + 30s stale（lead 窗口内 refresh 失败不失败请求；stale 窗口内必须刷新成功）
- [x] 4.7 device ticket 降级与退避：404 → 静默 1m、501 → 静默 15m（改用 access token 作为签名凭据）；其它失败 1s 起指数退避封顶 30s，退避期内未过期 ticket 继续可用
- [x] 4.8 `/v1/models` 目录收敛：保留位/namespaced 丢弃，dated 孪生在有 plain 时丢弃、无 plain 时保留
- [x] 4.9 已拆到独立 change `align-mirasim-thinking-forms-and-effort` 实现并发布：roster `adaptive` 驱动 thinking 形态；relay 模型 effort 白名单 + `ultra→max` + 不支持即 400
