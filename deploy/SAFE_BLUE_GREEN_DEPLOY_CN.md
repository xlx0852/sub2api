# Sub2API 生产安全蓝绿部署流程

本文档用于当前生产站点 `code.sicts.shop` 的后续版本更新。核心原则是：**本地打包、服务器只接收构建产物、新实例验证通过后再用 Nginx 切流、只保留上一个版本用于回滚**。

## 适用范围

- 站点：`https://code.sicts.shop`
- SSH：`root@89.208.254.77`
- 生产目录：`/opt/sub2api`
- Nginx 配置：`/etc/nginx/conf.d/code-sicts-shop.conf`
- 数据目录：`/opt/sub2api/data`
- Docker 网络：`sub2api_sub2api-network`

## 硬性约束

- 不在服务器打包。
- 不上传源码到服务器。
- 不重新初始化项目。
- 不修改数据库连接配置。
- 新容器验证通过前不停止旧容器。
- 切流后停止旧容器，并将其作为唯一回滚版本保留。
- 每次部署验证完成后删除更早的容器、二进制、资源目录和部署备份；线上始终只保留“当前版本 + 上一个版本”。
- `model-catalog/catalog.json` 固定保存在 `$REMOTE_BASE/shared/model-catalog/`，发布资源同步到该目录后再只读挂载，清理历史版本时不得删除。
- 密码、Token、`.env` 内容不得写进文档、提交或命令历史。

## 部署变量

在本机仓库根目录执行：

```bash
export SSH_TARGET="root@89.208.254.77"
export REMOTE_BASE="/opt/sub2api"
export TS="$(date +%Y%m%d%H%M%S)"
export VERSION="$(tr -d '\r\n' < backend/cmd/server/VERSION)"
export COMMIT="$(git rev-parse --short HEAD)"
export BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
export OUT="/tmp/sub2api-${VERSION}-${TS}-${COMMIT}-linux-amd64"
export REMOTE_BIN="${REMOTE_BASE}/sub2api-${VERSION}-${TS}-${COMMIT}-linux-amd64"
export REMOTE_RESOURCES="${REMOTE_BASE}/resources-${TS}"
export REMOTE_CATALOG="${REMOTE_BASE}/shared/model-catalog/catalog.json"
export BACKUP_DIR="${REMOTE_BASE}/backups/deploy-${TS}"
export IMAGE="sub2api:codex-ws-20260601044722"
export NEW_PORT="18098"
export NEW_NAME="sub2api-${VERSION//./-}-${COMMIT}-${TS}-blue"
```

`NEW_PORT` 每次部署需要选择一个未被占用的本机端口。可在服务器上查看当前使用情况：

```bash
ssh "$SSH_TARGET" "docker ps --format 'table {{.Names}}\t{{.Ports}}' | grep sub2api || true"
```

## 1. 本地预检查

```bash
git status --short --branch
```

确认当前分支和提交是要部署的版本。建议至少跑一次后端入口测试：

```bash
cd backend
go test ./cmd/server
cd ..
```

如果 `go test` 或 `go build` 报 Wire 注入过期，先重新生成再测试：

```bash
cd backend
go generate ./cmd/server
go test ./cmd/server
cd ..
```

## 2. 服务器备份

部署前备份 Compose、环境文件、Nginx 配置和 PostgreSQL 数据。数据库备份必须使用容器内真实的 `POSTGRES_USER` 和 `POSTGRES_DB`，不要假设数据库用户是 `root`。

```bash
ssh "$SSH_TARGET" "set -euo pipefail
mkdir -p '$BACKUP_DIR'
cp -a '$REMOTE_BASE/docker-compose.yml' '$BACKUP_DIR/docker-compose.yml'
cp -a '$REMOTE_BASE/.env' '$BACKUP_DIR/env'
cp -a /etc/nginx/conf.d/code-sicts-shop.conf '$BACKUP_DIR/code-sicts-shop.conf'
docker exec sub2api-postgres sh -lc 'pg_dump -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\"' | gzip > '$BACKUP_DIR/postgres.sql.gz'
gzip -t '$BACKUP_DIR/postgres.sql.gz'
ls -lh '$BACKUP_DIR'
"
```

## 3. 本地构建前端

前端构建产物会写入后端 embed 目录：

```bash
pnpm --dir frontend run build
```

如果只是后端补丁，也仍建议执行一次前端构建，确保 `backend/internal/web/dist` 与当前前端代码一致。

## 4. 本地构建后端二进制

```bash
cd backend
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -tags embed \
  -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${BUILD_DATE} -X main.BuildType=release" \
  -trimpath \
  -o "$OUT" \
  ./cmd/server
cd ..

ls -lh "$OUT"
```

再次强调：不要在服务器上执行 `pnpm build`、`go build` 或源码打包。

## 5. 上传构建产物

只上传二进制和运行资源目录：

```bash
scp "$OUT" "$SSH_TARGET:$REMOTE_BIN"

COPYFILE_DISABLE=1 tar -C backend -czf - resources | ssh "$SSH_TARGET" "set -euo pipefail
rm -rf '$REMOTE_RESOURCES'
mkdir -p '$REMOTE_RESOURCES'
tar -xzf - -C '$REMOTE_RESOURCES'
chmod +x '$REMOTE_BIN'
chmod -R a+rX '$REMOTE_RESOURCES'
mkdir -p '$REMOTE_BASE/shared/model-catalog'
cp -a '$REMOTE_RESOURCES/resources/model-catalog/catalog.json' '$REMOTE_CATALOG'
chmod 0644 '$REMOTE_CATALOG'
ls -lh '$REMOTE_BIN'
find '$REMOTE_RESOURCES' -maxdepth 2 -type f | head
"
```

不要上传整个仓库，也不要在服务器创建临时源码构建目录。

## 6. 启动新容器

新容器使用现有镜像作为运行壳，挂载本次本机构建出的二进制：

```bash
ssh "$SSH_TARGET" "set -euo pipefail
docker rm -f '$NEW_NAME' >/dev/null 2>&1 || true
docker run -d \
  --name '$NEW_NAME' \
  --restart unless-stopped \
  --network sub2api_sub2api-network \
  --env-file '$REMOTE_BASE/.env' \
  -e GIN_MODE=release \
  -e DATABASE_HOST=postgres \
  -e REDIS_HOST=redis \
  -v '$REMOTE_BIN:/app/sub2api:ro' \
  -v '$REMOTE_BASE/data:/app/data' \
  -v '$REMOTE_RESOURCES/resources:/app/resources:ro' \
  -v '$REMOTE_BASE/shared/model-catalog:/app/resources/model-catalog:ro' \
  -p 127.0.0.1:'$NEW_PORT':8080 \
  '$IMAGE'
docker ps --filter name='$NEW_NAME' --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
"
```

## 7. 验证新实例

先在服务器本机验证，不经过 Nginx：

```bash
ssh "$SSH_TARGET" "set -euo pipefail
for i in 1 2 3; do
  curl -fsS 'http://127.0.0.1:$NEW_PORT/health'
  echo
done
curl -fsS 'http://127.0.0.1:$NEW_PORT/api/v1/settings/public' | head -c 500
echo
curl -fsSI 'http://127.0.0.1:$NEW_PORT/admin/accounts' | head
docker logs --since 3m '$NEW_NAME' 2>&1 | egrep -i 'panic|fatal|migration|database|failed|error|5[0-9][0-9]' || true
"
```

健康标准：

- `/health` 返回 `{"status":"ok"}`。
- `/api/v1/settings/public` 能返回当前版本。
- `/admin/accounts` 返回前端页面。
- 最近启动日志无迁移失败、数据库连接失败、panic、fatal。

## 8. Nginx 切流

找到当前旧端口，并把 Nginx 上游端口替换为新端口：

```bash
ssh "$SSH_TARGET" "grep -n '127.0.0.1:' /etc/nginx/conf.d/code-sicts-shop.conf"
```

确认旧端口后执行，替换 `OLD_PORT`：

```bash
export OLD_PORT="18097"

ssh "$SSH_TARGET" "set -euo pipefail
cp -a /etc/nginx/conf.d/code-sicts-shop.conf '$BACKUP_DIR/code-sicts-shop.conf.before-switch'
python3 - '$OLD_PORT' '$NEW_PORT' <<'PY'
from pathlib import Path
import sys

path = Path('/etc/nginx/conf.d/code-sicts-shop.conf')
old = '127.0.0.1:' + sys.argv[1]
new = '127.0.0.1:' + sys.argv[2]
text = path.read_text()
if old not in text:
    raise SystemExit(f'old upstream {old} not found')
path.write_text(text.replace(old, new))
PY
nginx -t
nginx -s reload
grep -n '127.0.0.1:' /etc/nginx/conf.d/code-sicts-shop.conf
"
```

## 9. 公网验证

切流后从本机公网验证：

```bash
for i in 1 2 3; do
  curl -fsS https://code.sicts.shop/health
  echo
done

curl -fsS https://code.sicts.shop/api/v1/settings/public | head -c 500
echo
curl -fsSI https://code.sicts.shop/admin/accounts | head
```

健康标准：

- 公网 `/health` 连续返回 `ok`。
- 公网版本号与本次构建版本一致。
- 后台页面可返回 HTML。

## 10. 停止旧容器并记录元信息

公网验证通过后再停止旧容器。不要删除旧容器：

```bash
export OLD_NAME="sub2api-旧容器名"

ssh "$SSH_TARGET" "set -euo pipefail
docker stop '$OLD_NAME'
printf '%s\n' '$NEW_NAME' > '$REMOTE_BASE/.codex-current-container'
printf '%s\n' '$REMOTE_BIN' > '$REMOTE_BASE/.codex-current-binary'
printf '%s\n' '$NEW_PORT' > '$REMOTE_BASE/.codex-current-port'
printf '%s\n' '$REMOTE_RESOURCES' > '$REMOTE_BASE/.codex-current-resources'
printf '%s\n' '$REMOTE_CATALOG' > '$REMOTE_BASE/.codex-current-catalog'
printf '%s\n' '$OLD_NAME' > '$REMOTE_BASE/.codex-rollback-container'
printf '%s\n' '$OLD_PORT' > '$REMOTE_BASE/.codex-rollback-port'
docker ps -a --filter name=sub2api --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
"
```

旧容器及其挂载的二进制、资源目录是唯一回滚版本，不得在本次部署中删除。

## 11. 清理更早的发布历史

只在公网验证、新实例日志检查和第 10 步元信息更新全部完成后执行。清理目标包括：

- 当前和回滚标记之外的应用容器（PostgreSQL、Redis 不在清理范围）。
- 当前和回滚容器未挂载的历史二进制及 `resources-*` 目录。
- 当前部署的 `$BACKUP_DIR` 之外的 `backups/deploy-*` 目录。
- 已解压后不再参与运行或回滚的历史资源压缩包。

执行前先输出保留对象并确认 Nginx 正在使用当前端口：

```bash
ssh "$SSH_TARGET" "set -euo pipefail
CURRENT_NAME=\$(cat '$REMOTE_BASE/.codex-current-container')
CURRENT_PORT=\$(cat '$REMOTE_BASE/.codex-current-port')
ROLLBACK_NAME=\$(cat '$REMOTE_BASE/.codex-rollback-container')

grep -q \"127.0.0.1:\${CURRENT_PORT}\" /etc/nginx/conf.d/code-sicts-shop.conf
test \"\$(docker inspect \"\$CURRENT_NAME\" --format '{{.State.Health.Status}}')\" = healthy
docker inspect \"\$ROLLBACK_NAME\" >/dev/null

echo \"保留当前容器: \$CURRENT_NAME\"
echo \"保留回滚容器: \$ROLLBACK_NAME\"
echo \"保留部署备份: $BACKUP_DIR\"
"
```

清理时必须从当前、回滚容器的 Docker mounts 读取要保留的二进制和资源路径，不能仅按时间猜测。资源挂载源是 `resources-*/resources` 子目录，而实际清理对象是它的父目录 `resources-*`，因此保留集合必须使用挂载源的父目录：

```bash
CURRENT_RES_MOUNT=$(docker inspect "$CURRENT_NAME" --format '{{range .Mounts}}{{if eq .Destination "/app/resources"}}{{.Source}}{{end}}{{end}}')
ROLLBACK_RES_MOUNT=$(docker inspect "$ROLLBACK_NAME" --format '{{range .Mounts}}{{if eq .Destination "/app/resources"}}{{.Source}}{{end}}{{end}}')
CURRENT_RES_DIR=$(dirname "$CURRENT_RES_MOUNT")
ROLLBACK_RES_DIR=$(dirname "$ROLLBACK_RES_MOUNT")
```

不得直接拿 `resources-*/resources` 与 `resources-*` 比较，否则会误删当前和回滚版本的资源父目录。完成后再次检查公网健康、Nginx 上游和回滚容器是否仍存在，并分别确认当前、回滚资源目录中的 model pricing 与 catalog 文件可读。线上只保留一个历史版本，不累积第二个或更早的回滚版本。

## 回滚流程

如果切流后发现严重问题，优先做 Nginx 端口回滚：

```bash
ssh "$SSH_TARGET" "set -euo pipefail
docker start '$OLD_NAME'
python3 - '$NEW_PORT' '$OLD_PORT' <<'PY'
from pathlib import Path
import sys

path = Path('/etc/nginx/conf.d/code-sicts-shop.conf')
old = '127.0.0.1:' + sys.argv[1]
new = '127.0.0.1:' + sys.argv[2]
text = path.read_text()
if old not in text:
    raise SystemExit(f'new upstream {old} not found')
path.write_text(text.replace(old, new))
PY
nginx -t
nginx -s reload
curl -fsS http://127.0.0.1:'$OLD_PORT'/health
"
```

数据库回滚属于高风险操作，只在确认新版本执行了错误迁移或错误数据写入时使用，并且需要先再次备份当前数据库。恢复命令示例：

```bash
ssh "$SSH_TARGET" "set -euo pipefail
gzip -dc '$BACKUP_DIR/postgres.sql.gz' | docker exec -i sub2api-postgres sh -lc 'psql -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\"'
"
```

执行数据库恢复前必须确认影响范围。

## 常见问题

### `go build` 报 Wire 注入不存在

说明 `backend/cmd/server/wire_gen.go` 与当前依赖注入代码不同步：

```bash
cd backend
go generate ./cmd/server
go test ./cmd/server
cd ..
```

### `pg_dump` 只有几十字节或提示角色不存在

通常是用了宿主机用户或错误数据库名。必须通过 `docker exec sub2api-postgres` 使用容器内的 `POSTGRES_USER`、`POSTGRES_DB`。

### 新旧容器同时运行会不会重复任务

新容器本机验证阶段可能短暂与旧容器共存。验证窗口应尽量短；公网切流确认正常后立即停止旧容器。不要长时间让两个生产实例同时连接同一套数据库和 Redis。

### 版本号页面没变

优先确认：

```bash
curl -fsS https://code.sicts.shop/api/v1/settings/public
```

如果 API 版本正确但页面没变，通常是浏览器缓存或前端资源缓存；如果 API 版本也没变，检查 Nginx 是否切到新端口。

## 最终检查清单

- 本地构建完成，服务器没有源码构建动作。
- 服务器备份目录存在，`postgres.sql.gz` 已通过 `gzip -t`。
- 新容器本机 `/health` 正常。
- Nginx `nginx -t` 通过并已 reload。
- 公网 `/health` 正常。
- 公网版本号正确。
- 上一个容器已停止并作为唯一回滚版本保留。
- 更早的应用容器、二进制、资源目录及部署备份已清理。
- 当前与回滚容器的挂载文件均存在，持久化 model catalog 未被清理。
- `/opt/sub2api/.codex-current-*` 已更新。
