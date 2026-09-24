//go:build unit

// Claude（Anthropic 上游）OAuth / setup-token 供给退役后，所有账号写入路径必须
// fail-closed（创建 / 更新 / 状态变更 / 批量 / 恢复 / 清除错误 / 调度开关）。
// 见 openspec/changes/remove-claude-oauth-login。
//
// 与 account_vertex_onboarding_test.go 同构：用 accountRepoStub 隔离仓储，
// 断言守卫在触达仓储之前就返回 ErrRetiredClaudeOAuthAccount。
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaudeOAuthWritePathsFailClosed(t *testing.T) {
	ctx := context.Background()

	for _, typ := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run("existing/"+typ, func(t *testing.T) {
			repo := &accountRepoStub{account: &Account{
				ID:          77,
				Name:        "legacy-claude-" + typ,
				Platform:    PlatformAnthropic,
				Type:        typ,
				Status:      StatusActive,
				Schedulable: true,
			}}

			accountService := NewAccountService(repo, nil)
			active := StatusActive

			_, err := accountService.Update(ctx, repo.account.ID, UpdateAccountRequest{Status: &active})
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)
			require.Nil(t, repo.updated)

			err = accountService.UpdateStatus(ctx, repo.account.ID, StatusDisabled, "retired")
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)
			require.Nil(t, repo.updated)

			admin := &adminServiceImpl{accountRepo: repo}
			_, err = admin.UpdateAccount(ctx, repo.account.ID, &UpdateAccountInput{Status: StatusDisabled})
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)
			require.Nil(t, repo.updated)

			_, err = admin.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
				AccountIDs: []int64{repo.account.ID},
				Status:     StatusDisabled,
			})
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)

			_, err = admin.RestoreAccount(ctx, repo.account.ID)
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)

			_, err = admin.ClearAccountError(ctx, repo.account.ID)
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)

			// 仅“开启调度”路径带守卫（关闭调度对退役账号无害，允许）。
			_, err = admin.SetAccountSchedulable(ctx, repo.account.ID, true)
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)
		})

		t.Run("create/"+typ, func(t *testing.T) {
			repo := &accountRepoStub{}
			accountService := NewAccountService(repo, nil)

			_, err := accountService.Create(ctx, CreateAccountRequest{
				Name:     "new-claude-" + typ,
				Platform: PlatformAnthropic,
				Type:     typ,
			})
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)
			require.Nil(t, repo.created)

			admin := &adminServiceImpl{accountRepo: repo}
			_, err = admin.CreateAccount(ctx, &CreateAccountInput{
				Name:     "new-claude-" + typ,
				Platform: PlatformAnthropic,
				Type:     typ,
			})
			require.ErrorIs(t, err, ErrRetiredClaudeOAuthAccount)
			require.Nil(t, repo.created)
		})
	}
}

// TestClaudeOAuthWritePathsDoNotBlockOtherPlatforms 反向护栏：Antigravity 的
// setup-token 与 Anthropic 的 apikey 供给必须照常可通过账号写入路径。
func TestClaudeOAuthWritePathsDoNotBlockOtherPlatforms(t *testing.T) {
	ctx := context.Background()

	t.Run("antigravity setup-token", func(t *testing.T) {
		repo := &accountRepoStub{}
		accountService := NewAccountService(repo, nil)
		_, err := accountService.Create(ctx, CreateAccountRequest{
			Name:     "ag-setup-token",
			Platform: PlatformAntigravity,
			Type:     AccountTypeSetupToken,
		})
		require.NoError(t, err)
		require.NotNil(t, repo.created)
	})

	t.Run("anthropic apikey", func(t *testing.T) {
		repo := &accountRepoStub{}
		accountService := NewAccountService(repo, nil)
		_, err := accountService.Create(ctx, CreateAccountRequest{
			Name:     "anthropic-apikey",
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
		})
		require.NoError(t, err)
		require.NotNil(t, repo.created)
	})
}
