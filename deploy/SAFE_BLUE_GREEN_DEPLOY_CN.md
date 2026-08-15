# Sub2API 前后端分离安全发布手册

生产默认由 Nginx 提供公网入口：静态前端来自独立 release 目录，API 运行在内部网络或 `127.0.0.1` 端口。embedded 单容器只用于既有安装兼容和迁移回滚。

## 发布边界

- 每次发布显式选择 `frontend`、`backend` 或 `both`，并提供只列本次产物的 JSON manifest；禁止从整个 dirty worktree推断范围。
- `frontend` 不重启/替换 API，不执行数据库备份或迁移，不改变 API current/rollback。
- `backend` 不构建、上传或切换前端，不改变前端 current/rollback。
- OpenAPI/generated client、公开 runtime-config schema、认证契约等不兼容跨端变更必须用 `both`。
- 先 staging 后生产；生产迁移仍需当次明确批准。生产服务器不从 dirty 源码树构建。

典型归类：纯 Vue/CSS/文案修复为 `frontend`；不改变前端契约的调度、结算或计费实现修复为 `backend`；OpenAPI + generated client、登录协议或 runtime-config schema 变更为 `both`。若兼容测试能证明旧对端仍受支持，可以通过调整明确的版本门槛安全拆分发布，不能靠口头判断降级 scope。

## 版本和产物

| 组件 | 当前版本 | 对端最低版本 | 产物 |
|---|---|---|---|
| API | `backend/cmd/server/VERSION` | `backend/cmd/server/MIN_FRONTEND_VERSION` | 非 embed `sub2api` + `resources/` tar |
| 前端 | `frontend/VERSION` | `frontend/MIN_BACKEND_VERSION` | `sub2api-frontend-<version>.zip`，含 `sub2api-build.json` |

API 的 `GET /api/v1/public/version` 返回 `version` 与 `min_frontend_version`；前端的 `sub2api-build.json` 返回 `frontend_version` 与 `min_backend_version`。部署预检双向 fail-closed。

```bash
# 前端
pnpm --dir frontend run typecheck
pnpm --dir frontend run build
FV=$(tr -d '\r\n' < frontend/VERSION)
(cd backend/internal/web/dist && zip -r "$OLDPWD/sub2api-frontend-${FV}.zip" .)
sha256sum "sub2api-frontend-${FV}.zip"

# 非 embed API
BV=$(tr -d '\r\n' < backend/cmd/server/VERSION)
MFV=$(tr -d '\r\n' < backend/cmd/server/MIN_FRONTEND_VERSION)
mkdir -p artifact-api
(cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags="-s -w -X main.Version=${BV} -X main.MinFrontendVersion=${MFV} -X main.BuildType=release" \
  -o ../artifact-api/sub2api ./cmd/server)
cp -a backend/resources artifact-api/resources
tar -C artifact-api -czf "sub2api-api-${BV}-linux-amd64.tar.gz" sub2api resources
sha256sum "sub2api-api-${BV}-linux-amd64.tar.gz"
```

兼容 embedded 构建只能显式执行 `make build-embedded`。

## Manifest、上传和预检

复制 `component-manifest.example.json`。`backend` 对象使用 `version`、`min_frontend_version`、`artifact`、`sha256` 和布尔值 `db_change`。

```json
{
  "scope": "frontend",
  "contract_change": false,
  "frontend": {
    "version": "0.1.321",
    "min_backend_version": "0.1.320",
    "artifact": "/tmp/sub2api-release/sub2api-frontend-0.1.321.zip",
    "sha256": "<sha256>"
  }
}
```

逐文件上传，禁止通配整个工作树：

```bash
scp component-manifest.json deploy/component_deploy.py "$DEPLOY_HOST:/tmp/sub2api-release/"
scp sub2api-frontend-0.1.321.zip "$DEPLOY_HOST:/tmp/sub2api-release/"
python3 /tmp/sub2api-release/component_deploy.py check \
  --manifest /tmp/sub2api-release/component-manifest.json --root /opt/sub2api
```

状态布局：

```text
/opt/sub2api/
├── frontend/{releases,current,rollback}
├── api/{releases,current,rollback}
└── state/{.codex-frontend-current,.codex-frontend-rollback,
           .codex-api-current,.codex-api-rollback}
```

每端只保留 current 和 rollback 指向的两个不可变 release；清理本端时不删除另一端产物，也不删除迁移观察期的 embedded 回滚。

## Frontend-only

门禁：SHA/文件清单、`index.html`、build metadata、兼容预检、typecheck/build、登录与设置页渲染 smoke。

```bash
python3 /tmp/sub2api-release/component_deploy.py deploy \
  --manifest /tmp/sub2api-release/component-manifest.json --root /opt/sub2api
nginx -t && nginx -s reload
curl -fsS "$PUBLIC_ORIGIN/sub2api-build.json"
curl -fsS "$PUBLIC_ORIGIN/api/v1/public/version"
```

Nginx `root` 指向 `/opt/sub2api/frontend/current`。该命令不触碰 API/DB。独立回滚：

```bash
python3 /opt/sub2api/deploy/component_deploy.py rollback frontend --root /opt/sub2api
nginx -t && nginx -s reload
```

## Backend-only

候选 API 必须在备用 loopback 端口通过 `/health`、public config/version、未登录 401，以及与改动相关的 HTTP/SSE/WS/结算门禁。工具要求显式 verify/switch hook；`{release}` 替换为候选 release 绝对路径。

```bash
python3 /tmp/sub2api-release/component_deploy.py deploy \
  --manifest /tmp/sub2api-release/component-manifest.json --root /opt/sub2api \
  --backend-verify-command 'sudo /opt/sub2api/bin/verify-api-candidate {release}' \
  --backend-switch-command 'sudo /opt/sub2api/bin/switch-api-release {release}'
```

仅当 manifest 为 `db_change: true` 时才添加：

```bash
--db-backup-command 'sudo /opt/sub2api/bin/backup-postgres-before-api-release'
```

独立回滚不改前端：

```bash
python3 /opt/sub2api/deploy/component_deploy.py rollback backend --root /opt/sub2api \
  --backend-switch-command 'sudo /opt/sub2api/bin/switch-api-release {release}'
```

## Both

顺序固定为：候选 API 兼容当前前端 → 备用端口验证并切 API → 候选前端兼容新 API → 原子切前端。若候选 API 不兼容当前前端，预检会拒绝，必须先做向后兼容的 API 过渡版本。失败默认只回滚失败端；兼容契约要求成对回滚时按逆序回滚。

## Staging 门禁

- frontend-only：API 容器 ID/启动时间、API marker、DB 状态前后不变；登录/设置正常；独立回滚成功。
- backend-only：前端 SHA/symlink/marker 不变；HTTP/SSE/WS、粘性选号、故障切换、usage/计费对账正常；独立回滚成功。
- both：旧前端在新 API 上正常，新前端切换后全链路正常，成对回滚成功。
- Nginx：`nginx -t`；API 无公网监听；underscore header、SSE 不缓冲、WS Upgrade、SPA fallback 通过。

## 从 embedded 迁移与回滚

1. 记录旧 embedded 容器/二进制、镜像 digest、端口、Nginx config、env/config；不要删除。
2. 生成初始 API/前端 marker，建立不可变 frontend release，但暂不切公网。
3. 在新 loopback 端口启动非 embed API，使用同一 DB/Redis/config，完成协议和计费验证。
4. `nginx -t` 后切换为“静态 current + 新 API upstream”。
5. 观察期内保留旧 embedded 容器/config；失败就恢复旧 Nginx config 和旧容器，前后端成对回滚。
6. 观察期通过后才建立独立 frontend/API rollback 生命周期。

Compose 兼容回滚：

```bash
EMBEDDED_IMAGE=<old-image@digest> \
  docker compose -f docker-compose.yml -f docker-compose.embedded.yml up -d
```

最终门禁还包括：针对性 Go 测试与非 embed Linux build、前端 typecheck/build/关键测试、组件部署测试、Nginx route-contract/`nginx -t`、Compose render、`git diff --check`、`openspec validate --strict`。涉及调度/计费时继续跑对应差分测试床。在 staging 三种 scope 与独立回滚留证前，不得声称重构或本 change 已完成。
