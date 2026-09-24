# Change: 使用记录请求耗时详情

## Why

当前使用记录表已经展示入口/上游路径、首 Token 和总耗时，但排查单次慢请求时仍需要人工拼接多个字段，无法像运维请求详情一样快速区分：

- 客户端请求路径与请求模型；
- 网关首字前各阶段耗时；
- 上游连接、等待首输出、首字写出；
- 总耗时与输出速率；
- 映射后的上游模型与请求结果。

usage_logs 已经持久化 `duration_ms`、`first_token_ms`、`first_output_stages`、`inbound_endpoint`、`upstream_endpoint`、`model_mapping_chain` 等信息，但普通 usage DTO 和使用记录 UI 没有把 `first_output_stages` 展示出来。

## What Changes

- 普通用户和管理员 usage log DTO 增加只读 `first_output_stages` 结构。
- 使用记录表支持点击请求 ID/记录打开“请求耗时详情”弹窗。
- 弹窗展示：请求模型、请求 ID、时间、入口/上游路径、首 Token、总耗时、输出速率、网关阶段、上游阶段、缓存/Token/映射摘要和成功状态。
- 对历史数据或当前记录中不存在的阶段字段显示 `-`，不得根据其它耗时猜造细分阶段。
- 管理员与普通用户复用同一展示组件；管理员可以看到已有管理员字段，普通用户只看到自身授权范围内的 usage 数据。

## Impact

- Affected spec: `account-usage-presentation`。
- Backend: usage DTO/mapping only；不改数据库 schema，不新增请求路径。
- Frontend: usage table 增加详情入口和请求耗时详情 modal。
- Existing list/filter/export behavior remains unchanged。
