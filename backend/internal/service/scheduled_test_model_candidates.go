package service

import (
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const scheduledDiagnosticsMaxModels = 2

// ResolveScheduledDiagnosticsModels returns ordered candidate models for connectivity diagnostics.
// Primary model is tried first; remaining models are cheaper/fallback candidates.
func ResolveScheduledDiagnosticsModels(account *Account, primary string) []string {
	if account == nil {
		return uniqueNonEmptyModels(primary)
	}

	primary = strings.TrimSpace(primary)
	candidates := make([]string, 0, scheduledDiagnosticsMaxModels+2)
	if primary != "" {
		candidates = append(candidates, primary)
	}

	// Prefer account-declared cloud models / mapping keys when present.
	candidates = append(candidates, accountCloudModels(account)...)
	candidates = append(candidates, modelIDsFromMapping(account.GetModelMapping())...)
	candidates = append(candidates, platformDefaultDiagnosticModels(account)...)

	out := uniqueNonEmptyModels(candidates...)
	if len(out) > scheduledDiagnosticsMaxModels {
		out = out[:scheduledDiagnosticsMaxModels]
	}
	if len(out) == 0 {
		return uniqueNonEmptyModels(platformDefaultDiagnosticModels(account)...)
	}
	return out
}

func accountCloudModels(account *Account) []string {
	if account == nil || len(account.Extra) == 0 {
		return nil
	}
	raw, ok := account.Extra["cloud_models"]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func modelIDsFromMapping(mapping map[string]string) []string {
	if len(mapping) == 0 {
		return nil
	}
	out := make([]string, 0, len(mapping))
	for k, v := range mapping {
		key := strings.TrimSpace(k)
		// Skip wildcard keys entirely; they are client-side aliases, not probe targets.
		if key == "" || strings.Contains(key, "*") {
			continue
		}
		// Prefer upstream model IDs (values).
		candidate := strings.TrimSpace(v)
		if candidate == "" || strings.Contains(candidate, "*") {
			candidate = key
		}
		if candidate == "" || strings.Contains(candidate, "*") {
			continue
		}
		out = append(out, candidate)
	}
	sort.Strings(out)
	return out
}

func platformDefaultDiagnosticModels(account *Account) []string {
	if account == nil {
		return nil
	}
	switch {
	case account.IsOpenAI():
		// Prefer cheaper connectivity probes only.
		return []string{
			openai.CurrentDefaultTestModel(),
			"gpt-5.4-mini",
		}
	case account.IsAnthropic():
		return []string{
			claude.CurrentDefaultTestModel(),
			"claude-haiku-4-5-20251001",
			"claude-sonnet-4-5-20250929",
		}
	case account.Platform == PlatformGrok:
		return []string{
			xai.CurrentDefaultChatModel(),
			"grok-composer-2.5-fast",
		}
	case account.Platform == PlatformKimi:
		return KimiDefaultModelIDs()
	case account.IsGemini():
		return []string{
			GeminiDefaultTestModel(),
		}
	case account.Platform == PlatformAntigravity:
		models := antigravity.DefaultModels()
		out := make([]string, 0, min(3, len(models)))
		for _, m := range models {
			out = append(out, m.ID)
			if len(out) >= 3 {
				break
			}
		}
		return out
	default:
		return nil
	}
}

func uniqueNonEmptyModels(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// DefaultScheduledDiagnosticsModel picks a cheap default seed model for auto-enabled plans.
func DefaultScheduledDiagnosticsModel(account *Account) string {
	models := ResolveScheduledDiagnosticsModels(account, "")
	if len(models) == 0 {
		return "gpt-5.4-mini"
	}
	// Prefer known cheap IDs when present.
	cheapHints := []string{"mini", "haiku", "fast", "flash", "lite"}
	for _, m := range models {
		lower := strings.ToLower(m)
		for _, h := range cheapHints {
			if strings.Contains(lower, h) {
				return m
			}
		}
	}
	return models[0]
}

// DefaultScheduledDiagnosticsCron returns a 5-minute staggered cron for an account.
func DefaultScheduledDiagnosticsCron(accountID int64) string {
	offset := accountID % 5
	if offset < 0 {
		offset = -offset
	}
	// e.g. 3-59/5 * * * *
	return strings.TrimSpace(strings.Join([]string{
		itoaOffset(offset) + "-59/5", "*", "*", "*", "*",
	}, " "))
}

func itoaOffset(v int64) string {
	if v == 0 {
		return "0"
	}
	const digits = "0123456789"
	if v < 10 {
		return string(digits[v])
	}
	return string(digits[v/10]) + string(digits[v%10])
}
