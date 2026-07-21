## ADDED Requirements

### Requirement: Grok 视频编辑 API

系统 SHALL 为 Grok 分组提供 OpenAI 风格的 `POST /v1/videos/edits` 和 `/videos/edits`，并复用现有媒体安全、调度、计费和异步任务粘连能力。

#### Scenario: 提交视频编辑
- **WHEN** 已授权用户通过 Grok 分组向视频编辑端点提交有效请求
- **THEN** 系统将请求转发至 xAI `/videos/edits` 并返回上游异步任务响应

#### Scenario: 非 Grok 分组拒绝
- **WHEN** 非 Grok 分组调用视频编辑端点
- **THEN** 系统返回与现有 Grok 视频生成入口一致的受控不支持响应

#### Scenario: 媒体安全与计费
- **WHEN** 视频编辑请求通过校验并被转发
- **THEN** 系统执行现有媒体审核、并发与计费资格检查，并记录视频用量元数据

#### Scenario: 编辑任务账号粘连
- **WHEN** 视频编辑提交成功并返回 request ID
- **THEN** 系统将该 request ID 绑定到提交账号，使后续视频状态查询路由到同一账号

#### Scenario: 上游失败切换
- **WHEN** 视频编辑上游返回现有媒体 failover 规则认可的可重试错误
- **THEN** 系统按照现有 Grok 媒体账号切换限制重试其他可用账号
