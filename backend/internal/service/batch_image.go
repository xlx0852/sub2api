package service

import (
	"net/http"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	BatchImageItemStatusPending   = "pending"
	BatchImageItemStatusSuccess   = "success"
	BatchImageItemStatusFailed    = "failed"
	BatchImageItemStatusCancelled = "cancelled"
)

var (
	ErrBatchImageMissingProviderJobName     = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_MISSING_PROVIDER_JOB_NAME", "batch image provider job name is missing")
	ErrBatchImageMissingAccountID           = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_MISSING_ACCOUNT_ID", "batch image account id is missing")
	ErrBatchImageUnsupportedProvider        = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_UNSUPPORTED_PROVIDER", "unsupported batch image provider")
	ErrBatchImageIndexOutputMissing         = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_INDEX_OUTPUT_MISSING", "batch image provider output is missing")
	ErrBatchImageIndexParseFailed           = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_INDEX_PARSE_FAILED", "batch image provider output parse failed")
	ErrBatchImageIndexNoResultLines         = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_INDEX_NO_RESULT_LINES", "batch image provider output has no result lines")
	ErrBatchImageDuplicateCustomID          = infraerrors.New(http.StatusBadGateway, "DUPLICATE_CUSTOM_ID_IN_OUTPUT", "batch image provider output contains duplicate custom id")
	ErrBatchImageSettlementPricingMissing   = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_PRICING_MISSING", "batch image settlement pricing is missing")
	ErrBatchImageSettlementBillingFailed    = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_SETTLEMENT_BILLING_FAILED", "batch image settlement billing failed")
	ErrBatchImageSettlementMissingAPIKeyID  = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_MISSING_API_KEY_ID", "batch image settlement api key id is missing")
	ErrBatchImageSettlementMissingAccountID = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_MISSING_ACCOUNT_ID", "batch image settlement account id is missing")
	ErrBatchImageSettlementInvalidCounts    = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_SETTLEMENT_INVALID_COUNTS", "batch image settlement counts are invalid")
	ErrBatchImageBillingHoldFailed          = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_BILLING_HOLD_FAILED", "batch image balance hold failed")
	ErrBatchImageDisabled                   = infraerrors.New(http.StatusNotFound, "BATCH_IMAGE_DISABLED", "batch image API is disabled")
	ErrBatchImageGroupDisabled              = infraerrors.New(http.StatusForbidden, "BATCH_IMAGE_GROUP_DISABLED", "batch image API is disabled for this group")
	ErrBatchImageInvalidModel               = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_MODEL", "batch image model is required")
	ErrBatchImageNoAccountAvailable         = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_NO_ACCOUNT_AVAILABLE", "no compatible batch image account is available")
	ErrBatchImageInvalidItems               = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_ITEMS", "batch image items are invalid")
	ErrBatchImageDuplicateCustomIDInRequest = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_DUPLICATE_CUSTOM_ID", "batch image custom ids must be unique")
	ErrBatchImagePromptTooLong              = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_PROMPT_TOO_LONG", "batch image prompt is too long")
	ErrBatchImageInvalidReferenceImage      = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_INVALID_REFERENCE_IMAGE", "batch image reference image is invalid")
	ErrBatchImageTooManyReferenceImages     = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_TOO_MANY_REFERENCE_IMAGES", "too many batch image reference images for this model")
	ErrBatchImageReferenceImagesTooLarge    = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_REFERENCE_IMAGES_TOO_LARGE", "batch image reference images are too large")
	ErrBatchImageTooManyOutputImages        = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_TOO_MANY_OUTPUT_IMAGES", "too many batch image output images")
	ErrBatchImageProviderSubmitFailed       = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_PROVIDER_SUBMIT_FAILED", "batch image provider submit failed")
	ErrBatchImageQueueFailed                = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_QUEUE_FAILED", "batch image queue failed")
	ErrBatchImageIdempotencyConflict        = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_IDEMPOTENCY_CONFLICT", "idempotency key reused with different batch image request")
	ErrBatchImageCancelFailed               = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_CANCEL_FAILED", "batch image cancel failed")
	ErrBatchImageVertexGCSBucketMissing     = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_VERTEX_GCS_BUCKET_MISSING", "Vertex managed GCS bucket is not configured")

	ErrBatchImageNotReady                 = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_NOT_READY", "batch image job is not completed")
	ErrBatchImageOutputDeleted            = infraerrors.New(http.StatusGone, "BATCH_IMAGE_OUTPUT_DELETED", "batch image output has been deleted")
	ErrBatchImageItemFailed               = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_ITEM_FAILED", "batch image item did not succeed")
	ErrBatchImageResultMissing            = infraerrors.New(http.StatusInternalServerError, "BATCH_IMAGE_RESULT_MISSING", "batch image result is missing")
	ErrBatchImageDownloadFailed           = infraerrors.New(http.StatusInternalServerError, "BATCH_IMAGE_DOWNLOAD_FAILED", "batch image download failed")
	ErrBatchImageDownloadTooLarge         = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_DOWNLOAD_TOO_LARGE", "batch image download is too large")
	ErrBatchImageItemImageIndexOutOfRange = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_ITEM_IMAGE_INDEX_OUT_OF_RANGE", "batch image item image index is out of range")
	ErrBatchImageZipTooManyItems          = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_ZIP_TOO_MANY_ITEMS", "batch image ZIP contains too many items; use single item downloads")
	ErrBatchImageOutputDeleteNotReady     = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_OUTPUT_DELETE_NOT_READY", "batch image output can only be deleted after completion")
	ErrBatchImageCleanupFailed            = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_CLEANUP_FAILED", "batch image cleanup failed")
	ErrBatchImageCleanupUnsafePath        = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_CLEANUP_UNSAFE_PATH", "batch image cleanup path is unsafe")
	ErrBatchImageProviderCleanupFailed    = infraerrors.New(http.StatusBadGateway, "BATCH_IMAGE_PROVIDER_CLEANUP_FAILED", "batch image provider cleanup failed")
)

type BatchImageEvent struct {
	ID        int64
	JobID     string
	EventType string
	Payload   []byte
	EventHash *string
	CreatedAt time.Time
}
