//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type oauthMergeAccountRepoStub struct {
	mockAccountRepoForGemini

	byID            map[int64]*Account
	oauthByEmail    map[string][]Account
	created         []*Account
	deletedIDs      []int64
	hardDeletedIDs []int64
	reassigned      [][2]int64
	restoredIDs     []int64
	updatedIDs      []int64
	lastUsedIDs     []int64
	clearedErrorIDs []int64
	nextID          int64
}

func (s *oauthMergeAccountRepoStub) key(platform, email string) string {
	return platform + "|" + email
}

func (s *oauthMergeAccountRepoStub) FindOAuthByPlatformEmail(_ context.Context, platform, email string, includeDeleted bool) ([]Account, error) {
	rows := s.oauthByEmail[s.key(platform, email)]
	out := make([]Account, 0, len(rows))
	for _, row := range rows {
		if !includeDeleted && row.DeletedAt != nil {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *oauthMergeAccountRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	acc, ok := s.byID[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	copied := *acc
	return &copied, nil
}

func (s *oauthMergeAccountRepoStub) Create(_ context.Context, account *Account) error {
	if s.nextID == 0 {
		s.nextID = 1000
	}
	s.nextID++
	account.ID = s.nextID
	copied := *account
	s.byID[account.ID] = &copied
	s.created = append(s.created, &copied)
	return nil
}

func (s *oauthMergeAccountRepoStub) Update(_ context.Context, account *Account) error {
	s.updatedIDs = append(s.updatedIDs, account.ID)
	copied := *account
	s.byID[account.ID] = &copied
	// keep oauth index in sync for active rows
	email := oauthAccountEmail(account)
	if email != "" {
		key := s.key(account.Platform, email)
		rows := s.oauthByEmail[key]
		for i := range rows {
			if rows[i].ID == account.ID {
				rows[i] = copied
				s.oauthByEmail[key] = rows
				return nil
			}
		}
		s.oauthByEmail[key] = append(rows, copied)
	}
	return nil
}

func (s *oauthMergeAccountRepoStub) Delete(_ context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	acc, ok := s.byID[id]
	if !ok {
		return nil
	}
	now := time.Now()
	acc.DeletedAt = &now
	email := oauthAccountEmail(acc)
	if email == "" {
		return nil
	}
	if s.oauthByEmail == nil {
		return nil
	}
	key := s.key(acc.Platform, email)
	rows := s.oauthByEmail[key]
	for i := range rows {
		if rows[i].ID == id {
			rows[i].DeletedAt = &now
		}
	}
	s.oauthByEmail[key] = rows
	return nil
}

func (s *oauthMergeAccountRepoStub) HardDelete(_ context.Context, id int64) error {
	s.hardDeletedIDs = append(s.hardDeletedIDs, id)
	delete(s.byID, id)
	for key, rows := range s.oauthByEmail {
		filtered := rows[:0]
		for _, row := range rows {
			if row.ID != id {
				filtered = append(filtered, row)
			}
		}
		s.oauthByEmail[key] = filtered
	}
	return nil
}

func (s *oauthMergeAccountRepoStub) ReassignAccountReferences(_ context.Context, fromID, toID int64) error {
	s.reassigned = append(s.reassigned, [2]int64{fromID, toID})
	return nil
}

func (s *oauthMergeAccountRepoStub) Restore(_ context.Context, id int64) error {
	s.restoredIDs = append(s.restoredIDs, id)
	acc, ok := s.byID[id]
	if !ok {
		return ErrAccountNotFound
	}
	acc.DeletedAt = nil
	acc.Status = StatusActive
	acc.ErrorMessage = ""
	acc.Schedulable = true
	return nil
}

func (s *oauthMergeAccountRepoStub) UpdateLastUsed(_ context.Context, id int64) error {
	s.lastUsedIDs = append(s.lastUsedIDs, id)
	if acc, ok := s.byID[id]; ok {
		now := time.Now()
		acc.LastUsedAt = &now
	}
	return nil
}

func (s *oauthMergeAccountRepoStub) ClearError(_ context.Context, id int64) error {
	s.clearedErrorIDs = append(s.clearedErrorIDs, id)
	if acc, ok := s.byID[id]; ok {
		acc.ErrorMessage = ""
		acc.Status = StatusActive
	}
	return nil
}

func (s *oauthMergeAccountRepoStub) ClearRateLimit(context.Context, int64) error { return nil }
func (s *oauthMergeAccountRepoStub) ClearAntigravityQuotaScopes(context.Context, int64) error {
	return nil
}
func (s *oauthMergeAccountRepoStub) ClearModelRateLimits(context.Context, int64) error { return nil }
func (s *oauthMergeAccountRepoStub) ClearTempUnschedulable(context.Context, int64) error {
	return nil
}

func (s *oauthMergeAccountRepoStub) ListOAuthIncludingDeleted(context.Context) ([]Account, error) {
	out := make([]Account, 0, len(s.byID))
	for _, acc := range s.byID {
		if acc.Type != AccountTypeOAuth && acc.Type != AccountTypeSetupToken {
			continue
		}
		out = append(out, *acc)
	}
	return out, nil
}

func TestPickLatestOAuthAccount_PrefersLastUsedThenHigherID(t *testing.T) {
	older := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	// last_used 优先于 created_at；同时间戳取更大 ID。
	accounts := []Account{
		{ID: 1, LastUsedAt: &newer, CreatedAt: older},
		{ID: 2, LastUsedAt: &older, CreatedAt: older},
		{ID: 3, CreatedAt: older}, // 无 last_used，用 created_at，应落后
	}
	got := pickLatestOAuthAccount(accounts)
	require.NotNil(t, got)
	require.Equal(t, int64(1), got.ID)

	same := newer
	accounts = []Account{
		{ID: 10, LastUsedAt: &same},
		{ID: 12, LastUsedAt: &same},
		{ID: 11, LastUsedAt: &same},
	}
	got = pickLatestOAuthAccount(accounts)
	require.Equal(t, int64(12), got.ID)
}

func TestCreateAccount_MergesSamePlatformEmailIntoLatest(t *testing.T) {
	oldLogin := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	newLogin := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	repo := &oauthMergeAccountRepoStub{
		byID: map[int64]*Account{
			10: {
				ID: 10, Name: "old", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
				Credentials: map[string]any{"email": "user@example.com", "access_token": "old-at", "keep": "yes"},
				LastUsedAt:  &oldLogin,
			},
			20: {
				ID: 20, Name: "latest", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
				Credentials: map[string]any{"email": "user@example.com", "access_token": "mid-at"},
				LastUsedAt:  &newLogin,
			},
		},
		oauthByEmail: map[string][]Account{},
	}
	repo.oauthByEmail["openai|user@example.com"] = []Account{*repo.byID[10], *repo.byID[20]}

	svc := &adminServiceImpl{accountRepo: repo}
	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:     "from-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"email":         "User@Example.com",
			"access_token":  "fresh-at",
			"refresh_token": "fresh-rt",
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(20), account.ID)
	require.Empty(t, repo.created, "should not create a new account")
	require.Equal(t, []int64{10}, repo.deletedIDs)
	require.Equal(t, "fresh-at", repo.byID[20].Credentials["access_token"])
	require.Equal(t, "fresh-rt", repo.byID[20].Credentials["refresh_token"])
	require.Equal(t, "user@example.com", repo.byID[20].Extra["email"])
	require.Contains(t, repo.lastUsedIDs, int64(20))
}

func TestCreateAccount_RestoresSoftDeletedWinner(t *testing.T) {
	deletedAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	login := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	repo := &oauthMergeAccountRepoStub{
		byID: map[int64]*Account{
			33: {
				ID: 33, Name: "trashed", Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusError,
				Credentials:  map[string]any{"email": "a@b.com", "access_token": "old"},
				LastUsedAt:   &login,
				DeletedAt:    &deletedAt,
				ErrorMessage: "token revoked",
			},
		},
		oauthByEmail: map[string][]Account{},
	}
	repo.oauthByEmail["grok|a@b.com"] = []Account{*repo.byID[33]}

	svc := &adminServiceImpl{accountRepo: repo}
	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:     "relogin",
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"email":        "a@b.com",
			"access_token": "new",
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(33), account.ID)
	require.Equal(t, []int64{33}, repo.restoredIDs)
	require.Nil(t, repo.byID[33].DeletedAt)
	require.Equal(t, "new", repo.byID[33].Credentials["access_token"])
	require.Empty(t, repo.created)
}

func TestCleanupOAuthEmailDuplicates_KeepsLatestLogin(t *testing.T) {
	oldLogin := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	newLogin := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repo := &oauthMergeAccountRepoStub{
		byID: map[int64]*Account{
			1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"email": "x@y.com"}, LastUsedAt: &oldLogin},
			2: {ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError, Credentials: map[string]any{"email": "x@y.com"}, LastUsedAt: &newLogin},
			3: {ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Credentials: map[string]any{"email": "x@y.com"}},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}
	kept, deleted, err := svc.CleanupOAuthEmailDuplicates(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, kept)
	require.Equal(t, 1, deleted)
	require.Equal(t, []int64{1}, repo.hardDeletedIDs)
	require.Equal(t, [][2]int64{{1, 2}}, repo.reassigned)
	_, ok := repo.byID[1]
	require.False(t, ok)
	require.NotNil(t, repo.byID[2])
	require.NotNil(t, repo.byID[3], "apikey accounts are out of scope")
}

func TestCleanupOAuthEmailDuplicates_IncludesTrash(t *testing.T) {
	oldLogin := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	newLogin := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	deletedAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	repo := &oauthMergeAccountRepoStub{
		byID: map[int64]*Account{
			28: {
				ID: 28, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
				Credentials: map[string]any{"email": "omopencodex@outlook.com"},
				LastUsedAt:  &oldLogin, DeletedAt: &deletedAt,
			},
			6: {
				ID: 6, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
				Credentials: map[string]any{"email": "omopencodex@outlook.com"},
				CreatedAt:   oldLogin.Add(-time.Hour), DeletedAt: &deletedAt,
			},
			103: {
				ID: 103, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError,
				Credentials: map[string]any{"email": "omopencodex@outlook.com"},
				LastUsedAt:  &newLogin,
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}
	kept, deleted, err := svc.CleanupOAuthEmailDuplicates(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, kept)
	require.Equal(t, 2, deleted)
	require.ElementsMatch(t, []int64{28, 6}, repo.hardDeletedIDs)
	require.ElementsMatch(t, [][2]int64{{28, 103}, {6, 103}}, repo.reassigned)
	require.NotNil(t, repo.byID[103])
	_, ok28 := repo.byID[28]
	_, ok6 := repo.byID[6]
	require.False(t, ok28)
	require.False(t, ok6)
}
