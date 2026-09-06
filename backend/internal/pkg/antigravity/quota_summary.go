package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	antigravityDailyPublicBaseURL = "https://daily-cloudcode-pa.googleapis.com"
	quotaSummaryBodyLimit         = 8 << 20
)

var antigravityQuotaSummaryBaseURLs = []string{
	antigravityDailyPublicBaseURL,
	antigravityDailyBaseURL,
	antigravityProdBaseURL,
}

const antigravityQuotaUserAgent = "antigravity/cli/1.0.13 (aidev_client; os_type=darwin; arch=arm64)"

type QuotaSummaryBucket struct {
	BucketID          string   `json:"bucketId,omitempty"`
	DisplayName       string   `json:"displayName,omitempty"`
	Window            string   `json:"window,omitempty"`
	ResetTime         string   `json:"resetTime,omitempty"`
	RemainingFraction *float64 `json:"remainingFraction,omitempty"`
	Description       string   `json:"description,omitempty"`
}

type QuotaSummaryGroup struct {
	DisplayName string               `json:"displayName,omitempty"`
	Description string               `json:"description,omitempty"`
	Buckets     []QuotaSummaryBucket `json:"buckets,omitempty"`
}

type RetrieveUserQuotaSummaryResponse struct {
	Groups     []QuotaSummaryGroup `json:"groups,omitempty"`
	ServerTime *time.Time          `json:"-"`
}

// RetrieveUserQuotaSummary reads Antigravity's quota-window endpoint. This is
// separate from FetchAvailableModels: the latter remains the model-discovery
// source, while this endpoint is authoritative for quota presentation.
func (c *Client) RetrieveUserQuotaSummary(ctx context.Context, accessToken, projectID string) (*RetrieveUserQuotaSummaryResponse, map[string]any, error) {
	if c == nil || c.httpClient == nil {
		return nil, nil, errors.New("antigravity client is not configured")
	}
	body, err := json.Marshal(map[string]string{"project": strings.TrimSpace(projectID)})
	if err != nil {
		return nil, nil, fmt.Errorf("serialize quota summary request: %w", err)
	}

	var lastErr error
	var priorityErr error
	var emptyResponse *RetrieveUserQuotaSummaryResponse
	var emptyRaw map[string]any
	for _, baseURL := range antigravityQuotaSummaryBaseURLs {
		req, errReq := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1internal:retrieveUserQuotaSummary", strings.NewReader(string(body)))
		if errReq != nil {
			lastErr = errReq
			continue
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", antigravityQuotaUserAgent)

		resp, errDo := c.httpClient.Do(req)
		if errDo != nil {
			lastErr = fmt.Errorf("retrieveUserQuotaSummary request failed: %w", errDo)
			continue
		}
		responseBody, errRead := io.ReadAll(io.LimitReader(resp.Body, quotaSummaryBodyLimit+1))
		_ = resp.Body.Close()
		if errRead != nil {
			lastErr = fmt.Errorf("read quota summary response: %w", errRead)
			continue
		}
		if len(responseBody) > quotaSummaryBodyLimit {
			lastErr = fmt.Errorf("quota summary response exceeds %d bytes", quotaSummaryBodyLimit)
			continue
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			if resp.StatusCode == http.StatusForbidden {
				priorityErr = &ForbiddenError{StatusCode: resp.StatusCode, Body: string(responseBody)}
			} else {
				errStatus := fmt.Errorf("retrieveUserQuotaSummary failed (HTTP %d): %s", resp.StatusCode, string(responseBody))
				if resp.StatusCode == http.StatusNotFound && priorityErr == nil {
					priorityErr = errStatus
				}
				lastErr = errStatus
			}
			continue
		}

		var parsed RetrieveUserQuotaSummaryResponse
		if errJSON := json.Unmarshal(responseBody, &parsed); errJSON != nil {
			lastErr = fmt.Errorf("parse quota summary response: %w", errJSON)
			continue
		}
		if serverTime, errDate := http.ParseTime(resp.Header.Get("Date")); errDate == nil {
			parsed.ServerTime = &serverTime
		}
		var raw map[string]any
		_ = json.Unmarshal(responseBody, &raw)
		if len(parsed.Groups) > 0 {
			return &parsed, raw, nil
		}
		emptyResponse, emptyRaw = &parsed, raw
	}

	if emptyResponse != nil {
		return emptyResponse, emptyRaw, nil
	}
	if priorityErr != nil {
		return nil, nil, priorityErr
	}
	if lastErr != nil {
		return nil, nil, lastErr
	}
	return nil, nil, errors.New("retrieveUserQuotaSummary failed: no endpoint available")
}
