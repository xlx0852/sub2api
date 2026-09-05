## 1. Baseline

- [x] 1.1 固定一个干净基线并记录 Gateway/OpenAI handler 入口清单
- [ ] 1.2 为首个 HTTP/SSE 主路径补旧/新响应、usage、lease 和 settlement 差分测试

## 2. Migration

- [x] 2.1 提取 parse/admission/select/rewrite/transport/relay/settle 阶段
- [x] 2.2 将首个入口迁移到统一 pipeline Exchange
- [ ] 2.3 迁移 WS、图片和兼容入口

## 3. Gates

- [x] 3.1 `go test ./internal/...`
- [x] 3.2 `go vet ./internal/...`
- [x] 3.3 前端 ESLint、typecheck、Vitest
- [ ] 3.4 旧/新差分无未归因差异后删除旧编排代码
