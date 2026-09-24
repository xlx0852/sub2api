# 利润分析配额窗口移除 — 生产发布

日期：2026-09-25 06:42 JST（05:42 +08）
代码：`756ee2ecc`（同批包含 ops token 统计与看板邀请入口的移除）
change：`remove-profit-quota-window-panel`
生产机：74.82.195.84

## 1. 发布顺序与产物

### 1.1 前端（`vite build` → release 目录 + 符号链接切换）

| 项 | 值 |
| --- | --- |
| 制品 | `frontend-0.1.391-756ee2ecc.tgz`，17,381,203 B，`sha256=4c09bebf8a7b374d085e8f6bc67873d06e0ed71b4ae303e66d55fb8bf3a3891d` |
| release 目录 | `/opt/sub2api/frontend/releases/0.1.391-756ee2ecc`（171 文件，`._*` / `.DS_Store` = 0） |
| 全量聚合 | `02c771e352f3aa1b415ec17fa04ae4f03a8d7b0c6e10d03ecddd0c5e97667753` == 本地 `backend/internal/web/dist` |
| `index.html` | `sha256=49c8ff1317b4e6d7241f5d6e757290b84baeebbc95bd370c9db91adc2301d271`，引用 `/assets/index-D0JALEtV.js` + `/assets/index-6Es2MM7L.css` |
| `current` / `rollback` | `releases/0.1.391-756ee2ecc` / `releases/0.1.391-0c48b9f6b`（06:42 JST） |

### 1.2 后端（同容器 bind-mount 二进制热替换）

| 项 | 值 |
| --- | --- |
| 构建 | `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=true -tags embed ./cmd/server`（前端已先 `pnpm build`） |
| 制品 | 123,688,744 B，`sha256=5339fd48e6258c6ab9aeefd8377eede1eea383bc9ebae5b80c468699133b0284`，静态链接，`vcs.revision=756ee2ecc04ad675b2f588a31fa53af5ccc759ea` |
| 备份 | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api.bak-20260925-profitqwin`（替换前 `sha256=6e017abd1dbaafce1767d53dd8e97ec4e64055a55f24d9ba4eae21d9a39ff163`） |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`，`Up (healthy)`、`RestartCount=0`、启动 2026-09-24T21:42:40Z |

## 2. 发布后核验（只读）

| 校验 | 结果 |
| --- | --- |
| 前端聚合 | `cd <release> && find . -type f \| LC_ALL=C sort \| xargs sha256sum \| sha256sum` == 本地，171 文件 |
| 经 nginx 取回 | `https://code.sicts.shop/`（`--resolve` 到 127.0.0.1）`index.html` = `49c8ff13…`；`/assets/index-D0JALEtV.js` = 200（147,075 B） |
| 健康与版本 | `/health` = 200；`/api/v1/public/version` = `{"version":"0.1.391","min_frontend_version":"0.1.380"}` |
| 下线接口 | `POST /api/v1/admin/profit/accounts/1/window-economics` = **404**；`GET /api/v1/admin/ops/dashboard/openai-token-stats` = **404** |
| 在役接口 | `GET /api/v1/admin/profit/summary` = 401（未带管理凭证，路径存在） |
| 错误面 | 替换后 5 分钟内 5xx = **0**；容器日志 `panic`/`fatal` = 0；3 分钟请求分布 200×106 / 304×1 / 401×1 / 404×3（404 为本次探测） |

## 3. 回滚

1. 前端：`ln -sfn /opt/sub2api/frontend/releases/0.1.391-0c48b9f6b /opt/sub2api/frontend/current`
2. 后端：`docker stop sub2api-api-candidate-0.1.391-noinject-a33b47ef9` → `install -m 0755 .../sub2api.bak-20260925-profitqwin .../sub2api` → `docker start`（校验 `sha256=6e017abd…`）
3. 无 schema 变更、无环境变量变更，无需数据回滚。

## 4. 同批发布的其它改动

本二进制与前端同时包含工作区中已完成并提交的另外两项移除（提交 `756ee2ecc`）：

- **ops 看板 OpenAI token 统计**：删除 domain/repository/service/handler、前端卡片、设置开关（`display_openai_token_stats`）、API 类型与 zh/en i18n。
- **用户看板邀请返利入口**：删除 `UserDashboardQuickActions` 的 affiliate CTA 与相关 props/i18n。

## 5. 保留与限制

- 窗口账本（`account_quota_windows` / `account_quota_usage_observations`）、后台 sweep、账号用量的满额估算（`account-quota-estimation`）**未改动**。
- 利润会计（计费窗口来源、break-even、摊销参照窗口）**未改动**，`AccountProfitSummary` 中对应字段仍在（前端暂无消费者）。
- `openspec/changes/add-quota-window-timeline/`（给账号列表加配额窗口时间轴的未实施提案）未处理，如需彻底退役配额窗口应另行作废。
