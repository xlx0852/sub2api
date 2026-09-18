package codexcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyAndInspect(t *testing.T) {
	dir := t.TempDir()
	st, err := Apply(ApplyInput{
		CodexHome:  dir,
		BaseURL:    "https://code.example/v1",
		APIKey:     "sk-test",
		ProviderID: "sub2api",
		Model:      "gpt-5.6-sol",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !st.ManagedPresent || !st.HasAPIKey || st.Model != "gpt-5.6-sol" {
		t.Fatalf("unexpected status: %+v", st)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, startMark) || !strings.Contains(body, `wire_api = "responses"`) {
		t.Fatalf("managed block missing: %s", body)
	}
	// second apply should replace managed block, keep snapshot
	_, err = Apply(ApplyInput{
		CodexHome: dir,
		BaseURL:   "https://code.example/v2",
		APIKey:    "sk-test-2",
		Model:     "gpt-5.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "config.toml"))
	if !strings.Contains(string(data), "https://code.example/v2") {
		t.Fatalf("base url not updated: %s", data)
	}
	if !strings.Contains(string(data), startMark) {
		t.Fatal("marker lost")
	}
}

func TestRepairFromFallback(t *testing.T) {
	dir := t.TempDir()
	_, err := Repair(dir, ApplyInput{
		CodexHome: dir,
		BaseURL:   "https://x/v1",
		APIKey:    "k",
		Model:     "gpt-5.4",
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := Inspect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.ManagedPresent || st.Model != "gpt-5.4" {
		t.Fatalf("repair failed: %+v", st)
	}
}

func TestRefuseTamperedManagedBlock(t *testing.T) {
	dir := t.TempDir()
	_, err := Apply(ApplyInput{
		CodexHome: dir,
		BaseURL:   "https://x/v1",
		APIKey:    "k",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	raw, _ := os.ReadFile(path)
	// break ownership by removing wire_api inside managed segment
	tampered := strings.Replace(string(raw), `wire_api = "responses"`, `wire_api = "chat"`, 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Apply(ApplyInput{
		CodexHome: dir,
		BaseURL:   "https://y/v1",
		APIKey:    "k2",
	})
	if err == nil {
		t.Fatal("expected refusal")
	}
}
