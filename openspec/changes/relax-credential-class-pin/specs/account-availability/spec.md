# account-availability Specification Delta

## ADDED Requirements

### Requirement: credential class 钉选须校验模型可调

选号从 sticky / previous_response / user_pin 来源推导 `RequiredCredentialClass` 时，
SHALL 先校验该来源账号对 `RequestedModel` 可调；仅当支持时才钉住其 credential class。
当来源账号不支持请求模型时 MUST 跳过该钉选，`RequiredCredentialClass` 回落
`Unspecified`，使其它 credential class 的可调账号能参与竞争。

#### Scenario: sticky 不支持模型则不钉 class
- **WHEN** sticky 绑定账号为 apikey 类且不支持 `gpt-6-astra`，而分组内 oauth 类账号
  支持 `gpt-6-astra`
- **THEN** `RequiredCredentialClass` 不钉为 apikey，oauth 账号不被 `credential_class`
  门槛拒绝，可被选中而非空池 503

#### Scenario: sticky 支持模型时行为不变
- **WHEN** sticky 绑定账号支持 `RequestedModel`
- **THEN** 仍钉住其 credential class，候选过滤与既有语义一致

#### Scenario: user_pin / previous_response 同源校验
- **WHEN** user_pin 或 previous_response 推导的钉选账号不支持 `RequestedModel`
- **THEN** 同样跳过钉选，不因钉死不支持模型的 class 造成空池
