package pipeline

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// StageName 标识管道生命周期阶段（design.md D3）。
type StageName string

const (
	StageParse     StageName = "parse"
	StageAdmission StageName = "admission"
	StageSelect    StageName = "select"
	StageRewrite   StageName = "rewrite"
	StageTransport StageName = "transport"
	StageRelay     StageName = "relay"
	StageSettle    StageName = "settle"
)

// Exchange 是一次网关转发的可变上下文。
// 协议细节留在 provider 适配器中，管道只持有跨阶段共享的生命周期字段。
type Exchange struct {
	Ctx            context.Context
	Platform       string
	RequestedModel string
	Account        *domain.Account
	Group          *domain.Group
	Inbound        []byte
	Outbound       []byte
	UpstreamModel  string
	Values         map[string]any
	StageDuration  map[StageName]int64
	Err            error
}

// Set 写入 provider 私有状态。管道本身不解释 key。
func (ex *Exchange) Set(key string, value any) {
	if ex == nil {
		return
	}
	if ex.Values == nil {
		ex.Values = make(map[string]any)
	}
	ex.Values[key] = value
}

// Get 读取 provider 私有状态。
func (ex *Exchange) Get(key string) any {
	if ex == nil || ex.Values == nil {
		return nil
	}
	return ex.Values[key]
}

// Stage 是管道中的一个生命周期步骤。
type Stage interface {
	Name() StageName
	Run(ex *Exchange) error
}

// Provider 由各上游平台实现：协议相关的 Parse/Rewrite 与 usage 提取。
// Select / Admission / Relay / Settle 由共享阶段提供。
type Provider interface {
	Platform() string
	Parse(ex *Exchange) error
	Rewrite(ex *Exchange) error
	ExtractUsage(ex *Exchange) error
}
