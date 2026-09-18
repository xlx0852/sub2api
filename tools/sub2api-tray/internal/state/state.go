package state

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	DefaultProviderID = "sub2api"
	DefaultModel      = "gpt-5.6-sol"
)

type Settings struct {
	BaseURL    string   `json:"base_url"`
	APIKey     string   `json:"api_key"`
	ProviderID string   `json:"provider_id"`
	Model      string   `json:"model"`
	Models     []string `json:"models,omitempty"`
	UpdatedAt  string   `json:"updated_at,omitempty"`

	// Account login metadata (password is never persisted).
	SiteURL    string `json:"site_url,omitempty"`
	Email      string `json:"email,omitempty"`
	UserID     int64  `json:"user_id,omitempty"`
	APIKeyID   int64  `json:"api_key_id,omitempty"`
	APIKeyName string `json:"api_key_name,omitempty"`
	AuthMode   string `json:"auth_mode,omitempty"` // "password" | "api_key"
}

type Store struct {
	mu       sync.RWMutex
	dir      string
	token    string
	settings Settings
}

func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sub2api-tray"), nil
}

func Open(dir string) (*Store, error) {
	if dir == "" {
		var err error
		dir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir}
	if err := s.loadOrInitToken(); err != nil {
		return nil, err
	}
	_ = s.loadSettings()
	return s, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) Token() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.settings
	out.Models = append([]string(nil), s.settings.Models...)
	return out
}

func (s *Store) SaveSettings(next Settings) error {
	if next.ProviderID == "" {
		next.ProviderID = DefaultProviderID
	}
	if next.Model == "" {
		next.Model = DefaultModel
	}
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = next
	return s.writeSettingsLocked()
}

func (s *Store) SetModel(model string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if model == "" {
		return errors.New("model is required")
	}
	s.settings.Model = model
	s.settings.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.writeSettingsLocked()
}

func (s *Store) tokenPath() string    { return filepath.Join(s.dir, "capability") }
func (s *Store) settingsPath() string { return filepath.Join(s.dir, "settings.json") }

func (s *Store) loadOrInitToken() error {
	path := s.tokenPath()
	data, err := os.ReadFile(path)
	if err == nil {
		tok := string(bytesTrimSpace(data))
		if tok != "" {
			s.token = tok
			return nil
		}
	}
	tok, err := randomToken(24)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(tok+"\n"), 0o600); err != nil {
		return err
	}
	s.token = tok
	return nil
}

func (s *Store) loadSettings() error {
	data, err := os.ReadFile(s.settingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			s.settings = Settings{
				ProviderID: DefaultProviderID,
				Model:      DefaultModel,
				Models:     []string{DefaultModel, "gpt-5.5", "gpt-5.4"},
			}
			return nil
		}
		return err
	}
	var st Settings
	if err := json.Unmarshal(data, &st); err != nil {
		return err
	}
	if st.ProviderID == "" {
		st.ProviderID = DefaultProviderID
	}
	if st.Model == "" {
		st.Model = DefaultModel
	}
	if len(st.Models) == 0 {
		st.Models = []string{st.Model}
	}
	s.settings = st
	return nil
}

func (s *Store) writeSettingsLocked() error {
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.settingsPath(), data, 0o600)
}

func randomToken(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func bytesTrimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end {
		c := b[start]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' {
			break
		}
		start++
	}
	for end > start {
		c := b[end-1]
		if c != ' ' && c != '\n' && c != '\r' && c != '\t' {
			break
		}
		end--
	}
	return b[start:end]
}
