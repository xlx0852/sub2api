// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"encoding/json"
	"time"
)

const (
	ProviderStatusOpenAI = "openai"

	ProviderStatusFresh       = "fresh"
	ProviderStatusStale       = "stale"
	ProviderStatusUnavailable = "unavailable"
)

type ProviderStatusSnapshotRecord struct {
	ID                 int64
	Provider           string
	SourceURL          string
	OverallIndicator   string
	OverallDescription string
	ComponentsJSON     json.RawMessage
	IncidentsJSON      json.RawMessage
	SourceUpdatedAt    *time.Time
	FetchedAt          time.Time
	ContentHash        string
}

type ProviderStatusHistoryFilter struct {
	Provider  string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
}
