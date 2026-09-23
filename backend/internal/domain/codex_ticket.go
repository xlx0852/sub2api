package domain

// CodexTicketSummary 是账号详情页展示「打票」状态用的**派生摘要**。
//
// 为什么是派生而不是原始值：`x-codex-turn-state` 是上游签发的会话令牌，采集到的
// cookie 同样是会话令牌。本仓对 credentials 的既有约定是**不下发前端**、只给存在性
// （见 dto.RedactCredentials），票与 cookie 必须遵守同一条规则。
// 所以这里只给形态（票长）、时长、池位、cookie 数量——足够看清"打票"状态，
// 但拿不到任何可用于冒充该账号的令牌。
type CodexTicketSummary struct {
	// Pools 是按供应池归槽的票状态（真实落点池，见 service 侧归槽说明）。
	Pools []CodexTicketPool `json:"pools,omitempty"`
	// Cookies 是上游签发的会话 cookie 的**数量与新鲜度**，不含任何值。
	Cookies *CodexTicketCookies `json:"cookies,omitempty"`
	// ReplayTTLSeconds 是当前回放窗口（秒）。票龄超过它就不再回放。
	ReplayTTLSeconds int64 `json:"replay_ttl_seconds,omitempty"`
	// OverloadRotationEnabled 表示该账号是否开启了过载后身份轮换（账号级开关）。
	OverloadRotationEnabled bool `json:"overload_rotation_enabled,omitempty"`
}

// CodexTicketPool 是单个池槽位的状态。
type CodexTicketPool struct {
	Pool string `json:"pool"`
	// Model 是采集时记录的服务模型（可能为空）。
	Model string `json:"model,omitempty"`
	// Shape 是票的形态，即 `x-codex-turn-state` 的字符长度。
	// 实测对应关系（同账号同期）：astra↔292、luna↔312、sol↔312、terra↔312；
	// 跨账号并不一一对应，所以它只能当形态看，不能当池的标识。
	Shape int `json:"shape"`
	// AgeSeconds 是票龄（当前时间 - 采集时间）。
	AgeSeconds int64 `json:"age_seconds"`
	// Fresh 表示仍在回放窗口内、会被真正发给上游。
	Fresh bool `json:"fresh"`
	// CapturedAt 是采集时刻（RFC3339）。
	CapturedAt string `json:"captured_at,omitempty"`
}

// CodexTicketCookies 是会话 cookie 的聚合状态（无值）。
type CodexTicketCookies struct {
	// Count 是仍在有效期内的 cookie 数量。
	Count int `json:"count"`
	// Fresh 表示至少有一个 cookie 可回放。
	Fresh bool `json:"fresh"`
	// AgeSeconds 是最新一次刷新距今的秒数。
	AgeSeconds int64 `json:"age_seconds,omitempty"`
	// Egress 是采集时的出口指纹（协议/主机/端口的哈希前缀），不含代理地址与凭据。
	Egress string `json:"egress,omitempty"`
}
