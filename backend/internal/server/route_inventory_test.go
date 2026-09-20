package server

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// updateRouteGolden 通过 `go test ./internal/server -run TestRouteInventoryGolden -update`
// 重新生成路由清单 golden 文件。
var updateRouteGolden = flag.Bool("update", false, "重写路由清单 golden 文件")

const routeGoldenPath = "testdata/routes.golden"

// TestRouteInventoryGolden 是路由清单回归护栏（OpenSpec change: refactor-architecture,
// P0 边界冻结）：快照当前注册的全部路由（method + path），任何路由的增删改都会使
// 本测试失败，防止后续结构性搬移（P1+）无意间丢失或改变对外端点。
//
// 实现说明：直接调用 registerRoutes（与生产 SetupRouter 相同的注册入口），
// handler/service 依赖以零值/nil 打桩——路由注册期只创建闭包与方法值，不会调用它们，
// 因此清单与生产完全一致，且不依赖数据库/Redis。
func TestRouteInventoryGolden(t *testing.T) {
	gin.SetMode(gin.TestMode)

	actual := strings.Join(enumerateRoutes(t), "\n") + "\n"

	if *updateRouteGolden {
		if err := os.MkdirAll(filepath.Dir(routeGoldenPath), 0o755); err != nil {
			t.Fatalf("创建 testdata 目录失败: %v", err)
		}
		if err := os.WriteFile(routeGoldenPath, []byte(actual), 0o644); err != nil {
			t.Fatalf("写入 golden 文件失败: %v", err)
		}
		t.Logf("已更新 %s", routeGoldenPath)
		return
	}

	goldenBytes, err := os.ReadFile(routeGoldenPath)
	if err != nil {
		t.Fatalf("读取 golden 文件失败: %v\n如首次生成，请运行: go test ./internal/server -run TestRouteInventoryGolden -update", err)
	}
	golden := string(goldenBytes)

	if actual == golden {
		return
	}

	added, removed := diffLines(golden, actual)
	var b strings.Builder
	fmt.Fprint(&b, "注册路由清单与 golden 快照不一致。\n")
	if len(added) > 0 {
		fmt.Fprintf(&b, "新增路由（%d 条）：\n", len(added))
		for _, l := range added {
			fmt.Fprintf(&b, "  + %s\n", l)
		}
	}
	if len(removed) > 0 {
		fmt.Fprintf(&b, "消失路由（%d 条）：\n", len(removed))
		for _, l := range removed {
			fmt.Fprintf(&b, "  - %s\n", l)
		}
	}
	fmt.Fprint(&b, "若该变更是有意的（新增/下线端点），请核对后运行以下命令更新快照并随代码一并提交：\n")
	fmt.Fprint(&b, "  go test ./internal/server -run TestRouteInventoryGolden -update\n")
	fmt.Fprint(&b, "若并非有意变更，说明路由注册被意外改动（P0 边界冻结期间尤其注意结构性搬移导致的端点丢失）。")
	t.Fatal(b.String())
}

// TestRouteInventoryCoversGatewayEndpoints 确认护栏覆盖网关端点组（/v1/* 等核心协议端点），
// 防止 golden 被盲目 -update 后核心端点悄然消失。
func TestRouteInventoryCoversGatewayEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	routes := make(map[string]bool)
	for _, line := range enumerateRoutes(t) {
		routes[line] = true
	}

	canaries := []string{
		"POST /v1/messages",
		"POST /v1/messages/count_tokens",
		"POST /v1/chat/completions",
		"POST /v1/responses",
		"GET /v1/responses",
		"GET /v1/models",
		"POST /responses",
		"POST /chat/completions",
		"POST /backend-api/codex/responses",
		"POST /antigravity/v1/messages",
		"GET /antigravity/v1beta/models",
		"GET /health",
	}
	for _, canary := range canaries {
		if !routes[canary] {
			t.Errorf("核心网关端点未注册: %s", canary)
		}
	}
}

// enumerateRoutes 以最小打桩实例化全部路由注册，返回排序后的 "METHOD PATH" 清单。
func enumerateRoutes(t *testing.T) []string {
	t.Helper()

	r := gin.New()
	noop := gin.HandlerFunc(func(c *gin.Context) {})

	// Handlers 字段为 nil 指针即可：注册期仅取方法值（h.Xxx.Method），不会调用。
	// Admin 为嵌套指针结构，需实例化外层以便取内层字段。
	h := &handler.Handlers{Admin: &handler.AdminHandlers{}}

	cfg := &config.Config{}
	// RegisterPageRoutes 会在注册期对 DataDir 执行 MkdirAll，指向临时目录。
	cfg.Pricing.DataDir = t.TempDir()

	registerRoutes(
		r,
		h,
		middleware2.JWTAuthMiddleware(noop),
		middleware2.AdminAuthMiddleware(noop),
		middleware2.APIKeyAuthMiddleware(noop),
		nil, // apiKeyService：仅被请求期闭包捕获
		nil, // opsService：同上
		nil, // settingService：同上
		cfg,
		nil, // redisClient：注册期仅被 NewRateLimiter 持有引用
	)

	lines := make([]string, 0, len(r.Routes()))
	for _, route := range r.Routes() {
		lines = append(lines, route.Method+" "+route.Path)
	}
	sort.Strings(lines)
	return lines
}

// diffLines 返回 actual 相对 golden 的（新增行, 消失行），两者均已排序。
func diffLines(golden, actual string) (added, removed []string) {
	goldenSet := make(map[string]bool)
	for _, l := range strings.Split(strings.TrimSpace(golden), "\n") {
		if l != "" {
			goldenSet[l] = true
		}
	}
	actualSet := make(map[string]bool)
	for _, l := range strings.Split(strings.TrimSpace(actual), "\n") {
		if l != "" {
			actualSet[l] = true
		}
	}
	for l := range actualSet {
		if !goldenSet[l] {
			added = append(added, l)
		}
	}
	for l := range goldenSet {
		if !actualSet[l] {
			removed = append(removed, l)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}
