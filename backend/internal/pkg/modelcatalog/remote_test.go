package modelcatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestParseAndValidateRejectsUnsafeCatalogs(t *testing.T) {
	tests := []string{
		`{"version":0,"platforms":{},"fallback_pricing":{}}`,
		`{"version":1,"platforms":{"openai":{"models":[{"id":"x"},{"id":"x"}]}},"fallback_pricing":{}}`,
		`{"version":1,"platforms":{"openai":{"models":[{"id":"x"}],"aliases":{"alias":"missing"}}},"fallback_pricing":{}}`,
		`{"version":1,"platforms":{"openai":{"models":[{"id":"x"}]}},"fallback_pricing":{"a":{"alias_of":"b"},"b":{"alias_of":"a"}}}`,
	}
	for _, raw := range tests {
		if _, err := parseAndValidate([]byte(raw)); err == nil {
			t.Fatalf("expected catalog validation failure for %s", raw)
		}
	}
}

func TestEmbeddedCatalogPassesRemoteValidation(t *testing.T) {
	if _, err := parseAndValidate(embeddedCatalog); err != nil {
		t.Fatalf("embedded catalog must be remotely publishable: %v", err)
	}
}

func TestPersistSnapshotWritesMatchingLastKnownGood(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"version":1,"platforms":{"openai":{"models":[{"id":"x"}]}},"fallback_pricing":{},"image_defaults":{},"ui_presets":{}}`)
	digestBytes := sha256.Sum256(raw)
	digest := hex.EncodeToString(digestBytes[:])
	if err := persistSnapshot(dir, raw, digest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("persisted catalog = %q, want %q", got, raw)
	}
	hash, err := os.ReadFile(filepath.Join(dir, "catalog.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if string(hash) != digest+"  catalog.json\n" {
		t.Fatalf("persisted checksum = %q", hash)
	}
}

func TestValidateRemoteURLRejectsNonHTTPSAndPrivateIP(t *testing.T) {
	for _, raw := range []string{"http://example.com/catalog.json", "https://127.0.0.1/catalog.json", "https://user@example.com/catalog.json"} {
		if _, err := validateRemoteURL(raw); err == nil {
			t.Fatalf("expected URL rejection for %q", raw)
		}
	}
	if _, err := validateRemoteURL("https://raw.githubusercontent.com/x/y/main/catalog.json"); err != nil {
		t.Fatalf("expected public HTTPS URL to pass: %v", err)
	}
}

func TestCatalogSnapshotConcurrentSwap(t *testing.T) {
	original := Get()
	defer setCatalog(original)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cat := Get()
				if cat == nil || cat.Version < 1 {
					t.Errorf("reader observed invalid snapshot")
				}
			}
		}()
	}
	for i := 1; i <= 100; i++ {
		candidate := cloneCatalog(original)
		candidate.Version = i
		setCatalog(candidate)
	}
	wg.Wait()
}

func TestCompareCatalogsClassifiesSemanticChanges(t *testing.T) {
	base := Get()
	next := cloneCatalog(base)
	platform := next.Platforms["openai"]
	platform.Models[0].DisplayName = "changed"
	next.Platforms["openai"] = platform
	next.FallbackPricing["test"] = PriceEntry{InputCostPerToken: 1}
	next.UIPresets["openai"] = append(next.UIPresets["openai"], UIPreset{Label: "test"})
	change := compareCatalogs(base, next)
	if len(change.Platforms) != 1 || change.Platforms[0] != "openai" || !change.Pricing || !change.UIPresets {
		t.Fatalf("unexpected semantic diff: %#v", change)
	}
}

func TestJitterDurationStaysWithinTenPercent(t *testing.T) {
	base := 10 * time.Minute
	for i := 0; i < 100; i++ {
		got := jitterDuration(base)
		if got < 9*time.Minute || got > 11*time.Minute {
			t.Fatalf("jitterDuration(%v) = %v", base, got)
		}
	}
}
