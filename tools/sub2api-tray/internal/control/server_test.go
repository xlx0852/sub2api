package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/tools/sub2api-tray/internal/state"
)

func TestControlSetupAndAuth(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	codexHome := filepath.Join(dir, "codex")
	srv, err := Start(Options{Store: store, CodexHome: codexHome, Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	// unauthorized
	res, err := http.Get("http://" + srv.Addr() + "/health")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", res.StatusCode)
	}

	body := map[string]string{
		"base_url": "https://example.test/v1",
		"api_key":  "sk-demo",
		"model":    "gpt-5.5",
	}
	raw, _ := json.Marshal(body)
	res, err = http.Post(srv.BaseURL()+"/setup", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("setup status=%d body=%s", res.StatusCode, payload)
	}
	res, err = http.Get(srv.BaseURL() + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status code=%d", res.StatusCode)
	}
}

func TestControlLoginPassword(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"access_token": "atok",
				"user":         map[string]any{"id": 3, "email": "u@example.com"},
			},
		})
	})
	mux.HandleFunc("/api/v1/keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"id": 11, "key": "sk-from-login", "name": "sub2api-tray"},
		})
	})
	upstream := httptest.NewServer(mux)
	t.Cleanup(upstream.Close)

	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	srv, err := Start(Options{Store: store, CodexHome: filepath.Join(dir, "codex"), Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	body := map[string]string{
		"site_url": upstream.URL,
		"email":    "u@example.com",
		"password": "secret",
		"model":    "gpt-5.6-sol",
	}
	raw, _ := json.Marshal(body)
	res, err := http.Post(srv.BaseURL()+"/login", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("login status=%d body=%s", res.StatusCode, payload)
	}
	var out map[string]any
	_ = json.Unmarshal(payload, &out)
	if out["ok"] != true || out["requires_2fa"] == true {
		t.Fatalf("%s", payload)
	}
	st := store.Settings()
	if st.APIKey != "sk-from-login" || st.AuthMode != "password" || st.Email != "u@example.com" {
		t.Fatalf("%+v", st)
	}
	if st.BaseURL != upstream.URL+"/v1" {
		t.Fatalf("base %s", st.BaseURL)
	}
}
