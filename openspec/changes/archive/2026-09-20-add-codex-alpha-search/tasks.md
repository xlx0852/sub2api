## 1. Tests First

- [x] 1.1 增加三种 alpha/search 入站路径归一化和路由注册测试
- [x] 1.2 增加非 OpenAI 分组拒绝测试
- [x] 1.3 增加 OAuth wire、查询参数和未知字段透明保留测试
- [x] 1.4 增加 API Key 自定义 base URL、模型映射、错误透传和 failover 测试

## 2. Implementation

- [x] 2.1 增加 alpha/search endpoint 常量、归一化与上游 endpoint 记录
- [x] 2.2 注册 `/v1/alpha/search`、`/alpha/search` 和 Codex direct alias
- [x] 2.3 实现独立 AlphaSearch handler 的认证、计费资格、并发、调度和 failover
- [x] 2.4 实现 OAuth/API Key alpha/search 请求构建、URL 校验、头部处理和响应透传
- [x] 2.5 保持 Responses、compact、WS、Grok 和非 OpenAI 分组行为不变

## 3. Verification

- [x] 3.1 运行 endpoint、routes、AlphaSearch handler/service 定向测试
- [x] 3.2 运行 `go test ./internal/service` 和 `go test ./cmd/server`
- [x] 3.3 验证 OpenSpec 变更并复查未提交 Grok 改动未被混入
