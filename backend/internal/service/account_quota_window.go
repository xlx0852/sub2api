package service

import (
	"context"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"
)

// Account quota window sources / close reasons.
const (
	QuotaWindowSourceObserved  = "observed"
	QuotaWindowSourceResetCard = "reset_card"
	QuotaWindowSourceSeed      = "seed"

	QuotaWindowCloseObserved  = "observed_reset"
	QuotaWindowCloseCleared   = "provider_zero_reset"
	QuotaWindowCloseResetCard = "reset_card"
	QuotaWindowCloseReplaced  = "replaced"
)

// AccountQuotaWindow is one real quota period for an account (ledger row).
type AccountQuotaWindow struct {
	ID               int64
	AccountID        int64
	Platform         string
	Kind             string // 5h | 7d | 30d | 24h | ...
	StartAt          time.Time
	EndAt            time.Time
	WindowMinutes    *int
	Source           string
	ClosedReason     string
	UsedPercentOpen  *float64
	UsedPercentClose *float64
	IsOpen           bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AccountQuotaWindowRepository persists real quota-window history.
type AccountQuotaWindowRepository interface {
	ListByAccount(ctx context.Context, accountID int64, kind string, limit int) ([]*AccountQuotaWindow, error)
	ListOpenByAccount(ctx context.Context, accountID int64) ([]*AccountQuotaWindow, error)
	GetOpen(ctx context.Context, accountID int64, kind string) (*AccountQuotaWindow, error)
	// CloseAndOpen atomically closes the open row (if any) and inserts a new open row.
	CloseAndOpen(ctx context.Context, closeID int64, closeEnd time.Time, closeReason string, closeUsed *float64, open *AccountQuotaWindow) (*AccountQuotaWindow, error)
	// UpsertOpenRefresh updates end/used on the current open window without closing.
	UpsertOpenRefresh(ctx context.Context, accountID int64, kind string, endAt time.Time, used *float64, windowMinutes *int) error
	InsertOpen(ctx context.Context, w *AccountQuotaWindow) (*AccountQuotaWindow, error)
	CloseOpen(ctx context.Context, accountID int64, kind string, endAt time.Time, reason string, used *float64) error
}

// AccountQuotaUsageObservation is one upstream utilization snapshot aligned
// with local consumption ending at ObservedAt.
type AccountQuotaUsageObservation struct {
	ID            int64
	QuotaWindowID int64
	AccountID     int64
	Platform      string
	Kind          string
	ObservedAt    time.Time
	UsedPercent   float64
	Requests      int64
	Tokens        int64
	AccountCost   float64
	StandardCost  float64
	UserCost      float64
	CreatedAt     time.Time
}

// AccountQuotaObservationRepository is optional so existing window-ledger
// fakes remain source compatible while the SQL repository provides samples.
type AccountQuotaObservationRepository interface {
	HasObservation(ctx context.Context, quotaWindowID int64, usedPercent float64) (bool, error)
	InsertObservation(ctx context.Context, observation *AccountQuotaUsageObservation) (bool, error)
	ListObservations(ctx context.Context, quotaWindowID int64, limit int) ([]*AccountQuotaUsageObservation, error)
}

// QuotaWindowObservation is a live upstream snapshot for one kind.
type QuotaWindowObservation struct {
	AccountID     int64
	Platform      string
	Kind          string
	EndAt         time.Time
	WindowMinutes int
	UsedPercent   *float64
	// ObservedAt defaults to now when zero.
	ObservedAt time.Time
}

const quotaWindowSkew = 2 * time.Minute

// QuotaWindowLedger records real windows from upstream snapshots and reset cards.
type QuotaWindowLedger struct {
	repo            AccountQuotaWindowRepository
	observationRepo AccountQuotaObservationRepository
	statsReader     accountWindowStatsRangeReader
	// lockMu guards per account+kind mutexes so concurrent observe/reset on the
	// same window cannot double-seed or interleave close/open.
	lockMu sync.Mutex
	locks  map[string]*sync.Mutex
}

func (l *QuotaWindowLedger) SetObservationStatsReader(reader accountWindowStatsRangeReader) {
	if l == nil {
		return
	}
	l.statsReader = reader
}

func NewQuotaWindowLedger(repo AccountQuotaWindowRepository) *QuotaWindowLedger {
	if repo == nil {
		return nil
	}
	ledger := &QuotaWindowLedger{repo: repo, locks: make(map[string]*sync.Mutex)}
	if observationRepo, ok := repo.(AccountQuotaObservationRepository); ok {
		ledger.observationRepo = observationRepo
	}
	return ledger
}

// RecordUsageObservation appends a sample only when this window has not
// already recorded the same upstream percentage. Failures are returned to the
// caller, which must keep quota refresh/request forwarding non-blocking.
func (l *QuotaWindowLedger) RecordUsageObservation(ctx context.Context, accountID int64, kind string, observedAt time.Time, usedPercent float64, stats *WindowStats) error {
	if l == nil || l.repo == nil || l.observationRepo == nil || stats == nil || accountID <= 0 || kind == "" {
		return nil
	}
	if math.IsNaN(usedPercent) || math.IsInf(usedPercent, 0) || usedPercent < 0 || usedPercent > 100 {
		return nil
	}
	window, err := l.repo.GetOpen(ctx, accountID, kind)
	if err != nil || window == nil {
		return err
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	if observedAt.Before(window.StartAt) {
		return nil
	}
	if observedAt.After(window.EndAt) {
		observedAt = window.EndAt
	}
	_, err = l.observationRepo.InsertObservation(ctx, &AccountQuotaUsageObservation{
		QuotaWindowID: window.ID,
		AccountID:     accountID,
		Platform:      window.Platform,
		Kind:          kind,
		ObservedAt:    observedAt,
		UsedPercent:   usedPercent,
		Requests:      stats.Requests,
		Tokens:        stats.Tokens,
		AccountCost:   stats.Cost,
		StandardCost:  stats.StandardCost,
		UserCost:      stats.UserCost,
	})
	return err
}

func (l *QuotaWindowLedger) ListOpenWindowObservations(ctx context.Context, accountID int64, kind string, limit int) ([]*AccountQuotaUsageObservation, error) {
	if l == nil || l.repo == nil || l.observationRepo == nil || accountID <= 0 || kind == "" {
		return nil, nil
	}
	window, err := l.repo.GetOpen(ctx, accountID, kind)
	if err != nil || window == nil {
		return nil, err
	}
	return l.observationRepo.ListObservations(ctx, window.ID, limit)
}

func (l *QuotaWindowLedger) lockFor(accountID int64, kind string) *sync.Mutex {
	key := strconv.FormatInt(accountID, 10) + "|" + kind
	l.lockMu.Lock()
	defer l.lockMu.Unlock()
	m := l.locks[key]
	if m == nil {
		m = &sync.Mutex{}
		l.locks[key] = m
	}
	return m
}

func (l *QuotaWindowLedger) logErr(op string, accountID int64, kind string, err error) {
	if err == nil {
		return
	}
	slog.Warn("quota_window_ledger_failed", "op", op, "account_id", accountID, "kind", kind, "error", err)
}

// IsWaitingActivation reports whether the last known period ended without a
// successor. Callers use it to avoid inference probes that would themselves
// start a lazy rolling window.
func (l *QuotaWindowLedger) IsWaitingActivation(ctx context.Context, accountID int64, kind string, now time.Time) bool {
	if l == nil || l.repo == nil || accountID <= 0 || kind == "" {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	open, err := l.repo.GetOpen(ctx, accountID, kind)
	if err != nil {
		return false
	}
	if open != nil {
		return !open.EndAt.After(now)
	}
	rows, err := l.repo.ListByAccount(ctx, accountID, kind, 1)
	if err != nil || len(rows) == 0 || rows[0] == nil {
		return false
	}
	return rows[0].ClosedReason == QuotaWindowCloseCleared
}

// ObserveUpstream applies a live upstream window. Passive the open ledger row no longer
// matches (new reset_at / start jumped), the open row is closed and a new one opened.
func (l *QuotaWindowLedger) ObserveUpstream(ctx context.Context, obs QuotaWindowObservation) (returnErr error) {
	if l == nil || l.repo == nil {
		return nil
	}
	if obs.AccountID <= 0 || obs.Kind == "" || obs.EndAt.IsZero() {
		return nil
	}
	now := obs.ObservedAt
	if now.IsZero() {
		now = time.Now()
	}
	mins := obs.WindowMinutes
	if mins <= 0 {
		mins = defaultMinutesForQuotaKind(obs.Kind)
	}
	if mins <= 0 {
		return nil
	}
	start := obs.EndAt.Add(-time.Duration(mins) * time.Minute)
	if !obs.EndAt.After(start) {
		return nil
	}
	defer func() {
		if returnErr == nil {
			l.captureObservation(ctx, obs, now)
		}
	}()

	mu := l.lockFor(obs.AccountID, obs.Kind)
	mu.Lock()
	defer mu.Unlock()

	open, err := l.repo.GetOpen(ctx, obs.AccountID, obs.Kind)
	if err != nil {
		return err
	}
	// Some providers clear usage first and only start the next rolling period on
	// first real use. A zeroed observation whose countdown has elapsed closes the
	// old period without inventing a replacement window.
	dormant := !obs.EndAt.After(now.Add(quotaWindowSkew)) && quotaUsedNearZero(obs.UsedPercent)
	if open == nil {
		if dormant {
			return nil
		}
		w := &AccountQuotaWindow{
			AccountID:       obs.AccountID,
			Platform:        obs.Platform,
			Kind:            obs.Kind,
			StartAt:         start,
			EndAt:           obs.EndAt,
			WindowMinutes:   qwIntPtr(mins),
			Source:          QuotaWindowSourceSeed,
			UsedPercentOpen: obs.UsedPercent,
			IsOpen:          true,
		}
		_, err = l.repo.InsertOpen(ctx, w)
		return err
	}
	if dormant {
		closeEnd := obs.EndAt
		if closeEnd.After(now) {
			closeEnd = now
		}
		if closeEnd.Before(open.StartAt) {
			closeEnd = open.StartAt.Add(time.Second)
		}
		return l.repo.CloseOpen(ctx, obs.AccountID, obs.Kind, closeEnd, QuotaWindowCloseCleared, obs.UsedPercent)
	}

	// Same cycle: upstream often reports a RELATIVE countdown (reset_after_seconds),
	// so obs.end drifts with observation time even inside one window. Any overlap
	// between the observed window and the open one means "same cycle" — refresh only.
	// A real reset yields a NEW window whose start is after the open window's end
	// (no overlap), i.e. the upstream really moved on.
	endDelta := math.Abs(open.EndAt.Sub(obs.EndAt).Seconds())
	if endDelta <= quotaWindowSkew.Seconds() {
		return l.repo.UpsertOpenRefresh(ctx, obs.AccountID, obs.Kind, obs.EndAt, obs.UsedPercent, qwIntPtr(mins))
	}
	// A provider can reset a period early, so the new seven-day interval may
	// overlap the old interval's former expected range. A credible reset-at jump
	// together with usage returning near zero is a new real window regardless of
	// overlap. Used-percent alone is not enough because upstream can correct it.
	earlyReset := obs.EndAt.After(open.EndAt.Add(quotaWindowSkew)) && quotaUsageReset(open.UsedPercentOpen, obs.UsedPercent)
	// Otherwise, no overlap means the new period started after the old one ended.
	nonOverlap := !start.Before(open.EndAt.Add(-quotaWindowSkew))
	if !nonOverlap && !earlyReset {
		return l.repo.UpsertOpenRefresh(ctx, obs.AccountID, obs.Kind, obs.EndAt, obs.UsedPercent, qwIntPtr(mins))
	}

	// New cycle detected (official reset or reset card reflected upstream).
	closeEnd := start
	if closeEnd.Before(open.StartAt) {
		closeEnd = open.StartAt.Add(time.Second)
	}
	if closeEnd.After(open.EndAt) {
		closeEnd = open.EndAt
	}
	if !closeEnd.After(open.StartAt) {
		closeEnd = open.StartAt.Add(time.Second)
	}
	plat := obs.Platform
	if plat == "" {
		plat = open.Platform
	}
	next := &AccountQuotaWindow{
		AccountID:       obs.AccountID,
		Platform:        plat,
		Kind:            obs.Kind,
		StartAt:         start,
		EndAt:           obs.EndAt,
		WindowMinutes:   qwIntPtr(mins),
		Source:          QuotaWindowSourceObserved,
		UsedPercentOpen: obs.UsedPercent,
		IsOpen:          true,
	}
	_, err = l.repo.CloseAndOpen(ctx, open.ID, closeEnd, QuotaWindowCloseObserved, open.UsedPercentOpen, next)
	return err
}

func (l *QuotaWindowLedger) captureObservation(ctx context.Context, obs QuotaWindowObservation, observedAt time.Time) {
	if l == nil || l.repo == nil || l.observationRepo == nil || l.statsReader == nil || obs.UsedPercent == nil {
		return
	}
	window, err := l.repo.GetOpen(ctx, obs.AccountID, obs.Kind)
	if err != nil || window == nil {
		return
	}
	has, err := l.observationRepo.HasObservation(ctx, window.ID, *obs.UsedPercent)
	if err != nil || has {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	if observedAt.After(window.EndAt) {
		observedAt = window.EndAt
	}
	stats, err := l.statsReader.GetAccountWindowStatsRange(ctx, obs.AccountID, window.StartAt, observedAt)
	if err != nil || stats == nil {
		return
	}
	err = l.RecordUsageObservation(ctx, obs.AccountID, obs.Kind, observedAt, *obs.UsedPercent, windowStatsFromAccountStats(stats))
	l.logErr("capture_observation", obs.AccountID, obs.Kind, err)
}

func quotaUsedNearZero(used *float64) bool {
	return used != nil && *used >= 0 && *used <= 1
}

func quotaUsageReset(previous, current *float64) bool {
	if previous == nil || current == nil {
		return false
	}
	lowWatermark := math.Max(5, *previous*0.25)
	return *current >= 0 && *current <= lowWatermark && *previous-*current >= 5
}

// ForceResetCard closes the open window at now and opens a fresh cycle starting now.
// Call after a successful upstream reset-card redeem (before or after re-query).
func (l *QuotaWindowLedger) ForceResetCard(ctx context.Context, accountID int64, platform, kind string, windowMinutes int, usedAtClose *float64) error {
	if l == nil || l.repo == nil || accountID <= 0 || kind == "" {
		return nil
	}
	now := time.Now()
	mins := windowMinutes
	if mins <= 0 {
		mins = defaultMinutesForQuotaKind(kind)
	}
	if mins <= 0 {
		mins = 10080
	}
	mu := l.lockFor(accountID, kind)
	mu.Lock()
	defer mu.Unlock()

	open, err := l.repo.GetOpen(ctx, accountID, kind)
	if err != nil {
		return err
	}
	next := &AccountQuotaWindow{
		AccountID:     accountID,
		Platform:      platform,
		Kind:          kind,
		StartAt:       now,
		EndAt:         now.Add(time.Duration(mins) * time.Minute),
		WindowMinutes: qwIntPtr(mins),
		Source:        QuotaWindowSourceResetCard,
		IsOpen:        true,
	}
	if open == nil {
		_, err = l.repo.InsertOpen(ctx, next)
		return err
	}
	closeEnd := now
	if !closeEnd.After(open.StartAt) {
		closeEnd = open.StartAt.Add(time.Second)
	}
	plat := platform
	if plat == "" {
		plat = open.Platform
	}
	next.Platform = plat
	used := usedAtClose
	if used == nil {
		used = open.UsedPercentOpen
	}
	_, err = l.repo.CloseAndOpen(ctx, open.ID, closeEnd, QuotaWindowCloseResetCard, used, next)
	return err
}

func defaultMinutesForQuotaKind(kind string) int {
	switch kind {
	case "5h", "session":
		return 300
	case "7d":
		return 10080
	case "30d":
		return 30 * 24 * 60
	case "24h":
		return 1440
	default:
		return 0
	}
}

func qwIntPtr(v int) *int { return &v }

// quotaWindowFieldSpec maps one platform window kind onto extra/snapshot keys.
type quotaWindowFieldSpec struct {
	Kind        string
	EndKey      string // RFC3339 end/reset time in flat updates
	UsedKey     string // percent 0-100 when available
	MinutesKey  string
	DefaultMins int
	// UsedIsUtilization marks providers that persist utilization as a 0-1 ratio.
	UsedIsUtilization bool
}

// platformQuotaWindowSpecs is the cross-platform registry for passive observation.
var platformQuotaWindowSpecs = map[string][]quotaWindowFieldSpec{
	PlatformOpenAI: {
		{Kind: "5h", EndKey: "codex_5h_reset_at", UsedKey: "codex_5h_used_percent", MinutesKey: "codex_5h_window_minutes", DefaultMins: 300},
		{Kind: "7d", EndKey: "codex_7d_reset_at", UsedKey: "codex_7d_used_percent", MinutesKey: "codex_7d_window_minutes", DefaultMins: 10080},
	},
	PlatformKimi: {
		{Kind: "5h", EndKey: "kimi_quota_5h_reset_at", UsedKey: "kimi_quota_5h_utilization", DefaultMins: 300},
		{Kind: "7d", EndKey: "kimi_quota_7d_reset_at", UsedKey: "kimi_quota_7d_utilization", DefaultMins: 10080},
	},
	// Claude passive 7d sampled from response headers into extra (unix seconds).
	PlatformAnthropic: {
		{Kind: "7d", EndKey: "passive_usage_7d_reset", UsedKey: "passive_usage_7d_utilization", DefaultMins: 10080, UsedIsUtilization: true},
	},
}

// observeCodexQuotaWindowUpdates keeps the OpenAI call sites stable.
func observeCodexQuotaWindowUpdates(ctx context.Context, ledger *QuotaWindowLedger, accountID int64, updates map[string]any, now time.Time) {
	observePlatformQuotaWindowUpdates(ctx, ledger, accountID, PlatformOpenAI, updates, now)
}

// observePlatformQuotaWindowUpdates projects flat extra updates into the ledger for any platform.
func observePlatformQuotaWindowUpdates(ctx context.Context, ledger *QuotaWindowLedger, accountID int64, platform string, updates map[string]any, now time.Time) {
	if ledger == nil || accountID <= 0 || updates == nil || platform == "" {
		return
	}
	for _, spec := range platformQuotaWindowSpecs[platform] {
		obs := observationFromFlatUpdate(accountID, platform, updates, spec, now)
		if obs == nil {
			continue
		}
		if err := ledger.ObserveUpstream(ctx, *obs); err != nil {
			ledger.logErr("observe", accountID, spec.Kind, err)
		}
	}
	// Grok nested billing snapshot may arrive as a whole object under grok_billing_snapshot.
	if platform == PlatformGrok {
		if obs := observationFromGrokBillingUpdate(accountID, updates, now); obs != nil {
			if err := ledger.ObserveUpstream(ctx, *obs); err != nil {
				ledger.logErr("observe_grok", accountID, obs.Kind, err)
			}
		}
	}
}

// ObserveAccountQuotaWindows seeds/refreshes ledger rows from a full account snapshot
// (extra + session window columns). Safe to call after any platform quota fetch.
func ObserveAccountQuotaWindows(ctx context.Context, ledger *QuotaWindowLedger, acc *Account, now time.Time) {
	if ledger == nil || acc == nil || acc.ID <= 0 {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	platform := acc.Platform
	if platform == "" {
		return
	}
	// Flat extra keys (OpenAI / Kimi).
	if specs := platformQuotaWindowSpecs[platform]; len(specs) > 0 && acc.Extra != nil {
		observePlatformQuotaWindowUpdates(ctx, ledger, acc.ID, platform, acc.Extra, now)
	}
	// Grok billing object.
	if platform == PlatformGrok && acc.Extra != nil {
		if obs := observationFromGrokBillingUpdate(acc.ID, acc.Extra, now); obs != nil {
			if err := ledger.ObserveUpstream(ctx, *obs); err != nil {
				ledger.logErr("observe_grok_account", acc.ID, obs.Kind, err)
			}
		}
	}
	// Claude / Anthropic session rolling window on account columns.
	if (platform == PlatformAnthropic || platform == "claude") && acc.SessionWindowStart != nil && acc.SessionWindowEnd != nil {
		start := acc.SessionWindowStart.UTC()
		end := acc.SessionWindowEnd.UTC()
		if end.After(start) {
			mins := int(end.Sub(start).Minutes())
			if mins <= 0 {
				mins = 300
			}
			var used *float64
			if acc.Extra != nil {
				if u := extraFloat(acc.Extra, "session_window_utilization"); u != nil {
					// stored 0-1
					v := *u * 100
					if v < 0 {
						v = 0
					}
					if v > 100 {
						v = 100
					}
					used = &v
				}
			}
			if err := ledger.ObserveUpstream(ctx, QuotaWindowObservation{
				AccountID: acc.ID, Platform: platform, Kind: "session",
				EndAt: end, WindowMinutes: mins, UsedPercent: used, ObservedAt: now,
			}); err != nil {
				ledger.logErr("observe_session", acc.ID, "session", err)
			}
			// Also pin start via Observe: ledger derives start=end-mins; if session length
			// differs, WindowMinutes carries the true span.
			_ = mins
			_ = start
		}
	}
}

// ForceResetAccountWindows closes open ledger rows for the given kinds (or all open
// kinds when kinds empty) and opens fresh cycles starting now. Used by reset-card
// and any platform-local "reset quota" admin action.
func ForceResetAccountWindows(ctx context.Context, ledger *QuotaWindowLedger, acc *Account, kinds []string) {
	if ledger == nil || acc == nil || acc.ID <= 0 {
		return
	}
	platform := acc.Platform
	if len(kinds) == 0 {
		if open, err := ledger.repo.ListOpenByAccount(ctx, acc.ID); err == nil {
			for _, row := range open {
				if row != nil {
					kinds = append(kinds, row.Kind)
				}
			}
		}
		// If nothing open yet, fall back to platform defaults.
		if len(kinds) == 0 {
			for _, spec := range platformQuotaWindowSpecs[platform] {
				kinds = append(kinds, spec.Kind)
			}
			if platform == PlatformGrok {
				kinds = append(kinds, "7d")
			}
			if platform == PlatformAnthropic || platform == "claude" {
				kinds = append(kinds, "session")
			}
		}
	}
	seen := map[string]struct{}{}
	for _, kind := range kinds {
		if kind == "" {
			continue
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		mins := defaultMinutesForQuotaKind(kind)
		var used *float64
		if acc.Extra != nil {
			switch {
			case platform == PlatformOpenAI && kind == "7d":
				if v := extraInt(acc.Extra, "codex_7d_window_minutes"); v != nil {
					mins = *v
				}
				used = extraFloat(acc.Extra, "codex_7d_used_percent")
			case platform == PlatformOpenAI && kind == "5h":
				if v := extraInt(acc.Extra, "codex_5h_window_minutes"); v != nil {
					mins = *v
				}
				used = extraFloat(acc.Extra, "codex_5h_used_percent")
			case platform == PlatformKimi && kind == "7d":
				used = extraFloat(acc.Extra, "kimi_quota_7d_utilization")
			case platform == PlatformKimi && kind == "5h":
				used = extraFloat(acc.Extra, "kimi_quota_5h_utilization")
			}
		}
		if mins <= 0 {
			mins = defaultMinutesForQuotaKind(kind)
		}
		if err := ledger.ForceResetCard(ctx, acc.ID, platform, kind, mins, used); err != nil {
			ledger.logErr("force_reset", acc.ID, kind, err)
		}
	}
}

func observationFromFlatUpdate(accountID int64, platform string, updates map[string]any, spec quotaWindowFieldSpec, now time.Time) *QuotaWindowObservation {
	endRaw, ok := updates[spec.EndKey]
	if !ok || endRaw == nil {
		return nil
	}
	endAt, ok := anyToTimeValue(endRaw)
	if !ok {
		return nil
	}
	mins := spec.DefaultMins
	if spec.MinutesKey != "" {
		if v, ok := updates[spec.MinutesKey]; ok && v != nil {
			if n := anyToPositiveInt(v); n > 0 {
				mins = n
			}
		}
	}
	var used *float64
	if spec.UsedKey != "" {
		if v, ok := updates[spec.UsedKey]; ok && v != nil {
			if f, ok := anyToFloatValue(v); ok {
				if spec.UsedIsUtilization {
					f = f * 100
				}
				if f < 0 {
					f = 0
				}
				if f > 100 {
					f = 100
				}
				used = &f
			}
		}
	}
	return &QuotaWindowObservation{
		AccountID: accountID, Platform: platform, Kind: spec.Kind,
		EndAt: endAt, WindowMinutes: mins, UsedPercent: used, ObservedAt: now,
	}
}

func observationFromGrokBillingUpdate(accountID int64, updates map[string]any, now time.Time) *QuotaWindowObservation {
	raw, ok := updates[grokBillingSnapshotKey]
	if !ok || raw == nil {
		// also accept already-flattened nested map under same key from Extra
		if raw, ok = updates["grok_billing"]; !ok || raw == nil {
			return nil
		}
	}
	m := asStringAnyMap(raw)
	if m == nil {
		return nil
	}
	// billing_period_* was the retired calendar-month accounting period. Only
	// the official current period may create a quota ledger window now.
	endAt := anyToTimeFlexible(m["period_end"])
	startAt := anyToTimeFlexible(m["period_start"])
	if startAt == nil || endAt == nil || !endAt.After(*startAt) {
		return nil
	}
	mins := int(endAt.Sub(*startAt).Minutes())
	if mins <= 0 {
		return nil
	}
	kind, _ := classifyQuotaWindow("7d", mins)
	used := grokBillingUsedPercent(m)
	return &QuotaWindowObservation{
		AccountID: accountID, Platform: PlatformGrok, Kind: kind,
		EndAt: *endAt, WindowMinutes: mins, UsedPercent: used, ObservedAt: now,
	}
}

func anyToTimeValue(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, !t.IsZero()
	case *time.Time:
		if t == nil || t.IsZero() {
			return time.Time{}, false
		}
		return *t, true
	case int64:
		if t <= 0 {
			return time.Time{}, false
		}
		if t > 1e11 {
			t = t / 1000
		}
		return time.Unix(t, 0), true
	case int:
		return anyToTimeValue(int64(t))
	case float64:
		return anyToTimeValue(int64(t))
	case string:
		if t == "" {
			return time.Time{}, false
		}
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			return ts, true
		}
		if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return ts, true
		}
	}
	if p := anyToTimeFlexible(v); p != nil {
		return *p, true
	}
	return time.Time{}, false
}

func anyToPositiveInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	default:
		return 0
	}
}

func anyToFloatValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// observeOpenAIQuotaUsage maps /wham/usage rate_limit envelopes into ledger observations.
func observeOpenAIQuotaUsage(ctx context.Context, ledger *QuotaWindowLedger, accountID int64, usage *OpenAIQuotaUsage, now time.Time) {
	if ledger == nil || usage == nil || accountID <= 0 {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	// Primary path: top-level rate_limit (Codex default windows).
	feed := func(rl *OpenAIRateLimit) {
		if rl == nil {
			return
		}
		// Reuse snapshot normalizer via a tiny synthetic extra map when possible.
		snap := &OpenAICodexUsageSnapshot{}
		if w := rl.PrimaryWindow; w != nil {
			p := w.UsedPercent
			snap.PrimaryUsedPercent = &p
			ra := int(w.ResetAfterSeconds)
			snap.PrimaryResetAfterSeconds = &ra
			if w.LimitWindowSeconds > 0 {
				wm := int(w.LimitWindowSeconds / 60)
				snap.PrimaryWindowMinutes = &wm
			}
		}
		if w := rl.SecondaryWindow; w != nil {
			p := w.UsedPercent
			snap.SecondaryUsedPercent = &p
			ra := int(w.ResetAfterSeconds)
			snap.SecondaryResetAfterSeconds = &ra
			if w.LimitWindowSeconds > 0 {
				wm := int(w.LimitWindowSeconds / 60)
				snap.SecondaryWindowMinutes = &wm
			}
		}
		updates := buildCodexUsageExtraUpdates(snap, now)
		if len(updates) == 0 {
			return
		}
		// Persist extra so UI and future observes stay consistent.
		// Caller may not have accountRepo; ledger observe is enough here.
		observeCodexQuotaWindowUpdates(ctx, ledger, accountID, updates, now)
	}
	feed(usage.RateLimit)
	// Spark additional limits are intentionally not mixed into default 5h/7d parent ledger.
}
