# 生产部署记录：Anthropic 身份伪装彻底移除（后端 + 前端同批）

- 时间：2026-09-23 22:13–22:16（Asia/Shanghai）
- 发布标签：`0.1.391-noinject-a33b47ef9`
- 目标主机：`sub2api-prod`（74.82.195.84），站点 `https://code.sicts.shop`
- 方式：nginx 蓝绿（新建绿灯容器 → 校验 → 切 upstream → 旧容器保留作秒级回滚）

## 1. 发布内容

| 项 | 值 |
| --- | --- |
| 后端提交范围 | `ccef37122`（线上旧版）→ `a33b47ef9`（本次），共 11 个提交 |
| 前端提交范围 | `71fe371a8`（线上旧版）→ `a33b47ef9`，`frontend/` 纯删除 743 行 |
| 数据库迁移 | 无（`ccef37122..a33b47ef9` 未触及 `backend/migrations`） |
| 语义 | 移除 Claude/Anthropic 协议出站的系统提示词注入（后端 + 前端设置项）；含账号测试「空响应却报完成」修复与按模型分身份的测试提示词 |

两个旧版提交 `71fe371a8`（前端）与 `ccef37122`（后端）均为 `a33b47ef9` 的祖先，故从 `a33b47ef9` 构建不会回退线上既有功能（OpenCode Go 用量能力等）。

## 2. 制品与构建

- 构建环境：独立 worktree `/tmp/s2a-deploy-a33b47ef9`（detached @ `a33b47ef9`），无脏文件。
- 前端：`pnpm run build`（pnpm 11 需 `--dangerously-allow-all-builds`），`vite.config.ts:92` 的 `outDir` 直写 `backend/internal/web/dist`（`emptyOutDir: true`），172 文件 / 27M。
- 后端：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed -ldflags="-s -w -X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380" -trimpath -o bin/server ./cmd/server`，118M。

### 版本验证手段（重要）

本仓生产二进制**不含 `vcs.revision`**（buildinfo 只有 `path` / `build -trimpath=true`），AGENTS.md 中「用 `go version -m` 读 `vcs.revision` 验版本」的做法在本仓**不适用**。本轮改用两项可复现证据：

1. 产物 / 容器内二进制 **sha256 比对**；
2. 二进制与前端 bundle 的**关键字符串出现次数比对**。

| 证据 | 值 |
| --- | --- |
| 后端 sha256（本地 = 上传后远端 = 容器内 `/app/sub2api`） | `1d78d046980cbf4dd7c70e8fe5f0466d0f733c2eb33e691499084d5e900b1800` |
| `claude_oauth_system_prompt` 在二进制中的次数 | 旧线上 8 次 → 新 **0 次** |
| `enable_claude_oauth_system_prompt_injection` | 旧线上 4 次 → 新 **0 次** |
| 前端 release 中 `claude_oauth_system_prompt` / `enable_..._injection` 命中文件数 | 旧 `0.1.391-ocgcell-71fe371a8` 各 1 个文件 → 新 **0** |
| 公网首页引用的 `assets/index-*.js` / `*.css` hash | 与本地干净构建一致（`index-eM_iNuDO.js`、`index-Co4ONE0Z.css`） |

## 3. 蓝绿参数

| 项 | 蓝（旧，保留回滚） | 绿（新，承载流量） |
| --- | --- | --- |
| 容器名 | `sub2api-api-candidate-0.1.391-ocprotocol-ccef37122` | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9` |
| 宿主端口 | 127.0.0.1:18372 | 127.0.0.1:18373 |
| 二进制挂载 | `/opt/sub2api/api/releases/0.1.391-ocprotocol-ccef37122/sub2api` | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api` |

其余完全一致：镜像 `sub2api:codex-ws-20260601044722`（`sha256:fa0ad04e7f77...`）、网络 `sub2api_sub2api-network`、`--restart unless-stopped`、entrypoint `/app/docker-entrypoint.sh`、cmd `/app/sub2api`、workdir `/app`，共享只读挂载 `0.1.383-external/{resources,resources/model-catalog,release-data}` 与可写 `/opt/sub2api/data`。

- **env 保真做法**：`docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' <蓝灯> > /root/.green-<ver>.env`（`chmod 600`，**不打印**），`docker create --env-file` 注入。蓝绿 `.Config.Env` 逐行 md5 一致（116 条，`20c86c03e2e4bae1eea6d7fa2dd0d4f6`）。
- 该 env 文件在建容器后立即删除（容器自身已固化 env，重启不需要它）。
- 前端：新 release 目录 `/opt/sub2api/frontend/releases/0.1.391-noinject-a33b47ef9`（347 文件 / 22M），`current` 重指该目录，`rollback` 指向旧 `0.1.391-ocgcell-71fe371a8`。

## 4. 部署后验证（实测）

| 验证项 | 结果 |
| --- | --- |
| `docker start` | exit=0，`status=running`，`restarts=0` |
| 健康检查 | `starting` → **healthy，约 12s** |
| 容器内二进制 sha256 | 与本地构建一致 |
| `/health` 直连 18373 | `{"status":"ok"}` |
| `/api/v1/public/version` 直连 | 200 `{"version":"0.1.391","min_frontend_version":"0.1.380"}` |
| nginx `-t` | `syntax is ok` / `test is successful` |
| 切流后公网 `/api/v1/public/version` | 200；日志确认落在**绿灯**（绿灯 3 次 / 蓝灯 0 次） |
| 公网首页 | 200，资源 hash 与新构建一致 |
| 真实业务流量（切流后 5 分钟） | 26×`status_code:200`、6×SSE(`status_code:0`)、1×404、**0× 5xx**；路径含 `/responses`、`/api/v1/auth/me`、`/v1/user/balance`、`/models` |
| 启动期 ERROR 数 | 绿灯 1 条 = 既有 `[SubscriptionAutoRenew] ACCOUNT_NOT_FOUND`（数据条件，与蓝灯同类，非本次回归） |
| 旧容器 | 保持 `running` + `healthy`，未停（秒级回滚能力保留） |

注：`/api/v1/public/version` 只返回版本号、不含 release 标签，新旧一致，**不能用来区分版本**；版本判定依赖上文 sha256 与字符串证据。

## 5. 回滚

```bash
# 后端：把 upstream 指回蓝灯
sed -i 's/server 127.0.0.1:18373;/server 127.0.0.1:18372;/' /etc/nginx/conf.d/code-sicts-shop.conf
nginx -t && nginx -s reload
# 前端：symlink 回到 rollback
ln -sfn "$(readlink -f /opt/sub2api/frontend/rollback)" /opt/sub2api/frontend/current
```

- nginx 配置备份：`/root/code-sicts-shop.conf.bak-noinject-20260923-221515`
- 绿灯容器如需重建：用 `docker inspect` 重新导出同批容器的 env 到 600 权限文件，再按第 3 节参数 `docker create`。

## 6. 遗留与风险

1. **账号 139（OpenCode Go / Anthropic）仍在生产分组 34 且 `schedulable=t`**，其 API key 曾在对话中暴露；另有 1 个定时测试计划会周期性用它调用 OpenCode。经统计其**历史使用量为 0**（`usage_logs` 全表 0 条）、蓝灯近 3 小时 `selected_account_id=139` 出现 0 次，即**真实用户流量从未被转发到该账号**。仍建议：把 139 移出分组 34（或置 `schedulable=f`）并轮换其 key。
2. `/root/recreate_candidate.sh`（2026-09-11，775 权限）**明文包含** `JWT_SECRET`、`POSTGRES_PASSWORD`、`TOTP_ENCRYPTION_KEY`、OSS 访问密钥等，建议删除或改 600 并轮换其中密钥。
3. 待轮换凭据：OpenCode Go API key、OpenCode Console cookie、Admin API Key、`~/.pi/agent/auth.json` 中的 4 把 key。
4. 未处理的历史问题：账号 133 需重新 OAuth；账号 89 余额耗尽。
5. 旧制品保留：api release 目录 `0.1.391-ocprotocol-ccef37122`（线上回滚）、`0.1.391-ocsession2-d2f2b9bf9`、`0.1.391-ocsession-5caa42c49`、`0.1.391-ocgcell-71fe371a8`、`0.1.391-opencode-5051b7460`；前端 release 除本次外保留 `0.1.391-ocgcell-71fe371a8`（rollback）与 `0.1.391-opencode-5051b7460`。
