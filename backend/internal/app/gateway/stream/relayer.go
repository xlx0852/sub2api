package stream

// Relayer 是共享流中继阶段的端口：keepalive、interval timeout、
// 断连后 drain、usage 提取。具体 SSE/WS 实现在后续平台迁移中接入。
type Relayer interface {
	Relay() error
}
