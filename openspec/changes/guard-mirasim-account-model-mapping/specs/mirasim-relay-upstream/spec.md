# Mirasim Relay Upstream

## ADDED Requirements

### Requirement: Mirasim 账号的空模型映射必须对管理端可见

管理端账号响应（列表与详情，即 `dto.AccountFromServiceShallow` 派生出的所有响应）
SHALL 包含只读派生布尔字段 `mirasim_model_mapping_missing`。

该字段 SHALL 仅在账号**同时**满足以下条件时为 `true`：

- 平台为 `anthropic` 且类型为 `apikey`；
- 未被 `extra.mirasim_relay_enabled=false` 显式关闭（kill switch）；
- `credentials.mirasim_refresh_token` 非空（即服务层权威判定
  `isMirasimRelayAccount` 为真）；
- 账号解析后的 `model_mapping` 为空（`map[string]string` 长度为 0）。

其它任何情况 SHALL 为 `false`（含普通 anthropic apikey 账号、其它平台账号、
以及已配置非空映射的 mirasim 账号）。

该字段 SHALL 由服务层凭据原文在进程内计算得出，MUST NOT 泄漏任何凭据值，
MUST NOT 改变 `credentials` / `credentials_status` 的既有脱敏语义。

`model_mapping` 为空这一状态本身 MUST NOT 被自动改写：本变更只做「暴露 + 显式操作」，
不改变 `IsModelSupported` 对空映射的历史语义（空映射仍等价于「允许所有模型」）。

#### Scenario: mirasim 账号缺少映射时被标记

- **WHEN** 管理端请求账号列表或账号详情，且该账号为 mirasim relay 账号
  （`platform=anthropic`、`type=apikey`、`extra.mirasim_relay_enabled` 未置 false、
  `credentials.mirasim_refresh_token` 非空）且 `model_mapping` 为空
- **THEN** 响应中该账号的 `mirasim_model_mapping_missing` SHALL 为 `true`
- **AND** 响应 MUST NOT 包含 `mirasim_refresh_token` / `mirasim_device_key` 等凭据明文
  （与既有脱敏规则一致）

#### Scenario: 已配置映射的 mirasim 账号不被标记

- **WHEN** 该 mirasim relay 账号已配置至少一条 `model_mapping`（例如 5 条 identity 映射）
- **THEN** `mirasim_model_mapping_missing` SHALL 为 `false`

#### Scenario: 非 mirasim 账号恒不被标记

- **WHEN** 账号是普通 anthropic apikey（无 `mirasim_refresh_token`）、
  或 `extra.mirasim_relay_enabled=false`、或其它平台账号，即使 `model_mapping` 为空
- **THEN** `mirasim_model_mapping_missing` SHALL 为 `false`

### Requirement: 管理端必须支持以账号可用模型一键生成模型映射

系统 SHALL 提供管理端端点 `POST /api/v1/admin/accounts/:id/models/apply-mapping`，
把该账号的上游可用模型清单写成 **identity** 模型映射。

端点 SHALL ：

- 使用与「同步上游模型」相同的**账号维度**只读模型清单来源
  （mirasim relay 为 `GET /v1/models`，`force=true` 以绕过缓存），
  MUST NOT 使用全局 `/v1/model-roster`；
- 生成的映射 SHALL 为 `{modelID: modelID}`，键与值都取自该清单（去空白、去重）；
- 仅在该账号为 mirasim relay 账号（服务层权威判定）时才允许执行；
- 通过服务层账号更新路径写入 `credentials.model_mapping`，
  并**保留**该账号其它 non-sensitive 凭据子键与全部 sensitive 凭据子键不变；
- 返回写入后的模型清单与是否发生变更（`changed`）。

端点 SHALL 拒绝以下情况，且 MUST NOT 落库：

- 账号不存在 → `404`；
- 非 mirasim relay 账号 → `400`；
- 上游模型清单为空（或上游请求失败）→ 非 2xx 错误
  （**绝不允许**把 `model_mapping` 写成空映射）；
- 账号已有非空 `model_mapping` 且请求未显式携带 `overwrite=true` → `409`。

端点 SHALL 幂等：当目标 identity 映射与账号现值完全一致时，返回成功且 `changed=false`，
不再写库（因此「先 409、再带 `overwrite=true` 重试、再重试」中的最后一次调用返回
`changed=false`）。

日志 SHALL 只包含账号 ID、模型数量与错误类别，MUST NOT 记录凭据或上游响应正文。

#### Scenario: 空映射的 mirasim 账号被一键填满

- **WHEN** mirasim relay 账号 `model_mapping` 为空，管理员调用该端点（不带 `overwrite`）
- **THEN** 系统 SHALL 以账号维度 `GET /v1/models` 的返回集合生成 identity 映射并落库
- **AND** 响应 SHALL 返回该模型清单与 `changed=true`
- **AND** 该账号的 `mirasim_refresh_token` / `mirasim_device_key` / 其它非敏感凭据
  SHALL 保持不变

#### Scenario: 重复调用是幂等的

- **WHEN** 管理员对已经拥有与该清单一致的 identity 映射的账号再次调用该端点
- **THEN** 响应 SHALL 成功且 `changed=false`
- **AND** 系统 MUST NOT 再次写库

#### Scenario: 已有非空映射时默认拒绝覆盖

- **WHEN** 账号已有非空 `model_mapping`（内容与上游清单不一致），
  调用时未携带 `overwrite=true`
- **THEN** 系统 SHALL 返回 `409`，且账号 `model_mapping` MUST NOT 被修改

#### Scenario: 显式 overwrite 才允许覆盖

- **WHEN** 调用携带 `overwrite=true`
- **THEN** 系统 SHALL 用新的 identity 映射覆盖旧映射并返回 `changed=true`

#### Scenario: 空上游清单必须拒绝写入

- **WHEN** 上游模型清单为空或上游请求失败
- **THEN** 系统 SHALL 返回非 2xx 错误且 MUST NOT 写入（尤其是 MUST NOT 写入空映射）

#### Scenario: 非 mirasim 账号必须拒绝

- **WHEN** 对普通 anthropic apikey 账号或其它平台账号调用该端点
- **THEN** 系统 SHALL 返回 `400` 且 MUST NOT 修改账号

#### Scenario: 上游失败时响应不泄漏上游正文

- **WHEN** 上游模型清单请求失败（网络错误、非 2xx、响应过大或非 JSON）
- **THEN** 响应 SHALL 使用固定文案（含 HTTP 状态类别即可），
  MUST NOT 回显上游响应正文

### Requirement: 前端必须暴露 mirasim 账号空映射风险并可一键修复

管理端编辑账号弹窗（`EditAccountModal.vue`）SHALL 在账号满足
`mirasim_model_mapping_missing === true` 时显示风险提示，说明：
当前账号没有任何模型映射，等价于「认领所有模型」，可能被调度到上游不支持的模型
（上游 422，进而表现为网关 502）。

弹窗 SHALL 同时提供「以可用模型填充映射」按钮，调用上述端点后：

- 成功时 SHALL 用返回的模型清单回填模型选择器（白名单/映射），并提示成功或「无变化」；
- 失败时 SHALL 显示错误提示，且不改变当前编辑态。

该提示 SHALL 在映射非空后消失（保存后重新载入即由后端派生字段驱动）。

#### Scenario: 空映射账号打开编辑弹窗即可见风险

- **WHEN** 管理员打开一个 `mirasim_model_mapping_missing === true` 的账号的编辑弹窗
- **THEN** 弹窗 SHALL 显示空映射风险提示与「以可用模型填充映射」按钮

#### Scenario: 一键填充成功后回填选择器

- **WHEN** 管理员点击「以可用模型填充映射」且端点返回非空模型清单
- **THEN** 弹窗 SHALL 用该清单回填模型选择器
- **AND** SHALL 给出成功提示（清单为空或忽略时给出对应提示）

#### Scenario: 一键填充失败不破坏编辑态

- **WHEN** 端点返回错误（例如上游不可达）
- **THEN** 弹窗 SHALL 显示错误提示且 MUST NOT 清空或改写当前模型选择器内容
