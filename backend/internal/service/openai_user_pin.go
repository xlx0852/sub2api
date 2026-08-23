package service

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// openAIUserPinTTL 用户级钉号 TTL。对齐官方 7d 配额窗口：钉到窗口自然过期，
// 而不是像会话粘性那样 1 小时漂移。每次命中刷新，活跃用户不会中途掉钉。
const openAIUserPinTTL = 7 * 24 * time.Hour

// openAIUserPinCacheKey 生成 API Key 级钉号的缓存键。
// 与会话粘性（openai:<session-hash>）不同命名空间，互不干扰。
func openAIUserPinCacheKey(apiKeyID int64) string {
	if apiKeyID <= 0 {
		return ""
	}
	return "userpin:" + strconv.FormatInt(apiKeyID, 10)
}

// openAIUserPinAPIKeyIDFromContext 从请求 context 取认证后的本地 API Key ID。
func openAIUserPinAPIKeyIDFromContext(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	id, _ := ctx.Value(ctxkey.OpenAIUserPinAPIKeyID).(int64)
	if id <= 0 {
		return 0
	}
	return id
}

// openAIUserPinEnabled 是否启用用户级钉号。
func (s *OpenAIGatewayService) openAIUserPinEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIUserPinEnabled
}

// getUserPinnedAccountID 读取 API Key 当前钉住的上游账号 ID；0 表示未钉。
func (s *OpenAIGatewayService) getUserPinnedAccountID(ctx context.Context, groupID *int64, apiKeyID int64) int64 {
	if s == nil || s.cache == nil || !s.openAIUserPinEnabled() || apiKeyID <= 0 {
		return 0
	}
	key := openAIUserPinCacheKey(apiKeyID)
	if key == "" {
		return 0
	}
	accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), key)
	if err != nil || accountID <= 0 {
		return 0
	}
	return accountID
}

// bindUserPinAccount 把 API Key 钉到指定上游账号并刷新 TTL。
// 只对 OpenAI OAuth 账号生效：API Key 池账号按 key 计费，无共享指纹污染问题。
func (s *OpenAIGatewayService) bindUserPinAccount(ctx context.Context, groupID *int64, apiKeyID, accountID int64) {
	if s == nil || s.cache == nil || !s.openAIUserPinEnabled() || apiKeyID <= 0 || accountID <= 0 {
		return
	}
	key := openAIUserPinCacheKey(apiKeyID)
	if key == "" {
		return
	}
	if err := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), key, accountID, openAIUserPinTTL); err != nil {
		slog.Warn("openai_user_pin_bind_failed",
			"api_key_id", apiKeyID,
			"account_id", accountID,
			"error", err,
		)
	}
}

// refreshUserPinTTL 钉号命中时续期，活跃用户不掉钉。
func (s *OpenAIGatewayService) refreshUserPinTTL(ctx context.Context, groupID *int64, apiKeyID int64) {
	if s == nil || s.cache == nil || apiKeyID <= 0 {
		return
	}
	key := openAIUserPinCacheKey(apiKeyID)
	if key == "" {
		return
	}
	_ = s.cache.RefreshSessionTTL(ctx, derefGroupID(groupID), key, openAIUserPinTTL)
}

// deleteUserPin 账号硬失败后拆钉；下一次调度会重新选号并重钉。
func (s *OpenAIGatewayService) deleteUserPin(ctx context.Context, groupID *int64, apiKeyID int64) {
	if s == nil || s.cache == nil || apiKeyID <= 0 {
		return
	}
	key := openAIUserPinCacheKey(apiKeyID)
	if key == "" {
		return
	}
	_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), key)
}
