package stream

// Event 是 transport-neutral 的流事件。Data 由 provider adapter 编码，
// Sink 只负责把已经编码的事件交给具体传输层。
type Event struct {
	Kind string
	Data []byte
}

// Sink 是网关 relay 到 HTTP/SSE/WS 的最小输出边界。
// service 不应依赖 Gin、ResponseWriter 或具体 frame 类型。
type Sink interface {
	Emit(Event) error
	Flush() error
}
