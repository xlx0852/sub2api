package repository

import (
	"os"
	"strings"
	"testing"
)

// TestAccountLoadersPreloadTheProxyEdge guards a silently broken proxy binding.
//
// The forwarding, account-test and quota paths all decide with
// `account.ProxyID != nil && account.Proxy != nil`. Ent only fills Edges.Proxy when the query
// asks for it, so a single-account loader without WithProxy() yields account.Proxy == nil and
// the request goes direct — the bound proxy looks configured but is never used. This was live:
// a rotating residential proxy bound to an account was ignored by the account test until
// GetByID preloaded the edge.
func TestAccountLoadersPreloadTheProxyEdge(t *testing.T) {
	source, err := os.ReadFile("account_repo.go")
	if err != nil {
		t.Fatalf("read repository source: %v", err)
	}
	body := string(source)

	// Every loader whose result can be forwarded must preload the proxy edge.
	loaders := []string{"GetByID", "GetByIDs", "GetByCRSAccountID"}
	for _, loader := range loaders {
		block := funcBody(t, body, "func (r *accountRepository) "+loader+"(")
		if !strings.Contains(block, "WithProxy()") {
			t.Errorf("%s does not preload the proxy edge; bound proxies would be ignored", loader)
		}
	}
}

// funcBody returns the source of one function, from its signature to the closing brace.
func funcBody(t *testing.T, source, signature string) string {
	t.Helper()
	start := strings.Index(source, signature)
	if start < 0 {
		t.Fatalf("function %q not found in repository source", signature)
	}
	depth := 0
	seen := false
	for index := start; index < len(source); index++ {
		switch source[index] {
		case '{':
			depth++
			seen = true
		case '}':
			depth--
			if seen && depth == 0 {
				return source[start : index+1]
			}
		}
	}
	t.Fatalf("function %q is not terminated", signature)
	return ""
}
