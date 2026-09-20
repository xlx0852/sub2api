# relax-credential-class-pin

## 背景 / 问题

`selectAccountWithSchedulerStrategy` 在推导 `RequiredCredentialClass` 时，会从
`stickyAccountID` / `stickyPreviousAccountID` / `userPinAccountID` 中第一个非
`Unspecified` 的 credential class 取出并钉死（`openai_account_scheduler.go`）。该钉
选**不校验被钉账号是否支持当前 `RequestedModel`**。

结果：当 sticky/pin 落到一个 **apikey 类**账号（如 89/126），而请求模型
（`gpt-5.6-luna` / `gpt-6-astra`）只被 oauth 类账号（69/112/132）支持时——

```
112 → credential_class   (oauth 被 apikey 钉死)
69  → credential_class   (同上)
126 → requested_model    (apikey 不支持 astra/luna)
89  → requested_model    (同上)
```

全部候选被刷 → `routing` 空池 → 客户端收到 503 `Service temporarily unavailable`。

线上 ops_error_logs 证实该模式反复触发：11 点 61 条、13 点 72 条、14 点 17 条、
17 点 274 条 routing-503（account_id 为空，error_phase=routing）。

## 目标

钉选 credential class 时，**只有当被钉账号本身对请求模型可调**（通过
`IsSchedulableForModel` / 模型映射校验）才钉住该类；否则跳过该钉选来源，让
`RequiredCredentialClass` 回落 `Unspecified`，放开全类候选竞争。

## 非目标

- 不改变 sticky / previous_response / user_pin 的正常同号复用语义：被钉账号支持请求
  模型时行为完全不变。
- 不引入跨 credential class 的会话迁移偏好；仅去除"钉死一个不支持模型的 class"这一
  结构性空池成因。
- 不动 `OpenAICredentialClassMatches` 的判定语义（仍是硬门槛）。
