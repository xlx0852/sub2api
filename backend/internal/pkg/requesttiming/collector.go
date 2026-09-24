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
	apiKeyAuthMs    *int
	userQueueMs     *int
	accountSelectMs *int
	accountQueueMs  *int
	handlerTotalMs  *int

	// 上游
	attempts        int
	connReused      bool
	connReusedSet   bool
	upstreamConnect *int
	upstreamWrite   *int
	upstreamFirst   *int
	upstreamHeaders *int
	upstreamStream  *int
	streamComplete  bool
	streamDoneSet   bool

	outcome string
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
	c.mu.Unlock()
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
	out.UserQueueMs = c.userQueueMs
	out.AccountSelectMs = c.accountSelectMs
	out.AccountQueueMs = c.accountQueueMs
	out.HandlerBodyReadMs = out.BodyReadMs
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
	return out
}

// Setters for the gateway-internal spans (they are timed by callers).
func (c *Collector) SetAPIKeyAuthMs(d time.Duration)    { c.store(&c.apiKeyAuthMs, d) }
func (c *Collector) SetUserQueueMs(d time.Duration)     { c.store(&c.userQueueMs, d) }
func (c *Collector) SetAccountSelectMs(d time.Duration) { c.store(&c.accountSelectMs, d) }
func (c *Collector) SetAccountQueueMs(d time.Duration)  { c.store(&c.accountQueueMs, d) }
