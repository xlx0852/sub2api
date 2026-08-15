package scheduling

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Request 是统一账号调度的输入（design.md D4）。
type Request struct {
	Platform                string
	RequestedModel          string
	GroupID                 *int64
	Group                   *domain.Group
	StickyKey               string
	Sticky                  bool
	ExcludedIDs             map[int64]struct{}
	RequireWS               bool
	RequireCompact          bool
	RequiredCapability      string
	RequiredImageCapability string
	RequiredTransport       string
	MetadataUserID          string
	Sub2APIUserID           int64
	ForcePlatform           string
	AllowMixedPlatform      bool
	IncludeFlatGroupIDs     bool
	PreviousResponseID      string
	PreviousResponseCanMove bool
	// SkipAcquire is used by read-only routing probes such as count_tokens. The
	// platform strategy still runs policies and affinity but uses a passthrough slot.
	SkipAcquire bool
}

// WaitPlan 描述选号未立即拿到并发槽时的等待参数。
// 字段与现有 AccountWaitPlan 对齐，便于两套调度器无损适配。
type WaitPlan struct {
	AccountID      int64
	MaxConcurrency int
	Timeout        time.Duration
	MaxWaiting     int
}

// Result 是统一选号出口。Release 对应现有 ReleaseFunc。
type Result struct {
	Account      *domain.Account
	Acquired     bool
	Release      func()
	WaitPlan     *WaitPlan
	ModelLimited bool
}

// Outcome 是统一调度编排器的完整输出。Decision 由平台策略填充，
// 仅供现有的观测/重试逻辑读取，不参与通用选号语义。
type Outcome struct {
	Result   *Result
	Decision any
}

// Strategy 封装平台差异。通用编排只负责选择策略和统一输出，
// 粘性、quota、transport 等平台规则仍由显式策略插件实现。
type Strategy interface {
	Name() string
	Supports(Request) bool
	Select(context.Context, Request) (*Outcome, error)
}

var ErrNoStrategy = errors.New("no scheduling strategy supports request")

// Engine 是唯一 AccountSelector 实现，按注册顺序把请求交给首个支持的平台策略。
type Engine struct {
	strategies []Strategy
}

func NewEngine(strategies ...Strategy) *Engine {
	registered := make([]Strategy, 0, len(strategies))
	for _, strategy := range strategies {
		if strategy != nil {
			registered = append(registered, strategy)
		}
	}
	return &Engine{strategies: registered}
}

func (e *Engine) Select(ctx context.Context, req Request) (*Result, error) {
	outcome, err := e.SelectOutcome(ctx, req)
	if err != nil || outcome == nil {
		return nil, err
	}
	return outcome.Result, nil
}

func (e *Engine) SelectOutcome(ctx context.Context, req Request) (*Outcome, error) {
	if e != nil {
		for _, strategy := range e.strategies {
			if strategy.Supports(req) {
				return strategy.Select(ctx, req)
			}
		}
	}
	return nil, ErrNoStrategy
}

// AccountSelector 是所有平台共用的选号入口。
type AccountSelector interface {
	Select(ctx context.Context, req Request) (*Result, error)
}

var _ AccountSelector = (*Engine)(nil)
