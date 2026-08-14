package scheduling

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestEvaluateCandidateAllowsSharedBasePolicies(t *testing.T) {
	account := &domain.Account{
		ID:          7,
		Platform:    "openai",
		Status:      domain.StatusActive,
		Schedulable: true,
	}
	req := Request{Platform: "openai", RequestedModel: "gpt-5", ExcludedIDs: map[int64]struct{}{8: {}}}

	decision := EvaluateCandidate(
		context.Background(),
		account,
		req,
		NotExcludedPolicy,
		PlatformPolicy,
		SchedulableForModelPolicy,
	)

	require.Equal(t, PolicyDecision{Allowed: true}, decision)
}

func TestEvaluateCandidateStopsAtFirstRejectedPolicy(t *testing.T) {
	account := &domain.Account{ID: 7, Platform: "openai", Status: domain.StatusActive, Schedulable: true}
	req := Request{Platform: "grok", ExcludedIDs: map[int64]struct{}{7: {}}}

	decision := EvaluateCandidate(
		context.Background(),
		account,
		req,
		NotExcludedPolicy,
		PlatformPolicy,
		SchedulablePolicy,
	)

	require.Equal(t, PolicyDecision{RejectedPolicy: "not_excluded"}, decision)
}

func TestEvaluateCandidateRejectsUnschedulableAccount(t *testing.T) {
	account := &domain.Account{ID: 7, Platform: "openai", Status: domain.StatusError, Schedulable: true}

	decision := EvaluateCandidate(
		context.Background(),
		account,
		Request{},
		SchedulablePolicy,
	)

	require.Equal(t, PolicyDecision{RejectedPolicy: "schedulable"}, decision)
}

func TestNewCandidatePolicyKeepsPlatformRuleExplicit(t *testing.T) {
	policy := NewCandidatePolicy("custom", func(_ context.Context, account *domain.Account, req Request) bool {
		return account != nil && account.Type == req.RequiredCapability
	})
	account := &domain.Account{Type: "oauth"}

	decision := EvaluateCandidate(
		context.Background(),
		account,
		Request{RequiredCapability: "apikey"},
		policy,
	)

	require.Equal(t, PolicyDecision{RejectedPolicy: "custom"}, decision)
}

func TestGroupMembershipPolicyPreservesSnapshotRepresentationMode(t *testing.T) {
	groupID := int64(9)
	account := &domain.Account{GroupIDs: []int64{groupID}}

	legacy := EvaluateCandidate(
		context.Background(),
		account,
		Request{GroupID: &groupID},
		GroupMembershipPolicy,
	)
	snapshot := EvaluateCandidate(
		context.Background(),
		account,
		Request{GroupID: &groupID, IncludeFlatGroupIDs: true},
		GroupMembershipPolicy,
	)
	ungroupedSnapshot := EvaluateCandidate(
		context.Background(),
		account,
		Request{IncludeFlatGroupIDs: true},
		GroupMembershipPolicy,
	)

	require.Equal(t, PolicyDecision{RejectedPolicy: "group_membership"}, legacy)
	require.Equal(t, PolicyDecision{Allowed: true}, snapshot)
	require.Equal(t, PolicyDecision{RejectedPolicy: "group_membership"}, ungroupedSnapshot)
}
