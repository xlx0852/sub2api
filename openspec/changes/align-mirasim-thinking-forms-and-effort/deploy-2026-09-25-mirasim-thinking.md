# 生产发布记录：mirasim thinking 形态 / effort 阶梯（2026-09-25）

## 制品

| 项 | 值 |
| --- | --- |
| 源码 revision | `b3f8c5033f49aa35735776b1329519b6cef89ac0`（`feat(mirasim): relay thinking 形态/effort 阶梯本地校验与归一`） |
| 二进制 sha256 | `18637fd75a09e5622d9f221ebf54dfa83e521c83a1e3ee8bec68cc0713ba1139` |
| 二进制大小 | 123,760,802 B（`-tags embed`，`go1.26.4`） |
| 构建命令 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380" -trimpath -tags embed -o /tmp/sub2api-mirasimthinking ./cmd/server` |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`（`127.0.0.1:18373`） |
| bind-mount 路径 | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api` |
| 被替换版本 | sha `dd2d9192da00a6ac9604d5670904ba702c632332666188953abac00a817019fe`，revision `a17651943748100fffdd8f02ffbef13254f80775`（已核对为本 revision 的祖先） |
| 回滚备份 | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api.bak-20260925-mirasimthinking`（同一 sha） |

前端未随本次发布：线上 `current` 已是 `0.1.391-919283d25`，本次无前端改动。

## 验证（发布后实测）

1. **本地 400 生效且不转上游**：用组 34 的 key 发 `/v1/messages`，`model=deepseek-flash` + `output_config.effort=medium`
   → HTTP 400，`ops_error_logs` 记录 `error_type=invalid_request_error`、`upstream_status_code=-1`、消息
   `unsupported thinking effort "medium" for deepseek-flash; Mirasim accepts low, medium, high, xhigh, max and ultra`
   （文案与官方客户端 `effortLadder` 常量逐字一致）。
2. **`ultra → max` 折档生效**：同条件改 `effort=ultra` → HTTP 200，真实上游 `event: message_start`（`model=deepseek-flash`）；
   `usage_logs` 该请求记录 `account_id=140`、`inbound_endpoint=/v1/messages`、`reasoning_effort=max`。
3. **relay 主链路不回归**：账号 140 管理员测试端点真实请求 HTTP 200（4,153 B，带真实 thinking 增量）；
   部署后 25 分钟窗口内 `forward_failed=0`（mirasim 相关）、上游 `Status=40x`=0、`mirasim_roster_fetch_failed=0`、
   `mirasim_roster_parse_failed=0`；账号 140 成功流量持续（`/v1/responses` 144 条，平均 3.0 s）。
4. **现网 effort 分布风险**：部署前 24h relay 账号 `/v1/messages` 的 effort 为 `high`(320)/`(null)`(224)/`low`(4)，
   均在阶梯内；`/v1/responses`（`low`183/`xhigh`68/`high`25/`medium`7）不在本次改动路径上。

## 后续观察

- 若出现 `ops_error_logs` 中 `status_code=400` + `invalid_request_error` 且消息含 `unsupported thinking effort`
  / `budget` 字样，说明有客户端在请求阶梯外的档位：需评估放宽还是让客户端改。

## 回滚

```bash
BIN=/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api
docker stop sub2api-api-candidate-0.1.391-noinject-a33b47ef9
cp -p "${BIN}.bak-20260925-mirasimthinking" "$BIN"
docker start sub2api-api-candidate-0.1.391-noinject-a33b47ef9
docker inspect -f '{{.State.Status}}' sub2api-api-candidate-0.1.391-noinject-a33b47ef9
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18373/health
```

回滚后 sha 应回到 `dd2d9192…`（revision `a17651943…`）。本改动无数据迁移、无 Redis 键变更。
