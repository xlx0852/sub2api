package scheduling

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// CandidatePolicy 是单个候选账号门槛策略。
// 平台专属的 quota、RPM、transport 等规则由 service 适配器显式提供，
// 这里只承载两套调度器语义完全一致的基础规则。
type CandidatePolicy interface {
	Name() string
	Allows(ctx context.Context, account *domain.Account, req Request) bool
}

type candidatePolicyFunc struct {
	name string
	fn   func(context.Context, *domain.Account, Request) bool
}

// NewCandidatePolicy 将平台显式规则适配为策略插件。
func NewCandidatePolicy(name string, fn func(context.Context, *domain.Account, Request) bool) CandidatePolicy {
	return candidatePolicyFunc{name: name, fn: fn}
}

func (p candidatePolicyFunc) Name() string {
	return p.name
}

func (p candidatePolicyFunc) Allows(ctx context.Context, account *domain.Account, req Request) bool {
	return p.fn(ctx, account, req)
}

// PolicyDecision 保留首个拒绝门槛，便于后续统一失败诊断而不改变选号结果。
type PolicyDecision struct {
	Allowed        bool
	RejectedPolicy string
}

// EvaluateCandidate 按声明顺序执行门槛策略，首个拒绝即停止。
func EvaluateCandidate(ctx context.Context, account *domain.Account, req Request, policies ...CandidatePolicy) PolicyDecision {
	for _, policy := range policies {
		if policy == nil {
			continue
		}
		if !policy.Allows(ctx, account, req) {
			return PolicyDecision{RejectedPolicy: policy.Name()}
		}
	}
	return PolicyDecision{Allowed: true}
}

var (
	NotExcludedPolicy CandidatePolicy = candidatePolicyFunc{
		name: "not_excluded",
		fn: func(_ context.Context, account *domain.Account, req Request) bool {
			return account != nil && !IsExcluded(req.ExcludedIDs, account.ID)
		},
	}
	PlatformPolicy CandidatePolicy = candidatePolicyFunc{
		name: "platform",
		fn: func(_ context.Context, account *domain.Account, req Request) bool {
			return account != nil && account.Platform == req.Platform
		},
	}
	GroupMembershipPolicy CandidatePolicy = candidatePolicyFunc{
		name: "group_membership",
		fn: func(_ context.Context, account *domain.Account, req Request) bool {
			if account == nil {
				return false
			}
			if req.GroupID == nil {
				if len(account.AccountGroups) != 0 {
					return false
				}
				return !req.IncludeFlatGroupIDs || len(account.GroupIDs) == 0
			}
			if req.IncludeFlatGroupIDs {
				for _, groupID := range account.GroupIDs {
					if groupID == *req.GroupID {
						return true
					}
				}
			}
			for _, accountGroup := range account.AccountGroups {
				if accountGroup.GroupID == *req.GroupID {
					return true
				}
			}
			return false
		},
	}
	SchedulablePolicy CandidatePolicy = candidatePolicyFunc{
		name: "schedulable",
		fn: func(_ context.Context, account *domain.Account, _ Request) bool {
			return account != nil && account.IsSchedulable()
		},
	}
	SchedulableForModelPolicy CandidatePolicy = candidatePolicyFunc{
		name: "schedulable_for_model",
		fn: func(ctx context.Context, account *domain.Account, req Request) bool {
			return account != nil && account.IsSchedulableForModelWithContext(ctx, req.RequestedModel)
		},
	}
)
