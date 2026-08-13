package archtest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// modulePrefix 是本仓库 Go module 的 import 前缀，
// 报告与基线文件中的包路径均去掉该前缀（例如 internal/service）。
const modulePrefix = "github.com/Wei-Shaw/sub2api/"

// moduleRoot 从当前目录向上查找 go.mod，返回 backend 模块根目录。
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取工作目录失败: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("未在 %s 的上层目录中找到 go.mod", dir)
		}
		dir = parent
	}
}

// shortPath 去掉 module 前缀，便于阅读与基线比对。
func shortPath(pkgPath string) string {
	return strings.TrimPrefix(pkgPath, modulePrefix)
}

// hasPathPrefix 按路径段匹配前缀：prefix=internal/pkg 匹配 internal/pkg 与
// internal/pkg/...，但不匹配 internal/pkgfoo。
func hasPathPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// loadBaseline 读取基线文件，忽略空行与 # 注释，返回条目集合。
func loadBaseline(t *testing.T, filename string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("读取基线文件 %s 失败: %v", filename, err)
	}

	entries := make(map[string]bool)
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entries[line] = true
	}
	return entries
}

// diffAgainstBaseline 将实际违例集合与基线比对。
// 返回（基线外新增violations, 已不存在的过期基线条目），两者均排序。
func diffAgainstBaseline(actual map[string]string, baseline map[string]bool) (newViolations []string, staleEntries []string) {
	for key := range actual {
		if !baseline[key] {
			newViolations = append(newViolations, key)
		}
	}
	for key := range baseline {
		if _, ok := actual[key]; !ok {
			staleEntries = append(staleEntries, key)
		}
	}
	sort.Strings(newViolations)
	sort.Strings(staleEntries)
	return newViolations, staleEntries
}

// reportBaselineDiff 输出统一格式的失败信息。
// actual 的 value 为该违例的附加说明（规则名/位置等），仅用于报错展示。
func reportBaselineDiff(t *testing.T, baselineFile string, actual map[string]string, baseline map[string]bool, newHint, staleHint string) {
	t.Helper()
	newViolations, staleEntries := diffAgainstBaseline(actual, baseline)

	if len(newViolations) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "发现 %d 条基线外的新增违例：\n", len(newViolations))
		for _, v := range newViolations {
			fmt.Fprintf(&b, "  %s\n", v)
			if detail := actual[v]; detail != "" {
				fmt.Fprintf(&b, "      %s\n", detail)
			}
		}
		fmt.Fprint(&b, newHint)
		t.Error(b.String())
	}

	if len(staleEntries) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "以下 %d 条基线条目已不再违例（架构在收敛，很好）：\n", len(staleEntries))
		for _, v := range staleEntries {
			fmt.Fprintf(&b, "  %s\n", v)
		}
		fmt.Fprintf(&b, "请将上述条目从 %s 移除，收紧基线。%s", baselineFile, staleHint)
		t.Error(b.String())
	}
}
