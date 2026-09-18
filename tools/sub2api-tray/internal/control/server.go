package control

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/account"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/autostart"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/codexcfg"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/panel"
	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/state"
)

const DefaultAddr = "127.0.0.1:8787"

type Server struct {
	store      *state.Store
	httpServer *http.Server
	listener   net.Listener
	codexHome  string
	listenAddr string
	autoArgs   []string
}

type Options struct {
	Store     *state.Store
	CodexHome string
	Addr      string // default 127.0.0.1:8787
	// AutoArgs are CLI args used when enabling login autostart (without binary path).
	AutoArgs []string
}

func Start(opts Options) (*Server, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	addr := opts.Addr
	if addr == "" {
		addr = DefaultAddr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Fall back to an ephemeral port when the preferred port is busy.
		if addr != "127.0.0.1:0" {
			ln, err = net.Listen("tcp", "127.0.0.1:0")
		}
		if err != nil {
			return nil, err
		}
	}
	s := &Server{
		store:      opts.Store,
		listener:   ln,
		codexHome:  opts.CodexHome,
		listenAddr: ln.Addr().String(),
		autoArgs:   append([]string(nil), opts.AutoArgs...),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	s.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = s.httpServer.Serve(ln) }()
	_ = s.WritePanelURLFile()
	return s, nil
}

// WritePanelURLFile stores the panel URL under the state dir for easy discovery.
func (s *Server) WritePanelURLFile() error {
	if s == nil || s.store == nil {
		return nil
	}
	path := filepath.Join(s.store.Dir(), "panel.url")
	return os.WriteFile(path, []byte(s.PanelURL()+"\n"), 0o600)
}

func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

func (s *Server) BaseURL() string {
	return fmt.Sprintf("http://%s/c/%s", s.Addr(), s.store.Token())
}

func (s *Server) PanelURL() string {
	return s.BaseURL() + "/panel"
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	token, rest, ok := stripCapability(path, s.store.Token())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	_ = token
	switch {
	case r.Method == http.MethodGet && (rest == "" || rest == "/"):
		http.Redirect(w, r, s.PanelURL(), http.StatusFound)
	case r.Method == http.MethodGet && (rest == "/panel" || rest == "/panel/"):
		s.servePanel(w)
	case r.Method == http.MethodGet && rest == "/health":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case r.Method == http.MethodGet && rest == "/status":
		s.handleStatus(w)
	case r.Method == http.MethodPost && rest == "/setup":
		s.handleSetup(w, r)
	case r.Method == http.MethodPost && rest == "/login":
		s.handleLogin(w, r)
	case r.Method == http.MethodPost && rest == "/login/2fa":
		s.handleLogin2FA(w, r)
	case r.Method == http.MethodPost && rest == "/model-set":
		s.handleModelSet(w, r)
	case r.Method == http.MethodPost && rest == "/repair":
		s.handleRepair(w, r)
	case r.Method == http.MethodGet && rest == "/models":
		s.handleModels(w)
	case r.Method == http.MethodPost && rest == "/test":
		s.handleTest(w, r)
	case r.Method == http.MethodGet && rest == "/autostart":
		s.handleAutostartGet(w)
	case r.Method == http.MethodPost && rest == "/autostart":
		s.handleAutostartSet(w, r)
	default:
		writeErr(w, http.StatusNotFound, "not_found")
	}
}

func (s *Server) servePanel(w http.ResponseWriter) {
	data, err := panel.FS.ReadFile("index.html")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *Server) handleStatus(w http.ResponseWriter) {
	settings := s.store.Settings()
	codex, err := codexcfg.Inspect(s.codexHome)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	autoOn, autoErr := autostart.Enabled()
	auto := map[string]any{"enabled": autoOn}
	if autoErr != nil {
		auto["error"] = autoErr.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"settings": settings,
		"codex":    codex,
		"panel":    s.PanelURL(),
		"listen":   s.Addr(),
		"autostart": auto,
	})
}

func (s *Server) handleAutostartGet(w http.ResponseWriter) {
	on, err := autostart.Enabled()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "enabled": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": on})
}

func (s *Server) handleAutostartSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Enabled == nil {
		writeErr(w, http.StatusBadRequest, "enabled is required")
		return
	}
	if *body.Enabled {
		exe, err := autostart.DefaultExecutable()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		args := s.autoArgs
		if len(args) == 0 {
			args = []string{"--addr", DefaultAddr}
		}
		if err := autostart.Enable(autostart.Spec{
			Executable: exe,
			Args:       args,
			StateDir:   s.store.Dir(),
		}); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		if err := autostart.Disable(); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	on, _ := autostart.Enabled()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": on})
}

type setupBody struct {
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

type loginBody struct {
	SiteURL        string `json:"site_url"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	TotpCode       string `json:"totp_code"`
	TurnstileToken string `json:"turnstile_token"`
	Model          string `json:"model"`
	KeyName        string `json:"key_name"`
	// ApplyCodex defaults true; set false to only obtain/store key.
	ApplyCodex *bool `json:"apply_codex"`
}

type login2FABody struct {
	SiteURL   string `json:"site_url"`
	TempToken string `json:"temp_token"`
	TotpCode  string `json:"totp_code"`
	Email     string `json:"email"`
	Model     string `json:"model"`
	KeyName   string `json:"key_name"`
	ApplyCodex *bool `json:"apply_codex"`
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var body setupBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := codexcfg.Apply(codexcfg.ApplyInput{
		CodexHome:  s.codexHome,
		BaseURL:    body.BaseURL,
		APIKey:     body.APIKey,
		ProviderID: body.ProviderID,
		Model:      body.Model,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	settings := s.store.Settings()
	settings.BaseURL = strings.TrimRight(strings.TrimSpace(body.BaseURL), "/")
	settings.APIKey = strings.TrimSpace(body.APIKey)
	settings.AuthMode = "api_key"
	if body.ProviderID != "" {
		settings.ProviderID = body.ProviderID
	}
	if body.Model != "" {
		settings.Model = body.Model
	}
	if err := s.store.SaveSettings(settings); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "codex": st, "settings": s.store.Settings()})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cli := &account.Client{}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := cli.LoginWithPassword(ctx, account.LoginInput{
		SiteURL:        body.SiteURL,
		Email:          body.Email,
		Password:       body.Password,
		TotpCode:       body.TotpCode,
		TurnstileToken: body.TurnstileToken,
		KeyName:        body.KeyName,
		PreferExisting: true,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if out.Pending2FA != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":               true,
			"requires_2fa":     true,
			"temp_token":       out.Pending2FA.TempToken,
			"user_email_masked": out.Pending2FA.UserEmailMasked,
			"site_url":         out.Pending2FA.SiteURL,
		})
		return
	}
	s.finishAccountLogin(w, out.Result, body.Model, body.ApplyCodex)
}

func (s *Server) handleLogin2FA(w http.ResponseWriter, r *http.Request) {
	var body login2FABody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cli := &account.Client{}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	res, err := cli.Complete2FA(ctx, body.SiteURL, body.TempToken, body.TotpCode, body.Email, body.KeyName, true)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.finishAccountLogin(w, res, body.Model, body.ApplyCodex)
}

func (s *Server) finishAccountLogin(w http.ResponseWriter, res *account.LoginResult, model string, applyCodex *bool) {
	if res == nil || strings.TrimSpace(res.APIKey) == "" {
		writeErr(w, http.StatusBadRequest, "login did not yield api key")
		return
	}
	doApply := true
	if applyCodex != nil {
		doApply = *applyCodex
	}
	settings := s.store.Settings()
	settings.SiteURL = res.SiteURL
	settings.BaseURL = res.GatewayBase
	settings.APIKey = res.APIKey
	settings.Email = res.Email
	settings.UserID = res.UserID
	settings.APIKeyID = res.APIKeyID
	settings.APIKeyName = res.APIKeyName
	settings.AuthMode = "password"
	if strings.TrimSpace(model) != "" {
		settings.Model = strings.TrimSpace(model)
	}
	if settings.Model == "" {
		settings.Model = state.DefaultModel
	}
	if settings.ProviderID == "" {
		settings.ProviderID = state.DefaultProviderID
	}

	var codex any
	if doApply {
		st, err := codexcfg.Apply(codexcfg.ApplyInput{
			CodexHome:  s.codexHome,
			BaseURL:    settings.BaseURL,
			APIKey:     settings.APIKey,
			ProviderID: settings.ProviderID,
			Model:      settings.Model,
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		codex = st
	}
	if err := s.store.SaveSettings(settings); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Best-effort refresh model list
	if models, err := fetchModels(settings.BaseURL, settings.APIKey); err == nil && len(models) > 0 {
		settings.Models = models
		_ = s.store.SaveSettings(settings)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"requires_2fa": false,
		"created_key": res.CreatedKey,
		"reused_key":  res.ReusedKey,
		"login": map[string]any{
			"site_url":     res.SiteURL,
			"gateway_base": res.GatewayBase,
			"email":        res.Email,
			"api_key_id":   res.APIKeyID,
			"api_key_name": res.APIKeyName,
		},
		"codex":    codex,
		"settings": s.store.Settings(),
	})
}

func (s *Server) handleModelSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string `json:"model"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	settings := s.store.Settings()
	if settings.BaseURL == "" || settings.APIKey == "" {
		writeErr(w, http.StatusBadRequest, "run setup first")
		return
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		writeErr(w, http.StatusBadRequest, "model is required")
		return
	}
	st, err := codexcfg.Apply(codexcfg.ApplyInput{
		CodexHome:  s.codexHome,
		BaseURL:    settings.BaseURL,
		APIKey:     settings.APIKey,
		ProviderID: settings.ProviderID,
		Model:      model,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetModel(model); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "codex": st, "model": model})
}

func (s *Server) handleRepair(w http.ResponseWriter, r *http.Request) {
	settings := s.store.Settings()
	st, err := codexcfg.Repair(s.codexHome, codexcfg.ApplyInput{
		CodexHome:  s.codexHome,
		BaseURL:    settings.BaseURL,
		APIKey:     settings.APIKey,
		ProviderID: settings.ProviderID,
		Model:      settings.Model,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "codex": st})
}

func (s *Server) handleModels(w http.ResponseWriter) {
	settings := s.store.Settings()
	models := settings.Models
	if settings.BaseURL != "" && settings.APIKey != "" {
		if remote, err := fetchModels(settings.BaseURL, settings.APIKey); err == nil && len(remote) > 0 {
			models = remote
			settings.Models = remote
			_ = s.store.SaveSettings(settings)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": models})
}

func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	var body setupBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	baseURL := body.BaseURL
	apiKey := body.APIKey
	if baseURL == "" || apiKey == "" {
		st := s.store.Settings()
		if baseURL == "" {
			baseURL = st.BaseURL
		}
		if apiKey == "" {
			apiKey = st.APIKey
		}
	}
	if baseURL == "" || apiKey == "" {
		writeErr(w, http.StatusBadRequest, "base_url and api_key required")
		return
	}
	models, err := fetchModels(baseURL, apiKey)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "models": len(models), "sample": firstN(models, 5)})
}

func fetchModels(baseURL, apiKey string) ([]string, error) {
	endpoint, err := url.JoinPath(strings.TrimRight(baseURL, "/"), "models")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		id := strings.TrimSpace(m.ID)
		if id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

func stripCapability(path, want string) (token, rest string, ok bool) {
	path = strings.TrimPrefix(path, "/")
	if !strings.HasPrefix(path, "c/") {
		return "", "", false
	}
	path = strings.TrimPrefix(path, "c/")
	i := strings.IndexByte(path, '/')
	if i < 0 {
		token = path
		rest = "/"
	} else {
		token = path[:i]
		rest = path[i:]
	}
	if token == "" || token != want {
		return "", "", false
	}
	if rest == "" {
		rest = "/"
	}
	return token, rest, true
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("empty body")
	}
	return json.Unmarshal(data, dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func firstN(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return append([]string(nil), in[:n]...)
}
