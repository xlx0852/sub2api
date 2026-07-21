package service

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// UpstreamOutcomeClass classifies finished OpenAI upstream attempts before health/sticky updates.
type UpstreamOutcomeClass string

const (
	UpstreamOutcomeSuccess    UpstreamOutcomeClass = "success"
	UpstreamOutcomeCaller     UpstreamOutcomeClass = "caller"
	UpstreamOutcomeCredential UpstreamOutcomeClass = "credential"
	UpstreamOutcomeQuota      UpstreamOutcomeClass = "quota"
	UpstreamOutcomeTransient  UpstreamOutcomeClass = "transient"
	UpstreamOutcomeUnknown    UpstreamOutcomeClass = "unknown"
)

// OpenAICredentialClass isolates OAuth/ChatGPT-login pools from API-key pools.
type OpenAICredentialClass string

const (
	OpenAICredentialClassUnspecified OpenAICredentialClass = ""
	OpenAICredentialClassOAuth       OpenAICredentialClass = "oauth_chatgpt"
	OpenAICredentialClassAPIKey      OpenAICredentialClass = "api_key"
)

const (
	openAIDefaultQuotaCooldown = 60 * time.Second
	openAIMinQuotaCooldown     = time.Second
	openAIMaxQuotaCooldown     = 24 * time.Hour
	openAIMaxKeyPoolCooldown   = 10 * time.Minute
	openAIUnknownUsageScore    = 100.0
	openAIStickyReevalInterval = 60 * time.Second
	openAIStickyAutoSwitchPct  = 80.0
)

var (
	openAIOutcomeSuccessTotal    atomic.Int64
	openAIOutcomeCallerTotal     atomic.Int64
	openAIOutcomeCredentialTotal atomic.Int64
	openAIOutcomeQuotaTotal      atomic.Int64
	openAIOutcomeTransientTotal  atomic.Int64
	openAIOutcomeUnknownTotal    atomic.Int64
)

// ClassifyUpstreamOutcome maps status/transport failures to a single outcome class.
func ClassifyUpstreamOutcome(statusCode int, transportErr error, stalled bool) UpstreamOutcomeClass {
	if stalled {
		return UpstreamOutcomeTransient
	}
	if transportErr != nil && statusCode == 0 {
		return UpstreamOutcomeTransient
	}
	if statusCode >= 200 && statusCode < 300 {
		return UpstreamOutcomeSuccess
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return UpstreamOutcomeCredential
	case http.StatusTooManyRequests:
		return UpstreamOutcomeQuota
	case http.StatusRequestTimeout, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return UpstreamOutcomeTransient
	}
	if statusCode >= 500 && statusCode <= 599 {
		return UpstreamOutcomeTransient
	}
	if statusCode >= 400 && statusCode <= 499 {
		return UpstreamOutcomeCaller
	}
	if statusCode == 0 {
		return UpstreamOutcomeTransient
	}
	return UpstreamOutcomeUnknown
}

func recordOpenAIUpstreamOutcome(class UpstreamOutcomeClass) {
	switch class {
	case UpstreamOutcomeSuccess:
		openAIOutcomeSuccessTotal.Add(1)
	case UpstreamOutcomeCaller:
		openAIOutcomeCallerTotal.Add(1)
	case UpstreamOutcomeCredential:
		openAIOutcomeCredentialTotal.Add(1)
	case UpstreamOutcomeQuota:
		openAIOutcomeQuotaTotal.Add(1)
	case UpstreamOutcomeTransient:
		openAIOutcomeTransientTotal.Add(1)
	default:
		openAIOutcomeUnknownTotal.Add(1)
	}
}

// OpenAIUpstreamOutcomeMetricsSnapshot exposes non-PII counters for ops.
func OpenAIUpstreamOutcomeMetricsSnapshot() map[string]int64 {
	return map[string]int64{
		"success":    openAIOutcomeSuccessTotal.Load(),
		"caller":     openAIOutcomeCallerTotal.Load(),
		"credential": openAIOutcomeCredentialTotal.Load(),
		"quota":      openAIOutcomeQuotaTotal.Load(),
		"transient":  openAIOutcomeTransientTotal.Load(),
		"unknown":    openAIOutcomeUnknownTotal.Load(),
	}
}

// ParseRetryAfterDuration parses Retry-After as delta-seconds or HTTP-date.
func ParseRetryAfterDuration(value string, now time.Time) (time.Duration, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseFloat(text, 64); err == nil && seconds > 0 {
		return clampDuration(time.Duration(seconds*float64(time.Second)), openAIMinQuotaCooldown, openAIMaxQuotaCooldown), true
	}
	if ts, err := http.ParseTime(text); err == nil {
		delay := ts.Sub(now)
		if delay <= 0 {
			return 0, false
		}
		return clampDuration(delay, openAIMinQuotaCooldown, openAIMaxQuotaCooldown), true
	}
	return 0, false
}

func clampDuration(value, min, max time.Duration) time.Duration {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// ComputeOpenAIQuotaCooldownUntil prefers Retry-After, then codex/reset-derived resetAt, then default.
func ComputeOpenAIQuotaCooldownUntil(headers http.Header, resetHint *time.Time, now time.Time) time.Time {
	if headers != nil {
		if delay, ok := ParseRetryAfterDuration(headers.Get("Retry-After"), now); ok {
			return now.Add(delay)
		}
	}
	if resetHint != nil && resetHint.After(now) {
		delay := clampDuration(resetHint.Sub(now), openAIMinQuotaCooldown, openAIMaxQuotaCooldown)
		return now.Add(delay)
	}
	return now.Add(openAIDefaultQuotaCooldown)
}

// OpenAIAccountCredentialClass returns the scheduling credential class for an account.
func OpenAIAccountCredentialClass(account *Account) OpenAICredentialClass {
	if account == nil {
		return OpenAICredentialClassUnspecified
	}
	if account.IsOpenAIOAuth() {
		return OpenAICredentialClassOAuth
	}
	if account.IsOpenAI() && (account.Type == AccountTypeAPIKey || account.IsAPIKeyOrBedrock()) {
		return OpenAICredentialClassAPIKey
	}
	if account.IsOpenAICompatible() && account.Type == AccountTypeAPIKey {
		return OpenAICredentialClassAPIKey
	}
	if account.IsOpenAICompatible() && account.IsOAuth() {
		return OpenAICredentialClassOAuth
	}
	return OpenAICredentialClassUnspecified
}

// OpenAICredentialClassMatches reports whether account satisfies a required class filter.
func OpenAICredentialClassMatches(account *Account, required OpenAICredentialClass) bool {
	if required == OpenAICredentialClassUnspecified {
		return true
	}
	got := OpenAIAccountCredentialClass(account)
	if got == OpenAICredentialClassUnspecified {
		return false
	}
	return got == required
}

// ComputeOpenAIUsageScore returns max known window usage percent; unknown => 100 (hottest).
func ComputeOpenAIUsageScore(account *Account, now time.Time) float64 {
	if account == nil || len(account.Extra) == 0 || openAIQuotaHeadroomSnapshotStale(account.Extra, now) {
		return openAIUnknownUsageScore
	}
	values := make([]float64, 0, 2)
	if primary, ok := resolveAccountExtraNumber(account.Extra, "codex_primary_used_percent", "codex_7d_used_percent"); ok &&
		!openAIQuotaWindowResetAny(account.Extra, now, "primary", "7d") {
		values = append(values, clamp01(primary/100)*100)
	}
	if secondary, ok := resolveAccountExtraNumber(account.Extra, "codex_secondary_used_percent", "codex_5h_used_percent"); ok &&
		!openAIQuotaWindowResetAny(account.Extra, now, "secondary", "5h") {
		values = append(values, clamp01(secondary/100)*100)
	}
	if len(values) == 0 {
		return openAIUnknownUsageScore
	}
	maxValue := values[0]
	for _, value := range values[1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

// OpenAIAccountCredentialGeneration returns a sticky generation token for credential identity.
// Missing generation is treated as 0 for backward compatibility.
func OpenAIAccountCredentialGeneration(account *Account) int64 {
	if account == nil {
		return 0
	}
	if account.Extra != nil {
		if raw, ok := account.Extra["credential_generation"]; ok {
			switch v := raw.(type) {
			case float64:
				if v > 0 {
					return int64(v)
				}
			case int64:
				if v > 0 {
					return v
				}
			case int:
				if v > 0 {
					return int64(v)
				}
			case string:
				if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
					return n
				}
			}
		}
	}
	// Fallback: UpdatedAt unix when present so credential rotations that bump UpdatedAt invalidate sticky.
	if !account.UpdatedAt.IsZero() {
		return account.UpdatedAt.Unix()
	}
	return 0
}

// apiKeyPoolState is a small CAS helper for multi-key 429 rotation.
type apiKeyPoolState struct {
	liveKey      string
	cooledUntil  map[string]int64 // key -> unix ms
}

func newAPIKeyPoolState(liveKey string) *apiKeyPoolState {
	return &apiKeyPoolState{
		liveKey:     strings.TrimSpace(liveKey),
		cooledUntil: make(map[string]int64),
	}
}

// CoolAndRotate cools the attempted key and CAS-rotates live key only when it still matches attempted.
// Returns the live key after the operation and whether a rotate happened.
func (s *apiKeyPoolState) CoolAndRotate(attemptedKey string, candidates []string, cooldown time.Duration, now time.Time) (liveKey string, rotated bool) {
	if s == nil {
		return "", false
	}
	attemptedKey = strings.TrimSpace(attemptedKey)
	if attemptedKey == "" {
		return s.liveKey, false
	}
	if cooldown <= 0 {
		cooldown = openAIDefaultQuotaCooldown
	}
	cooldown = clampDuration(cooldown, openAIMinQuotaCooldown, openAIMaxKeyPoolCooldown)
	until := now.Add(cooldown).UnixMilli()
	if s.cooledUntil == nil {
		s.cooledUntil = make(map[string]int64)
	}
	s.cooledUntil[attemptedKey] = until

	// Only rotate if live key is still the attempted key (CAS).
	if strings.TrimSpace(s.liveKey) != attemptedKey {
		return s.liveKey, false
	}
	nowMs := now.UnixMilli()
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == attemptedKey {
			continue
		}
		if untilMs, cooled := s.cooledUntil[candidate]; cooled && untilMs > nowMs {
			continue
		}
		s.liveKey = candidate
		return s.liveKey, true
	}
	return s.liveKey, false
}

func (s *apiKeyPoolState) IsCooled(key string, now time.Time) bool {
	if s == nil || s.cooledUntil == nil {
		return false
	}
	untilMs, ok := s.cooledUntil[strings.TrimSpace(key)]
	return ok && untilMs > now.UnixMilli()
}

// Process-local sticky invalidation epoch. Bumped on credential failures so
// existing sticky bindings (which stored the prior epoch) miss and fall through.
// Cross-instance cleanup still relies on unschedulable/rate-limit sticky clear.
var openAIStickyInvalidEpoch sync.Map // accountID int64 -> epoch int64

// BumpOpenAIAccountStickyEpoch invalidates sticky bindings that captured an older epoch.
func BumpOpenAIAccountStickyEpoch(accountID int64) int64 {
	if accountID <= 0 {
		return 0
	}
	for {
		raw, _ := openAIStickyInvalidEpoch.LoadOrStore(accountID, int64(0))
		current, _ := raw.(int64)
		next := current + 1
		if openAIStickyInvalidEpoch.CompareAndSwap(accountID, current, next) {
			return next
		}
	}
}

// OpenAIAccountStickyEpoch combines credential generation with invalidation epoch.
func OpenAIAccountStickyEpoch(account *Account) int64 {
	if account == nil {
		return 0
	}
	epoch := OpenAIAccountCredentialGeneration(account)
	if raw, ok := openAIStickyInvalidEpoch.Load(account.ID); ok {
		if bump, ok := raw.(int64); ok && bump > 0 {
			// Keep epochs monotonic and distinct from plain UpdatedAt values.
			if bump > epoch {
				return bump
			}
			return epoch + bump
		}
	}
	return epoch
}

func openAIStickyEpochCacheKey(sessionHash string) string {
	normalized := strings.TrimSpace(sessionHash)
	if normalized == "" {
		return ""
	}
	return "openai:sticky-epoch:" + normalized
}

func openAIResponseStickyEpochCacheKey(responseID string) string {
	normalized := strings.TrimSpace(responseID)
	if normalized == "" {
		return ""
	}
	return "openai:resp-epoch:" + normalized
}
