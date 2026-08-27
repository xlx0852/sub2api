package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type ContentModerationEvidenceStore interface {
	Configured() bool
	CurrentKeyVersion() string
	Put(ctx context.Context, objectKey, keyVersion string, plaintext []byte) (storedBytes int64, err error)
	Get(ctx context.Context, objectKey, keyVersion string) ([]byte, error)
	Delete(ctx context.Context, objectKey string) error
	List(ctx context.Context, cursor string, limit int) (domain.EvidenceObjectPage, error)
	Probe(ctx context.Context) error
}

type RetrospectiveRiskRepository interface {
	ReserveEvidence(ctx context.Context, item *domain.RetrospectiveEvidence, maxItems int, maxBytes int64) (bool, error)
	MarkEvidenceStored(ctx context.Context, id string, storedBytes int64) error
	MarkEvidenceFailed(ctx context.Context, id string, reason string) error
	MoveEvidenceObject(ctx context.Context, id, objectKey string, storedBytes int64, expiresAt time.Time) error
	ListExpiredEvidence(ctx context.Context, now time.Time, limit int) ([]domain.RetrospectiveEvidence, error)
	MarkEvidenceDeleted(ctx context.Context, id string) error
	EvidenceObjectExists(ctx context.Context, objectKey string) (bool, error)
	CreateReviewRun(ctx context.Context, runKey string, scheduledDay time.Time, manual bool, policyVersion string, configSnapshot []byte) (*domain.RetrospectiveReviewRun, bool, error)
	TryReviewLock(ctx context.Context) (release func(), acquired bool, err error)
	ClaimEvidence(ctx context.Context, runID int64, policyVersion string, limit int) ([]domain.RetrospectiveEvidence, error)
	CompleteReviewItem(ctx context.Context, runID int64, policyVersion string, evidence domain.RetrospectiveEvidence, result domain.RetrospectiveReviewResult) (int64, error)
	FailReviewItem(ctx context.Context, runID int64, evidenceID, policyVersion, reason string) error
	UpdateReviewRun(ctx context.Context, run *domain.RetrospectiveReviewRun) error
	LatestReviewRun(ctx context.Context) (*domain.RetrospectiveReviewRun, error)
	ListReviewRuns(ctx context.Context, limit, offset int) ([]domain.RetrospectiveReviewRun, int64, error)
	EvidenceStats(ctx context.Context) (items int64, storedBytes int64, err error)
	UpsertRiskProfileFromFinding(ctx context.Context, evidence domain.RetrospectiveEvidence, reviewItemID int64, result domain.RetrospectiveReviewResult, ttl time.Duration, activate bool) (*domain.UserRiskProfile, error)
	ListRiskProfiles(ctx context.Context, level string, limit, offset int) ([]domain.UserRiskProfile, int64, error)
	ListRiskEvents(ctx context.Context, userID int64, limit, offset int) ([]domain.RiskEvent, int64, error)
	ListEvidenceCases(ctx context.Context, userID int64, status string, limit, offset int) ([]domain.RetrospectiveEvidenceCase, int64, error)
	GetEvidence(ctx context.Context, evidenceID string) (*domain.RetrospectiveEvidence, error)
	CreateEvidenceAccessGrant(ctx context.Context, evidenceID string, adminID int64, tokenHash, reason string, expiresAt time.Time) error
	ConsumeEvidenceAccessGrant(ctx context.Context, evidenceID string, adminID int64, tokenHash string, now time.Time) (*domain.EvidenceAccessGrant, error)
	AuditOperatorAction(ctx context.Context, adminID int64, action, evidenceID string, userID int64, reason, outcome string) error
	SetRiskProfile(ctx context.Context, userID int64, level, reviewStatus, note string, adminID *int64, ttl time.Duration) (*domain.UserRiskProfile, error)
	IsRiskElevated(ctx context.Context, userID int64, now time.Time) (bool, error)
	CountActiveRiskProfiles(ctx context.Context) (int64, error)
	ExpireRiskProfiles(ctx context.Context, now time.Time, limit int) ([]int64, error)
}
