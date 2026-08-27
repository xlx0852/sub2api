package domain

// FirstOutputStages contains mutually exclusive gateway-side latency stages.
// Nil fields mean the stage was not observable for that transport.
type FirstOutputStages struct {
	GatewayPreUpstreamMs      *int `json:"gateway_pre_upstream_ms,omitempty"`
	UpstreamConnectMs         *int `json:"upstream_connect_ms,omitempty"`
	UpstreamFirstOutputMs     *int `json:"upstream_first_output_ms,omitempty"`
	RetryFailoverMs           *int `json:"retry_failover_ms,omitempty"`
	GatewayFirstOutputWriteMs *int `json:"gateway_first_output_write_ms,omitempty"`
	GatewayFirstOutputTotalMs *int `json:"gateway_first_output_total_ms,omitempty"`
}
