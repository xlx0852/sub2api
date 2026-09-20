# Tasks

## 1. 钉选前校验模型可调

- [x] 1.1 `selectAccountWithSchedulerStrategy` 推导 `requiredCredentialClass` 的循环里，
      取到候选账号后先校验其是否支持 `requestedModel`（复用 `IsSchedulableForModel` /
      `accountSupportsOpenAICapabilities` 同等判定）；不支持则 `continue`，不钉其 class。
- [x] 1.2 三个钉选来源（sticky / stickyPrevious / userPin）统一走该校验；全部不支持时
      `requiredCredentialClass` 保持 `Unspecified`。

## 2. 回归测试

- [x] 2.1 新增用例：sticky 账号为 apikey 且不支持 `requestedModel`，候选中有支持该模型的
      oauth 号 → `requiredCredentialClass` 不被钉为 apikey，oauth 号可被选中。
- [x] 2.2 新增用例：sticky 账号支持 `requestedModel` → 仍钉其 class，行为不变。
- [x] 2.3 userPin / stickyPrevious 同样覆盖"不支持模型则不钉"。

## 3. 验证门禁

- [x] 3.1 `go build ./internal/...` 通过；`gofmt`/`go vet` 干净。
- [x] 3.2 相关调度测试通过。

## 4. 部署观测

- [ ] 4.1 部署后观察 `reject_trace` 中 `credential_class` 拒绝是否消失（luna/astra 模型）。
- [ ] 4.2 观察 routing-503（account_id 为空）频次下降。
