package archtest

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestTransportPersistenceImportBaseline freezes the remaining transport/persistence
// leaks while the runtime-boundary migration is in progress. It intentionally checks
// direct package edges, not transitive imports: adding a new direct edge fails before
// merge, and removing an old edge requires deleting its baseline entry.
func TestTransportPersistenceImportBaseline(t *testing.T) {
	root := moduleRoot(t)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports,
		Dir:  root,
	}
	pkgs, err := packages.Load(cfg, modulePrefix+"internal/service", modulePrefix+"internal/handler")
	if err != nil {
		t.Fatalf("加载 transport/persistence 目标包失败: %v", err)
	}
	for _, pkg := range pkgs {
		for _, loadErr := range pkg.Errors {
			t.Errorf("包 %s 加载错误: %v", pkg.PkgPath, loadErr)
		}
	}
	if t.Failed() {
		t.Fatal("目标包加载存在错误，无法继续边界检查")
	}

	forbidden := map[string]map[string]struct{}{
		modulePrefix + "internal/service": {
			"github.com/gin-gonic/gin":                  {},
			"database/sql":                              {},
			modulePrefix + "ent":                        {},
			modulePrefix + "internal/handler":           {},
			modulePrefix + "internal/server":            {},
			modulePrefix + "internal/server/middleware": {},
		},
		modulePrefix + "internal/handler": {
			modulePrefix + "ent": {},
		},
	}
	actual := make(map[string]struct{})
	for _, pkg := range pkgs {
		for imported := range pkg.Imports {
			if _, ok := forbidden[pkg.PkgPath][imported]; !ok {
				continue
			}
			actual[shortPath(pkg.PkgPath)+" -> "+shortImportPath(imported)] = struct{}{}
		}
	}

	baselinePath := root + "/internal/archtest/baseline_transport_imports.txt"
	raw, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", baselinePath, err)
	}
	baseline := make(map[string]struct{})
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		baseline[line] = struct{}{}
	}

	for edge := range actual {
		if _, ok := baseline[edge]; !ok {
			t.Errorf("发现新增 transport/persistence 依赖：%s；请迁移到 ports/repository 或加入明确的过渡计划", edge)
		}
	}
	for edge := range baseline {
		if _, ok := actual[edge]; !ok {
			t.Errorf("依赖已消失：%s；请从 baseline_transport_imports.txt 移除", edge)
		}
	}
}

func shortImportPath(path string) string {
	if strings.HasPrefix(path, modulePrefix) {
		return shortPath(path)
	}
	return path
}
