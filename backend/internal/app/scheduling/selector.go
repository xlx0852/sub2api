package scheduling

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Request 是统一账号调度的输入（design.md D4）。
type Request struct {
	Platform       string
	RequestedModel string
	Group          *domain.Group
	StickyKey      string
	RequireWS      bool
	RequireCompact bool
}

// AccountSelector 是所有平台共用的选号入口。
// 现有两套调度器将先适配本接口（行为不变），再逐策略合并。
type AccountSelector interface {
	Select(ctx context.Context, req Request) (*domain.Account, error)
}
