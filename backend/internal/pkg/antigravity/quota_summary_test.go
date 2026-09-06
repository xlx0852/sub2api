package antigravity

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRetrieveUserQuotaSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:retrieveUserQuotaSummary" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization header not set")
		}
		if r.Header.Get("User-Agent") != antigravityQuotaUserAgent {
			t.Fatalf("user-agent = %s", r.Header.Get("User-Agent"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"project":"project-1"`) {
			t.Fatalf("body = %s", body)
		}
		w.Header().Set("Date", "Sun, 06 Sep 2026 09:00:00 GMT")
		_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"five-hour","window":"5h","remainingFraction":0.75,"resetTime":"2026-09-06T14:00:00Z"}]}]}`))
	}))
	defer server.Close()

	original := antigravityQuotaSummaryBaseURLs
	antigravityQuotaSummaryBaseURLs = []string{server.URL}
	defer func() { antigravityQuotaSummaryBaseURLs = original }()
	client := &Client{httpClient: server.Client()}
	result, raw, err := client.RetrieveUserQuotaSummary(context.Background(), "token", "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 || len(result.Groups[0].Buckets) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Groups[0].Buckets[0].RemainingFraction == nil || *result.Groups[0].Buckets[0].RemainingFraction != 0.75 {
		t.Fatalf("remaining fraction = %v", result.Groups[0].Buckets[0].RemainingFraction)
	}
	if result.ServerTime == nil || raw == nil {
		t.Fatalf("missing server time or raw payload")
	}
}
