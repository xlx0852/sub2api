// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"errors"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrAccountNotFound             = infraerrors.NotFound("ACCOUNT_NOT_FOUND", "account not found")
	ErrAccountNilInput             = infraerrors.BadRequest("ACCOUNT_NIL_INPUT", "account input cannot be nil")
	ErrAccountNotInFallback        = infraerrors.BadRequest("ACCOUNT_NOT_IN_FALLBACK", "account is not in proxy fallback state")
	ErrRetiredBedrockAccount       = infraerrors.BadRequest("BEDROCK_ACCOUNT_RETIRED", "aws bedrock account type has been retired; use anthropic api-key or other providers")
	ErrRetiredVertexServiceAccount = infraerrors.BadRequest("VERTEX_SERVICE_ACCOUNT_RETIRED", "vertex service account has been retired; use anthropic/gemini api-key, oauth, or other providers")
	// ErrRetiredClaudeOAuthAccount 表示 Anthropic 上游账号的 OAuth / setup-token
	// 授权接入能力已下线，控制面拒绝新增、转换为该系统类型，运行时拒绝刷新与调用。
	ErrRetiredClaudeOAuthAccount = infraerrors.BadRequest("CLAUDE_OAUTH_ACCOUNT_RETIRED", "anthropic claude oauth/setup-token sign-in has been retired; use anthropic api-key instead")
	// ErrAccountNotSchedulable 表示账号处于 error/disabled 等不可打开调度的状态。
	ErrAccountNotSchedulable = errors.New("account cannot be enabled for scheduling in current status")
)

const RetiredAccountTypeBedrock = "bedrock"

const AccountListGroupUngrouped int64 = -1

const AccountPrivacyModeUnsetFilter = "__unset__"

// 账号列表特殊 status 过滤值（前端回收站）
const (
	AccountListStatusTrash   = "trash"
	AccountListStatusDeleted = "deleted"
	AccountListStatusBanned  = "banned"
)

// AccountBulkUpdate describes the fields that can be updated in a bulk operation.
// Nil pointers mean "do not change".
type AccountBulkUpdate struct {
	Name           *string
	ProxyID        *int64
	Concurrency    *int
	Priority       *int
	RateMultiplier *float64
	LoadFactor     *int
	Status         *string
	Schedulable    *bool
	Credentials    map[string]any
	Extra          map[string]any
}
