package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Sell price source constants for admin pricing summary.
const (
	SellPriceSourceOfficial = "official"
	SellPriceSourcePolicy   = "policy"
)

// GroupSellPriceSource is a compact sell-source snapshot for list views.
type GroupSellPriceSource struct {
	Source       string `json:"source"` // official | policy
	PolicyID     *int64 `json:"policy_id,omitempty"`
	PolicyName   string `json:"policy_name,omitempty"`
	PolicyStatus string `json:"policy_status,omitempty"`
	// Effective is true when a bound policy is active and will override official prices.
	Effective bool `json:"effective"`
	// UnboundPolicy means a policy exists in DB but is inactive or otherwise not applied.
	InactivePolicy bool `json:"inactive_policy"`
	// ModelCount is the number of sell pricing entries on the bound policy.
	ModelCount int `json:"model_count"`
	// SampleModels is a short list of models covered by the policy (for UI chips).
	SampleModels []string `json:"sample_models,omitempty"`
}

// GroupPricingModelPreview is one model row in the group pricing summary.
type GroupPricingModelPreview struct {
	Model          string   `json:"model"`
	BillingMode    string   `json:"billing_mode"`
	OfficialInput  *float64 `json:"official_input"`  // USD / 1M tokens (token mode)
	OfficialOutput *float64 `json:"official_output"` // USD / 1M tokens
	SellInput      *float64 `json:"sell_input"`
	SellOutput     *float64 `json:"sell_output"`
	// Source for this model: official | policy
	Source string `json:"source"`
	// Effective = sell unit × group rate (what user is charged per 1M before cache etc.)
	EffectiveInput  *float64 `json:"effective_input"`
	EffectiveOutput *float64 `json:"effective_output"`
	// MarkupN is sell/official when both sides have positive input price; nil if unknown.
	MarkupN *float64 `json:"markup_n,omitempty"`
}

// GroupPricingSummary is the admin-facing pricing view for one group.
type GroupPricingSummary struct {
	GroupID        int64   `json:"group_id"`
	GroupName      string  `json:"group_name"`
	Platform       string  `json:"platform"`
	RateMultiplier float64 `json:"rate_multiplier"`

	Source         string `json:"source"` // official | policy
	PolicyID       *int64 `json:"policy_id,omitempty"`
	PolicyName     string `json:"policy_name,omitempty"`
	PolicyStatus   string `json:"policy_status,omitempty"`
	Effective      bool   `json:"effective"`
	InactivePolicy bool   `json:"inactive_policy"`

	// Hint is a short human-readable source line for UI badges.
	Hint string `json:"hint"`

	BillingModelSource         string `json:"billing_model_source,omitempty"`
	RestrictModels             bool   `json:"restrict_models"`
	ApplyPricingToAccountStats bool   `json:"apply_pricing_to_account_stats"`
	AccountStatsRuleCount      int    `json:"account_stats_rule_count"`

	Models []GroupPricingModelPreview `json:"models"`

	// AvailablePolicies lists active sell-price policies for the bind selector.
	AvailablePolicies []SellPricePolicyOption `json:"available_policies,omitempty"`
}

// SellPricePolicyOption is a selectable policy in the group pricing UI.
type SellPricePolicyOption struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	GroupCount   int      `json:"group_count"`
	ModelCount   int      `json:"model_count"`
	SampleModels []string `json:"sample_models,omitempty"`
	// BoundHere is true when this policy is currently bound to the requesting group.
	BoundHere bool `json:"bound_here"`
	// BoundOtherGroups lists other group names already using this policy (share warning).
	BoundOtherGroupNames []string `json:"bound_other_group_names,omitempty"`
}

// BindGroupSellPricePolicy binds or unbinds a group's sell-price policy.
// policyID == nil unbinds (group follows official pricing).
// Underlying storage remains channels + channel_groups.
func (s *ChannelService) BindGroupSellPricePolicy(ctx context.Context, groupID int64, policyID *int64) error {
	if groupID <= 0 {
		return fmt.Errorf("%w: invalid group id", ErrInvalidInput)
	}

	currentID, err := s.repo.GetChannelIDByGroupID(ctx, groupID)
	if err != nil {
		return fmt.Errorf("get current policy for group: %w", err)
	}

	// Unbind
	if policyID == nil || *policyID <= 0 {
		if currentID == 0 {
			return nil
		}
		return s.removeGroupFromPolicy(ctx, currentID, groupID)
	}

	targetID := *policyID
	if currentID == targetID {
		// Ensure target is loadable
		if _, err := s.GetByID(ctx, targetID); err != nil {
			return err
		}
		return nil
	}

	target, err := s.GetByID(ctx, targetID)
	if err != nil {
		return err
	}

	// Remove from old policy first if needed
	if currentID > 0 {
		if err := s.removeGroupFromPolicy(ctx, currentID, groupID); err != nil {
			return err
		}
	}

	// Add to target
	newGroupIDs := append([]int64{}, target.GroupIDs...)
	already := false
	for _, gid := range newGroupIDs {
		if gid == groupID {
			already = true
			break
		}
	}
	if !already {
		newGroupIDs = append(newGroupIDs, groupID)
	}

	_, err = s.Update(ctx, targetID, &UpdateChannelInput{GroupIDs: &newGroupIDs})
	return err
}

func (s *ChannelService) removeGroupFromPolicy(ctx context.Context, policyID, groupID int64) error {
	ch, err := s.GetByID(ctx, policyID)
	if err != nil {
		return err
	}
	next := make([]int64, 0, len(ch.GroupIDs))
	for _, gid := range ch.GroupIDs {
		if gid != groupID {
			next = append(next, gid)
		}
	}
	// Always pass a non-nil slice so Update applies the change (including empty = unbind all).
	_, err = s.Update(ctx, policyID, &UpdateChannelInput{GroupIDs: &next})
	return err
}

// ListGroupSellPriceSources returns sell-source info for many groups (list column).
func (s *ChannelService) ListGroupSellPriceSources(ctx context.Context, groupIDs []int64) (map[int64]GroupSellPriceSource, error) {
	out := make(map[int64]GroupSellPriceSource, len(groupIDs))
	for _, gid := range groupIDs {
		out[gid] = GroupSellPriceSource{Source: SellPriceSourceOfficial}
	}
	if len(groupIDs) == 0 {
		return out, nil
	}

	cache, err := s.loadCache(ctx)
	if err != nil {
		return nil, err
	}

	// Also load inactive policies via full list so inactive bound channels still show up.
	// Cache only keeps active channels in channelByGroupID; inactive need DB lookup.
	needDB := make([]int64, 0)
	for _, gid := range groupIDs {
		if ch, ok := cache.channelByGroupID[gid]; ok && ch != nil {
			// Cache may still hold inactive policies; hot path ignores them, summary must surface them.
			out[gid] = buildSellSourceFromChannel(ch, ch.IsActive())
			continue
		}
		needDB = append(needDB, gid)
	}

	for _, gid := range needDB {
		chID, err := s.repo.GetChannelIDByGroupID(ctx, gid)
		if err != nil {
			return nil, err
		}
		if chID == 0 {
			continue
		}
		ch, err := s.repo.GetByID(ctx, chID)
		if err != nil {
			// Policy deleted mid-flight — treat as official.
			continue
		}
		// Bound but inactive → not effective
		out[gid] = buildSellSourceFromChannel(ch, ch.IsActive())
	}

	return out, nil
}

func buildSellSourceFromChannel(ch *Channel, effective bool) GroupSellPriceSource {
	if ch == nil {
		return GroupSellPriceSource{Source: SellPriceSourceOfficial}
	}
	id := ch.ID
	src := GroupSellPriceSource{
		Source:         SellPriceSourcePolicy,
		PolicyID:       &id,
		PolicyName:     ch.Name,
		PolicyStatus:   ch.Status,
		Effective:      effective && ch.IsActive(),
		InactivePolicy: !ch.IsActive(),
		ModelCount:     countPricingModels(ch.ModelPricing),
		SampleModels:   samplePricingModels(ch.ModelPricing, 3),
	}
	if !src.Effective {
		// Bound but not applied — UI should still show policy name with warning.
		src.Source = SellPriceSourcePolicy
	}
	return src
}

func countPricingModels(pricing []ChannelModelPricing) int {
	n := 0
	for _, p := range pricing {
		n += len(p.Models)
	}
	return n
}

func samplePricingModels(pricing []ChannelModelPricing, limit int) []string {
	if limit <= 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, limit)
	for _, p := range pricing {
		for _, m := range p.Models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			key := strings.ToLower(m)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, m)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

// BuildGroupPricingSummary builds the full pricing summary for one group.
func (s *ChannelService) BuildGroupPricingSummary(ctx context.Context, group *Group, billing *BillingService) (*GroupPricingSummary, error) {
	if group == nil {
		return nil, fmt.Errorf("%w: group is nil", ErrInvalidInput)
	}

	summary := &GroupPricingSummary{
		GroupID:        group.ID,
		GroupName:      group.Name,
		Platform:       group.Platform,
		RateMultiplier: group.RateMultiplier,
		Source:         SellPriceSourceOfficial,
		Hint:           "official",
		Models:         []GroupPricingModelPreview{},
	}
	if summary.RateMultiplier <= 0 {
		summary.RateMultiplier = 1
	}

	// Load bound policy (including inactive)
	var policy *Channel
	if chID, err := s.repo.GetChannelIDByGroupID(ctx, group.ID); err != nil {
		return nil, err
	} else if chID > 0 {
		if ch, err := s.GetByID(ctx, chID); err == nil {
			policy = ch
		}
	}

	if policy != nil {
		id := policy.ID
		summary.PolicyID = &id
		summary.PolicyName = policy.Name
		summary.PolicyStatus = policy.Status
		summary.Source = SellPriceSourcePolicy
		summary.Effective = policy.IsActive()
		summary.InactivePolicy = !policy.IsActive()
		summary.BillingModelSource = policy.BillingModelSource
		summary.RestrictModels = policy.RestrictModels
		summary.ApplyPricingToAccountStats = policy.ApplyPricingToAccountStats
		summary.AccountStatsRuleCount = len(policy.AccountStatsPricingRules)
		if summary.Effective {
			summary.Hint = "policy"
		} else {
			summary.Hint = "policy_inactive"
		}
	}

	// Model previews from policy entries (or empty → official-only message)
	if policy != nil && len(policy.ModelPricing) > 0 {
		summary.Models = buildModelPreviews(policy, group, billing, summary.Effective)
	}

	// Available policies for selector
	all, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	// Need group names for "shared with" hints — best-effort via GroupRepository if present.
	groupNameByID := map[int64]string{group.ID: group.Name}
	if s.groupRepo != nil {
		// Collect all referenced group IDs
		need := make([]int64, 0)
		for i := range all {
			need = append(need, all[i].GroupIDs...)
		}
		if len(need) > 0 {
			if platforms, pErr := s.repo.GetGroupPlatforms(ctx, need); pErr == nil {
				_ = platforms // platforms already known; names need separate path
			}
		}
		// Prefer groupRepo.GetByIDLite for names when available
		for i := range all {
			for _, gid := range all[i].GroupIDs {
				if _, ok := groupNameByID[gid]; ok {
					continue
				}
				if g, gErr := s.groupRepo.GetByIDLite(ctx, gid); gErr == nil && g != nil {
					groupNameByID[gid] = g.Name
				}
			}
		}
	}

	opts := make([]SellPricePolicyOption, 0, len(all))
	for i := range all {
		ch := &all[i]
		opt := SellPricePolicyOption{
			ID:           ch.ID,
			Name:         ch.Name,
			Status:       ch.Status,
			GroupCount:   len(ch.GroupIDs),
			ModelCount:   countPricingModels(ch.ModelPricing),
			SampleModels: samplePricingModels(ch.ModelPricing, 3),
			BoundHere:    summary.PolicyID != nil && *summary.PolicyID == ch.ID,
		}
		others := make([]string, 0)
		for _, gid := range ch.GroupIDs {
			if gid == group.ID {
				continue
			}
			if name, ok := groupNameByID[gid]; ok && name != "" {
				others = append(others, name)
			} else {
				others = append(others, fmt.Sprintf("#%d", gid))
			}
		}
		sort.Strings(others)
		opt.BoundOtherGroupNames = others
		opts = append(opts, opt)
	}
	sort.SliceStable(opts, func(i, j int) bool {
		// Bound first, then active, then name
		if opts[i].BoundHere != opts[j].BoundHere {
			return opts[i].BoundHere
		}
		if (opts[i].Status == StatusActive) != (opts[j].Status == StatusActive) {
			return opts[i].Status == StatusActive
		}
		return strings.ToLower(opts[i].Name) < strings.ToLower(opts[j].Name)
	})
	summary.AvailablePolicies = opts

	return summary, nil
}

func buildModelPreviews(policy *Channel, group *Group, billing *BillingService, policyEffective bool) []GroupPricingModelPreview {
	rate := 1.0
	if group != nil && group.RateMultiplier > 0 {
		rate = group.RateMultiplier
	}

	previews := make([]GroupPricingModelPreview, 0)
	seen := make(map[string]struct{})

	for _, entry := range policy.ModelPricing {
		// Skip entries for other platforms when group platform is set
		if group != nil && group.Platform != "" && entry.Platform != "" &&
			!isPlatformPricingMatch(group.Platform, entry.Platform) {
			continue
		}
		mode := string(entry.BillingMode)
		if mode == "" {
			mode = string(BillingModeToken)
		}
		for _, model := range entry.Models {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			key := strings.ToLower(model)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			p := GroupPricingModelPreview{
				Model:       model,
				BillingMode: mode,
				Source:      SellPriceSourceOfficial,
			}

			// Official
			if billing != nil {
				if mp, err := billing.GetModelPricing(model); err == nil && mp != nil {
					p.OfficialInput = floatPtr(mp.InputPricePerToken * 1_000_000)
					p.OfficialOutput = floatPtr(mp.OutputPricePerToken * 1_000_000)
				}
			}

			// Sell from channel entry (token flat prices; intervals omitted in summary)
			if entry.InputPrice != nil {
				p.SellInput = floatPtr(*entry.InputPrice * 1_000_000)
			}
			if entry.OutputPrice != nil {
				p.SellOutput = floatPtr(*entry.OutputPrice * 1_000_000)
			}

			if policyEffective && (entry.InputPrice != nil || entry.OutputPrice != nil || len(entry.Intervals) > 0 || entry.PerRequestPrice != nil) {
				p.Source = SellPriceSourcePolicy
			}

			// Effective unit for user = sell (or official if sell empty) × rate
			effIn := firstFloatPtr(p.SellInput, p.OfficialInput)
			effOut := firstFloatPtr(p.SellOutput, p.OfficialOutput)
			if effIn != nil {
				p.EffectiveInput = floatPtr(*effIn * rate)
			}
			if effOut != nil {
				p.EffectiveOutput = floatPtr(*effOut * rate)
			}

			if p.SellInput != nil && p.OfficialInput != nil && *p.OfficialInput > 0 {
				n := *p.SellInput / *p.OfficialInput
				// Round to 2 decimals for display stability
				n = math.Round(n*100) / 100
				p.MarkupN = &n
			}

			previews = append(previews, p)
		}
	}

	sort.SliceStable(previews, func(i, j int) bool {
		return strings.ToLower(previews[i].Model) < strings.ToLower(previews[j].Model)
	})
	return previews
}

func floatPtr(v float64) *float64 { return &v }

func firstFloatPtr(vals ...*float64) *float64 {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}
