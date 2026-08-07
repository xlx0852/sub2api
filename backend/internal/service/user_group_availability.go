package service

import (
	"context"
	"fmt"
)

// UserGroupAvailabilityItem is per-group real-traffic availability for the
// authenticated user, limited to groups the user actually uses (via their
// API keys' group_id or active subscriptions).
type UserGroupAvailabilityItem struct {
	GroupID   int64                     `json:"group_id"`
	GroupName string                    `json:"group_name"`
	Platform  string                    `json:"platform"`
	Day       *GroupTrafficAvailability `json:"day"`
	Week      *GroupTrafficAvailability `json:"week"`
}

type UserGroupAvailabilityResult struct {
	Items []UserGroupAvailabilityItem `json:"items"`
}

// GetUserGroupAvailability returns real-traffic availability for the groups
// the user belongs to (API keys' group + active subscriptions).
func (s *UsageService) GetUserGroupAvailability(ctx context.Context, userID int64) (*UserGroupAvailabilityResult, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	if s.apiKeyService == nil {
		return nil, fmt.Errorf("api key service unavailable")
	}

	groupIDs, err := s.apiKeyService.GetUserUsedGroupIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &UserGroupAvailabilityResult{Items: make([]UserGroupAvailabilityItem, 0, len(groupIDs))}
	for _, gid := range groupIDs {
		summary, err := s.GetGroupTrafficAvailability(ctx, gid)
		if err != nil {
			// Skip a failing group rather than fail the whole dashboard card.
			continue
		}
		item := UserGroupAvailabilityItem{GroupID: gid, Day: summary.Day, Week: summary.Week}
		if g, err := s.apiKeyService.GetGroupBasic(ctx, gid); err == nil && g != nil {
			item.GroupName = g.Name
			item.Platform = g.Platform
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
