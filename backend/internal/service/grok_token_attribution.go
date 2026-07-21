package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type grokCredentialDescriptor struct {
	accountID   int64
	version     int64
	fingerprint string
	oauth       bool
	source      string
}

type grokUnauthorizedAttribution struct {
	state              string
	sentVersion        int64
	currentVersion     int64
	sentFingerprint    string
	currentFingerprint string
	source             string
}

const grokUnauthorizedAttributionKey = "grok_unauthorized_token_attribution"

func describeGrokCredential(account *Account, token string) grokCredentialDescriptor {
	if account == nil {
		return grokCredentialDescriptor{}
	}
	fingerprint := hashSensitiveValueForLog(token)
	version := int64(0)
	if fingerprint != "" && fingerprint == hashSensitiveValueForLog(account.GetGrokAccessToken()) {
		version = account.GetCredentialAsInt64("_token_version")
	}
	return grokCredentialDescriptor{
		accountID:   account.ID,
		version:     version,
		fingerprint: fingerprint,
		oauth:       account.Type == AccountTypeOAuth,
	}
}

func (s *OpenAIGatewayService) getGrokAccessTokenWithDescriptor(
	ctx context.Context,
	account *Account,
) (string, string, grokCredentialDescriptor, error) {
	credentialAccount := account
	if account != nil && account.IsShadow() {
		if s == nil || s.accountRepo == nil || account.ParentAccountID == nil {
			return "", "", grokCredentialDescriptor{}, fmt.Errorf("resolve grok credential owner: account repository unavailable")
		}
		parent, err := s.accountRepo.GetByID(ctx, *account.ParentAccountID)
		if err != nil {
			return "", "", grokCredentialDescriptor{}, fmt.Errorf("resolve grok credential owner %d: %w", *account.ParentAccountID, err)
		}
		if parent == nil || parent.IsShadow() || parent.Platform != PlatformGrok {
			return "", "", grokCredentialDescriptor{}, fmt.Errorf("invalid grok credential owner %d", *account.ParentAccountID)
		}
		credentialAccount = parent
	}
	token, kind, err := s.GetAccessToken(ctx, credentialAccount)
	if err != nil {
		return "", "", grokCredentialDescriptor{}, err
	}
	descriptor := describeGrokCredential(credentialAccount, token)
	descriptor.source = "account"
	if credentialAccount.Type == AccountTypeOAuth && s.grokTokenProvider != nil {
		descriptor.source = "provider"
	}
	return token, kind, descriptor, nil
}

func (s *OpenAIGatewayService) attributeGrokUnauthorized(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	statusCode int,
	sent grokCredentialDescriptor,
) grokUnauthorizedAttribution {
	if statusCode != http.StatusUnauthorized {
		return grokUnauthorizedAttribution{}
	}
	attribution := grokUnauthorizedAttribution{
		state:           "unknown",
		sentVersion:     sent.version,
		sentFingerprint: sent.fingerprint,
		source:          sent.source,
	}
	if sent.oauth && s != nil && s.accountRepo != nil && sent.accountID > 0 {
		latest, err := s.accountRepo.GetByID(ctx, sent.accountID)
		if err == nil && latest != nil {
			attribution.currentVersion = latest.GetCredentialAsInt64("_token_version")
			attribution.currentFingerprint = hashSensitiveValueForLog(latest.GetGrokAccessToken())
			if attribution.sentFingerprint != "" && attribution.currentFingerprint != "" {
				if attribution.sentFingerprint == attribution.currentFingerprint {
					attribution.state = "current"
				} else {
					attribution.state = "stale"
				}
			}
		}
	}
	logger.L().Info("grok.unauthorized_token_attribution",
		zap.Int64("account_id", accountID(account)),
		zap.Int64("credential_account_id", sent.accountID),
		zap.String("token_state", attribution.state),
		zap.String("token_source", sent.source),
		zap.Int64("sent_token_version", attribution.sentVersion),
		zap.Int64("current_token_version", attribution.currentVersion),
		zap.String("sent_token_fingerprint", attribution.sentFingerprint),
		zap.String("current_token_fingerprint", attribution.currentFingerprint),
		zap.Int("upstream_attempt", grokUpstreamAttempt(c)),
	)
	if c != nil {
		c.Set(grokUnauthorizedAttributionKey, attribution)
	}
	return attribution
}

func shouldPenalizeGrokUnauthorized(statusCode int, attribution grokUnauthorizedAttribution) bool {
	return statusCode != http.StatusUnauthorized || !strings.EqualFold(attribution.state, "stale")
}

func applyGrokUnauthorizedAttribution(ev *OpsUpstreamErrorEvent, attribution grokUnauthorizedAttribution) {
	if ev == nil || attribution.state == "" {
		return
	}
	ev.TokenState = attribution.state
	ev.TokenSource = attribution.source
	ev.SentTokenVersion = attribution.sentVersion
	ev.CurrentTokenVersion = attribution.currentVersion
	ev.SentTokenFingerprint = attribution.sentFingerprint
	ev.CurrentTokenFingerprint = attribution.currentFingerprint
}

func applyGrokUnauthorizedAttributionFromContext(c *gin.Context, ev *OpsUpstreamErrorEvent) {
	if c == nil {
		return
	}
	value, ok := c.Get(grokUnauthorizedAttributionKey)
	if !ok {
		return
	}
	attribution, ok := value.(grokUnauthorizedAttribution)
	if !ok {
		return
	}
	applyGrokUnauthorizedAttribution(ev, attribution)
}

func accountID(account *Account) int64 {
	if account == nil {
		return 0
	}
	return account.ID
}
