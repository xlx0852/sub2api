# 生产发布记录：mirasim relay 客户端元数据对齐

- 日期：2026-09-25（容器时区 +08 为 23:05–23:10）
- 变更：`align-mirasim-relay-client-metadata`
- 代码基线：本地 `9c229e1f0`

## 产物

| 项 | 值 |
|---|---|
| 后端二进制 | sha256 `e32fd2025d08b087adbb0ae880e672022bba978fdf7bf7c43d1b3007544b7dbc`（123,732,130 B，linux/amd64，`-tags embed`） |
| 容器内 `/app/sub2api` | sha256 与上一致 |
| 构建内嵌 revision | `vcs.revision=9c229e1f01de347abdea1f0f19773fc5dce70207` |
| 前端 | 未发布（本次为纯后端；SPA 由 nginx 外部提供） |

## 部署方式

热替换绑定挂载二进制（`docker stop` → 备份 → `mv` → `docker start`）：

- 容器：`sub2api-api-candidate-0.1.391-noinject-a33b47ef9`（`127.0.0.1:18373`）
- 二进制路径：`/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api`
- 回滚点：`sub2api.bak-20260925-mirasimmetadata`（sha256 `c9163dbd548385c41b0bcc11226a1f57c3abd8e0bb6b572c3624f6f17f02df57`，vcs.revision `0f340c3700bd99b3f9210b522e174642906581aa`）
- 替换前 revision（`0f340c37…`）是本变更 HEAD 的祖先，无回退并发窗口工作的风险
- 健康检查：`/health` 200 `{"status":"ok"}`，容器 `healthy`（6s 内）

## 真实核验（写入元数据后的签名是否被 relay 接受）

账号 140（mirasim-relay）走真实 relay 请求两次（`POST /api/v1/admin/accounts/140/test`，模型 `deepseek-flash`）：

- 第 1 次：HTTP 200，SSE 有真实推理增量
- 第 2 次：HTTP 200，SSE 有真实推理增量
- `forward_failed`（账号 140）：0
- 上游 401/403：0
- 只读端点（`/v1/models`、`/v1/limits`）形态未变，账号诊断链继续正常

真实用户流量复核：账号 140 在部署后 20 分钟内有持续 `/v1/responses` 成功记录
（`requested_model=deepseek-v4.1-flash` → `upstream_model=deepseek-flash`，含 `cache_read_tokens`）。

## 观察项

1. `mirasim_relay_credential_updated`（refresh token 轮换）频率与部署前一致（部署前 30 分钟 2 次、部署后 12 分钟 2 次），无 401/403；轮换率偏高属**既有行为**，非本变更引入，可另开项排查。
2. `x-mirasim-client` 仍为 `0.0.322`（官方客户端 `0.0.354`）——见 tasks 4.5，未在本变更内改动。
3. 前端「请求耗时详情」的新实现（瀑布/全分段）尚未发布到 nginx，线上仍是旧产物。

## 回滚步骤

```bash
docker stop sub2api-api-candidate-0.1.391-noinject-a33b47ef9
cp -p /opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api.bak-20260925-mirasimmetadata \
      /opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api
docker start sub2api-api-candidate-0.1.391-noinject-a33b47ef9
```
