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
- [ ] 4.4 真实核验（**待批准**）：单账号一次 `/v1/messages` 返回 200（签名错误会在推理前被拒）；观察 `forward_failed` 无上升
  - 待批准原因：需要要么把新二进制热替换到生产候选容器（有备份/回滚点），要么用一次性探针直连 relay；两者都会触碰生产或凭据，按项目纪律需你点头
- [ ] 4.5 后续独立项（不在本变更范围）：`mirasim_default_client_version` 仍是 0.0.322，官方客户端为 0.0.354；`x-mirasim-client` 在线上明文可见，是否跟进由你决定
