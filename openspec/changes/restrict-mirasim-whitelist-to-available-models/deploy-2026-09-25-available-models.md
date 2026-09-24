# 生产发布记录：白名单改用账号维度 `/v1/models`（2026-09-25）

- 时间：2026-09-25 00:44–00:52（Asia/Shanghai）
- 提交：`fc42ac50c`
- 目标：`sub2api-prod`（74.82.195.84），站点 `https://code.sicts.shop`
- 范围：**仅后端**（前端零改动，见下）

## 1. 前端：无需发布（已证明字节一致）

`CreateAccountModal.vue` 只改了一行**注释**。重新构建后与线上 release
`0.1.391-mirasimfix-6a273dc65` 做全量 sha256 比对：

| 项 | 结果 |
| --- | --- |
| 本地 `backend/internal/web/dist` 文件数 / 线上 release 文件数 | 172 / 172 |
| sha256 不一致的文件数 | **0** |

因此未重新发布前端，`current` / `rollback` 软链保持不动。

## 2. 后端构建与热替换

- 构建：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed -ldflags='-s -w -X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380' -trimpath -o bin/sub2api ./cmd/server`
- 容器（未变）：`sub2api-api-candidate-0.1.391-noinject-a33b47ef9`
- bind-mount 二进制：`/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api`
- 方式：`docker stop` → `cp -a` 备份 → 覆盖二进制 → `docker start`

| 项 | 值 |
| --- | --- |
| 新二进制 sha256（本地 = 远端 = 容器内） | `d9130d3a19a0ce8807eaab46ee21e601`（123777186 B） |
| 回滚点 | `sub2api.bak-20260925-mirasimmodels`（sha256 前缀 `0edd4e64c2d9c45d`） |
| 更早备份 | `sub2api.bak-20260925-mirasimfix`（`0edd4e64c2d9c45d`）、`sub2api.bak-20260924-mirasim` |

二进制内容核对（`strings | grep -c`）：

| 标记 | 次数 |
| --- | --- |
| `/v1/models` | 8 |
| `mirasim_models_fetch_failed` | 1 |
| `/v1/model-roster` | **0**（全局 roster 路径已从实现中移除） |

## 3. 部署后核验（实测）

| 项 | 结果 |
| --- | --- |
| `/health`（127.0.0.1:18373） | `{"status":"ok"}` |
| `docker inspect` | `status=running`，`restarts=0` |
| 容器内二进制 sha256 | 与本地一致（`d9130d3a…`） |
| `POST /api/v1/admin/accounts/140/models/sync-upstream` | HTTP 200，`code=0`，**返回 5 个模型** |

账号 140 实际返回（= 140 的账号维度可用集合）：

```
deepseek-flash
deepseek-v4-flash
deepseek-v4-flash-vision-exp
glm-5.3-flash
kimi-k3
```

结论：**账号 140 的可用集合与桌面客户端凭据实测集合一致**（同为这 5 个），
不再出现此前那 6 个「同步进白名单但一调就 404」的 claude 模型。

## 4. 日志脱敏核验

`docker logs --tail 3000` 落盘后统计（只打匹配次数）：

| 模式 | 命中 |
| --- | --- |
| `eyJhbGciOi` | 0 |
| `mrs1:` | 0 |
| `BEGIN PRIVATE KEY` | 0 |
| `x-mirasim-enc` | 0 |
| `mirasim_models_fetch_failed` / `mirasim_models_parse_failed` | 0 / 0 |
| `mirasim_roster_fetch_failed` | 0 |

mirasim 相关日志仅 3 条 `mirasim_relay_credential_updated`，字段只有
`account_id: 140` 与 `key: "mirasim_refresh_token"`（**字段名，非值**）。

时间点：`23:45:10`、`00:20:09`、`00:48:55`（+0800）——分别对应三个轮次的显式模型同步。

> 行为说明（非缺陷）：进程重启后首次走 mirasim 只读元数据时，进程内 ticket / access token
> 缓存为空，因此会经 `/auth/refresh` 换凭据并落盘轮换后的 refresh token。账号 140 有独立
> 会话族（见文档 §9.2），不会踢掉桌面客户端；轮换结果已回写 DB，账号继续可用。

## 5. 回滚

```bash
C=sub2api-api-candidate-0.1.391-noinject-a33b47ef9
BIN=/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api
docker stop "$C"
cp -a "$BIN.bak-20260925-mirasimmodels" "$BIN"
docker start "$C"
```

前端无需回滚（未发布）。注意旧二进制会把白名单来源退回全局 roster（即回到「同步出 6 个
调用即 404 的 claude 模型」的行为），因此回滚后应同时清空账号 140 的白名单。
