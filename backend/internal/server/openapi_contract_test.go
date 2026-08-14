package server

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// controlPlaneRoutePrefixes is the 7.1 OpenAPI coverage set (user / auth / payment).
// Admin and gateway protocol routes are intentionally excluded.
var controlPlaneRoutePrefixes = []string{
	"/api/v1/auth",
	"/api/v1/user",
	"/api/v1/keys",
	"/api/v1/groups",
	"/api/v1/channels",
	"/api/v1/usage",
	"/api/v1/announcements",
	"/api/v1/redeem",
	"/api/v1/payment",
	"/api/v1/settings",
	"/api/v1/public",
	"/api/v1/model-catalog",
}

var openAPIPathParam = regexp.MustCompile(`\{([^{}/]+)\}`)

var openAPIHTTPMethods = map[string]bool{
	"get": true, "post": true, "put": true, "delete": true,
	"patch": true, "head": true, "options": true, "trace": true,
}

// TestControlPlaneOpenAPIMatchesRoutes fails when a user/auth/payment route is
// added or removed without updating openapi/control-plane.yaml.
func TestControlPlaneOpenAPIMatchesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	implemented := map[string]bool{}
	for _, line := range enumerateRoutes(t) {
		method, path, ok := splitMethodPath(line)
		if !ok || !isControlPlaneRoute(path) {
			continue
		}
		implemented[method+" "+path] = true
	}

	specified := loadControlPlaneSpecRoutes(t)

	var missing, extra []string
	for route := range implemented {
		if !specified[route] {
			missing = append(missing, route)
		}
	}
	for route := range specified {
		if !implemented[route] {
			extra = append(extra, route)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) == 0 && len(extra) == 0 {
		if len(implemented) == 0 {
			t.Fatal("control-plane route set is empty; enumerateRoutes or prefix filter is broken")
		}
		return
	}

	var b strings.Builder
	b.WriteString("openapi/control-plane.yaml is out of sync with registered user/auth/payment routes.\n")
	if len(missing) > 0 {
		b.WriteString("Implemented but missing from spec:\n")
		for _, line := range missing {
			b.WriteString("  + " + line + "\n")
		}
	}
	if len(extra) > 0 {
		b.WriteString("In spec but not registered:\n")
		for _, line := range extra {
			b.WriteString("  - " + line + "\n")
		}
	}
	b.WriteString("Update openapi/control-plane.yaml (path + method + operationId) to match the route change.")
	t.Fatal(b.String())
}

func isControlPlaneRoute(path string) bool {
	if strings.HasPrefix(path, "/api/v1/admin/") {
		return false
	}
	for _, prefix := range controlPlaneRoutePrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func loadControlPlaneSpecRoutes(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(controlPlaneSpecPath(t))
	if err != nil {
		t.Fatalf("read OpenAPI spec: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse OpenAPI spec: %v", err)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("OpenAPI spec has no paths")
	}

	out := make(map[string]bool)
	for oaPath, ops := range doc.Paths {
		ginPath := openAPIPathParam.ReplaceAllString(oaPath, ":$1")
		for method := range ops {
			if !openAPIHTTPMethods[strings.ToLower(method)] {
				continue
			}
			out[strings.ToUpper(method)+" "+ginPath] = true
		}
	}
	return out
}

func controlPlaneSpecPath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("wd: %v", err)
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "openapi", "control-plane.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("openapi/control-plane.yaml not found from working directory")
	return ""
}

func splitMethodPath(line string) (method, path string, ok bool) {
	method, path, ok = strings.Cut(line, " ")
	return method, path, ok && method != "" && path != ""
}
