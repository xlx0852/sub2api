# 生产发布记录：guard-mirasim-account-model-mapping

- 日期：2026-09-25（+08:00）
- 代码：`19d1d927c feat(mirasim): guard empty account model mapping`
- change：`openspec/changes/guard-mirasim-account-model-mapping/`
- 生产机：`sub2api-prod`（74.82.195.84），域名 `code.sicts.shop`

## 1. 发布顺序与产物

前端先、后端后；后端为同容器热替换（bind-mount 二进制）。

### 1.1 前端

| 项 | 值 |
| --- | --- |
| 构建 | `frontend/`：`vue-tsc --noEmit` exit 0；`vite build` → 172 个文件 |
| 打包 | `COPYFILE_DISABLE=1 tar --no-mac-metadata --no-xattrs` → tgz `sha256=af6398606354b86d…`（17,379,218 B，`._*` = 0） |
| 发布目录 | `/opt/sub2api/frontend/releases/0.1.391-mappingguard-19d1d927c/`（172 文件） |
| `current` | → 上述目录（发布前 `0.1.391-mirasimfix-6a273dc65`） |
| `rollback` | → `0.1.391-mirasimfix-6a273dc65` |
| `index.html` | `sha256=4dd68b80b0bd4b27…`，本地 `curl -H 'Host: code.sicts.shop' /` 返回 200 且 sha 一致 |
| 内容自检 | 线上 `assets/` 命中 `edit-mirasim-apply-mapping`（`AccountsView-CEDaavE0.js`）与 `applyMapping` 文案 |

### 1.2 后端

| 项 | 值 |
| --- | --- |
| 构建 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380" -trimpath -tags embed ./cmd/server` |
| 二进制 | `sha256=8797e9694c5c9aa3d248e8d37baab10f97f2b173149f2e1742a660b1a960e907`（123,797,666 B） |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`（`127.0.0.1:18373`，nginx upstream `sub2api_api`） |
| 挂载点 | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api → /app/sub2api` |
| 热替换 | `docker stop` → `install -m 0755` 覆盖 → `docker start`；健康检查 2s 内 `{"status":"ok"}` |
| 发布前版本 | `vcs.revision=fc42ac50c`（前一次 mirasimmodels 发布） |
| 发布后版本 | `go version -m` → `vcs.revision=19d1d927c` |
| 备份 | `sub2api.bak-20260925-mappingguard`，`sha256=d9130d3a19a0ce88…`（= 发布前 live 二进制，回滚即 `install` 覆盖回去 + 重启） |

## 2. 发布后核验（只读）

### 2.1 派生标记与接口

| 核验 | 结果 |
| --- | --- |
| `GET /api/v1/admin/accounts/140` | 200；JSON 含 `mirasim_model_mapping_missing`，值 **false**（映射已就位） |
| 账号 140 凭据 | `credentials.keys = [base_url, model_mapping]`，映射 5 键：`deepseek-flash`、`deepseek-v4-flash`、`deepseek-v4-flash-vision-exp`、`glm-5.3-flash`、`kimi-k3`；`has_mirasim_refresh_token=true` |
| 响应脱敏 | `eyJ` 0 次、`mrs1:` 0 次、`sk-` 0 次（仅出现字段名 `has_mirasim_refresh_token`） |
| `GET /api/v1/admin/accounts?page=1&page_size=50` | 200；12 个账号，12 次出现该键（每账号一次），**无** `true`（线上唯一 relay 账号映射已完整） |
| `POST /api/v1/admin/accounts/140/models/apply-mapping`（body `{}`） | 200，`changed=false`，`models` 5 个且与库内映射**逐键相同** → 幂等，未写库（`updated_at` 保持 `2026-09-25 01:48:23+08`） |
| 端点响应脱敏 | `eyJ`/`mrs1:`/`sk-` 均 0 次 |

> 说明：线上 140 的空映射此前已由外部操作补齐（见
> `restrict-mirasim-whitelist-to-available-models/tasks.md` 第 7 节），
> 因此本次核验的是「幂等 + 不破坏现有映射」，而非「从空填满」。
> 正向用例（`changed=true` 写入）由单测覆盖。

### 2.2 日志（窗口：`--since 2026-09-25T01:47:41+08:00`，即热替换时刻）

| 项 | 结果 |
| --- | --- |
| 日志行数 | 799（约 4 分钟） |
| `gateway.forward_failed` | **0**（账号 140 亦为 0） |
| 凭据泄漏 | `eyJ` 0、`mrs1:` 0、`sk-` 0；`mirasim_refresh_token` 仅作为**字段名**出现（`mirasim_relay_credential_updated` 的 `key` 字段） |
| 唯一 ERROR | 1 条 `[SubscriptionAutoRenew] ACCOUNT_NOT_FOUND`（既有噪声，与本变更无关；另有 Grok 额度 402 的账号测试错误） |

发布前同一账号的最后一次失败为 `2026-09-25T01:41:08+08`（`kimi-k3` 上游 400
`The request was rejected as invalid.`，映射本身正确、属上游拒单）；
更早的 6 条为 01:14 的 `deepseek-v4.1-flash` 422（事故本体，映射修复前）。

### 2.3 冷启动副作用

容器重启后首次上游调用触发 `mirasim_relay_credential_updated`
（`/auth/refresh` 轮换 refresh token 并落库），属已知且可接受的行为，仅记录
`account_id` + 字段名，无凭据明文。

## 3. 回滚

1. 前端：`ln -sfn /opt/sub2api/frontend/releases/0.1.391-mirasimfix-6a273dc65 /opt/sub2api/frontend/current`（新目录可保留，无副作用）。
2. 后端：`docker stop sub2api-api-candidate-0.1.391-noinject-a33b47ef9` →
   `install -m 0755 /opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api.bak-20260925-mappingguard /opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api` →
   `docker start`。
3. 回滚后确认 `go version -m` → `vcs.revision=fc42ac50c`。

## 4. 遗留

- 线上 `dist` 里的 `sub2api-build.json` 为 `{"frontend_version":"0.1.391","min_backend_version":"0.1.320"}`，与新后端兼容。
- 本次未推送远端（该分支 12 个本地提交均未 push，沿用既有节奏）。
