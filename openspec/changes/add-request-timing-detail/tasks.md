## 1. 采集器与存储

- [x] 1.1 `internal/pkg/requesttiming`：请求级时间线（body 分段、命名区间、上游时间点、尝试/复用/流完整/结果），未采集保持 nil
- [x] 1.2 `domain.FirstOutputStages` 扩展新字段（JSONB，无迁移；旧字段语义不变）
- [x] 1.3 `service.MergeFirstOutputStages` 合并新旧快照（旧值优先，新字段只填 nil）

## 2. 接入点

- [x] 2.1 中间件 `middleware.RequestTiming()`：创建采集器、包装请求体（首读等待/接收区间/阻塞累计/字节/完整）、Handler 总耗时；接入 `/v1` 组、`/responses`、`/responses/*subpath`、`/alpha/search`、codex 直连组
- [x] 2.2 API Key 鉴权耗时：`apiKeyAuthWithBalance` 内埋点
- [x] 2.3 账号选择：`selectAccountThroughEngine` 埋点
- [x] 2.4 并发等待：`acquireResponsesUserSlot` / `acquireResponsesAccountSlot` 埋点
- [x] 2.5 上游：`httpUpstream.Do` 注入 `httptrace`（连接/复用、写入、首字节、响应头、流传输、尝试次数、流完整）
- [x] 2.6 Handler 结果与合并：成功/失败 outcome，`result.FirstOutputStages` 合并新旧分段

## 3. 前端

- [x] 3.1 `UsageRequestTimingModal.vue` 按 客户端 → Sub2API / Sub2API 内部 / Sub2API → 上游 / 请求结果 四组渲染，未采集显示「未采集」
- [x] 3.2 `FirstOutputStages` TS 类型扩展；zh/en i18n 新增键（含 outcome 映射）
- [x] 3.3 组件测试覆盖新分段与未采集占位

## 4. 门禁

- [x] 4.1 `go build ./...`；`go test ./internal/pkg/requesttiming/ ./internal/service/ ./internal/handler/ ./internal/repository/ ./internal/server/middleware/` 通过
- [x] 4.2 `npx vue-tsc --noEmit`、`pnpm build` 通过；vitest 失败集与基线一致

## 5. 发布

- [ ] 5.1 提交 + 前端 release + 后端热替换
- [ ] 5.2 线上真实流式请求核验 `first_output_stages` 新字段落库
