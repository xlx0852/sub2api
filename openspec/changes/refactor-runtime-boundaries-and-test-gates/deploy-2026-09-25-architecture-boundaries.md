# 生产发布记录：运行时边界与架构门禁批次（2026-09-25）

## 制品

| 项 | 值 |
| --- | --- |
| 源码 revision | `d857fc0593fe7e5fe5cdb2fe6984fd69378d278f` |
| 二进制 sha256 | `90226596882aa5ce0b109fa02c8de441d37931861d53c392c5369e4faa4e876c` |
| 二进制大小 | 123,777,186 B |
| 构建 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380" -trimpath -tags embed` |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9` |
| bind mount | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api` |
| 回滚点 | `sub2api.bak-20260925-architecture-boundaries`，原 sha `18637fd7…`，revision `b3f8c5033…` |

本次无前端修改，未重新发布前端 release。

## 发布前验证

- `openspec validate refactor-runtime-boundaries-and-test-gates --strict`：通过
- `make test-unit-architecture`：通过
- `go build ./...`：通过
- 被替换版本 `b3f8c5033…` 已核对为新 revision 祖先

## 发布后验证

1. 容器状态 `running`，health HTTP `200`，restart count `0`。
2. 账号 140 Mirasim 管理员测试真实请求 HTTP `200`，收到真实 thinking 增量。
3. 发布后日志窗口：
   - `mirasim_roster_fetch_failed=0`
   - `mirasim_roster_parse_failed=0`
   - `mirasim_effort_invalid=0`
   - `forward_failed=0`
   - 上游 401/403=0
4. 生产日志无 panic/fatal。

管理员测试输出中出现的 Grok 402 是同一测试脚本的既有其他账号诊断噪音，不属于账号 140 的 Mirasim relay 失败；账号 140 请求本身返回 200。

## 回滚

```bash
BIN=/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api
CTR=sub2api-api-candidate-0.1.391-noinject-a33b47ef9
docker stop "$CTR"
cp -p "${BIN}.bak-20260925-architecture-boundaries" "$BIN"
docker start "$CTR"
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18373/health
```
