// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrBatchImageQueueEmpty = infraerrors.New(http.StatusNotFound, "BATCH_IMAGE_QUEUE_EMPTY", "batch image queue is empty")

var ErrBatchImageAlreadyQueued = infraerrors.New(http.StatusConflict, "BATCH_IMAGE_ALREADY_QUEUED", "batch image job is already queued")

var ErrInvalidBatchImageQueuePayload = infraerrors.New(http.StatusBadRequest, "BATCH_IMAGE_QUEUE_INVALID_PAYLOAD", "invalid batch image queue payload")

type ReservedBatchImageJob struct {
	BatchID string
}

func IsValidBatchImageID(batchID string) bool {
	return strings.HasPrefix(batchID, "imgbatch_") && len(batchID) > len("imgbatch_")
}
