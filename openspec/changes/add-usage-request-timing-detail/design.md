# Design: Usage Request Timing Detail

## D1 数据来源

复用已有 usage_logs 字段，不新增 SQL：

- `duration_ms`：请求总耗时；
- `first_token_ms`：首 Token 延迟；
- `first_output_stages`：网关/上游首输出阶段；
- `inbound_endpoint` / `upstream_endpoint`：请求路径；
- `requested_model` / `upstream_model` / `model_mapping_chain`：模型与映射；
- token/cache 字段：结果摘要。

`first_output_stages` 缺失时各阶段显示 `-`。

## D2 UI 交互

- UsageTable 的 request ID 单元格保持复制功能，并增加详情按钮/点击入口；
- 详情组件接收完整 `UsageLog`，不额外请求接口，避免点击时产生新的后端负载；
- 弹窗分为：摘要卡、客户端/路径、网关内部、上游、请求结果；
- 详情组件不显示 request body、凭据或上游错误原文。

## D3 阶段语义

`first_output_stages` 的现有语义保持不变：

- `gateway_pre_upstream_ms`：网关进入上游前；
- `upstream_connect_ms`：建立/获取上游连接；
- `upstream_first_output_ms`：上游等待首输出；
- `retry_failover_ms`：失败重试/切换耗时；
- `gateway_first_output_write_ms`：网关首字写出；
- `gateway_first_output_total_ms`：网关首输出总耗时。

展示层只做格式化，不把这些值相加成新的“伪精确”耗时。

## D4 兼容性

- DTO 字段使用 `omitempty`，兼容历史记录；
- 用户接口与管理员接口同时下发该非敏感字段；
- 无路由、数据库 schema 或 usage 写入变更；
- 详情入口在移动端使用同一 modal 布局。
