# account-availability Specification Delta

## ADDED Requirements

### Requirement: 候选过滤决策可观测

当选号候选集为空时，系统 SHALL 输出每个平台级候选被哪条调度 policy 拒绝的明细（账号 ID + 拒绝的 policy 名），MUST NOT 依赖人工逐字段排查来定位"无可用账号"的根因。

#### Scenario: 空候选输出拒绝明细
- **WHEN** 分组内账号经前置过滤后无剩余候选
- **THEN** 日志包含每个候选被拒绝的具体 policy（schedulable/runtime_blocked/requested_model/transport/capability 等）及请求模型、compact、能力要求上下文

### Requirement: 传输层故障不触发票失效

上游传输层错误（proxy/DNS/TCP/TLS/连接超时，无真实 HTTP 响应）SHALL 通过显式标记与真实上游 5xx 区分，MUST NOT 计入连续 5xx 阈值或触发 turn-state 票 invalidate/重抓。

#### Scenario: 代理断开不清票
- **WHEN** 账号因代理连接超时失败且上游未返回 HTTP 响应
- **THEN** 该失败不计入票失效判定，账号已有 turn-state 票保留

#### Scenario: 真实上游 5xx 仍清票
- **WHEN** 账号收到上游真实返回的 HTTP 5xx 且达到连续阈值
- **THEN** 按既有阈值 invalidate 票并触发重采

### Requirement: 无候选错误区分层级

选号返回"无可用账号"时 SHALL 区分缺失层级（平台级候选池空 / 前置过滤全刷 / 模型或能力过滤 / acquire 复核失败），错误信息与日志 MUST 携带该原因。

#### Scenario: 模型无映射给出可定位错误
- **WHEN** 分组内有平台级可调度账号但均被模型映射/能力要求刷掉
- **THEN** 错误标记为模型/能力级缺失而非笼统的无账号
