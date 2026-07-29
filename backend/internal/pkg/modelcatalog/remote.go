package modelcatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultMaxBodyBytes int64 = 8 << 20

type RemoteOptions struct {
	Enabled           bool
	RemoteURL         string
	HashURL           string
	DataDir           string
	FallbackFile      string
	UpdateInterval    time.Duration
	HashCheckInterval time.Duration
	RequestTimeout    time.Duration
	MaxBodyBytes      int64
}

type RefreshStatus struct {
	Version          int       `json:"version"`
	UpdatedAt        string    `json:"updated_at,omitempty"`
	Source           string    `json:"source"`
	Hash             string    `json:"hash"`
	LastCheck        time.Time `json:"last_check,omitempty"`
	LastSuccess      time.Time `json:"last_success,omitempty"`
	NextCheck        time.Time `json:"next_check,omitempty"`
	Refreshing       bool      `json:"refreshing"`
	RemoteEnabled    bool      `json:"remote_enabled"`
	LastError        string    `json:"last_error,omitempty"`
	ChangedPlatforms []string  `json:"changed_platforms,omitempty"`
	PricingChanged   bool      `json:"pricing_changed,omitempty"`
	UIPresetsChanged bool      `json:"ui_presets_changed,omitempty"`
	SuccessCount     uint64    `json:"success_count"`
	FailureCount     uint64    `json:"failure_count"`
}

var remoteState = struct {
	sync.RWMutex
	opts   RemoteOptions
	status RefreshStatus
	cancel context.CancelFunc
}{status: RefreshStatus{Source: "embedded"}}

func Start(opts RemoteOptions) {
	Stop()
	applyOptionDefaults(&opts)
	loadLastKnownGood(opts)
	remoteState.Lock()
	remoteState.opts = opts
	remoteState.status.RemoteEnabled = opts.Enabled
	ctx, cancel := context.WithCancel(context.Background())
	remoteState.cancel = cancel
	remoteState.Unlock()
	if !opts.Enabled {
		return
	}
	go runUpdater(ctx, opts)
}

func Stop() {
	remoteState.Lock()
	cancel := remoteState.cancel
	remoteState.cancel = nil
	remoteState.Unlock()
	if cancel != nil {
		cancel()
	}
}

func Status() RefreshStatus {
	remoteState.RLock()
	defer remoteState.RUnlock()
	status := remoteState.status
	if cat := Get(); cat != nil {
		status.Version, status.UpdatedAt = cat.Version, cat.UpdatedAt
	}
	return status
}

func Refresh(ctx context.Context) error {
	remoteState.Lock()
	if remoteState.status.Refreshing {
		remoteState.Unlock()
		return errors.New("model catalog refresh already in progress")
	}
	opts := remoteState.opts
	if !opts.Enabled {
		remoteState.Unlock()
		return errors.New("remote model catalog refresh is disabled")
	}
	remoteState.status.Refreshing = true
	remoteState.status.LastCheck = time.Now().UTC()
	remoteState.Unlock()

	err := refresh(ctx, opts)
	remoteState.Lock()
	remoteState.status.Refreshing = false
	if err != nil {
		remoteState.status.LastError = sanitizeError(err)
		remoteState.status.FailureCount++
	} else {
		remoteState.status.LastError = ""
		remoteState.status.LastSuccess = time.Now().UTC()
		remoteState.status.SuccessCount++
	}
	remoteState.Unlock()
	return err
}

func runUpdater(ctx context.Context, opts RemoteOptions) {
	if err := Refresh(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("model catalog startup refresh failed", "error", sanitizeError(err))
	}
	interval := opts.HashCheckInterval
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	for {
		delay := jitterDuration(interval)
		remoteState.Lock()
		remoteState.status.NextCheck = time.Now().UTC().Add(delay)
		remoteState.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			if err := Refresh(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("model catalog periodic refresh failed", "error", sanitizeError(err))
			}
		}
	}
}

func refresh(ctx context.Context, opts RemoteOptions) error {
	remoteURL, err := validateRemoteURL(opts.RemoteURL)
	if err != nil {
		return err
	}
	hashURL, err := validateRemoteURL(opts.HashURL)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: opts.RequestTimeout}
	expectedRaw, err := fetchWithRetry(ctx, client, hashURL, 4096)
	if err != nil {
		return fmt.Errorf("fetch checksum: %w", err)
	}
	expected := strings.Fields(string(expectedRaw))
	if len(expected) == 0 || len(expected[0]) != 64 {
		return errors.New("invalid remote checksum")
	}
	current := Status()
	if strings.EqualFold(current.Hash, expected[0]) {
		return nil
	}
	raw, err := fetchWithRetry(ctx, client, remoteURL, opts.MaxBodyBytes)
	if err != nil {
		return fmt.Errorf("fetch catalog: %w", err)
	}
	digest := sha256.Sum256(raw)
	actual := hex.EncodeToString(digest[:])
	if !strings.EqualFold(expected[0], actual) {
		return errors.New("remote catalog checksum mismatch")
	}
	cat, err := parseAndValidate(raw)
	if err != nil {
		return err
	}
	active := Get()
	if active != nil && cat.Version < active.Version {
		return fmt.Errorf("remote catalog version %d is older than active version %d", cat.Version, active.Version)
	}
	if err := persistSnapshot(opts.DataDir, raw, actual); err != nil {
		return err
	}
	setCatalog(cat)
	change := compareCatalogs(active, cat)
	remoteState.Lock()
	remoteState.status.Source, remoteState.status.Hash = "remote", actual
	remoteState.status.ChangedPlatforms = change.Platforms
	remoteState.status.PricingChanged = change.Pricing
	remoteState.status.UIPresetsChanged = change.UIPresets
	remoteState.Unlock()
	slog.Info("model catalog refreshed", "version", cat.Version, "hash", actual[:12], "platforms", change.Platforms, "pricing_changed", change.Pricing, "ui_presets_changed", change.UIPresets)
	return nil
}

func applyOptionDefaults(opts *RemoteOptions) {
	if opts.DataDir == "" {
		opts.DataDir = "./data/model-catalog"
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 30 * time.Second
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = defaultMaxBodyBytes
	}
}

func loadLastKnownGood(opts RemoteOptions) {
	paths := []struct{ path, source string }{{filepath.Join(opts.DataDir, "catalog.json"), "local"}, {opts.FallbackFile, "local"}}
	for _, candidate := range paths {
		if strings.TrimSpace(candidate.path) == "" {
			continue
		}
		raw, err := os.ReadFile(candidate.path)
		if err != nil {
			continue
		}
		cat, err := parseAndValidate(raw)
		if err != nil {
			continue
		}
		digest := sha256.Sum256(raw)
		setCatalog(cat)
		remoteState.Lock()
		remoteState.status.Source = candidate.source
		remoteState.status.Hash = hex.EncodeToString(digest[:])
		remoteState.Unlock()
		return
	}
}

func parseAndValidate(raw []byte) (*Catalog, error) {
	var cat Catalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		return nil, fmt.Errorf("parse model catalog: %w", err)
	}
	if err := validateCatalog(&cat); err != nil {
		return nil, err
	}
	return &cat, nil
}

func validateCatalog(cat *Catalog) error {
	if cat == nil || cat.Version < 1 || len(cat.Platforms) == 0 {
		return errors.New("invalid model catalog root")
	}
	for platform, cfg := range cat.Platforms {
		seen := map[string]bool{}
		known := map[string]bool{}
		for _, model := range cfg.Models {
			id := strings.TrimSpace(model.ID)
			if id == "" {
				return fmt.Errorf("platform %s contains empty model id", platform)
			}
			if seen[id] {
				return fmt.Errorf("platform %s contains duplicate model %s", platform, id)
			}
			seen[id], known[id] = true, true
		}
		for id := range cfg.DefaultMapping {
			known[id] = true
		}
		for alias, target := range cfg.Aliases {
			if strings.TrimSpace(alias) == "" || !known[target] {
				return fmt.Errorf("platform %s contains invalid alias %s", platform, alias)
			}
		}
	}
	for name, entry := range cat.FallbackPricing {
		if entry.InputCostPerToken < 0 || entry.OutputCostPerToken < 0 || entry.OutputCostPerImage < 0 {
			return fmt.Errorf("negative fallback price for %s", name)
		}
		seen := map[string]bool{name: true}
		for target := strings.TrimSpace(entry.AliasOf); target != ""; {
			if seen[target] {
				return fmt.Errorf("fallback pricing alias cycle at %s", target)
			}
			seen[target] = true
			next, ok := cat.FallbackPricing[target]
			if !ok {
				return fmt.Errorf("fallback pricing alias %s targets unknown entry %s", name, target)
			}
			target = strings.TrimSpace(next.AliasOf)
		}
	}
	return nil
}

func validateRemoteURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return "", errors.New("model catalog URL must be public HTTPS")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
		return "", errors.New("model catalog URL must not target a private address")
	}
	return u.String(), nil
}

func fetchLimited(ctx context.Context, client *http.Client, target string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json,text/plain")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("remote response exceeds %d bytes", limit)
	}
	return raw, nil
}

func fetchWithRetry(ctx context.Context, client *http.Client, target string, limit int64) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := fetchLimited(ctx, client, target, limit)
		if err == nil {
			return raw, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 200 * time.Millisecond):
		}
	}
	return nil, lastErr
}

func jitterDuration(base time.Duration) time.Duration {
	if base <= 0 {
		return time.Minute
	}
	window := base / 10
	if window <= 0 {
		return base
	}
	return base - window + time.Duration(rand.Int64N(int64(window*2)+1))
}

func persistSnapshot(dir string, raw []byte, digest string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create catalog data directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".catalog-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(raw); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Chmod(name, 0o644); err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(dir, "catalog.json")); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "catalog.sha256"), []byte(digest+"  catalog.json\n"), 0o644)
}

func setCatalog(cat *Catalog) { globalMu.Lock(); global = cat; globalMu.Unlock() }

type catalogChange struct {
	Platforms []string
	Pricing   bool
	UIPresets bool
}

func compareCatalogs(oldCatalog, newCatalog *Catalog) catalogChange {
	if oldCatalog == nil || newCatalog == nil {
		return catalogChange{}
	}
	change := catalogChange{
		Pricing:   !reflect.DeepEqual(oldCatalog.FallbackPricing, newCatalog.FallbackPricing) || !reflect.DeepEqual(oldCatalog.ImageDefaults, newCatalog.ImageDefaults),
		UIPresets: !reflect.DeepEqual(oldCatalog.UIPresets, newCatalog.UIPresets),
	}
	seen := make(map[string]struct{}, len(oldCatalog.Platforms)+len(newCatalog.Platforms))
	for platform := range oldCatalog.Platforms {
		seen[platform] = struct{}{}
	}
	for platform := range newCatalog.Platforms {
		seen[platform] = struct{}{}
	}
	for platform := range seen {
		if !reflect.DeepEqual(oldCatalog.Platforms[platform], newCatalog.Platforms[platform]) {
			change.Platforms = append(change.Platforms, platform)
		}
	}
	sort.Strings(change.Platforms)
	return change
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 240 {
		message = message[:240]
	}
	return strconv.QuoteToASCII(message)[1 : len(strconv.QuoteToASCII(message))-1]
}
