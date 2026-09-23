package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestValidateExternalBridgeConfiguration(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken, "upstream"} {
		t.Run(accountType, func(t *testing.T) {
			require.Error(t, validateExternalBridgeConfiguration(PlatformOpenAI, accountType, map[string]any{"external_bridge": true, "external_bridge_kind": "devin"}))
		})
	}
	require.Error(t, validateExternalBridgeConfiguration(PlatformGrok, AccountTypeAPIKey, map[string]any{"external_bridge": true, "external_bridge_kind": "devin"}))
	for _, kind := range []any{nil, "", "  ", 42} {
		require.Error(t, validateExternalBridgeConfiguration(PlatformOpenAI, AccountTypeAPIKey, map[string]any{"external_bridge": "true", "external_bridge_kind": kind}))
	}
	for _, kind := range []string{"devin", "cursor", "custom-bridge"} {
		require.NoError(t, validateExternalBridgeConfiguration(PlatformOpenAI, AccountTypeAPIKey, map[string]any{"external_bridge": true, "external_bridge_kind": kind}))
	}
	require.NoError(t, validateExternalBridgeConfiguration(PlatformOpenAI, AccountTypeOAuth, nil))
	require.NoError(t, validateExternalBridgeConfiguration(PlatformOpenAI, AccountTypeOAuth, map[string]any{"external_bridge": false}))
}

func TestCreateAccountRejectsInvalidBridgeBeforeRepositoryAccess(t *testing.T) {
	svc := &adminServiceImpl{}
	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"external_bridge": true, "external_bridge_kind": "devin"}})
	require.ErrorContains(t, err, "API-key")
	_, err = svc.CreateAccount(context.Background(), &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"external_bridge": true}})
	require.ErrorContains(t, err, "kind is required")
}

type bridgeConfigTestRepo struct {
	AccountRepository
	account *Account
	writes  int
}

func (r *bridgeConfigTestRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *bridgeConfigTestRepo) GetByIDs(context.Context, []int64) ([]*Account, error) {
	return []*Account{r.account}, nil
}
func (r *bridgeConfigTestRepo) Update(context.Context, *Account) error { r.writes++; return nil }

func TestUpdateAccountRejectsInvalidBridgeBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name, accountType string
		extra             map[string]any
	}{
		{"oauth cannot enable", AccountTypeOAuth, map[string]any{"external_bridge": true, "external_bridge_kind": "devin"}},
		{"kind required", AccountTypeAPIKey, map[string]any{"external_bridge": true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := &Account{ID: 1, Platform: PlatformOpenAI, Type: test.accountType, Name: "unchanged"}
			repo := &bridgeConfigTestRepo{account: original}
			svc := &adminServiceImpl{accountRepo: repo}
			_, err := svc.UpdateAccount(context.Background(), 1, &UpdateAccountInput{Name: "changed", Extra: test.extra})
			require.Error(t, err)
			require.Zero(t, repo.writes)
			require.Equal(t, "unchanged", original.Name)
			require.Empty(t, original.Extra)
		})
	}
}

func TestBulkUpdateAccountValidatesMergedBridgeKindBeforeWrite(t *testing.T) {
	repo := &bridgeConfigTestRepo{account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"external_bridge": true, "external_bridge_kind": "devin"}}}
	svc := &adminServiceImpl{accountRepo: repo}
	_, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1}, Extra: map[string]any{"external_bridge_kind": " "}})
	require.ErrorContains(t, err, "kind is required")
	require.Zero(t, repo.writes)
	require.Equal(t, "devin", repo.account.Extra["external_bridge_kind"])
}
