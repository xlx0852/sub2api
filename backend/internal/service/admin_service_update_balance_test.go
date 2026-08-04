//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type balanceUserRepoStub struct {
	*userRepoStub
	updateErr error
	updated   []*User
}

func (s *balanceUserRepoStub) Update(ctx context.Context, user *User) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	if user == nil {
		return nil
	}
	clone := *user
	s.updated = append(s.updated, &clone)
	if s.userRepoStub != nil {
		s.userRepoStub.user = &clone
	}
	return nil
}

type balanceRedeemRepoStub struct {
	*redeemRepoStub
	created []*RedeemCode
}

func (s *balanceRedeemRepoStub) Create(ctx context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
	}
	clone := *code
	s.created = append(s.created, &clone)
	return nil
}

type authCacheInvalidatorStub struct {
	userIDs  []int64
	groupIDs []int64
	keys     []string
}

type balanceAffiliateStub struct {
	userID int64
	amount float64
}

func (s *balanceAffiliateStub) AccrueInviteRebate(_ context.Context, userID int64, amount float64) (float64, error) {
	s.userID = userID
	s.amount = amount
	return amount * 0.2, nil
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	s.keys = append(s.keys, key)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	s.groupIDs = append(s.groupIDs, groupID)
}

func TestAdminService_UpdateUserBalance_InvalidatesAuthCache(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       redeemRepo,
		authCacheInvalidator: invalidator,
	}

	_, err := svc.UpdateUserBalance(context.Background(), 7, 5, "add", "")
	require.NoError(t, err)
	require.Equal(t, []int64{7}, invalidator.userIDs)
	require.Len(t, redeemRepo.created, 1)
}

func TestAdminService_UpdateUserBalance_NoChangeNoInvalidate(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       redeemRepo,
		authCacheInvalidator: invalidator,
	}

	_, err := svc.UpdateUserBalance(context.Background(), 7, 10, "set", "")
	require.NoError(t, err)
	require.Empty(t, invalidator.userIDs)
	require.Empty(t, redeemRepo.created)
}

func TestAdminService_UpdateUserBalance_AccruesRebateOnlyForPositiveDelta(t *testing.T) {
	for _, tc := range []struct {
		name      string
		balance   float64
		operation string
		want      float64
	}{
		{name: "add", balance: 5, operation: "add", want: 5},
		{name: "set increase", balance: 15, operation: "set", want: 5},
		{name: "subtract", balance: 5, operation: "subtract", want: 0},
		{name: "set decrease", balance: 5, operation: "set", want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
			repo := &balanceUserRepoStub{userRepoStub: baseRepo}
			redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}}
			affiliate := &balanceAffiliateStub{}
			svc := &adminServiceImpl{userRepo: repo, redeemCodeRepo: redeemRepo, affiliateService: affiliate}

			_, err := svc.UpdateUserBalance(context.Background(), 7, tc.balance, tc.operation, "")
			require.NoError(t, err)
			require.InDelta(t, tc.want, affiliate.amount, 1e-9)
		})
	}
}
