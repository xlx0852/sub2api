package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/identityadoptiondecision"
	"github.com/Wei-Shaw/sub2api/ent/pendingauthsession"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestApplySuggestedProfileToCompletionResponse(t *testing.T) {
	payload := map[string]any{
		"access_token": "token",
	}
	upstream := map[string]any{
		"suggested_display_name": "Alice",
		"suggested_avatar_url":   "https://cdn.example/avatar.png",
	}

	applySuggestedProfileToCompletionResponse(payload, upstream)

	require.Equal(t, "Alice", payload["suggested_display_name"])
	require.Equal(t, "https://cdn.example/avatar.png", payload["suggested_avatar_url"])
	require.Equal(t, true, payload["adoption_required"])
}

func TestApplySuggestedProfileToCompletionResponseKeepsExistingPayloadValues(t *testing.T) {
	payload := map[string]any{
		"suggested_display_name": "Existing",
		"adoption_required":      false,
	}
	upstream := map[string]any{
		"suggested_display_name": "Alice",
		"suggested_avatar_url":   "https://cdn.example/avatar.png",
	}

	applySuggestedProfileToCompletionResponse(payload, upstream)

	require.Equal(t, "Existing", payload["suggested_display_name"])
	require.Equal(t, "https://cdn.example/avatar.png", payload["suggested_avatar_url"])
	require.Equal(t, true, payload["adoption_required"])
}

func TestSetOAuthPendingSessionCookieUsesProviderCompletionPathPrefix(t *testing.T) {
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/callback", nil)

	setOAuthPendingSessionCookie(ginCtx, "pending-session-token", false)

	cookie := findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName)
	require.NotNil(t, cookie)
	require.Equal(t, "/api/v1/auth/oauth", cookie.Path)
}

func TestExchangePendingOAuthCompletionPreviewThenFinalizeAppliesAdoptionDecision(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("linuxdo-123@linuxdo-connect.invalid").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("pending-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "linuxdo_user",
			"suggested_display_name": "Alice Example",
			"suggested_avatar_url":   "https://cdn.example/alice.png",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
				"redirect":     "/dashboard",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	previewRecorder := httptest.NewRecorder()
	previewCtx, _ := gin.CreateTestContext(previewRecorder)
	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	previewReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	previewReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("browser-session-key")})
	previewCtx.Request = previewReq

	handler.ExchangePendingOAuthCompletion(previewCtx)

	require.Equal(t, http.StatusOK, previewRecorder.Code)
	previewData := decodeJSONResponseData(t, previewRecorder)
	require.Equal(t, "Alice Example", previewData["suggested_display_name"])
	require.Equal(t, "https://cdn.example/alice.png", previewData["suggested_avatar_url"])
	require.Equal(t, true, previewData["adoption_required"])

	storedUser, err := client.User.Get(ctx, userEntity.ID)
	require.NoError(t, err)
	require.Equal(t, "legacy-name", storedUser.Username)

	previewSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Nil(t, previewSession.ConsumedAt)

	body := bytes.NewBufferString(`{"adopt_display_name":true,"adopt_avatar":true}`)
	finalizeRecorder := httptest.NewRecorder()
	finalizeCtx, _ := gin.CreateTestContext(finalizeRecorder)
	finalizeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	finalizeReq.Header.Set("Content-Type", "application/json")
	finalizeReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	finalizeReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("browser-session-key")})
	finalizeCtx.Request = finalizeReq

	handler.ExchangePendingOAuthCompletion(finalizeCtx)

	require.Equal(t, http.StatusOK, finalizeRecorder.Code)

	storedUser, err = client.User.Get(ctx, userEntity.ID)
	require.NoError(t, err)
	require.Equal(t, "Alice Example", storedUser.Username)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("123"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, userEntity.ID, identity.UserID)
	require.Equal(t, "Alice Example", identity.Metadata["display_name"])
	require.Equal(t, "https://cdn.example/alice.png", identity.Metadata["avatar_url"])

	avatar := loadUserAvatarRecord(t, client, userEntity.ID)
	require.NotNil(t, avatar)
	require.Equal(t, "remote_url", avatar.StorageProvider)
	require.Equal(t, "https://cdn.example/alice.png", avatar.URL)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)
	require.True(t, decision.AdoptDisplayName)
	require.True(t, decision.AdoptAvatar)

	consumed, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)
}

func TestExchangePendingOAuthCompletionSkipsInvalidAvatarAdoptionWithoutBlockingCompletion(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("invalid-avatar@example.com").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("pending-invalid-avatar-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("invalid-avatar-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("browser-invalid-avatar-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "linuxdo_user",
			"suggested_display_name": "Alice Example",
			"suggested_avatar_url":   "/avatars/alice.png",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
				"redirect":     "/dashboard",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"adopt_display_name":true,"adopt_avatar":true}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("browser-invalid-avatar-key")})
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("invalid-avatar-123"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "Alice Example", identity.Metadata["display_name"])
	_, hasAdoptedAvatar := identity.Metadata["avatar_url"]
	require.False(t, hasAdoptedAvatar)

	avatar := loadUserAvatarRecord(t, client, userEntity.ID)
	require.Nil(t, avatar)

	consumed, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)
}

func TestExchangePendingOAuthCompletionBindCurrentUserPreviewThenFinalizeBindsIdentityWithoutAdoption(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("bind-target@example.com").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-pending-session-token").
		SetIntent("bind_current_user").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("bind-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("bind-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "linuxdo_user",
			"suggested_display_name": "Bound Example",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
				"redirect":     "/settings/profile",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	previewRecorder := httptest.NewRecorder()
	previewCtx, _ := gin.CreateTestContext(previewRecorder)
	previewReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	previewReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	previewReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-browser-session-key")})
	previewCtx.Request = previewReq

	handler.ExchangePendingOAuthCompletion(previewCtx)

	require.Equal(t, http.StatusOK, previewRecorder.Code)
	previewData := decodeJSONResponseData(t, previewRecorder)
	require.Equal(t, "Bound Example", previewData["suggested_display_name"])
	require.Equal(t, "https://cdn.example/bound.png", previewData["suggested_avatar_url"])
	require.Equal(t, true, previewData["adoption_required"])

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("bind-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, identityCount)

	previewSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Nil(t, previewSession.ConsumedAt)

	body := bytes.NewBufferString(`{"adopt_display_name":false,"adopt_avatar":false}`)
	finalizeRecorder := httptest.NewRecorder()
	finalizeCtx, _ := gin.CreateTestContext(finalizeRecorder)
	finalizeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	finalizeReq.Header.Set("Content-Type", "application/json")
	finalizeReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	finalizeReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-browser-session-key")})
	finalizeCtx.Request = finalizeReq

	handler.ExchangePendingOAuthCompletion(finalizeCtx)

	require.Equal(t, http.StatusOK, finalizeRecorder.Code)

	storedUser, err := client.User.Get(ctx, userEntity.ID)
	require.NoError(t, err)
	require.Equal(t, "legacy-name", storedUser.Username)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("bind-123"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, userEntity.ID, identity.UserID)
	require.Equal(t, "Bound Example", identity.Metadata["suggested_display_name"])
	require.Equal(t, "https://cdn.example/bound.png", identity.Metadata["suggested_avatar_url"])
	_, hasDisplayName := identity.Metadata["display_name"]
	require.False(t, hasDisplayName)
	_, hasAvatarURL := identity.Metadata["avatar_url"]
	require.False(t, hasAvatarURL)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)
	require.False(t, decision.AdoptDisplayName)
	require.False(t, decision.AdoptAvatar)

	consumed, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, consumed.ConsumedAt)
}

func TestExchangePendingOAuthCompletionBindCurrentUserOwnershipConflict(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	targetUser, err := client.User.Create().
		SetEmail("bind-conflict-target@example.com").
		SetUsername("target-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	ownerUser, err := client.User.Create().
		SetEmail("bind-conflict-owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	existingIdentity, err := client.AuthIdentity.Create().
		SetUserID(ownerUser.ID).
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("conflict-123").
		SetMetadata(map[string]any{"username": "owner-user"}).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-conflict-session-token").
		SetIntent("bind_current_user").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("conflict-123").
		SetTargetUserID(targetUser.ID).
		SetResolvedEmail(targetUser.Email).
		SetBrowserSessionKey("bind-conflict-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Conflict Example",
			"suggested_avatar_url":   "https://cdn.example/conflict.png",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"adopt_display_name":false,"adopt_avatar":false}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-conflict-browser-session-key")})
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	payload := decodeJSONBody(t, recorder)
	require.Equal(t, "PENDING_AUTH_ADOPTION_APPLY_FAILED", payload["reason"])

	identity, err := client.AuthIdentity.Get(ctx, existingIdentity.ID)
	require.NoError(t, err)
	require.Equal(t, ownerUser.ID, identity.UserID)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Nil(t, decision.IdentityID)
	require.False(t, decision.AdoptDisplayName)
	require.False(t, decision.AdoptAvatar)

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestExchangePendingOAuthCompletionLoginFalseFalseBindsIdentityWithoutAdoption(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("login-false@example.com").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("login-false-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("login-false-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("login-false-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Login Example",
			"suggested_avatar_url":   "https://cdn.example/login.png",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"adopt_display_name":false,"adopt_avatar":false}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("login-false-browser-session-key")})
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("login-false-123"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, userEntity.ID, identity.UserID)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, decision.IdentityID)
	require.Equal(t, identity.ID, *decision.IdentityID)
	require.False(t, decision.AdoptDisplayName)
	require.False(t, decision.AdoptAvatar)

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, storedSession.ConsumedAt)
}

func TestExchangePendingOAuthCompletionLoginReassignsExistingDecisionIdentityReference(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("login-reassign@example.com").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	existingIdentity, err := client.AuthIdentity.Create().
		SetUserID(userEntity.ID).
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("login-reassign-123").
		SetMetadata(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	previousSession, err := client.PendingAuthSession.Create().
		SetSessionToken("login-reassign-previous-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("login-reassign-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("login-reassign-previous-browser-session-key").
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "previous-access-token",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	previousDecision, err := client.IdentityAdoptionDecision.Create().
		SetPendingAuthSessionID(previousSession.ID).
		SetIdentityID(existingIdentity.ID).
		SetAdoptDisplayName(true).
		SetAdoptAvatar(true).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("login-reassign-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("login-reassign-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("login-reassign-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Login Reassign",
			"suggested_avatar_url":   "https://cdn.example/login-reassign.png",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.IdentityAdoptionDecision.Create().
		SetPendingAuthSessionID(session.ID).
		SetAdoptDisplayName(false).
		SetAdoptAvatar(false).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"adopt_display_name":false,"adopt_avatar":false}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("login-reassign-browser-session-key")})
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	reloadedPrevious, err := client.IdentityAdoptionDecision.Get(ctx, previousDecision.ID)
	require.NoError(t, err)
	require.Nil(t, reloadedPrevious.IdentityID)

	currentDecision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, currentDecision.IdentityID)
	require.Equal(t, existingIdentity.ID, *currentDecision.IdentityID)

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, storedSession.ConsumedAt)
}

func TestExchangePendingOAuthCompletionLoginWithoutDecisionStillBindsIdentity(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("login-nodecision@example.com").
		SetUsername("legacy-name").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("login-nodecision-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("login-nodecision-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("login-nodecision-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "login-nodecision-user",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token": "access-token",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("login-nodecision-browser-session-key")})
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("login-nodecision-123"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, userEntity.ID, identity.UserID)

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, storedSession.ConsumedAt)
}

func TestExchangePendingOAuthCompletionExistingLoginWithSuggestedProfileSkipsAdoptionPrompt(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("existing-login@example.com").
		SetUsername("existing-login-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.AuthIdentity.Create().
		SetUserID(userEntity.ID).
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("existing-login-123").
		SetMetadata(map[string]any{
			"username": "existing-login-user",
		}).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("existing-login-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("existing-login-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("existing-login-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Existing Login Example",
			"suggested_avatar_url":   "https://cdn.example/existing-login.png",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token":  "legacy-access-token",
				"refresh_token": "legacy-refresh-token",
				"expires_in":    float64(3600),
				"token_type":    "Bearer",
				"redirect":      "/dashboard",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("existing-login-browser-session-key")})
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	payload := decodeJSONResponseData(t, recorder)
	require.NotEmpty(t, payload["access_token"])
	require.NotEmpty(t, payload["refresh_token"])
	require.NotEqual(t, "legacy-access-token", payload["access_token"])
	require.NotEqual(t, "legacy-refresh-token", payload["refresh_token"])
	require.Equal(t, "/dashboard", payload["redirect"])
	require.Equal(t, "Existing Login Example", payload["suggested_display_name"])
	require.Equal(t, "https://cdn.example/existing-login.png", payload["suggested_avatar_url"])
	require.NotContains(t, payload, "adoption_required")

	accessToken, ok := payload["access_token"].(string)
	require.True(t, ok)
	claims, err := handler.authService.ValidateToken(accessToken)
	require.NoError(t, err)
	reloadedUser, err := handler.userService.GetByID(ctx, userEntity.ID)
	require.NoError(t, err)
	require.Equal(t, reloadedUser.TokenVersion, claims.TokenVersion)

	decisionCount, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, decisionCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, storedSession.ConsumedAt)

	completion, ok := storedSession.LocalFlowState[oauthCompletionResponseKey].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, completion, "access_token")
	require.NotContains(t, completion, "refresh_token")
	require.NotContains(t, completion, "expires_in")
	require.NotContains(t, completion, "token_type")
	require.Equal(t, "/dashboard", completion["redirect"])
}

func TestExchangePendingOAuthCompletionBlocksBackendModeBeforeReturningTokenPayload(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyBackendModeEnabled: "true",
		},
	})
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("blocked@example.com").
		SetUsername("blocked-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("blocked-backend-mode-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("blocked-subject-123").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("blocked-backend-mode-browser-session-key").
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"access_token":  "access-token",
				"refresh_token": "refresh-token",
				"expires_in":    float64(3600),
				"token_type":    "Bearer",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("blocked-backend-mode-browser-session-key")})
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusForbidden, recorder.Code)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestExchangePendingOAuthCompletionRejectsDisabledTargetUser(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	userEntity, err := client.User.Create().
		SetEmail("disabled-linked@example.com").
		SetUsername("disabled-linked-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusDisabled).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("disabled-linked-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("disabled-linked-subject").
		SetTargetUserID(userEntity.ID).
		SetResolvedEmail(userEntity.Email).
		SetBrowserSessionKey("disabled-linked-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Disabled Linked User",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"redirect": "/dashboard",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", nil)
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("disabled-linked-browser-session-key")})
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusForbidden, recorder.Code)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestNormalizePendingOAuthCompletionResponseScrubsLegacyTokenPayload(t *testing.T) {
	payload := normalizePendingOAuthCompletionResponse(map[string]any{
		"access_token":  "legacy-access-token",
		"refresh_token": "legacy-refresh-token",
		"expires_in":    float64(3600),
		"token_type":    "Bearer",
		"redirect":      "/dashboard",
	})

	require.NotContains(t, payload, "access_token")
	require.NotContains(t, payload, "refresh_token")
	require.NotContains(t, payload, "expires_in")
	require.NotContains(t, payload, "token_type")
	require.Equal(t, "/dashboard", payload["redirect"])
}

func TestExchangePendingOAuthCompletionInvitationRequiredFalseFalsePersistsDecisionWithoutBinding(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, true)
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("invitation-required-session-token").
		SetIntent("login").
		SetProviderType("linuxdo").
		SetProviderKey("linuxdo").
		SetProviderSubject("invitation-123").
		SetBrowserSessionKey("invitation-required-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Invite Example",
			"suggested_avatar_url":   "https://cdn.example/invite.png",
		}).
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"error": "invitation_required",
			},
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"adopt_display_name":false,"adopt_avatar":false}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/exchange", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("invitation-required-browser-session-key")})
	ginCtx.Request = req

	handler.ExchangePendingOAuthCompletion(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	data := decodeJSONResponseData(t, recorder)
	require.Equal(t, "invitation_required", data["error"])
	require.Equal(t, true, data["adoption_required"])

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("linuxdo"),
			authidentity.ProviderKeyEQ("linuxdo"),
			authidentity.ProviderSubjectEQ("invitation-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, identityCount)

	decision, err := client.IdentityAdoptionDecision.Query().
		Where(identityadoptiondecision.PendingAuthSessionIDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Nil(t, decision.IdentityID)
	require.False(t, decision.AdoptDisplayName)
	require.False(t, decision.AdoptAvatar)

	storedSession, err := client.PendingAuthSession.Query().
		Where(pendingauthsession.IDEQ(session.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestCreateOIDCOAuthAccountCreatesUserBindsIdentityAndConsumesSession(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithEmailVerification(t, false, "fresh@example.com", "246810")
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("create-account-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-create-123").
		SetBrowserSessionKey("create-account-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Fresh OIDC User",
			"suggested_avatar_url":   "https://cdn.example/fresh.png",
		}).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"fresh@example.com","verify_code":"246810","password":"secret-123","adopt_display_name":false,"adopt_avatar":false}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("create-account-browser-session-key")})
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.NotEmpty(t, payload["access_token"])
	require.NotEmpty(t, payload["refresh_token"])
	require.Equal(t, "Bearer", payload["token_type"])

	createdUser, err := client.User.Query().Where(dbuser.EmailEQ("fresh@example.com")).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, createdUser.Status)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-create-123"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, createdUser.ID, identity.UserID)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, storedSession.ConsumedAt)
}



func TestCreateOIDCOAuthAccountDoesNotApplyPromoWhenDisabled(t *testing.T) {
	emailCache := &oauthPendingFlowEmailCacheStub{
		verificationCodes: map[string]*service.VerificationCodeData{
			"promo-disabled@example.com": {
				Code:      "246810",
				CreatedAt: time.Now().UTC(),
				ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
			},
		},
	}
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailVerifyEnabled: true,
		emailCache:         emailCache,
		settingValues: map[string]string{
		},
	})
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("promo-disabled-session-token").
		SetIntent(oauthIntentLogin).
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-promo-disabled-123").
		SetBrowserSessionKey("promo-disabled-browser-key").
		SetUpstreamIdentityClaims(map[string]any{"username": "promo_disabled_user"}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"promo-disabled@example.com","verify_code":"246810","password":"secret-123"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(session.BrowserSessionKey)})
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	createdUser, err := client.User.Query().Where(dbuser.EmailEQ("promo-disabled@example.com")).Only(ctx)
	require.NoError(t, err)
	require.Zero(t, createdUser.Balance)
}


func TestCreateOIDCOAuthAccountExistingEmailReturnsChoicePendingSessionState(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithEmailVerification(t, false, "owner@example.com", "135790")
	ctx := context.Background()

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("existing-email-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-existing-123").
		SetBrowserSessionKey("existing-email-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Existing OIDC User",
			"suggested_avatar_url":   "https://cdn.example/existing.png",
		}).
		SetRedirectTo("/dashboard").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"owner@example.com","verify_code":"135790","password":"secret-123"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("existing-email-browser-session-key")})
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "pending_session", payload["auth_result"])
	require.Equal(t, oauthIntentLogin, payload["intent"])
	require.Equal(t, "oidc", payload["provider"])
	require.Equal(t, "/dashboard", payload["redirect"])
	require.Equal(t, true, payload["adoption_required"])
	require.Equal(t, oauthPendingChoiceStep, payload["step"])
	require.Equal(t, "owner@example.com", payload["email"])
	require.Equal(t, "Existing OIDC User", payload["suggested_display_name"])
	require.Equal(t, "https://cdn.example/existing.png", payload["suggested_avatar_url"])

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, oauthIntentLogin, storedSession.Intent)
	require.NotNil(t, storedSession.TargetUserID)
	require.Equal(t, existingUser.ID, *storedSession.TargetUserID)
	require.Equal(t, "owner@example.com", storedSession.ResolvedEmail)
	require.Nil(t, storedSession.ConsumedAt)

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-existing-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, identityCount)
}

func TestCreateOIDCOAuthAccountExistingEmailNormalizesLegacySpacingAndCase(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithEmailVerification(t, false, "owner@example.com", "135790")
	ctx := context.Background()

	existingUser, err := client.User.Create().
		SetEmail(" Owner@Example.com ").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("existing-email-normalized-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-existing-normalized-123").
		SetBrowserSessionKey("existing-email-normalized-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Existing OIDC User",
			"suggested_avatar_url":   "https://cdn.example/existing.png",
		}).
		SetRedirectTo("/dashboard").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"owner@example.com","verify_code":"135790","password":"secret-123"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("existing-email-normalized-browser-session-key")})
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, oauthIntentLogin, payload["intent"])
	require.Equal(t, oauthPendingChoiceStep, payload["step"])

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, storedSession.TargetUserID)
	require.Equal(t, existingUser.ID, *storedSession.TargetUserID)
	require.Equal(t, "owner@example.com", storedSession.ResolvedEmail)
}

func TestCreateOIDCOAuthAccountRejectsEmailOutsideRegistrationSuffixWhitelist(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailVerifyEnabled: true,
		emailCache: &oauthPendingFlowEmailCacheStub{
			verificationCodes: map[string]*service.VerificationCodeData{
				"foo@gmail.com": {
					Code:      "135790",
					CreatedAt: time.Now().UTC(),
					ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
				},
			},
		},
		settingValues: map[string]string{
			service.SettingKeyRegistrationEmailSuffixWhitelist: `["@qq.com"]`,
		},
	})
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("suffix-whitelist-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-suffix-whitelist-123").
		SetBrowserSessionKey("suffix-whitelist-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"foo@gmail.com","verify_code":"135790","password":"secret-123"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("suffix-whitelist-browser-session-key")})
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	payload := decodeJSONBody(t, recorder)
	require.Equal(t, "EMAIL_SUFFIX_NOT_ALLOWED", payload["reason"])

	count, err := client.User.Query().Where(dbuser.EmailEQ("foo@gmail.com")).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestSendPendingOAuthVerifyCodeExistingEmailReturnsBindLoginState(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithEmailVerification(t, false, "owner@example.com", "135790")
	ctx := context.Background()

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("existing-email-send-code-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-existing-send-code-123").
		SetBrowserSessionKey("existing-email-send-code-browser-session-key").
		SetLocalFlowState(map[string]any{
			oauthCompletionResponseKey: map[string]any{
				"step": "email_required",
			},
		}).
		SetRedirectTo("/dashboard").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"owner@example.com"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/pending/send-verify-code", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("existing-email-send-code-browser-session-key")})
	ginCtx.Request = req

	handler.SendPendingOAuthVerifyCode(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "pending_session", payload["auth_result"])
	require.Equal(t, oauthPendingChoiceStep, payload["step"])
	require.Equal(t, "owner@example.com", payload["email"])

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, oauthIntentLogin, storedSession.Intent)
	require.NotNil(t, storedSession.TargetUserID)
	require.Equal(t, existingUser.ID, *storedSession.TargetUserID)
	require.Equal(t, "owner@example.com", storedSession.ResolvedEmail)
}

func TestCreateOIDCOAuthAccountBlocksBackendModeBeforeCreatingUser(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailVerifyEnabled: true,
		emailCache: &oauthPendingFlowEmailCacheStub{
			verificationCodes: map[string]*service.VerificationCodeData{
				"fresh@example.com": {
					Code:      "246810",
					CreatedAt: time.Now().UTC(),
					ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
				},
			},
		},
		settingValues: map[string]string{
			service.SettingKeyBackendModeEnabled: "true",
		},
	})
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("create-account-backend-mode-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-create-backend-mode-123").
		SetBrowserSessionKey("create-account-backend-mode-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"fresh@example.com","verify_code":"246810","password":"secret-123"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("create-account-backend-mode-browser-session-key")})
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusForbidden, recorder.Code)

	userCount, err := client.User.Query().Where(dbuser.EmailEQ("fresh@example.com")).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, userCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestLogoutClearsPendingOAuthAndBindCookies(t *testing.T) {
	handler, _ := newOAuthPendingFlowTestHandler(t, false)

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue("pending-session-token")})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("pending-browser-key")})
	req.AddCookie(&http.Cookie{Name: oauthBindAccessTokenCookieName, Value: "bind-token"})
	ginCtx.Request = req

	handler.Logout(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, -1, findCookie(recorder.Result().Cookies(), oauthPendingSessionCookieName).MaxAge)
	require.Equal(t, -1, findCookie(recorder.Result().Cookies(), oauthPendingBrowserCookieName).MaxAge)
	require.Equal(t, -1, findCookie(recorder.Result().Cookies(), oauthBindAccessTokenCookieName).MaxAge)
}

func TestCreateOIDCOAuthAccountRollsBackCreatedUserWhenBindingFails(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithEmailVerification(t, true, "fresh@example.com", "246810")
	ctx := context.Background()

	conflictOwner, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.AuthIdentity.Create().
		SetUserID(conflictOwner.ID).
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-conflict-123").
		SetMetadata(map[string]any{
			"username": "owner-user",
		}).
		Save(ctx)
	require.NoError(t, err)

	invitation, err := client.RedeemCode.Create().
		SetCode("INVITE123").
		SetType(service.RedeemTypeInvitation).
		SetStatus(service.StatusUnused).
		SetValue(0).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("create-account-conflict-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-conflict-123").
		SetBrowserSessionKey("create-account-conflict-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
		}).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"fresh@example.com","verify_code":"246810","password":"secret-123","invitation_code":"INVITE123"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("create-account-conflict-browser-session-key")})
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusConflict, recorder.Code)

	userCount, err := client.User.Query().Where(dbuser.EmailEQ("fresh@example.com")).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, userCount)

	storedInvitation, err := client.RedeemCode.Get(ctx, invitation.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusUnused, storedInvitation.Status)
	require.Nil(t, storedInvitation.UsedBy)
	require.Nil(t, storedInvitation.UsedAt)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestCreateOIDCOAuthAccountRollsBackPostBindFailureBeforeIdentityCanCommit(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		emailVerifyEnabled: true,
		emailCache: &oauthPendingFlowEmailCacheStub{
			verificationCodes: map[string]*service.VerificationCodeData{
				"fresh@example.com": {
					Code:      "246810",
					CreatedAt: time.Now().UTC(),
					ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
				},
			},
		},
		userRepoOptions: oauthPendingFlowUserRepoOptions{
			rejectDeleteWhileAuthIdentityExists: true,
		},
	})
	ctx := context.Background()

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("create-account-finalize-failure-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-finalize-failure-123").
		SetBrowserSessionKey("create-account-finalize-failure-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
		}).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	pendingOAuthCreateAccountPreCommitHook = func(context.Context, *dbent.PendingAuthSession) error {
		return errors.New("forced post-bind failure")
	}
	t.Cleanup(func() {
		pendingOAuthCreateAccountPreCommitHook = nil
	})

	body := bytes.NewBufferString(`{"email":"fresh@example.com","verify_code":"246810","password":"secret-123"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/create-account", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("create-account-finalize-failure-browser-session-key")})
	ginCtx.Request = req

	handler.CreateOIDCOAuthAccount(ginCtx)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	userCount, err := client.User.Query().Where(dbuser.EmailEQ("fresh@example.com")).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, userCount)

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-finalize-failure-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, identityCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestBindOIDCOAuthLoginBindsExistingUserAndConsumesSession(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
	require.NoError(t, err)

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-login-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("bind-login-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Bound OIDC User",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
		}).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123","adopt_display_name":false,"adopt_avatar":false}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-login-browser-session-key")})
	ginCtx.Request = req

	handler.BindOIDCOAuthLogin(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.NotEmpty(t, payload["access_token"])
	require.NotEmpty(t, payload["refresh_token"])
	require.Equal(t, "Bearer", payload["token_type"])

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-bind-123"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, existingUser.ID, identity.UserID)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, storedSession.ConsumedAt)
}

func TestBindOIDCOAuthLoginBlocksBackendModeBeforeTokenIssue(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyBackendModeEnabled: "true",
		},
	})
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
	require.NoError(t, err)

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-login-backend-mode-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-backend-mode-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("bind-login-backend-mode-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username": "oidc_user",
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-login-backend-mode-browser-session-key")})
	ginCtx.Request = req

	handler.BindOIDCOAuthLogin(ginCtx)

	require.Equal(t, http.StatusForbidden, recorder.Code)

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-bind-backend-mode-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, identityCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestBindOIDCOAuthLoginRejectsInvalidPasswordWithoutConsumingSession(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
	require.NoError(t, err)

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-login-invalid-password-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-invalid-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("bind-login-invalid-password-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Bound OIDC User",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
		}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"owner@example.com","password":"wrong-password"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-login-invalid-password-browser-session-key")})
	ginCtx.Request = req

	handler.BindOIDCOAuthLogin(ginCtx)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	payload := decodeJSONBody(t, recorder)
	require.Equal(t, "INVALID_CREDENTIALS", payload["reason"])

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-bind-invalid-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, identityCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestBindOIDCOAuthLoginReclaimsIdentityOwnedBySoftDeletedUser(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	ctx := context.Background()

	oldOwnerHash, err := handler.authService.HashPassword("old-secret")
	require.NoError(t, err)
	oldOwner, err := client.User.Create().
		SetEmail("old-owner@example.com").
		SetUsername("old-owner").
		SetPasswordHash(oldOwnerHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	identity, err := client.AuthIdentity.Create().
		SetUserID(oldOwner.ID).
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-soft-deleted-123").
		SetMetadata(map[string]any{"username": "old-owner"}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.User.Delete().Where(dbuser.IDEQ(oldOwner.ID)).Exec(ctx)
	require.NoError(t, err)

	newOwnerHash, err := handler.authService.HashPassword("secret-123")
	require.NoError(t, err)
	newOwner, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(newOwnerHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-login-soft-deleted-owner-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-soft-deleted-123").
		SetTargetUserID(newOwner.ID).
		SetResolvedEmail(newOwner.Email).
		SetBrowserSessionKey("bind-login-soft-deleted-owner-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"username":               "oidc_user",
			"suggested_display_name": "Recovered OIDC User",
		}).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123","adopt_display_name":false,"adopt_avatar":false}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-login-soft-deleted-owner-browser-session-key")})
	ginCtx.Request = req

	handler.BindOIDCOAuthLogin(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)

	identity, err = client.AuthIdentity.Get(ctx, identity.ID)
	require.NoError(t, err)
	require.Equal(t, newOwner.ID, identity.UserID)
}

func TestBindOIDCOAuthLoginAppliesFirstBindGrantOnce(t *testing.T) {
	defaultSubAssigner := &oauthPendingFlowDefaultSubAssignerStub{}
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyAuthSourceDefaultOIDCBalance:          "12.5",
			service.SettingKeyAuthSourceDefaultOIDCConcurrency:      "3",
			service.SettingKeyAuthSourceDefaultOIDCSubscriptions:    `[{"group_id":101,"validity_days":30}]`,
			service.SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind: "true",
		},
		defaultSubAssigner: defaultSubAssigner,
	})
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
	require.NoError(t, err)

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetBalance(5).
		SetConcurrency(2).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	firstSession, err := client.PendingAuthSession.Create().
		SetSessionToken("first-bind-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-first-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("first-bind-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Bound OIDC User",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
		}).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	firstBody := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123","adopt_display_name":false,"adopt_avatar":false}`)
	firstRecorder := httptest.NewRecorder()
	firstGinCtx, _ := gin.CreateTestContext(firstRecorder)
	firstReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", firstBody)
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(firstSession.SessionToken)})
	firstReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("first-bind-browser-session-key")})
	firstGinCtx.Request = firstReq

	handler.BindOIDCOAuthLogin(firstGinCtx)

	require.Equal(t, http.StatusOK, firstRecorder.Code)

	storedUser, err := client.User.Get(ctx, existingUser.ID)
	require.NoError(t, err)
	require.Equal(t, 17.5, storedUser.Balance)
	require.Equal(t, 5, storedUser.Concurrency)
	require.Zero(t, storedUser.TotalRecharged)
	require.Len(t, defaultSubAssigner.calls, 1)
	require.Equal(t, int64(existingUser.ID), defaultSubAssigner.calls[0].UserID)
	require.Equal(t, int64(101), defaultSubAssigner.calls[0].GroupID)
	require.Equal(t, 30, defaultSubAssigner.calls[0].ValidityDays)
	require.Equal(t, 1, countProviderGrantRecords(t, client, existingUser.ID, "oidc", "first_bind"))

	secondSession, err := client.PendingAuthSession.Create().
		SetSessionToken("second-bind-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-second-456").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("second-bind-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Second OIDC User",
			"suggested_avatar_url":   "https://cdn.example/second.png",
		}).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	secondBody := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123","adopt_display_name":false,"adopt_avatar":false}`)
	secondRecorder := httptest.NewRecorder()
	secondGinCtx, _ := gin.CreateTestContext(secondRecorder)
	secondReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", secondBody)
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(secondSession.SessionToken)})
	secondReq.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("second-bind-browser-session-key")})
	secondGinCtx.Request = secondReq

	handler.BindOIDCOAuthLogin(secondGinCtx)

	require.Equal(t, http.StatusOK, secondRecorder.Code)

	storedUser, err = client.User.Get(ctx, existingUser.ID)
	require.NoError(t, err)
	require.Equal(t, 17.5, storedUser.Balance)
	require.Equal(t, 5, storedUser.Concurrency)
	require.Zero(t, storedUser.TotalRecharged)
	require.Len(t, defaultSubAssigner.calls, 1)
	require.Equal(t, 1, countProviderGrantRecords(t, client, existingUser.ID, "oidc", "first_bind"))
}

func TestResolvePendingOAuthTargetUserIDNormalizesLegacySpacingAndCase(t *testing.T) {
	handler, client := newOAuthPendingFlowTestHandler(t, false)
	_ = handler
	ctx := context.Background()

	existingUser, err := client.User.Create().
		SetEmail(" Owner@Example.com ").
		SetUsername("owner-user").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("resolve-target-session-token").
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-target-123").
		SetResolvedEmail("owner@example.com").
		SetBrowserSessionKey("resolve-target-browser-session-key").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	resolvedUserID, err := resolvePendingOAuthTargetUserID(ctx, client, session)
	require.NoError(t, err)
	require.Equal(t, existingUser.ID, resolvedUserID)
}

func TestBindOIDCOAuthLoginReturns2FAChallengeWhenUserHasTotp(t *testing.T) {
	totpCache := &oauthPendingFlowTotpCacheStub{}
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyTotpEnabled: "true",
		},
		totpCache:     totpCache,
		totpEncryptor: oauthPendingFlowTotpEncryptorStub{},
	})
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
	require.NoError(t, err)
	totpEnabledAt := time.Now().UTC().Add(-time.Hour)
	secret := "JBSWY3DPEHPK3PXP"

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetTotpEnabled(true).
		SetTotpSecretEncrypted(secret).
		SetTotpEnabledAt(totpEnabledAt).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("bind-login-2fa-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-bind-2fa-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("bind-login-2fa-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Bound OIDC User",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
		}).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"owner@example.com","password":"secret-123","adopt_display_name":false,"adopt_avatar":false}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/oidc/bind-login", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue("bind-login-2fa-browser-session-key")})
	ginCtx.Request = req

	handler.BindOIDCOAuthLogin(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	data := decodeJSONResponseData(t, recorder)
	require.Equal(t, true, data["requires_2fa"])
	require.Equal(t, "o***r@example.com", data["user_email_masked"])
	tempToken, ok := data["temp_token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, tempToken)

	loginSession, err := totpCache.GetLoginSession(ctx, tempToken)
	require.NoError(t, err)
	require.NotNil(t, loginSession)
	require.NotNil(t, loginSession.PendingOAuthBind)
	require.Equal(t, session.SessionToken, loginSession.PendingOAuthBind.PendingSessionToken)
	require.Equal(t, session.BrowserSessionKey, loginSession.PendingOAuthBind.BrowserSessionKey)

	identityCount, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-bind-2fa-123"),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, identityCount)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Nil(t, storedSession.ConsumedAt)
}

func TestLogin2FACompletesPendingOAuthBindAndConsumesSession(t *testing.T) {
	totpCache := &oauthPendingFlowTotpCacheStub{}
	defaultSubAssigner := &oauthPendingFlowDefaultSubAssignerStub{}
	handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		settingValues: map[string]string{
			service.SettingKeyTotpEnabled:                           "true",
			service.SettingKeyAuthSourceDefaultOIDCBalance:          "8",
			service.SettingKeyAuthSourceDefaultOIDCConcurrency:      "2",
			service.SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind: "true",
		},
		defaultSubAssigner: defaultSubAssigner,
		totpCache:          totpCache,
		totpEncryptor:      oauthPendingFlowTotpEncryptorStub{},
	})
	ctx := context.Background()

	passwordHash, err := handler.authService.HashPassword("secret-123")
	require.NoError(t, err)
	totpEnabledAt := time.Now().UTC().Add(-time.Hour)
	secret := "JBSWY3DPEHPK3PXP"

	existingUser, err := client.User.Create().
		SetEmail("owner@example.com").
		SetUsername("owner-user").
		SetPasswordHash(passwordHash).
		SetBalance(1.5).
		SetConcurrency(4).
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetTotpEnabled(true).
		SetTotpSecretEncrypted(secret).
		SetTotpEnabledAt(totpEnabledAt).
		Save(ctx)
	require.NoError(t, err)

	session, err := client.PendingAuthSession.Create().
		SetSessionToken("login-2fa-pending-session-token").
		SetIntent("adopt_existing_user_by_email").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-login-2fa-123").
		SetTargetUserID(existingUser.ID).
		SetResolvedEmail(existingUser.Email).
		SetBrowserSessionKey("login-2fa-browser-session-key").
		SetUpstreamIdentityClaims(map[string]any{
			"suggested_display_name": "Bound OIDC User",
			"suggested_avatar_url":   "https://cdn.example/bound.png",
		}).
		SetRedirectTo("/profile").
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.IdentityAdoptionDecision.Create().
		SetPendingAuthSessionID(session.ID).
		SetAdoptDisplayName(false).
		SetAdoptAvatar(false).
		Save(ctx)
	require.NoError(t, err)

	tempToken, err := handler.totpService.CreatePendingOAuthBindLoginSession(
		ctx,
		existingUser.ID,
		existingUser.Email,
		session.SessionToken,
		session.BrowserSessionKey,
	)
	require.NoError(t, err)

	code, err := totp.GenerateCode(secret, time.Now().UTC())
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"temp_token":"` + tempToken + `","totp_code":"` + code + `"}`)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/2fa", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(session.SessionToken)})
	req.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(session.BrowserSessionKey)})
	ginCtx.Request = req

	handler.Login2FA(ginCtx)

	require.Equal(t, http.StatusOK, recorder.Code)
	payload := decodeJSONResponseData(t, recorder)
	require.NotEmpty(t, payload["access_token"])
	require.NotEmpty(t, payload["refresh_token"])
	accessToken, ok := payload["access_token"].(string)
	require.True(t, ok)
	claims, err := handler.authService.ValidateToken(accessToken)
	require.NoError(t, err)
	reloadedUser, err := handler.userService.GetByID(ctx, existingUser.ID)
	require.NoError(t, err)
	require.Equal(t, reloadedUser.TokenVersion, claims.TokenVersion)

	identity, err := client.AuthIdentity.Query().
		Where(
			authidentity.ProviderTypeEQ("oidc"),
			authidentity.ProviderKeyEQ("https://issuer.example"),
			authidentity.ProviderSubjectEQ("oidc-login-2fa-123"),
		).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, existingUser.ID, identity.UserID)

	storedSession, err := client.PendingAuthSession.Get(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, storedSession.ConsumedAt)

	loginSession, err := totpCache.GetLoginSession(ctx, tempToken)
	require.NoError(t, err)
	require.Nil(t, loginSession)

	storedUser, err := client.User.Get(ctx, existingUser.ID)
	require.NoError(t, err)
	require.Equal(t, 9.5, storedUser.Balance)
	require.Equal(t, 6, storedUser.Concurrency)
	require.Equal(t, 1, countProviderGrantRecords(t, client, existingUser.ID, "oidc", "first_bind"))
	require.Empty(t, defaultSubAssigner.calls)
}

func newOAuthPendingFlowTestHandler(t *testing.T, invitationEnabled bool) (*AuthHandler, *dbent.Client) {
	t.Helper()

	return newOAuthPendingFlowTestHandlerWithOptions(t, invitationEnabled, false, nil)
}

func newOAuthPendingFlowTestHandlerWithEmailVerification(
	t *testing.T,
	invitationEnabled bool,
	email string,
	code string,
) (*AuthHandler, *dbent.Client) {
	t.Helper()

	cache := &oauthPendingFlowEmailCacheStub{
		verificationCodes: map[string]*service.VerificationCodeData{
			email: {
				Code:      code,
				Attempts:  0,
				CreatedAt: time.Now().UTC(),
				ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
			},
		},
	}
	return newOAuthPendingFlowTestHandlerWithOptions(t, invitationEnabled, true, cache)
}

func newOAuthPendingFlowTestHandlerWithOptions(
	t *testing.T,
	invitationEnabled bool,
	emailVerifyEnabled bool,
	emailCache service.EmailCache,
) (*AuthHandler, *dbent.Client) {
	return newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
		invitationEnabled:  invitationEnabled,
		emailVerifyEnabled: emailVerifyEnabled,
		emailCache:         emailCache,
	})
}

type oauthPendingFlowTestHandlerOptions struct {
	invitationEnabled  bool
	emailVerifyEnabled bool
	emailCache         service.EmailCache
	settingValues      map[string]string
	defaultSubAssigner service.DefaultSubscriptionAssigner
	affiliateService   *service.AffiliateService
	affiliateFactory   func(*dbent.Client, *service.SettingService) *service.AffiliateService
	totpCache          service.TotpCache
	totpEncryptor      service.SecretEncryptor
	userRepoOptions    oauthPendingFlowUserRepoOptions
}

func newOAuthPendingFlowTestHandlerWithDependencies(
	t *testing.T,
	options oauthPendingFlowTestHandlerOptions,
) (*AuthHandler, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:auth_oauth_pending_flow_handler?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS user_provider_default_grants (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	provider_type TEXT NOT NULL,
	grant_reason TEXT NOT NULL DEFAULT 'first_bind',
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(user_id, provider_type, grant_reason)
)`)
	require.NoError(t, err)
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS user_avatars (
	user_id INTEGER PRIMARY KEY,
	storage_provider TEXT NOT NULL,
	storage_key TEXT NOT NULL DEFAULT '',
	url TEXT NOT NULL,
	content_type TEXT NOT NULL DEFAULT '',
	byte_size INTEGER NOT NULL DEFAULT 0,
	sha256 TEXT NOT NULL DEFAULT '',
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
)`)
	require.NoError(t, err)
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS user_affiliates (
	user_id INTEGER PRIMARY KEY,
	aff_code TEXT NOT NULL UNIQUE,
	aff_code_custom BOOLEAN NOT NULL DEFAULT false,
	aff_rebate_rate_percent REAL NULL,
	inviter_id INTEGER NULL,
	aff_count INTEGER NOT NULL DEFAULT 0,
	aff_quota REAL NOT NULL DEFAULT 0,
	aff_frozen_quota REAL NOT NULL DEFAULT 0,
	aff_history_quota REAL NOT NULL DEFAULT 0,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
)`)
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
		},
		Default: config.DefaultConfig{
			UserBalance:     0,
			UserConcurrency: 1,
		},
	}
	settingValues := map[string]string{
		service.SettingKeyRegistrationEnabled:              "true",
		service.SettingKeyInvitationCodeEnabled:            boolSettingValue(options.invitationEnabled),
		service.SettingKeyEmailVerifyEnabled:               boolSettingValue(options.emailVerifyEnabled),
		service.SettingKeyRegistrationEmailSuffixWhitelist: "[]",
	}
	for key, value := range options.settingValues {
		settingValues[key] = value
	}
	settingSvc := service.NewSettingService(&oauthPendingFlowSettingRepoStub{values: settingValues}, cfg)
	affiliateService := options.affiliateService
	if affiliateService == nil && options.affiliateFactory != nil {
		affiliateService = options.affiliateFactory(client, settingSvc)
	}
	userRepo := &oauthPendingFlowUserRepo{
		client:  client,
		options: options.userRepoOptions,
	}
	redeemRepo := &oauthPendingFlowRedeemCodeRepo{client: client}
	}
	var emailService *service.EmailService
	if options.emailCache != nil {
		emailService = service.NewEmailService(&oauthPendingFlowSettingRepoStub{
			values: map[string]string{
				service.SettingKeyEmailVerifyEnabled: boolSettingValue(options.emailVerifyEnabled),
			},
		}, options.emailCache)
	}
	authSvc := service.NewAuthService(
		client,
		userRepo,
		redeemRepo,
		&oauthPendingFlowRefreshTokenCacheStub{},
		cfg,
		settingSvc,
		emailService,
		nil,
		nil,
		
		options.defaultSubAssigner,
		affiliateService,
		nil,
	)
	userSvc := service.NewUserService(userRepo, nil, nil, nil)
	var totpSvc *service.TotpService
	if options.totpCache != nil || options.totpEncryptor != nil {
		totpCache := options.totpCache
		if totpCache == nil {
			totpCache = &oauthPendingFlowTotpCacheStub{}
		}
		totpEncryptor := options.totpEncryptor
		if totpEncryptor == nil {
			totpEncryptor = oauthPendingFlowTotpEncryptorStub{}
		}
		totpSvc = service.NewTotpService(userRepo, totpEncryptor, totpCache, settingSvc, nil, nil)
	}

	return &AuthHandler{
		authService:  authSvc,
		userService:  userSvc,
		settingSvc:   settingSvc,
		totpService:  totpSvc,
	}, client
}

func boolSettingValue(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func boolPtr(v bool) *bool {
	return &v
}

type oauthPendingFlowSettingRepoStub struct {
	values map[string]string
}

