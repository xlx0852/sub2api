package codexcfg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	ProviderID = "sub2api"
	startMark  = "# BEGIN sub2api-tray managed"
	endMark    = "# END sub2api-tray managed"
)

type ApplyInput struct {
	CodexHome  string
	BaseURL    string
	APIKey     string
	ProviderID string
	Model      string
}

type Status struct {
	CodexHome       string `json:"codex_home"`
	ConfigPath      string `json:"config_path"`
	AuthPath        string `json:"auth_path"`
	ConfigExists    bool   `json:"config_exists"`
	ManagedPresent  bool   `json:"managed_present"`
	ModelProvider   string `json:"model_provider,omitempty"`
	Model           string `json:"model,omitempty"`
	BaseURL         string `json:"base_url,omitempty"`
	HasAPIKey       bool   `json:"has_api_key"`
	SnapshotExists  bool   `json:"snapshot_exists"`
	OwnershipOK     bool   `json:"ownership_ok"`
	LastError       string `json:"last_error,omitempty"`
}

func DefaultCodexHome() (string, error) {
	if v := strings.TrimSpace(os.Getenv("CODEX_HOME")); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func Paths(codexHome string) (configPath, authPath, snapshotPath string, err error) {
	if codexHome == "" {
		codexHome, err = DefaultCodexHome()
		if err != nil {
			return "", "", "", err
		}
	}
	return filepath.Join(codexHome, "config.toml"),
		filepath.Join(codexHome, "auth.json"),
		filepath.Join(codexHome, "config.toml.sub2api-tray.bak"),
		nil
}

func Inspect(codexHome string) (Status, error) {
	var st Status
	configPath, authPath, snapshotPath, err := Paths(codexHome)
	if err != nil {
		return st, err
	}
	st.CodexHome = filepath.Dir(configPath)
	st.ConfigPath = configPath
	st.AuthPath = authPath
	if _, err := os.Stat(configPath); err == nil {
		st.ConfigExists = true
	}
	if _, err := os.Stat(snapshotPath); err == nil {
		st.SnapshotExists = true
	}
	if data, err := os.ReadFile(configPath); err == nil {
		content := string(data)
		st.ManagedPresent = hasManagedBlock(content)
		st.OwnershipOK = !st.ManagedPresent || ownershipOK(content)
		st.ModelProvider = rootString(content, "model_provider")
		st.Model = rootString(content, "model")
		st.BaseURL = providerBaseURL(content, st.ModelProvider)
	}
	if data, err := os.ReadFile(authPath); err == nil {
		var auth map[string]any
		if json.Unmarshal(data, &auth) == nil {
			if v, ok := auth["OPENAI_API_KEY"].(string); ok && strings.TrimSpace(v) != "" {
				st.HasAPIKey = true
			}
		}
	}
	return st, nil
}

func Apply(in ApplyInput) (Status, error) {
	if strings.TrimSpace(in.BaseURL) == "" {
		return Status{}, errors.New("base_url is required")
	}
	if strings.TrimSpace(in.APIKey) == "" {
		return Status{}, errors.New("api_key is required")
	}
	providerID := strings.TrimSpace(in.ProviderID)
	if providerID == "" {
		providerID = ProviderID
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = "gpt-5.6-sol"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")

	configPath, authPath, snapshotPath, err := Paths(in.CodexHome)
	if err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return Status{}, err
	}

	existing := ""
	if data, err := os.ReadFile(configPath); err == nil {
		existing = string(data)
		if hasManagedBlock(existing) && !ownershipOK(existing) {
			return Status{}, errors.New("managed config block was modified; refusing to overwrite")
		}
		if err := os.WriteFile(snapshotPath, data, 0o600); err != nil {
			return Status{}, fmt.Errorf("snapshot config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return Status{}, err
	}

	next := upsertManagedConfig(existing, providerID, baseURL, model)
	if err := os.WriteFile(configPath, []byte(next), 0o600); err != nil {
		return Status{}, err
	}
	auth := map[string]string{"OPENAI_API_KEY": strings.TrimSpace(in.APIKey)}
	authJSON, _ := json.MarshalIndent(auth, "", "  ")
	authJSON = append(authJSON, '\n')
	if err := os.WriteFile(authPath, authJSON, 0o600); err != nil {
		return Status{}, err
	}
	return Inspect(in.CodexHome)
}

func Repair(codexHome string, fallback ApplyInput) (Status, error) {
	configPath, _, snapshotPath, err := Paths(codexHome)
	if err != nil {
		return Status{}, err
	}
	st, _ := Inspect(codexHome)
	if st.ManagedPresent && st.OwnershipOK && st.HasAPIKey {
		return st, nil
	}
	// Prefer re-applying known settings when provided.
	if strings.TrimSpace(fallback.BaseURL) != "" && strings.TrimSpace(fallback.APIKey) != "" {
		return Apply(fallback)
	}
	// Otherwise restore last snapshot if present.
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		return st, errors.New("nothing to repair: provide setup values or keep a snapshot")
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		return st, err
	}
	return Inspect(codexHome)
}

func managedBlock(providerID, baseURL, model string) string {
	name := providerID
	return strings.Join([]string{
		startMark,
		fmt.Sprintf("# written %s", time.Now().UTC().Format(time.RFC3339)),
		fmt.Sprintf("model_provider = %q", providerID),
		fmt.Sprintf("model = %q", model),
		fmt.Sprintf("review_model = %q", model),
		`disable_response_storage = true`,
		``,
		fmt.Sprintf("[model_providers.%s]", providerID),
		fmt.Sprintf("name = %q", name),
		fmt.Sprintf("base_url = %q", baseURL),
		`wire_api = "responses"`,
		`requires_openai_auth = false`,
		endMark,
	}, "\n")
}

func upsertManagedConfig(existing, providerID, baseURL, model string) string {
	block := managedBlock(providerID, baseURL, model)
	if !hasManagedBlock(existing) {
		existing = stripRootProviderKeys(existing)
		existing = stripProviderSection(existing, providerID)
		return joinConfig(existing, block)
	}
	re := regexp.MustCompile(`(?s)# BEGIN sub2api-tray managed.*?# END sub2api-tray managed`)
	return re.ReplaceAllString(existing, block)
}

func hasManagedBlock(content string) bool {
	return strings.Contains(content, startMark) && strings.Contains(content, endMark)
}

func ownershipOK(content string) bool {
	start := strings.Index(content, startMark)
	end := strings.Index(content, endMark)
	if start < 0 || end < 0 || end < start {
		return false
	}
	segment := content[start : end+len(endMark)]
	return strings.Contains(segment, "[model_providers.") && strings.Contains(segment, `wire_api = "responses"`)
}

func joinConfig(existing, block string) string {
	existing = strings.TrimRight(existing, "\n")
	if existing == "" {
		return block + "\n"
	}
	return existing + "\n\n" + block + "\n"
}

func stripRootProviderKeys(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "model_provider ") || strings.HasPrefix(trim, "model_provider=") {
			continue
		}
		if strings.HasPrefix(trim, "model ") || strings.HasPrefix(trim, "model=") {
			// Keep non-root? Root model key is bare `model =`.
			if !strings.Contains(trim, ".") {
				continue
			}
		}
		if strings.HasPrefix(trim, "review_model ") || strings.HasPrefix(trim, "review_model=") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func stripProviderSection(content, providerID string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	header := fmt.Sprintf("[model_providers.%s]", providerID)
	skipping := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			skipping = trim == header
			if skipping {
				continue
			}
		}
		if skipping {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func rootString(content, key string) string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `\s*=\s*("([^"]*)"|'([^']*)')\s*$`)
	m := re.FindStringSubmatch(content)
	if len(m) >= 4 {
		if m[2] != "" {
			return m[2]
		}
		return m[3]
	}
	return ""
}

func providerBaseURL(content, providerID string) string {
	if providerID == "" {
		return ""
	}
	header := fmt.Sprintf("[model_providers.%s]", providerID)
	idx := strings.Index(content, header)
	if idx < 0 {
		return ""
	}
	rest := content[idx+len(header):]
	if next := strings.Index(rest, "\n["); next >= 0 {
		rest = rest[:next]
	}
	return rootString(rest, "base_url")
}
