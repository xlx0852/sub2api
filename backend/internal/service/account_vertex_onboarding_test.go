//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVertexServiceAccountOnboardingIsCreateOnlyRetired(t *testing.T) {
	ctx := context.Background()
	repo := &accountRepoStub{account: &Account{
		ID:          92,
		Name:        "existing-vertex",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeServiceAccount,
		Status:      StatusActive,
		Schedulable: true,
	}}

	accountService := NewAccountService(repo, nil)
	for _, platform := range []string{PlatformAnthropic, PlatformGemini} {
		_, err := accountService.Create(ctx, CreateAccountRequest{
			Name:     "new-vertex",
			Platform: platform,
			Type:     AccountTypeServiceAccount,
		})
		require.ErrorIs(t, err, ErrVertexServiceAccountOnboardingDisabled)
		require.Nil(t, repo.created)
	}

	admin := &adminServiceImpl{accountRepo: repo}
	for _, platform := range []string{PlatformAnthropic, PlatformGemini} {
		_, err := admin.CreateAccount(ctx, &CreateAccountInput{
			Name:     "new-vertex",
			Platform: platform,
			Type:     AccountTypeServiceAccount,
		})
		require.ErrorIs(t, err, ErrVertexServiceAccountOnboardingDisabled)
		require.Nil(t, repo.created)
	}

	updatedName := "existing-vertex-updated"
	updated, err := accountService.Update(ctx, repo.account.ID, UpdateAccountRequest{Name: &updatedName})
	require.NoError(t, err)
	require.Equal(t, updatedName, updated.Name)
	require.Same(t, repo.account, repo.updated)
}
