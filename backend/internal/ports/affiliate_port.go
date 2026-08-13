// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type AffiliateRepository interface {
	EnsureUserAffiliate(ctx context.Context, userID int64) (*domain.AffiliateSummary, error)
	GetAffiliateByCode(ctx context.Context, code string) (*domain.AffiliateSummary, error)
	BindInviter(ctx context.Context, userID, inviterID int64) (bool, error)
	IsAffiliateDescendant(ctx context.Context, userID, candidateDescendantID int64) (bool, error)
	AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64, freezeHours int, sourceOrderID *int64) (bool, error)
	GetAccruedRebateFromInvitee(ctx context.Context, inviterID, inviteeUserID int64) (float64, error)
	ThawFrozenQuota(ctx context.Context, userID int64) (float64, error)
	TransferQuotaToBalance(ctx context.Context, userID int64) (float64, float64, error)
	ListInvitees(ctx context.Context, inviterID int64, limit int) ([]domain.AffiliateInvitee, error)

	// 管理端：用户级专属配置
	UpdateUserAffCode(ctx context.Context, userID int64, newCode string) error
	ResetUserAffCode(ctx context.Context, userID int64) (string, error)
	SetUserRebateRate(ctx context.Context, userID int64, ratePercent *float64) error
	BatchSetUserRebateRate(ctx context.Context, userIDs []int64, ratePercent *float64) error
	ListUsersWithCustomSettings(ctx context.Context, filter domain.AffiliateAdminFilter) ([]domain.AffiliateAdminEntry, int64, error)
	ListAffiliateInviteRecords(ctx context.Context, filter domain.AffiliateRecordFilter) ([]domain.AffiliateInviteRecord, int64, error)
	ListAffiliateRebateRecords(ctx context.Context, filter domain.AffiliateRecordFilter) ([]domain.AffiliateRebateRecord, int64, error)
	ListAffiliateTransferRecords(ctx context.Context, filter domain.AffiliateRecordFilter) ([]domain.AffiliateTransferRecord, int64, error)
	GetAffiliateUserOverview(ctx context.Context, userID int64) (*domain.AffiliateUserOverview, error)
}
