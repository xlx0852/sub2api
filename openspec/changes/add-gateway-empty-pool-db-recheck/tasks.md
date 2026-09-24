# Tasks

## 1. 实现

- [x] 1.1 新增 `backend/internal/service/gateway_empty_pool_recheck.go`：`gatewaySelectionPoolStats` + `recheckEmptyGatewayPool`（含中文「关键约束」注释与三个事件）
- [x] 1.2 `gateway_scheduling.go`：把现有 `selectAccountWithLoadAwarenessStrategy` 主体拆为 `selectGatewayAccountOnce`（回传池统计），新增 `selectGatewayAccountWithEmptyPoolRecheck` 编排并按 `EmptyPool && 无排除账号` 触发
- [x] 1.3 单测 `backend/internal/service/gateway_empty_pool_recheck_test.go`：复核成功并重试、复核后仍失败、有排除账号不复核、忙池不复核、非空池错误不处理、节流不复核、无快照服务不复核

## 2. 验证

- [x] 2.1 `cd backend && go build ./...`（通过）
- [x] 2.2 隔离环境跑 `go vet -tags unit ./internal/service/`（干净）与 `go test -tags unit ./internal/service/ -count=1`（ok；隔离时需额外汇出 `account_repo_gemini_test_helpers.go`，它是带 unit tag 的旧 helper 而非 `_test.go`）
- [x] 2.3 `go test ./internal/service/ -count=1` 失败集合与 HEAD 基线完全一致（TestIsModelRateLimited / TestGetModelRateLimitRemainingTime / TestGetRateLimitRemainingTime / 三个 OpenAI gateway 用例），无新增失败；`go test ./internal/app/scheduling/ -count=1` 通过
- [x] 2.4 `gofmt -l` 新增/修改文件干净
- [x] 2.5 `openspec validate add-gateway-empty-pool-db-recheck --strict`（valid）

## 3. 发布（待批准后执行）

- [ ] 3.1 构建后端二进制（`-tags embed`）并热替换到生产容器，记录回滚点
- [ ] 3.2 线上核验：空池路径日志出现 `gateway_scheduler_empty_pool_db_recheck`；正常请求无新增延迟
- [ ] 3.3 补「生产发布记录」markdown 到本 change 目录
