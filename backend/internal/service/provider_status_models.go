package service

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

type ProviderStatusComponent struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type ProviderStatusIncidentUpdate struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProviderStatusIncident struct {
	ID           string                         `json:"id"`
	Name         string                         `json:"name"`
	Status       string                         `json:"status"`
	Impact       string                         `json:"impact"`
	CreatedAt    time.Time                      `json:"created_at"`
	UpdatedAt    time.Time                      `json:"updated_at"`
	ResolvedAt   *time.Time                     `json:"resolved_at,omitempty"`
	MonitoringAt *time.Time                     `json:"monitoring_at,omitempty"`
	Updates      []ProviderStatusIncidentUpdate `json:"updates"`
}

type ProviderStatusSnapshot struct {
	ID                 int64                     `json:"id"`
	Provider           string                    `json:"provider"`
	SourceURL          string                    `json:"source_url"`
	OverallIndicator   string                    `json:"overall_indicator"`
	OverallDescription string                    `json:"overall_description"`
	Components         []ProviderStatusComponent `json:"components"`
	Incidents          []ProviderStatusIncident  `json:"incidents"`
	SourceUpdatedAt    *time.Time                `json:"source_updated_at,omitempty"`
	FetchedAt          time.Time                 `json:"fetched_at"`
	ContentHash        string                    `json:"-"`
}

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

type ProviderStatusCurrent struct {
	Provider      string                  `json:"provider"`
	Freshness     string                  `json:"freshness"`
	Snapshot      *ProviderStatusSnapshot `json:"snapshot,omitempty"`
	LastAttemptAt *time.Time              `json:"last_attempt_at,omitempty"`
	LastSuccessAt *time.Time              `json:"last_success_at,omitempty"`
	LastErrorAt   *time.Time              `json:"last_error_at,omitempty"`
	LastError     string                  `json:"last_error,omitempty"`
}

type ProviderStatusHistoryFilter struct {
	Provider  string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
}
