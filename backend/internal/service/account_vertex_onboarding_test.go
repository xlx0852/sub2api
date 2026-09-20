//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVertexServiceAccountWritePathsFailClosed(t *testing.T) {
	ctx := context.Background()
	repo := &accountRepoStub{account: &Account{
		ID:          92,
		Name:        "existing-vertex",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeServiceAccount,
		Status:      StatusDisabled,
		Schedulable: false,
	}}

	accountService := NewAccountService(repo, nil)
	for _, platform := range []string{PlatformAnthropic, PlatformGemini} {
		_, err := accountService.Create(ctx, CreateAccountRequest{
			Name:     "new-vertex",
			Platform: platform,
			Type:     AccountTypeServiceAccount,
		})
		require.ErrorIs(t, err, ErrRetiredVertexServiceAccount)
		require.Nil(t, repo.created)
	}

	admin := &adminServiceImpl{accountRepo: repo}
	for _, platform := range []string{PlatformAnthropic, PlatformGemini} {
		_, err := admin.CreateAccount(ctx, &CreateAccountInput{
			Name:     "new-vertex",
			Platform: platform,
			Type:     AccountTypeServiceAccount,
		})
		require.ErrorIs(t, err, ErrRetiredVertexServiceAccount)
		require.Nil(t, repo.created)
	}

	active := StatusActive
	_, err := accountService.Update(ctx, repo.account.ID, UpdateAccountRequest{Status: &active})
	require.ErrorIs(t, err, ErrRetiredVertexServiceAccount)
	require.Nil(t, repo.updated)
	require.ErrorIs(t, accountService.UpdateStatus(ctx, repo.account.ID, StatusActive, ""), ErrRetiredVertexServiceAccount)

	_, err = admin.UpdateAccount(ctx, repo.account.ID, &UpdateAccountInput{Status: StatusActive})
	require.ErrorIs(t, err, ErrRetiredVertexServiceAccount)
	_, err = admin.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{AccountIDs: []int64{repo.account.ID}, Status: StatusActive})
	require.ErrorIs(t, err, ErrRetiredVertexServiceAccount)
	_, err = admin.RestoreAccount(ctx, repo.account.ID)
	require.ErrorIs(t, err, ErrRetiredVertexServiceAccount)
	_, err = admin.ClearAccountError(ctx, repo.account.ID)
	require.ErrorIs(t, err, ErrRetiredVertexServiceAccount)
	_, err = admin.SetAccountSchedulable(ctx, repo.account.ID, true)
	require.ErrorIs(t, err, ErrRetiredVertexServiceAccount)
}
