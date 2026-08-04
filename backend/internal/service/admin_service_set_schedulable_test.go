//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type accountRepoStubForSetSchedulable struct {
	mockAccountRepoForGemini
	account           *Account
	setSchedulableN   int
	lastSchedulable   bool
	setSchedulableErr error
}

func (r *accountRepoStubForSetSchedulable) GetByID(ctx context.Context, id int64) (*Account, error) {
	return r.account, nil
}

func (r *accountRepoStubForSetSchedulable) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	r.setSchedulableN++
	r.lastSchedulable = schedulable
	if r.setSchedulableErr != nil {
		return r.setSchedulableErr
	}
	if r.account != nil {
		r.account.Schedulable = schedulable
	}
	return nil
}
func (r *accountRepoStubForSetSchedulable) SetExpiresAt(ctx context.Context, id int64, expiresAt *time.Time) error {
	return nil
}

func TestAdminService_SetAccountSchedulable_RejectsErrorAccount(t *testing.T) {
	repo := &accountRepoStubForSetSchedulable{
		account: &Account{
			ID:           9,
			Status:       StatusError,
			ErrorMessage: "Authentication failed (401): token invalidated",
			Schedulable:  false,
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.SetAccountSchedulable(context.Background(), 9, true)
	require.Error(t, err)
	require.Nil(t, updated)
	require.Equal(t, 0, repo.setSchedulableN)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Equal(t, "ACCOUNT_NOT_SCHEDULABLE", infraerrors.Reason(err))
}

func TestAdminService_SetAccountSchedulable_AllowsActiveAccount(t *testing.T) {
	repo := &accountRepoStubForSetSchedulable{
		account: &Account{
			ID:          10,
			Status:      StatusActive,
			Schedulable: false,
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.SetAccountSchedulable(context.Background(), 10, true)
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.setSchedulableN)
	require.True(t, repo.lastSchedulable)
	require.True(t, updated.Schedulable)
}

func TestAdminService_SetAccountSchedulable_AllowsDisableErrorAccount(t *testing.T) {
	repo := &accountRepoStubForSetSchedulable{
		account: &Account{
			ID:          11,
			Status:      StatusError,
			Schedulable: true,
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.SetAccountSchedulable(context.Background(), 11, false)
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, 1, repo.setSchedulableN)
	require.False(t, repo.lastSchedulable)
}

func TestAdminService_SetAccountSchedulable_RejectsSubscriptionBannedAccount(t *testing.T) {
	repo := &accountRepoStubForSetSchedulable{
		account: &Account{
			ID:                 12,
			Status:             StatusActive,
			Schedulable:        false,
			SubscriptionBanned: true,
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.SetAccountSchedulable(context.Background(), 12, true)
	require.Error(t, err)
	require.Nil(t, updated)
	require.Equal(t, 0, repo.setSchedulableN)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Equal(t, "ACCOUNT_NOT_SCHEDULABLE", infraerrors.Reason(err))
}
