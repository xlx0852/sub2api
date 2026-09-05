# Change: 拆分 Provider Handler 生命周期编排

## Why

Gateway 与 OpenAI handler 仍包含数百行级别的请求处理函数，把校验、账号选择、重试、流式转发和结算交织在一个函数中。后续修复容易遗漏 provider 分支，也难以验证资源释放和结算交接边界。

## What Changes

- 将 Gateway/OpenAI 的请求入口收敛为显式生命周期编排：parse → admission → select → rewrite → transport → relay → settle。
- 把 provider 差异限制在请求改写、上游传输、响应解析、错误分类和 usage extractor 适配器。
- 将账号 lease、客户端断连 drain、durable settlement handoff 作为显式阶段输出，不改变对外协议。
- 先迁移一个 HTTP/SSE 主路径并建立旧/新差分测试，再逐步迁移 WS、图片和兼容入口。

## Impact

- Affected specs: `architecture-governance`, `usage-billing-core`
- Affected code: `backend/internal/handler`, `backend/internal/service`, `backend/internal/app/gateway/pipeline`
- **BREAKING**: 无对外 API、配置、计费口径或默认部署形态变更。

