## 1. 实现：/v1/models 只读通道

- [x] 1.1 `mirasim_relay.go` 新增常量 `mirasimAvailableModelsPath = "/v1/models"` 与独立超时
  （复用 `mirasimRosterTimeout`/`mirasimLimitsTimeout` 之一，≤10s），新增
  `func (c *mirasimRelayClient) fetchAvailableModels(ctx) ([]byte, error)`：空 body、明文签名头、
  **不**带 `x-mirasim-enc` 密封头、凭据优先复用缓存 ticket/access token。
- [x] 1.2 新增 `mirasim_available_models.go`：`parseMirasimAvailableModels([]byte) ([]string, error)`
  按 `{object:"list", data:[{id}]}` 解析；`id` 必须为非空 JSON 字符串；`trim` 后去重 +
  `sort.Strings` 稳定排序；根不是对象 / `data` 非法 / 结果为空 → 返回错误。
- [x] 1.3 失败日志只允许 `account_id` + `category`（事件名 `mirasim_models_fetch_failed` /
  `mirasim_models_parse_failed`），**不得** `log err.Error()`（`mirasimHTTPError.Error()` 会带上游正文）。

## 2. 实现：元数据门面与调用方接线

- [x] 2.1 `mirasim_metadata.go`：接口新增 `AvailableModels(ctx, account, force) ([]string, error)`；
  实现按账号缓存（TTL 10 分钟 + 配置指纹失效），失败不写负缓存；`InvalidateMirasimMetadata`
  同时清除该账号的 available-models 缓存。
- [x] 2.2 `upstream_models.go`：mirasim 分支改调 `AvailableModels(ctx, account, true)`
  （显式同步绕过缓存）；错误映射保持：config error → `UpstreamModelSyncErrorConfiguration`，
  其余 → `UpstreamModelSyncErrorUpstream`（`Message` 仍为固定文案 + `category`，
  不含上游正文与凭据）。
- [x] 2.3 确认 `mirasimRelayAccountCandidate` 判定与「凭据缺失不发请求」语义不变。

## 3. 实现：移除 roster 白名单用途（design D2 方案 A1）

- [x] 3.1 删除 `mirasim_roster.go`（含 agent 选择、roster 校验/归一化）及其单测中对应用例，
  保留 `/v1/limits` 相关文件（`mirasim_limits.go`）不动。
- [x] 3.2 `mirasim_metadata.go` 移除 `ModelRoster` 方法与 roster 缓存条目；
  `mirasim_relay.go` 移除 `/v1/model-roster` 常量与 `fetchModelRoster`。
- [x] 3.3 更新既有单测：删除 `TestFetchUpstreamSupportedModelsUsesMirasimRoster` 等 roster 用例，
  改为 `/v1/models` 用例（含「agent 覆盖不再影响白名单」的负向断言）。
- [x] 3.4 标注被取代的变更：在 `add-mirasim-model-roster-and-limits/tasks.md` 与其 delta spec 的
  「Mirasim Upstream Model Sync」处加引用块，指向本变更（沿用 `add-mirasim-relay-upstream`
  tasks 4.4 的既有做法），避免两份 requirement 互相矛盾。

## 4. 单测（`//go:build unit`，`package service`）

- [x] 4.1 解析：正常列表 / 去重 / `trim` / 空 `data` / `data` 非数组 / 根为数组 / `id` 非字符串 /
  `id` 为空 → 各自断言（含「空列表必须报错」）。
- [x] 4.2 同步：mirasim 账号命中 `/v1/models`，`force=true`；`/v1/model-roster` **不再被调用**
  （负向断言路径计数为 0）；`/v1/models` 为空 → `upstream` 类错误。
- [x] 4.3 错误映射：凭据缺失 → `configuration` 且零上游请求；500 → `upstream_http_500`；
  解析失败 → `upstream`；断言 `syncErr.Error()` 不含上游正文哨兵。
- [x] 4.4 缓存：同一账号两次 `force=false` 只打一次上游；不同账号不串缓存；
  配置指纹变化（换 device key / client version）后缓存失效。
- [x] 4.5 日志脱敏：失败路径日志只含 `account_id` + `category`，不含 token / ticket / 上游正文
  （复用既有 `mirasimMetaLogCapture` 手法）。

## 5. E2E（`//go:build e2e`，需真实凭据，缺凭据即 skip）

- [x] 5.1 `GET /v1/models` 返回非空列表，且每个 id 通过 `POST /v1/messages`（`max_tokens` 极小）
  返回 2xx 且有内容 → 断言「声明可用 == 实际可调用」。
- [x] 5.2 反向断言：用 `parseMirasimModelRoster` 会被删除，改为断言**不再**采用 roster 的默认
  agent 集合（例如 `claude-opus-5-5` 不在 `/v1/models` 输出中时，白名单也不得包含它）。
- [x] 5.3 全程不触发 `/auth/refresh`（refresh token 用占位值并在结束时断言未被替换）。
- [x] 5.4 日志只打印模型 id 数量与 id 列表（模型 id 非敏感），不打印凭据。

## 6. 验证与发布

- [x] 6.1 `cd backend && go build ./...` 通过；隔离目录（`/tmp` 硬链接副本 + 移走陈旧
  `*_test.go`）内 `go vet -tags unit ./internal/service/` 与
  `go test -tags unit -run 'Mirasim' ./internal/service/` 全绿。
- [x] 6.2 前端：无代码改动；跑白名单相关既有回归（`ModelWhitelistSelector` / 账号页面 spec）
  确认无回归；`vue-tsc --noEmit` 通过。
- [x] 6.3 文档：更新 mirasim relay 文档中「模型清单来源」一节，写明 `/v1/models`（账号维度）
  与 `/v1/model-roster`（全局目录，已不再用于白名单）的区别与实测证据。
- [ ] 6.4 生产发布（后端热替换 + 回滚点）：
  `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed -ldflags='-s -w -X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380' -trimpath`，
  `docker stop` → 备份并替换 bind-mount 二进制 → `docker start`，记录 sha256 与备份路径。
- [ ] 6.5 生产核验：`POST /api/v1/admin/accounts/140/models/sync-upstream` 返回该账号的
  `/v1/models` 集合（预期不再包含那批 404 的 claude/gpt 模型），并记录实际返回的 id 列表。
- [ ] 6.6 生产日志核验：发布后 `docker logs` 落盘统计，确认无凭据/JWT/上游正文泄漏，
  mirasim 事件只含 `account_id` + `category`。
