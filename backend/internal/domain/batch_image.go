// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	BatchImageProviderVertex = "vertex"
)

const (
	BatchImageItemStatusPending   = "pending"
	BatchImageItemStatusSuccess   = "success"
	BatchImageItemStatusFailed    = "failed"
	BatchImageItemStatusCancelled = "cancelled"
)

const (
	BatchImageJobStatusCreated       = "created"
	BatchImageJobStatusUploading     = "uploading"
	BatchImageJobStatusSubmitted     = "submitted"
	BatchImageJobStatusRunning       = "running"
	BatchImageJobStatusIndexing      = "indexing"
	BatchImageJobStatusSettling      = "settling"
	BatchImageJobStatusCompleted     = "completed"
	BatchImageJobStatusFailed        = "failed"
	BatchImageJobStatusCancelled     = "cancelled"
	BatchImageJobStatusOutputDeleted = "output_deleted"
)

var ErrBatchImageJobNotFound = infraerrors.New(http.StatusNotFound, "BATCH_IMAGE_JOB_NOT_FOUND", "batch image job not found")

var ErrBatchImageJobExists = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_JOB_EXISTS", "batch image job already exists")

var ErrBatchImageItemExists = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_ITEM_EXISTS", "batch image item already exists")

var ErrBatchImageInvalidTransition = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_TRANSITION", "invalid batch image job status transition")

var ErrBatchImageInvalidProvider = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_PROVIDER", "invalid batch image provider")

var ErrBatchImageIndexStateConflict = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_INDEX_STATE_CONFLICT", "batch image job is no longer in indexing state")

var ErrBatchImageSettlementInvalidStatus = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_INVALID_STATUS", "batch image job is not ready for settlement")

var ErrBatchImageSettlementManifestConflict = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_SETTLEMENT_MANIFEST_CONFLICT", "batch image settlement manifest hash conflict")

var ErrBatchImageAlreadySettled = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_ALREADY_SETTLED", "batch image job is already settled")

var ErrBatchImageSettlementCostExceedsHold = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_SETTLEMENT_COST_EXCEEDS_HOLD", "batch image settlement cost exceeds held balance")

var ErrBatchImageInsufficientBalance = infraerrors.New(http.StatusPaymentRequired, "BATCH_IMAGE_INSUFFICIENT_BALANCE", "insufficient balance for batch image hold")

var ErrBatchImageItemNotFound = infraerrors.New(http.StatusNotFound, "BATCH_IMAGE_ITEM_NOT_FOUND", "batch image item not found")

var ErrBatchImageDownloadLimited = infraerrors.New(http.StatusTooManyRequests, "BATCH_IMAGE_DOWNLOAD_LIMITED", "too many batch image downloads")

var ErrBatchImageRecordDeleteNotReady = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_RECORD_DELETE_NOT_READY", "batch image record can only be deleted after the job finishes")

type BatchImageJob struct {
	ID                int64
	BatchID           string
	UserID            int64
	APIKeyID          *int64
	AccountID         *int64
	Provider          string
	Model             string
	TaskName          string
	ParentBatchID     *string
	Status            string
	ProviderJobName   *string
	ProviderInputRef  *string
	ProviderOutputRef *string
	GCSInputURI       *string
	GCSOutputURI      *string

	ItemCount      int
	SuccessCount   int
	FailCount      int
	CancelledCount int

	EstimatedCost           float64
	HoldAmount              *float64
	ActualCost              *float64
	BaseUnitPrice           float64
	GroupRateMultiplier     float64
	AccountRateMultiplier   float64
	BatchDiscountMultiplier float64
	HoldMultiplier          float64
	BillableUnitPrice       float64
	HoldUnitPrice           float64
	PricingSnapshotVersion  int
	Currency                string
	HoldID                  *string

	IdempotencyKey *string
	RequestHash    *string
	ManifestHash   *string

	RetryCount int
	Version    int

	OutputExpiresAt *time.Time
	InputDeletedAt  *time.Time
	OutputDeletedAt *time.Time
	DownloadedAt    *time.Time
	UserDeletedAt   *time.Time

	LastErrorCode    *string
	LastErrorMessage *string

	CreatedAt   time.Time
	UpdatedAt   time.Time
	SubmittedAt *time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
	SettledAt   *time.Time
}

type CreateBatchImageJobParams struct {
	BatchID           string
	UserID            int64
	APIKeyID          *int64
	AccountID         *int64
	Provider          string
	Model             string
	TaskName          string
	ParentBatchID     *string
	Status            string
	ProviderJobName   *string
	ProviderInputRef  *string
	ProviderOutputRef *string
	GCSInputURI       *string
	GCSOutputURI      *string

	ItemCount      int
	SuccessCount   int
	FailCount      int
	CancelledCount int

	EstimatedCost           float64
	HoldAmount              *float64
	ActualCost              *float64
	BaseUnitPrice           float64
	GroupRateMultiplier     float64
	AccountRateMultiplier   float64
	BatchDiscountMultiplier float64
	HoldMultiplier          float64
	BillableUnitPrice       float64
	HoldUnitPrice           float64
	PricingSnapshotVersion  int
	Currency                string
	HoldID                  *string

	IdempotencyKey *string
	RequestHash    *string
	ManifestHash   *string

	RetryCount int

	OutputExpiresAt *time.Time
}

type BatchImageItem struct {
	ID                   int64
	JobID                string
	CustomID             string
	Status               string
	RequestHash          *string
	PromptPreview        *string
	ProviderSourceObject *string
	SourceLineNumber     *int
	SourceByteOffset     *int64
	SourceByteLength     *int64
	MimeType             *string
	FileExtension        *string
	ImageCount           int
	ErrorCode            *string
	ErrorMessage         *string
	BilledAmount         *float64
	CreatedAt            time.Time
	IndexedAt            *time.Time
}

type CreateBatchImageItemParams struct {
	JobID                string
	CustomID             string
	Status               string
	RequestHash          *string
	PromptPreview        *string
	ProviderSourceObject *string
	SourceLineNumber     *int
	SourceByteOffset     *int64
	SourceByteLength     *int64
	MimeType             *string
	FileExtension        *string
	ImageCount           int
	ErrorCode            *string
	ErrorMessage         *string
	BilledAmount         *float64
	IndexedAt            *time.Time
}

type BatchImageItemFilter struct {
	Status string
	Limit  int
	Offset int
}

type BatchImageJobFilter struct {
	Status         string
	TaskNameLike   string
	Downloaded     *bool
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	ExcludeDeleted bool
	Limit          int
	Offset         int
}

type BatchImageCounts struct {
	SuccessCount int
	FailCount    int
}

type UpdateBatchImageJobProviderSubmitParams struct {
	BatchID           string
	ProviderJobName   string
	ProviderInputRef  string
	ProviderOutputRef string
	GCSInputURI       string
	GCSOutputURI      string
	EventPayload      any
}

type BatchImageTransitionOptions struct {
	EventType    string
	EventPayload any
	ErrorCode    *string
	ErrorMessage *string
	Now          *time.Time
}

type MarkBatchImageJobSettledParams struct {
	BatchID         string
	ActualCost      float64
	ManifestHash    string
	EventPayload    any
	Now             *time.Time
	OutputExpiresAt *time.Time
}

func NewBatchImageID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "imgbatch_" + hex.EncodeToString(b[:]), nil
}

func IsSupportedBatchImageProvider(provider string) bool {
	return provider == BatchImageProviderVertex
}

func IsTerminalBatchImageJobStatus(status string) bool {
	switch status {
	case BatchImageJobStatusCompleted, BatchImageJobStatusFailed, BatchImageJobStatusCancelled, BatchImageJobStatusOutputDeleted:
		return true
	default:
		return false
	}
}

func CanTransitionBatchImageJob(from, to string) bool {
	if from == "" || to == "" {
		return false
	}
	if IsTerminalBatchImageJobStatus(from) {
		return to == BatchImageJobStatusOutputDeleted &&
			from != BatchImageJobStatusOutputDeleted &&
			(from == BatchImageJobStatusCompleted || from == BatchImageJobStatusFailed || from == BatchImageJobStatusCancelled)
	}
	if to == BatchImageJobStatusFailed {
		return true
	}

	allowed := map[string]map[string]struct{}{
		BatchImageJobStatusCreated: {
			BatchImageJobStatusUploading: {},
			BatchImageJobStatusSubmitted: {},
			BatchImageJobStatusCancelled: {},
		},
		BatchImageJobStatusUploading: {
			BatchImageJobStatusSubmitted: {},
			BatchImageJobStatusCancelled: {},
		},
		BatchImageJobStatusSubmitted: {
			BatchImageJobStatusRunning:   {},
			BatchImageJobStatusIndexing:  {},
			BatchImageJobStatusFailed:    {},
			BatchImageJobStatusCancelled: {},
		},
		BatchImageJobStatusRunning: {
			BatchImageJobStatusRunning:   {},
			BatchImageJobStatusIndexing:  {},
			BatchImageJobStatusFailed:    {},
			BatchImageJobStatusCancelled: {},
		},
		BatchImageJobStatusIndexing: {
			BatchImageJobStatusSettling: {},
			BatchImageJobStatusFailed:   {},
		},
		BatchImageJobStatusSettling: {
			BatchImageJobStatusCompleted: {},
		},
	}
	_, ok := allowed[from][to]
	return ok
}
