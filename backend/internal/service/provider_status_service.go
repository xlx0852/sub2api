package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func providerStatusSnapshotFromRecord(record *ProviderStatusSnapshotRecord) (*ProviderStatusSnapshot, error) {
	if record == nil {
		return nil, nil
	}
	item := &ProviderStatusSnapshot{
		ID:                 record.ID,
		Provider:           record.Provider,
		SourceURL:          record.SourceURL,
		OverallIndicator:   record.OverallIndicator,
		OverallDescription: record.OverallDescription,
		SourceUpdatedAt:    record.SourceUpdatedAt,
		FetchedAt:          record.FetchedAt,
		ContentHash:        record.ContentHash,
		Components:         []ProviderStatusComponent{},
		Incidents:          []ProviderStatusIncident{},
	}
	if len(record.ComponentsJSON) > 0 {
		if err := json.Unmarshal(record.ComponentsJSON, &item.Components); err != nil {
			return nil, fmt.Errorf("decode provider status components: %w", err)
		}
	}
	if len(record.IncidentsJSON) > 0 {
		if err := json.Unmarshal(record.IncidentsJSON, &item.Incidents); err != nil {
			return nil, fmt.Errorf("decode provider status incidents: %w", err)
		}
	}
	return item, nil
}

func (s *OpsService) GetProviderStatusCurrent(ctx context.Context, provider string) (*ProviderStatusCurrent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = ProviderStatusOpenAI
	}
	if provider != ProviderStatusOpenAI {
		return nil, fmt.Errorf("unsupported provider status source")
	}
	result := &ProviderStatusCurrent{Provider: provider, Freshness: ProviderStatusUnavailable}
	if s.opsRepo == nil {
		return result, nil
	}
	record, err := s.opsRepo.GetLatestProviderStatusSnapshot(ctx, provider)
	if err != nil {
		return nil, err
	}
	if record != nil {
		result.Snapshot, err = providerStatusSnapshotFromRecord(record)
		if err != nil {
			return nil, err
		}
		staleAfter := time.Duration(openAIStatusConfig(s.cfg).StaleAfterSeconds) * time.Second
		result.Freshness = ProviderStatusFresh
		if time.Since(record.FetchedAt) > staleAfter {
			result.Freshness = ProviderStatusStale
		}
	}
	heartbeats, err := s.opsRepo.ListJobHeartbeats(ctx)
	if err == nil {
		for _, heartbeat := range heartbeats {
			if heartbeat == nil || heartbeat.JobName != openAIStatusCollectorJobName {
				continue
			}
			result.LastAttemptAt = heartbeat.LastRunAt
			result.LastSuccessAt = heartbeat.LastSuccessAt
			result.LastErrorAt = heartbeat.LastErrorAt
			if heartbeat.LastError != nil {
				result.LastError = *heartbeat.LastError
			}
			break
		}
	}
	return result, nil
}

func (s *OpsService) ListProviderStatusHistory(ctx context.Context, filter *ProviderStatusHistoryFilter) ([]*ProviderStatusSnapshot, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if filter == nil {
		filter = &ProviderStatusHistoryFilter{}
	}
	filter.Provider = strings.ToLower(strings.TrimSpace(filter.Provider))
	if filter.Provider == "" {
		filter.Provider = ProviderStatusOpenAI
	}
	if filter.Provider != ProviderStatusOpenAI {
		return nil, fmt.Errorf("unsupported provider status source")
	}
	if s.opsRepo == nil {
		return []*ProviderStatusSnapshot{}, nil
	}
	records, err := s.opsRepo.ListProviderStatusSnapshots(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]*ProviderStatusSnapshot, 0, len(records))
	for _, record := range records {
		item, err := providerStatusSnapshotFromRecord(record)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}
