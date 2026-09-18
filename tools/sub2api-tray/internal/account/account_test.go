package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSiteAndGatewayURL(t *testing.T) {
	cases := []struct {
		in, site, gw string
	}{
		{"https://code.example", "https://code.example", "https://code.example/v1"},
		{"https://code.example/", "https://code.example", "https://code.example/v1"},
		{"https://code.example/v1", "https://code.example", "https://code.example/v1"},
		{"https://code.example/v1/", "https://code.example", "https://code.example/v1"},
		{"code.example", "https://code.example", "https://code.example/v1"},
	}
	for _, c := range cases {
		site, err := SiteURL(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if site != c.site {
			t.Fatalf("site %s: got %s want %s", c.in, site, c.site)
		}
		gw, err := GatewayBaseURL(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if gw != c.gw {
			t.Fatalf("gw %s: got %s want %s", c.in, gw, c.gw)
		}
	}
}

func TestLoginCreatesKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "success",
			"data": map[string]any{
				"access_token": "tok-1",
				"token_type":   "Bearer",
				"user":         map[string]any{"id": 7, "email": "a@b.com"},
			},
		})
	})
	mux.HandleFunc("/api/v1/keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "data": []any{},
			})
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			http.Error(w, "unauthorized", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"id": 9, "key": "sk-tray-1", "name": "sub2api-tray"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	out, err := c.LoginWithPassword(context.Background(), LoginInput{
		SiteURL:  srv.URL,
		Email:    "a@b.com",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Pending2FA != nil {
		t.Fatal("unexpected 2fa")
	}
	if out.Result.APIKey != "sk-tray-1" || !out.Result.CreatedKey {
		t.Fatalf("%+v", out.Result)
	}
	if out.Result.GatewayBase != srv.URL+"/v1" {
		t.Fatalf("gateway %s", out.Result.GatewayBase)
	}
}

func TestLogin2FAThenCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"requires_2fa": true,
				"temp_token":   "tmp-1",
			},
		})
	})
	mux.HandleFunc("/api/v1/auth/login/2fa", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"access_token": "tok-2",
				"user":         map[string]any{"id": 1, "email": "a@b.com"},
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
			"data": map[string]any{"id": 3, "key": "sk-2", "name": "sub2api-tray"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}

	out, err := c.LoginWithPassword(context.Background(), LoginInput{
		SiteURL: srv.URL, Email: "a@b.com", Password: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Pending2FA == nil || out.Pending2FA.TempToken != "tmp-1" {
		t.Fatalf("%+v", out.Pending2FA)
	}
	res, err := c.Complete2FA(context.Background(), srv.URL, out.Pending2FA.TempToken, "123456", "a@b.com", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.APIKey != "sk-2" {
		t.Fatalf("%+v", res)
	}
}

func TestReuseExistingKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"access_token": "tok", "user": map[string]any{"id": 1, "email": "a@b.com"}},
		})
	})
	mux.HandleFunc("/api/v1/keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("should not create")
		}
		// Real control-plane shape: data.items
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"items": []any{
					map[string]any{"id": 2, "name": "sub2api-tray", "key": "sk-old"},
				},
				"total": 1, "page": 1, "page_size": 100,
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	out, err := c.LoginWithPassword(context.Background(), LoginInput{
		SiteURL: srv.URL, Email: "a@b.com", Password: "x", PreferExisting: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Result.ReusedKey || out.Result.APIKey != "sk-old" {
		t.Fatalf("%+v", out.Result)
	}
}
