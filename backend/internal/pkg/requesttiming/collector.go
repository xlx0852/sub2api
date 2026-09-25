// Package requesttiming 采集单次请求的详细分段，用于「请求耗时详情」。
//
// 设计约束（改动前请先读）：
//   - 本包只负责新分段（入站请求体、网关内部、上游传输、结果），旧的 6 段由
//     service.FirstOutputStageTracker 继续产出，最后合并进同一个 JSONB；
//   - 所有耗时以「请求开始」为零点，输出毫秒整数；未采集用 nil 表示，
//     禁止用 0 冒充「没有耗时」；
//   - 采集器通过 context 传递，未接入的路径自然得到 nil（前端显示「未采集」）；
//   - 采集器只做内存累加，不发起 IO，可在热路径安全调用。
package requesttiming

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type collectorContextKey struct{}

// Collector 是一个请求级时间线。
type Collector struct {
	mu    sync.Mutex
	start time.Time

	// 入站请求体
	bodyFirstReadAt time.Time
	bodyLastReadAt  time.Time
	bodyReadBlocked time.Duration
	bodyBytes       int64
	bodyEOF         bool
	bodyReadStarted bool

	// 网关内部
	apiKeyAuthMs     *int
	securityAuditMs  *int
	userQueueMs      *int
	accountSelectMs  *int
	accountQueueMs   *int
	handlerTotalMs   *int
	upstreamReqBytes *int64

	// 上游
	attempts             int
	connReused           bool
	connReusedSet        bool
	upstreamConnect      *int
	upstreamWrite        *int
	upstreamWroteHeaders *int
	upstreamDial         *int
	upstreamTLS          *int
	upstreamDialed       *bool
	upstreamProto        string
	upstreamDo           *int
	upstreamBuild        *int
	upstreamFirst        *int
	upstreamHeaders      *int
	upstreamStream       *int
	streamComplete       bool
	streamDoneSet        bool

	outcome  string
	segments []domain.TimingSegment

	attemptStarts    map[int]time.Time
	lastAttemptStart time.Time
}

// New 创建一个以 start 为零点的采集器。
func New(start time.Time) *Collector {
	if start.IsZero() {
		start = time.Now()
	}
	return &Collector{start: start}
}

// WithCollector 把采集器放入 context。
func WithCollector(ctx context.Context, c *Collector) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, collectorContextKey{}, c)
}

// FromContext 取出采集器，未接入时返回 nil。
func FromContext(ctx context.Context) *Collector {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(collectorContextKey{}).(*Collector)
	return c
}

// StartedAt 返回零点。
func (c *Collector) StartedAt() time.Time {
	if c == nil {
		return time.Time{}
	}
	return c.start
}

func ms(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int(d.Milliseconds())
}

func intPtr(v int) *int { return &v }

func (c *Collector) store(slot **int, d time.Duration) {
	if c == nil {
		return
	}
	value := ms(d)
	c.mu.Lock()
	*slot = &value
	c.mu.Unlock()
}

// AddSegment 记录一个阶段区间（相对请求开始的偏移），供瀑布图渲染。
func (c *Collector) AddSegment(key, lane string, start, end time.Time) {
	if c == nil || key == "" || end.Before(start) {
		return
	}
	seg := domain.TimingSegment{
		Key:   key,
		Lane:  lane,
		Start: ms(start.Sub(c.start)),
		End:   ms(end.Sub(c.start)),
	}
	if seg.End < seg.Start {
		seg.End = seg.Start
	}
	c.mu.Lock()
	c.segments = append(c.segments, seg)
	c.mu.Unlock()
}

// AddAttemptSegment 记录一次上游尝试内的区间（带尝试序号）。
func (c *Collector) AddAttemptSegment(key string, attempt int, start, end time.Time) {
	c.AddSegment(key, "upstream", start, end)
	if c == nil {
		return
	}
	c.mu.Lock()
	if n := len(c.segments); n > 0 {
		c.segments[n-1].Attempt = attempt
	}
	c.mu.Unlock()
}

// Span 记录一个命名区间（返回结束函数，可安全重复调用）。
func (c *Collector) Span(slot **int) func() {
	if c == nil {
		return func() {}
	}
	start := time.Now()
	return func() { c.store(slot, time.Since(start)) }
}

// -- 入站请求体 ------------------------------------------------------------

// BodyRead 由包装后的请求体在每次 Read 后调用。
func (c *Collector) BodyRead(blocked time.Duration, n int, eof bool) {
	if c == nil {
		return
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.bodyReadStarted {
		c.bodyReadStarted = true
		c.bodyFirstReadAt = now
	}
	c.bodyLastReadAt = now
	c.bodyReadBlocked += blocked
	if n > 0 {
		c.bodyBytes += int64(n)
	}
	if eof {
		c.bodyEOF = true
	}
}

// MarkBodyComplete 标记请求体已被完整读取（读取方主动读完，但未观察到 EOF 时使用）。
func (c *Collector) MarkBodyComplete() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.bodyEOF = true
	c.mu.Unlock()
}

// -- 上游 ----------------------------------------------------------------

// Attempt 记一次出口尝试。
func (c *Collector) Attempt() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.attempts++
	if c.attemptStarts == nil {
		c.attemptStarts = make(map[int]time.Time)
	}
	c.attemptStarts[c.attempts] = time.Now()
	c.lastAttemptStart = c.attemptStarts[c.attempts]
	if c.attempts > 1 {
		// 尝试级标量只保留最后一次尝试：否则重试会与上一次尝试的时间点混在一
		// 起（曾出现「请求头写出早于取到连接」的负值）。按尝试的区间由
		// segments 数组单独记录，不受影响。
		c.upstreamConnect = nil
		c.upstreamDialed = nil
		c.upstreamDial = nil
		c.upstreamTLS = nil
		c.upstreamWroteHeaders = nil
		c.upstreamWrite = nil
		c.upstreamFirst = nil
		c.upstreamHeaders = nil
		c.upstreamStream = nil
		c.upstreamDo = nil
		c.upstreamBuild = nil
		c.connReusedSet = false
		c.streamDoneSet = false
	}
	c.mu.Unlock()
}

// AttemptCount 返回已记录的出口尝试次数。
func (c *Collector) AttemptCount() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempts
}

// UpstreamAttemptStart 返回第 attempt 次尝试的开始时刻（缺省用请求开始）。
func (c *Collector) UpstreamAttemptStart(attempt int) time.Time {
	if c == nil {
		return time.Now()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if started, ok := c.attemptStarts[attempt]; ok {
		return started
	}
	if !c.lastAttemptStart.IsZero() {
		return c.lastAttemptStart
	}
	return c.start
}

// SetConnectionReused 记录连接是否复用。
func (c *Collector) SetConnectionReused(reused bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.connReused = reused
	c.connReusedSet = true
	c.mu.Unlock()
}

// SetStreamComplete 记录响应流是否完整读到 EOF。
func (c *Collector) SetStreamComplete(complete bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.streamComplete = complete
	c.streamDoneSet = true
	c.mu.Unlock()
}

// SetOutcome 记录请求结果（success|failed|canceled|timeout|transport_error|incomplete）。
func (c *Collector) SetOutcome(outcome string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.outcome = outcome
	c.mu.Unlock()
}

// MarkUpstreamConnect 从请求开始到连接就绪。
func (c *Collector) MarkUpstreamConnect() { c.markSinceSlot(&c.upstreamConnect) }

// MarkUpstreamWrite 从请求开始到请求体写完。
func (c *Collector) MarkUpstreamWrite() { c.markSinceSlot(&c.upstreamWrite) }

// SetUpstreamDialMs 记录拨号（TCP 建连）耗时。
func (c *Collector) SetUpstreamDialMs(d time.Duration) { c.store(&c.upstreamDial, d) }

// SetUpstreamDialed 记录本次请求是否新建了连接（发生拨号）。
func (c *Collector) SetUpstreamDialed(dialed bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.upstreamDialed = &dialed
	c.mu.Unlock()
}

// MarkUpstreamDo 记录「请求开始 → 即将调用 client.Do」的时间点，
// 用于把请求开始到取到连接之间的时间拆成「调用前（handler/构造/客户端条目）」与
// 「连接获取」两段。
func (c *Collector) MarkUpstreamDo() { c.markSinceSlot(&c.upstreamDo) }

// ObserveUpstreamBuild 记录构造上游请求的耗时。
func (c *Collector) ObserveUpstreamBuild(d time.Duration) { c.store(&c.upstreamBuild, d) }

// SetUpstreamProto 记录上游响应使用的 HTTP 协议版本（HTTP/1.1、HTTP/2.0）。
func (c *Collector) SetUpstreamProto(proto string) {
	if c == nil || proto == "" {
		return
	}
	c.mu.Lock()
	c.upstreamProto = proto
	c.mu.Unlock()
}

// SetUpstreamTLSMs 记录 TLS 握手耗时。
func (c *Collector) SetUpstreamTLSMs(d time.Duration) { c.store(&c.upstreamTLS, d) }

// MarkUpstreamWroteHeaders 从请求开始到请求头写完。
func (c *Collector) MarkUpstreamWroteHeaders() { c.markSinceSlot(&c.upstreamWroteHeaders) }

// MarkUpstreamFirstByte 从请求开始到上游首字节。
func (c *Collector) MarkUpstreamFirstByte() { c.markSinceSlot(&c.upstreamFirst) }

// MarkUpstreamHeaders 从请求开始到响应头就绪。
func (c *Collector) MarkUpstreamHeaders() { c.markSinceSlot(&c.upstreamHeaders) }

// MarkUpstreamStream 从请求开始到响应流传输结束。
func (c *Collector) MarkUpstreamStream() { c.markSinceSlot(&c.upstreamStream) }

func (c *Collector) markSinceSlot(slot **int) {
	if c == nil {
		return
	}
	c.store(slot, time.Since(c.start))
}

// MarkHandlerEnd 记录 Handler 退出时刻（Handler 总耗时）。
// 必须在响应写出后调用；未调用时 Snapshot 用当前时间兜底。
func (c *Collector) MarkHandlerEnd(t time.Time) {
	if c == nil {
		return
	}
	if t.IsZero() {
		t = time.Now()
	}
	c.store(&c.handlerTotalMs, t.Sub(c.start))
}

// SetUpstreamRequestBytes 记录实际写往上游的请求体字节数（压缩后）。
func (c *Collector) SetUpstreamRequestBytes(n int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.upstreamReqBytes = &n
	c.mu.Unlock()
}

// -- 快照 ----------------------------------------------------------------

// Snapshot 输出新分段；未采集的字段保持 nil。
func (c *Collector) Snapshot(now time.Time) *domain.FirstOutputStages {
	if c == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	out := &domain.FirstOutputStages{}
	if c.bodyReadStarted {
		out.BodyFirstWaitMs = intPtr(ms(c.bodyFirstReadAt.Sub(c.start)))
		out.BodyReceiveMs = intPtr(ms(c.bodyLastReadAt.Sub(c.bodyFirstReadAt)))
		out.BodyReadMs = intPtr(ms(c.bodyReadBlocked))
		bytes := c.bodyBytes
		out.BodyBytes = &bytes
		complete := c.bodyEOF
		out.BodyComplete = &complete
	}
	out.APIKeyAuthMs = c.apiKeyAuthMs
	out.SecurityAuditMs = c.securityAuditMs
	out.UserQueueMs = c.userQueueMs
	out.AccountSelectMs = c.accountSelectMs
	out.AccountQueueMs = c.accountQueueMs
	out.HandlerBodyReadMs = out.BodyReadMs
	out.UpstreamRequestBytes = c.upstreamReqBytes
	if c.handlerTotalMs != nil {
		total := *c.handlerTotalMs
		out.HandlerTotalMs = &total
	} else {
		total := ms(now.Sub(c.start))
		out.HandlerTotalMs = &total
	}
	if c.attempts > 0 {
		attempts := c.attempts
		out.UpstreamAttempts = &attempts
	}
	if c.connReusedSet {
		reused := c.connReused
		out.ConnectionReused = &reused
	}
	out.UpstreamConnectMs = c.upstreamConnect
	out.UpstreamWriteMs = c.upstreamWrite
	out.UpstreamWroteHeadersMs = c.upstreamWroteHeaders
	out.UpstreamDialMs = c.upstreamDial
	out.UpstreamTLSMs = c.upstreamTLS
	out.UpstreamDialed = c.upstreamDialed
	out.UpstreamProto = c.upstreamProto
	out.UpstreamDoMs = c.upstreamDo
	out.UpstreamBuildMs = c.upstreamBuild
	out.UpstreamFirstByteMs = c.upstreamFirst
	out.UpstreamResponseHeadersMs = c.upstreamHeaders
	out.UpstreamStreamTransferMs = c.upstreamStream
	if c.streamDoneSet {
		complete := c.streamComplete
		out.ResponseStreamComplete = &complete
	}
	if c.outcome != "" {
		out.Outcome = c.outcome
	}
	segments := make([]domain.TimingSegment, 0, len(c.segments)+2)
	if c.bodyReadStarted {
		waitEnd := ms(c.bodyFirstReadAt.Sub(c.start))
		segments = append(segments,
			domain.TimingSegment{Key: "body_wait", Lane: "client", Start: 0, End: waitEnd},
			domain.TimingSegment{Key: "body_receive", Lane: "client", Start: waitEnd, End: ms(c.bodyLastReadAt.Sub(c.start))},
		)
	}
	segments = append(segments, c.segments...)
	if len(segments) > 0 {
		out.Segments = segments
	}
	return out
}

// Setters for the gateway-internal spans (they are timed by callers).
func (c *Collector) SetAPIKeyAuthMs(d time.Duration) {
	c.store(&c.apiKeyAuthMs, d)
	c.AddSegment("api_key_auth", "gateway", time.Now().Add(-d), time.Now())
}
func (c *Collector) SetSecurityAuditMs(d time.Duration) {
	c.store(&c.securityAuditMs, d)
	c.AddSegment("security_audit", "gateway", time.Now().Add(-d), time.Now())
}
func (c *Collector) SetUserQueueMs(d time.Duration) {
	c.store(&c.userQueueMs, d)
	c.AddSegment("user_queue", "gateway", time.Now().Add(-d), time.Now())
}
func (c *Collector) SetAccountSelectMs(d time.Duration) {
	c.store(&c.accountSelectMs, d)
	c.AddSegment("account_select", "gateway", time.Now().Add(-d), time.Now())
}
func (c *Collector) SetAccountQueueMs(d time.Duration) {
	c.store(&c.accountQueueMs, d)
	c.AddSegment("account_queue", "gateway", time.Now().Add(-d), time.Now())
}
