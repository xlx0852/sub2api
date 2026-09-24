package domain

// FirstOutputStages 是 usage_logs.first_output_stages（JSONB）的载荷。
//
// 字段来源分两类：
//   - 旧 6 段（GatewayPreUpstreamMs … GatewayFirstOutputTotalMs）由
//     service.FirstOutputStageTracker 产出，语义与历史记录保持一致；
//   - 其余分段由 pkg/requesttiming 采集，未采集时为 nil（前端显示「未采集」，
//     禁止用 0 冒充「没有耗时」）。
//
// 该列是 JSONB，新增字段不需要数据库迁移，旧记录自然缺字段。
type FirstOutputStages struct {
	GatewayPreUpstreamMs      *int `json:"gateway_pre_upstream_ms,omitempty"`
	UpstreamConnectMs         *int `json:"upstream_connect_ms,omitempty"`
	UpstreamFirstOutputMs     *int `json:"upstream_first_output_ms,omitempty"`
	RetryFailoverMs           *int `json:"retry_failover_ms,omitempty"`
	GatewayFirstOutputWriteMs *int `json:"gateway_first_output_write_ms,omitempty"`
	GatewayFirstOutputTotalMs *int `json:"gateway_first_output_total_ms,omitempty"`

	// 客户端 → Sub2API：入站请求体
	BodyFirstWaitMs *int   `json:"body_first_wait_ms,omitempty"`
	BodyReceiveMs   *int   `json:"body_receive_ms,omitempty"`
	BodyReadMs      *int   `json:"body_read_ms,omitempty"`
	BodyBytes       *int64 `json:"body_bytes,omitempty"`
	BodyComplete    *bool  `json:"body_complete,omitempty"`

	// Sub2API 内部
	APIKeyAuthMs      *int `json:"api_key_auth_ms,omitempty"`
	SecurityAuditMs   *int `json:"security_audit_ms,omitempty"`
	UserQueueMs       *int `json:"user_queue_ms,omitempty"`
	AccountSelectMs   *int `json:"account_select_ms,omitempty"`
	AccountQueueMs    *int `json:"account_queue_ms,omitempty"`
	HandlerBodyReadMs *int `json:"handler_body_read_ms,omitempty"`
	HandlerTotalMs    *int `json:"handler_total_ms,omitempty"`

	// Sub2API → 上游
	UpstreamAttempts          *int  `json:"upstream_attempts,omitempty"`
	ConnectionReused          *bool `json:"connection_reused,omitempty"`
	UpstreamWriteMs           *int  `json:"upstream_write_ms,omitempty"`
	UpstreamFirstByteMs       *int  `json:"upstream_first_byte_ms,omitempty"`
	UpstreamResponseHeadersMs *int  `json:"upstream_response_headers_ms,omitempty"`
	UpstreamStreamTransferMs  *int  `json:"upstream_stream_transfer_ms,omitempty"`

	// 请求结果
	ResponseStreamComplete *bool  `json:"response_stream_complete,omitempty"`
	Outcome                string `json:"outcome,omitempty"`
}
