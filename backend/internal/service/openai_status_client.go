package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const OpenAIStatusSummaryURL = "https://status.openai.com/api/v2/summary.json"

type OpenAIStatusClient struct {
	httpClient   *http.Client
	maxBodyBytes int64
	sourceURL    string
}

func NewOpenAIStatusClient(timeout time.Duration, maxBodyBytes int64) *OpenAIStatusClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if maxBodyBytes <= 0 {
		maxBodyBytes = 512 * 1024
	}
	client := &http.Client{Timeout: timeout}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		if !strings.EqualFold(req.URL.Scheme, "https") || !strings.EqualFold(req.URL.Hostname(), "status.openai.com") {
			return fmt.Errorf("status redirect host is not allowed")
		}
		return nil
	}
	return &OpenAIStatusClient{httpClient: client, maxBodyBytes: maxBodyBytes, sourceURL: OpenAIStatusSummaryURL}
}

func NewOpenAIStatusClientForTest(client *http.Client, sourceURL string, maxBodyBytes int64) *OpenAIStatusClient {
	return &OpenAIStatusClient{httpClient: client, sourceURL: sourceURL, maxBodyBytes: maxBodyBytes}
}

type openAIStatusSummaryPayload struct {
	Page struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		URL       string    `json:"url"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"page"`
	Status struct {
		Description string `json:"description"`
		Indicator   string `json:"indicator"`
	} `json:"status"`
	Components []struct {
		ID        string     `json:"id"`
		Name      string     `json:"name"`
		Status    string     `json:"status"`
		UpdatedAt *time.Time `json:"updated_at"`
	} `json:"components"`
	Incidents []openAIStatusIncidentPayload `json:"incidents"`
}

type openAIStatusIncidentPayload struct {
	ID              string                         `json:"id"`
	Name            string                         `json:"name"`
	Status          string                         `json:"status"`
	Impact          string                         `json:"impact"`
	CreatedAt       time.Time                      `json:"created_at"`
	UpdatedAt       time.Time                      `json:"updated_at"`
	ResolvedAt      *time.Time                     `json:"resolved_at"`
	MonitoringAt    *time.Time                     `json:"monitoring_at"`
	IncidentUpdates []ProviderStatusIncidentUpdate `json:"incident_updates"`
}

func (c *OpenAIStatusClient) Fetch(ctx context.Context, fetchedAt time.Time) (*ProviderStatusSnapshotRecord, error) {
	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("openai status client is not configured")
	}
	sourceURL := strings.TrimSpace(c.sourceURL)
	parsedURL, err := url.Parse(sourceURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid status source URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sub2api-provider-status/1")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch openai status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openai status returned HTTP %d", resp.StatusCode)
	}
	limit := c.maxBodyBytes
	if limit <= 0 {
		limit = 512 * 1024
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read openai status: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("openai status response exceeds %d bytes", limit)
	}
	var payload openAIStatusSummaryPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode openai status: %w", err)
	}
	indicator := strings.ToLower(strings.TrimSpace(payload.Status.Indicator))
	if strings.TrimSpace(payload.Page.ID) == "" || indicator == "" {
		return nil, fmt.Errorf("openai status payload missing required fields")
	}
	components := make([]ProviderStatusComponent, 0, len(payload.Components))
	for _, component := range payload.Components {
		if strings.TrimSpace(component.ID) == "" || strings.TrimSpace(component.Name) == "" {
			continue
		}
		components = append(components, ProviderStatusComponent{
			ID:        strings.TrimSpace(component.ID),
			Name:      strings.TrimSpace(component.Name),
			Status:    strings.ToLower(strings.TrimSpace(component.Status)),
			UpdatedAt: component.UpdatedAt,
		})
	}
	incidents := make([]ProviderStatusIncident, 0, len(payload.Incidents))
	for _, incident := range payload.Incidents {
		if strings.TrimSpace(incident.ID) == "" {
			continue
		}
		updates := incident.IncidentUpdates
		if updates == nil {
			updates = []ProviderStatusIncidentUpdate{}
		}
		incidents = append(incidents, ProviderStatusIncident{
			ID: strings.TrimSpace(incident.ID), Name: strings.TrimSpace(incident.Name),
			Status: strings.ToLower(strings.TrimSpace(incident.Status)), Impact: strings.ToLower(strings.TrimSpace(incident.Impact)),
			CreatedAt: incident.CreatedAt, UpdatedAt: incident.UpdatedAt, ResolvedAt: incident.ResolvedAt,
			MonitoringAt: incident.MonitoringAt, Updates: updates,
		})
	}
	componentsJSON, err := json.Marshal(components)
	if err != nil {
		return nil, err
	}
	incidentsJSON, err := json.Marshal(incidents)
	if err != nil {
		return nil, err
	}
	normalized := struct {
		Indicator   string                    `json:"indicator"`
		Description string                    `json:"description"`
		Components  []ProviderStatusComponent `json:"components"`
		Incidents   []ProviderStatusIncident  `json:"incidents"`
	}{indicator, strings.TrimSpace(payload.Status.Description), components, incidents}
	normalizedJSON, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(normalizedJSON)
	var sourceUpdatedAt *time.Time
	if !payload.Page.UpdatedAt.IsZero() {
		t := payload.Page.UpdatedAt.UTC()
		sourceUpdatedAt = &t
	}
	if fetchedAt.IsZero() {
		fetchedAt = time.Now().UTC()
	}
	return &ProviderStatusSnapshotRecord{
		Provider:           ProviderStatusOpenAI,
		SourceURL:          sourceURL,
		OverallIndicator:   indicator,
		OverallDescription: strings.TrimSpace(payload.Status.Description),
		ComponentsJSON:     componentsJSON,
		IncidentsJSON:      incidentsJSON,
		SourceUpdatedAt:    sourceUpdatedAt,
		FetchedAt:          fetchedAt.UTC(),
		ContentHash:        hex.EncodeToString(digest[:]),
	}, nil
}
