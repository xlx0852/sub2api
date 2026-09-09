package service

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
)

type CursorModel struct {
	ID          string   `json:"id"`
	Object      string   `json:"object"`
	OwnedBy     string   `json:"owned_by"`
	DisplayName string   `json:"display_name,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

func (s *OpenAIGatewayService) CursorModels(ctx context.Context, groupID int64) ([]CursorModel, error) {
	accounts, err := s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, PlatformCursor)
	if err != nil {
		return nil, err
	}
	out := []CursorModel{}
	seen := map[string]bool{}
	var lastErr error
	succeeded := false
	for i := range accounts {
		a := &accounts[i]
		token, _, err := s.GetAccessToken(ctx, a)
		if err != nil {
			lastErr = err
			continue
		}
		proxy := ""
		if a.Proxy != nil {
			proxy = a.Proxy.URL()
		}
		client := cursor.NewClient(token, a.GetCredential("machine_id"), createCursorHTTPClient(proxy), a.GetCredential("base_url"))
		result, err := client.AvailableModels(ctx)
		if err != nil {
			lastErr = err
			continue
		}
		succeeded = true
		for _, m := range result.Models {
			if m.Name == "" || seen[m.Name] {
				continue
			}
			seen[m.Name] = true
			vendor := m.VendorName
			if vendor == "" {
				vendor = "cursor"
			}
			out = append(out, CursorModel{m.Name, "model", vendor, m.ClientDisplayName, m.IdAliases})
		}
	}
	if !succeeded {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("no available Cursor accounts")
	}
	return out, nil
}
