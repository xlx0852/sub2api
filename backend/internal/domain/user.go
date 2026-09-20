// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrUserNotFound = infraerrors.NotFound("USER_NOT_FOUND", "user not found")
var ErrEmailExists = infraerrors.Conflict("EMAIL_EXISTS", "email already exists")

var ErrIdentityProviderInvalid = infraerrors.BadRequest("IDENTITY_PROVIDER_INVALID", "identity provider is invalid")

// UserListFilters contains all filter options for listing users
type UserListFilters struct {
	Status    string // User status filter
	Role      string // User role filter
	Search    string // Search in email, username
	GroupName string // Filter by allowed group name (fuzzy match)
	// APIKeyGroupID filters users who own at least one non-soft-deleted API key
	// bound to this group (api_keys.group_id). 0 = no filter. Covers all three
	// group types since it matches the key's group directly, not allowed_groups.
	APIKeyGroupID int64
	Attributes    map[int64]string // Custom attribute filters: attributeID -> value
	// IncludeSubscriptions controls whether ListWithFilters should load active subscriptions.
	// User subscription mode is retired; default is false even when the query param is omitted.
	IncludeSubscriptions *bool
	// IncludeDeleted 为 true 时绕过软删除过滤，返回含已删除（deleted_at 非空）的用户。
	// 仅供 /admin/usage 的 SearchUsers 端点使用，其他列表调用方不要设置。
	IncludeDeleted bool
}

type UserAuthIdentityRecord struct {
	ProviderType    string
	ProviderKey     string
	ProviderSubject string
	VerifiedAt      *time.Time
	Issuer          *string
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type UserAvatar struct {
	StorageProvider string
	StorageKey      string
	URL             string
	ContentType     string
	ByteSize        int
	SHA256          string
}

type UpsertUserAvatarInput struct {
	StorageProvider string
	StorageKey      string
	URL             string
	ContentType     string
	ByteSize        int
	SHA256          string
}
