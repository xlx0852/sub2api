package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	coderws "github.com/coder/websocket"
)

func main() {
	token := strings.TrimSpace(os.Getenv("GROK_ACCESS_TOKEN"))
	if token == "" {
		fmt.Println("GROK_ACCESS_TOKEN is required")
		os.Exit(1)
	}

	targets := []struct {
		name string
		url  string
	}{
		{name: "api.x.ai", url: "wss://api.x.ai/v1/responses"},
		{name: "cli-chat-proxy", url: "wss://cli-chat-proxy.grok.com/v1/responses"},
	}

	profiles := []struct {
		name    string
		headers func() http.Header
	}{
		{
			name: "bearer_only",
			headers: func() http.Header {
				h := make(http.Header)
				h.Set("Authorization", "Bearer "+token)
				return h
			},
		},
		{
			name: "grok_cli_headers",
			headers: func() http.Header {
				h := make(http.Header)
				h.Set("Authorization", "Bearer "+token)
				xai.SetGrokCLIRequestHeaders(h, "grok-composer-2.5-fast")
				h.Set("x-grok-conv-id", "probe-session-1")
				return h
			},
		},
		{
			name: "codex_like_headers",
			headers: func() http.Header {
				h := make(http.Header)
				h.Set("Authorization", "Bearer "+token)
				xai.SetGrokCLIRequestHeaders(h, "grok-composer-2.5-fast")
				h.Set("x-grok-conv-id", "probe-session-1")
				h.Set("User-Agent", "codex-cli/0.142.5")
				h.Set("originator", "codex_cli")
				return h
			},
		},
	}

	for _, target := range targets {
		for _, profile := range profiles {
			status, err := dial(target.url, profile.headers())
			fmt.Printf("%s | %s | status=%d | err=%v\n", target.name, profile.name, status, errString(err))
		}
	}
}

func dial(wsURL string, headers http.Header) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, resp, err := coderws.Dial(ctx, wsURL, &coderws.DialOptions{
		HTTPHeader:      headers,
		CompressionMode: coderws.CompressionContextTakeover,
	})
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	if conn != nil {
		_ = conn.Close(coderws.StatusNormalClosure, "probe")
		if status == 0 {
			status = http.StatusSwitchingProtocols
		}
		return status, nil
	}
	return status, err
}

func errString(err error) string {
	if err == nil {
		return "-"
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 160 {
		return msg[:160] + "..."
	}
	return msg
}