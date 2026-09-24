# Tasks: Add Mirasim Model Roster And Quota Windows

> 状态：**全部完成**（1.x–6.x 与 7.1–7.7，2026-09-24/25）。生产发布已上线：后端热替换 binary
> `0.1.391-noinject-a33b47ef9/sub2api`（sha256 前缀 `34c3e9f2b8072233`），前端 release
> `0.1.391-mirasimmeta-199d284ac`（rollback → `0.1.391-mirasim-0608a3402`）。发布细节见文末「生产发布」。
>
> **已被取代（sync 部分）**：本变更中「上游模型同步走 `/v1/model-roster` + agent 选择」这条设计
> 经实机验证不成立——默认选中的 `claude` agent 6 个模型在该账号上真实调用全部 404，且 roster
> 缺少 `deepseek-v4-flash` / `deepseek-v4-flash-vision-exp` 两个可用 id。白名单来源已改由
> `openspec/changes/restrict-mirasim-whitelist-to-available-models` 接管（账号维度 `GET /v1/models`）。
> **`/v1/limits` 用量链路与前端展示不受影响**，仍以本变更为准。

## 1. 只读元数据客户端（`mirasim_relay.go`）

- [x] 1.1 回读客户端 `server.cjs` 复核 limits 超时常量（`tor`）与 roster 超时（`ddo=8000ms`），
      把两个值钉成包级常量并在注释里标注来源
- [x] 1.2 新增 `mirasimHeaderProbe = "x-mirasim-probe"` 与 `mirasimProbeUsage = "usage"` 常量
- [x] 1.3 新增 `mirasimRelayClient.fetchModelRoster(ctx)`：空 body、明文签名头、不密封、
      `Bearer <credential>`、独立超时；非 2xx 返回带状态码的错误
- [x] 1.4 新增 `mirasimRelayClient.fetchLimits(ctx)`：同上并加 `x-mirasim-probe: usage`
- [x] 1.5 新增 `mirasimRelayClient.metadataCredential(ctx)`：缓存 ticket → 缓存 access token →
      `ticket(ctx)`；不得在已有缓存时触发 refresh
- [x] 1.6 401/403 兜底：失效本地缓存、换凭据重试一次；非鉴权错误不抖动凭据
- [x] 1.7 按账号缓存：roster 10 分钟、limits 30 秒，附 `force` 绕过与缓存失效钩子
- [x] 1.8 单测：签名头集合（不含 `x-mirasim-enc`）、probe 头、超时、401 重试一次、
      非 401 不重试、缓存命中/绕过/不跨账号

## 2. Roster 解析（`mirasim_roster.go`）

- [x] 2.1 定义 `mirasimModelRoster` / `mirasimRosterAgent` 结构（agent id、label、
      contextWindow、default、models、cli）与可选顶层 `models`/`cli` 元数据
- [x] 2.2 实现校验/归一化（对齐 `znr`）：`agents` 对象、数组、`id` 非空字符串、
      `contextWindow > 0`、trim+lowercase、`default` 首个胜出、无有效条目 → 错误
- [x] 2.3 实现 agent 选择：`extra.mirasim_roster_agent` → `default` → `claude` → 字典序首个
- [x] 2.4 实现模型 id 提取与稳定排序去重
- [x] 2.5 单测：钉住归一化向量（trim/lowercase/label 回退/default 胜出/无效条目丢弃/
      整组无效报错/agent 选择优先级）

## 3. 上游模型同步接线（`upstream_models.go`）

- [x] 3.1 mirasim 账号在 `FetchUpstreamSupportedModels` 首分支改走 roster（保留
      `httpUpstream` 未配置与账号为空的既有 `configuration` 错误）
- [x] 3.2 删除 `buildAnthropicUpstreamModelsRequest` 中 mirasim 的 `unsupported` 分支
      （取代 `add-mirasim-relay-upstream/tasks.md` 4.4），并在该处留注释指向本 change
- [x] 3.3 错误映射：上游/解析失败 → `upstream`；凭据/配置缺失 → `configuration`；
      错误消息不含上游正文与凭据
- [x] 3.4 单测：成功同步、不再 `unsupported`、凭据缺失走 `configuration`、上游 500 走 `upstream`、
      响应体不回显

## 4. 额度窗口（`mirasim_limits.go` + `account_usage_service.go`）

- [x] 4.1 定义 `mirasimLimitsSnapshot`（windows/paid/degraded/status/capturedAt）
- [x] 4.2 实现窗口归一化：校验规则、unlimited、夹逼 [0,100]、`>= 99 → 100`、
      remaining、RFC3339 reset、status 阈值、顶层状态先非 `model_scoped` 窗口、
      为空时回退全部窗口
- [x] 4.3 实现窗口映射：`5h` → `five_hour`、`7d` → `seven_day`（仅 `budget > 0`），
      全部窗口进 `mirasim_windows`；`model_scoped` 不参与 `five_hour`/`seven_day` 映射
- [x] 4.4 `UsageInfo` 新增 `mirasim_windows`、`mirasim_paid`、`mirasim_degraded`
- [x] 4.5 `GetUsage` 增加 mirasim 分支（位置在 OpenAI/Kimi OAuth 之后、`CanGetUsage` 之前），
      注入只读元数据查询器；失败返回 200 + `error` 降级
- [x] 4.6 `wire.go`：新增 `MirasimMetadataQuerier` 接口与 `SetMirasimMetadataService(...)`
      注入（形态对标 Kimi 的 `SetKimiQuotaService`），并用 `ProvideAccountUsageService` 接线
- [x] 4.7 单测：窗口映射、unlimited、`>= 99` 夹逼、warning/limit_reached 阈值、
      `model_scoped` 隔离、无有效窗口、超时降级 200、降级不改调度状态、
      缓存命中/force 绕过/不跨账号

## 5. 前端展示（`frontend/`）

- [x] 5.1 `types/index.ts`：`UsageProgress` 不变，`AccountUsageInfo` 增加
      `mirasim_windows` / `mirasim_paid` / `mirasim_degraded`
- [x] 5.2 `utils/accountUsagePresentation.ts`：在 `anthropic` 分支后追加 mirasim 窗口渲染
      （复用 `makeQuotaWindow` 颜色分级），并透出 paid/degraded 提示
- [x] 5.3 i18n（zh/en）新增 `mirasim*` 文案键
- [x] 5.4 `utils/__tests__/accountUsagePresentation.spec.ts` 增补用例；
      `npx vitest run` 与 `npx vue-tsc --noEmit` 通过
- [x] 5.5 确认模型同步按钮无需改动（后端分支打开即可用），若需文案调整一并落地

## 6. 文档

- [x] 6.1 `docs/MIRASIM_RELAY_UPSTREAM.md` 新增「元数据接口（模型清单 / 额度窗口）」章节：
      端点、头、凭据选择、缓存 TTL、归一化规则、降级语义、非目标（`/usage/calls`、账本联动）
- [x] 6.2 在文档中记录实测结论（真实 roster agent 名、真实窗口名集合、limits 超时常量）

## 7. 验证

- [x] 7.1 `cd backend && go build ./...` 通过
- [x] 7.2 隔离式单测（沿用既定手法）：`cp -al backend /tmp/mrsverify/backend`，移走
      `internal/service/` 下非 mirasim 的 `*_test.go` 与 `account_repo_gemini_test_helpers.go`，
      再 `go test -tags unit ./internal/service/ -run 'Mirasim'` 通过
      （禁止为既有陈旧测试文件补 stub）
- [x] 7.3 联网只读 E2E（`//go:build e2e`）：用生产账号凭据文件拉 roster 与 limits，
      断言返回结构与窗口非空；**不得**触发 `/auth/code`、`/auth/verify`，
      **不得**触发 `/auth/refresh`（必须命中缓存 ticket）；若不慎轮换 refresh token，
      必须立即回写 DB 并记录
- [x] 7.4 真实管理端核验：账号 `id=140` 的「同步上游模型」返回真实模型列表；
      用量抽屉出现额度窗口（接口响应留档，凭据仅存在于 shell 变量，无回显）
- [x] 7.5 日志核验：`docker logs` 中 roster/limits 事件不含任何 token/ticket/device key 明文
      （并补 `TestMirasimMetadataFailureLogsStaySanitized` 钉死日志字段契约）
- [x] 7.6 `openspec validate add-mirasim-model-roster-and-limits --strict` 通过
- [x] 7.7 生产发布（前后端）与回滚点记录，按 `add-mirasim-relay-upstream` 第 8 章的既有流程执行

## 验证记录（2026-09-24）

| 条目 | 命令 / 手段 | 结果 |
| --- | --- | --- |
| 7.1 | `cd backend && go build ./...` | 通过（无输出） |
| 7.2 | `rm -rf /tmp/mrsverify && cp -al backend /tmp/mrsverify/backend`，移走 `internal/service/` 下非 mirasim 的 `*_test.go` 与 `account_repo_gemini_test_helpers.go`（共 522 个），再 `go vet -tags unit ./internal/service/` + `go test -tags unit -count=1 -run 'Mirasim' -v` | vet 无输出；**110 条 PASS 行（顶层 56）/ 0 FAIL**（补 7.5 日志契约用例后重跑） |
| 7.3 | `go test -tags e2e -count=1 -run 'TestMirasimRealRelayMetadataEndToEnd' -v`，凭据取本机客户端解封出的 0600 device key + access token 文件（与生产账号 140 **同账号**；为避开 refresh 轮换，未用生产 refresh token 手工探测） | 通过（2.0s）：`agents=4 order=[claude codex dsh kimi] default="claude" models=6`；窗口 `7d`、`7d_claude`，`paid=true degraded=false`；断言 refresh token 未被轮换 |
| 5.4 | `npx vitest run src/utils/__tests__/accountUsagePresentation.spec.ts` + `npx vue-tsc --noEmit` | 12/12 通过；tsc 退出码 0 |
| 6.1 / 6.2 | `docs/MIRASIM_RELAY_UPSTREAM.md` §12（含 §12.7 实测键结构、§12.8 复现步骤） | 已写 |
| 7.6 | `openspec validate add-mirasim-model-roster-and-limits --strict` | 通过 |
| 7.4 | 生产管理端接口（`x-api-key` 仅存在于 shell 变量，未落盘/未回显）：`POST /api/v1/admin/accounts/140/models/sync-upstream`、`GET /api/v1/admin/accounts/140/usage` | 均 HTTP 200。模型同步返回 6 个真实模型：`claude-fable-5`、`claude-fable-5-1`、`claude-opus-4-8`、`claude-opus-5`、`claude-opus-5-5`、`claude-sonnet-5`（此前该按钮走 `unsupported` 分支）；用量返回 `seven_day{utilization:0.1,resets_at:2026-10-01T08:49:16Z}`、`five_hour:null`（上游无 5h 窗口）、`mirasim_windows=[7d(budget 18640, used 10.59, remaining 99.9), 7d_claude(model_scoped)]`、`mirasim_paid:true`；`mirasim_degraded` 为 `omitempty` 故缺省＝false |
| 7.5 | `docker logs --tail 3000` 落盘后统计：`eyJhbGciOi` / `mrs1:` / `BEGIN PRIVATE KEY` / `x-mirasim-enc` 各 0 命中；mirasim 事件 0 条（成功路径不打日志，与实现一致）；唯一 mirasim 行为 `mirasim_relay_credential_updated`，字段仅 `account_id:140` 与 `key:"mirasim_refresh_token"`（**字段名，非值**） | 无凭据明文；另补 2 个单测（见下） |
| 7.5（单测） | `TestMirasimMetadataFailureLogsStaySanitized`（5 个子场景：roster 500/roster 401 重试后仍失败/roster 解析失败/limits 500/limits 解析失败）、`TestMirasimMetadataConfigFailureLogsNothing` | PASS：日志仅含 `account_id` + `category`（`upstream_http_500` / `upstream_http_401` / `upstream`），断言不含上游正文哨兵、`meta-access-token`、`meta-rotated-refresh`、device ticket、`BEGIN`/`PRIVATE KEY`、`"error":`/`"err":`/`"body":`/`"credential":`/`"token":`；配置错误 0 日志 0 请求 |
| 7.7 | 见文末「生产发布」 | 前后端均已上线；前端已逐文件内容等价复现（`ecf8583ac3ff6cfc`），后端以字符串 + 构建溯源 + 线上行为核验 |

## 生产发布（7.7，2026-09-25 00:44 主机时区 / 2026-09-24 23:44 +0800）

流程沿用 `add-mirasim-relay-upstream` 第 8 章：**前端先、后端后**，后端为同容器热替换
（`docker stop` → 覆盖 bind-mount 路径下的 `sub2api` → `docker start`）。

### 前端（nginx `code-sicts-shop.conf` 的 SPA 根）

| 项 | 值 |
| --- | --- |
| 新 release | `/opt/sub2api/frontend/releases/0.1.391-mirasimmeta-199d284ac` |
| `current` 软链 | → 上述新 release（已重指） |
| `rollback` 软链 | → `/opt/sub2api/frontend/releases/0.1.391-mirasim-0608a3402`（发布前版本） |
| 打包 | `COPYFILE_DISABLE=1 tar --no-mac-metadata --no-xattrs`，解包后 `find -type f \| wc -l` = **172**（含 assets 162），`find -name '._*' \| wc -l` = **0** |
| 内容核对 | ①**逐文件内容等价已证**：用当前 HEAD 源码重跑 `vite build`，对两端 `find -type f -exec sha256sum {} \; \| sort \| sha256sum` 均为 `ecf8583ac3ff6cfc`（172 文件），入口 chunk 均为 `assets/index-iYqvFu5b.js`；②`mirasim_windows` / `mirasim_degraded` / `mirasim_paid` 各命中 1 个 chunk；i18n 键 `mirasimUnlimited` / `mirasimModelScoped` / `mirasimDegradedHint` / `mirasimPlan` 各命中 3 个 chunk |

### 后端（同容器热替换，容器名带历史 tag，实际代码取 bind-mount 二进制）

| 项 | 值 |
| --- | --- |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`（`State.StartedAt=2026-09-24T15:44:46Z`） |
| 挂载 | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api -> /app/sub2api (rw)` |
| 新二进制 | 同目录 `sub2api`（123789474 B，`sha256` 前缀 `34c3e9f2b8072233`，mtime 2026-09-25 00:44:46） |
| 上传暂存 | 同目录 `sub2api.new`（与线上字节数一致，发布后保留） |
| 回滚点 | 同目录 `sub2api.bak-20260925-mirasimmeta`（123715746 B，`sha256` 前缀 `924e4c5e165c9ad9`，即发布前的 mirasim 元数据构建）；更早备份 `sub2api.bak-20260924-mirasim` 仍在 |
| 内容核对 | 线上二进制含 `/v1/model-roster`、`/v1/limits`、`x-mirasim-probe`、`mirasim_roster_fetch_failed`、`mirasim_limits_parse_failed` 各 1 处 |
| 构建溯源 | `go version -m sub2api` → `go1.26.4`、`-tags=embed`、`-trimpath=true`、`CGO_ENABLED=0`、`GOOS=linux`/`GOARCH=amd64`、`vcs.revision=d5d1be536`（发布时的 HEAD）、`vcs.modified=true`（含本轮未提交改动，与事实一致）；本轮全部**非测试**源码 mtime 为 `23:08–23:28`，均早于发布时刻 `23:44:46`，发布后仅改过 `mirasim_metadata_test.go`（不参与产物） |
| 二进制等价性 | **未做逐字节复现**：提交后 `vcs.revision`/`vcs.modified` 戳必然变化，二进制不可复现；以后端行为证据替代：线上 `/sync-upstream` 返回 roster 派生模型、`/usage` 返回 `mirasim_windows`，均只能由本 change 代码产生 |

### 上线后观测（与 7.4 同一窗口）

- 首次只读调用触发一次冷启动 refresh（`mirasim_relay_credential_updated`，`account_id=140`，
  字段名 `mirasim_refresh_token`），随后 roster/limits 复用同一运行时 client 的 device ticket：
  两次调用（同步模型 + 拉用量）只出现 **1 条**该事件 → 未出现每调用一次轮换。
- 元数据成功路径不打日志（事件日志只在失败时写），与 7.5 的单测契约一致。

### 回滚操作

1. 后端：`docker stop <容器>` → `cp -a sub2api.bak-20260925-mirasimmeta sub2api` → `docker start <容器>`。
2. 前端：`ln -sfn /opt/sub2api/frontend/releases/0.1.391-mirasim-0608a3402 /opt/sub2api/frontend/current`。
3. Redis `sched:acc:140` 快照在直接改 SQL 后必须删除（本次未直接改账号 SQL，仅运行时回写 refresh token）。
