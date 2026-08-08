//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOAuthEmailTakeover_RealEmailTaken_Rejected 验证漏洞修复：OAuth 身份未绑定 +
// 真实邮箱已存在 → 拒绝自动登录（否则攻击者凭受害者邮箱即可接管账号）。
func TestOAuthEmailTakeover_RealEmailTaken_Rejected(t *testing.T) {
	victim := &User{ID: 501, Email: "victim@example.com", Username: "victim", Role: RoleUser, Status: StatusActive}
	repo := &userRepoStub{usersByEmail: map[string]*User{"victim@example.com": victim}}
	svc := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "true"}, nil, nil)
	svc.entClient = newAdminServiceAuthIdentityBindingTestClient(t) // 空表 → 身份未绑定
	svc.refreshTokenCache = &refreshTokenCacheStub{}

	// 攻击者用受害者邮箱走 GitHub 直登（身份未绑定）
	_, _, err := svc.LoginOrRegisterVerifiedEmailOAuth(context.Background(), EmailOAuthIdentityInput{
		ProviderType:    "github",
		ProviderKey:     "github",
		ProviderSubject: "attacker-github-subject",
		Email:           "victim@example.com",
		EmailVerified:   true,
		Username:        "attacker",
	})

	require.ErrorIs(t, err, ErrOAuthEmailOwnershipRequired, "真实邮箱已存在且身份未绑定必须拒绝，防账号接管")
	require.Empty(t, repo.created, "不应创建新账号")
	require.Empty(t, repo.updated, "不应更新受害者账号")
}

// TestOAuthEmailTakeover_PendingFlowRealEmailTaken_Rejected 验证漏洞 B：LinuxDo/DingTalk/
// OIDC/WeChat 补全流程（loginOrRegisterOAuthWithTokenPair）同样拒绝真实邮箱直登。
func TestOAuthEmailTakeover_PendingFlowRealEmailTaken_Rejected(t *testing.T) {
	victim := &User{ID: 502, Email: "victim2@example.com", Username: "victim2", Role: RoleUser, Status: StatusActive}
	repo := &userRepoStub{usersByEmail: map[string]*User{"victim2@example.com": victim}}
	service := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "true"}, nil, nil)
	service.refreshTokenCache = &refreshTokenCacheStub{}

	_, _, err := service.LoginOrRegisterOAuthWithTokenPairAndPromoCode(context.Background(), "victim2@example.com", "attacker", "", "", "", "linuxdo")

	require.ErrorIs(t, err, ErrOAuthEmailOwnershipRequired, "真实邮箱已存在必须拒绝，防账号接管")
	require.Empty(t, repo.created)
}

// TestOAuthEmailTakeover_SyntheticEmailTaken_Allowed 验证合成邮箱（基于 subject 生成、
// 不可伪造）命中既有账号 = 同一第三方身份再次登录，必须放行（LinuxDo 主路径依赖）。
func TestOAuthEmailTakeover_SyntheticEmailTaken_Allowed(t *testing.T) {
	existing := &User{ID: 503, Email: "linuxdo-123@linuxdo-connect.invalid", Username: "existing", Role: RoleUser, Status: StatusActive}
	repo := &userRepoStub{usersByEmail: map[string]*User{"linuxdo-123@linuxdo-connect.invalid": existing}}
	service := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "true"}, nil, nil)
	service.refreshTokenCache = &refreshTokenCacheStub{}

	tokenPair, user, err := service.LoginOrRegisterOAuthWithTokenPairAndPromoCode(context.Background(), "linuxdo-123@linuxdo-connect.invalid", "linuxdo_user", "", "", "", "linuxdo")
	require.NoError(t, err)
	require.NotNil(t, tokenPair)
	require.Equal(t, existing.ID, user.ID)
}

// TestOAuthEmailTakeover_EmailNotFound_Registers 验证邮箱不存在时正常走注册。
func TestOAuthEmailTakeover_EmailNotFound_Registers(t *testing.T) {
	repo := &userRepoStub{nextID: 504}
	service := newAuthService(repo, map[string]string{SettingKeyRegistrationEnabled: "true"}, nil, nil)
	service.refreshTokenCache = &refreshTokenCacheStub{}

	tokenPair, user, err := service.LoginOrRegisterOAuthWithTokenPairAndPromoCode(context.Background(), "newuser@example.com", "newbie", "", "", "", "oidc")
	require.NoError(t, err)
	require.NotNil(t, tokenPair)
	require.NotNil(t, user)
	require.Equal(t, "newuser@example.com", user.Email)
	require.Len(t, repo.created, 1)
}

// TestOAuthSyntheticEmailDomain 验证合成邮箱域判定（不可伪造的 subject 邮箱 vs 真实邮箱）。
func TestOAuthSyntheticEmailDomain(t *testing.T) {
	require.True(t, isOAuthSyntheticEmail("linuxdo-123@linuxdo-connect.invalid"))
	require.True(t, isOAuthSyntheticEmail("oidc-9@oidc-connect.invalid"))
	require.True(t, isOAuthSyntheticEmail("wechat-1@wechat-connect.invalid"))
	require.True(t, isOAuthSyntheticEmail("dingtalk-2@dingtalk-connect.invalid"))
	require.False(t, isOAuthSyntheticEmail("victim@example.com"), "真实邮箱不是合成邮箱")
	require.False(t, isOAuthSyntheticEmail("user@gmail.com"))
}
