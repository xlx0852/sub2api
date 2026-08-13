package archtest

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// importRule 描述一条依赖方向规则。
// appliesTo 命中的包，不得 import 命中 forbidden 且未命中 allowed 的 internal 包。
// 前缀均为去掉 module 前缀后的路径，按路径段匹配（见 hasPathPrefix）。
type importRule struct {
	name      string
	desc      string
	appliesTo []string
	forbidden []string
	allowed   []string
}

// importRules 编码 design.md D1 的目标分层规则与当前已成立的依赖不变量。
//
// 说明：
//   - 目标包（internal/ports、internal/app、internal/infra、internal/transport）
//     目前尚不存在，对应规则自动对未来生效；
//   - 现存违例（repository→service 拆包死锁、domain→pkg）记录在
//     baseline_imports.txt 中，禁止新增、只允许收敛。
var importRules = []importRule{
	// ---- 目标分层（design.md D1），对未来包预先生效 ----
	{
		name:      "domain-zero-deps",
		desc:      "domain 层不得 import 任何 internal 包（领域类型/常量/错误零依赖）",
		appliesTo: []string{"internal/domain"},
		forbidden: []string{"internal"},
	},
	{
		name:      "ports-only-domain",
		desc:      "ports 层只允许依赖 domain（Repository/Cache/UpstreamClient 接口层）",
		appliesTo: []string{"internal/ports"},
		forbidden: []string{"internal"},
		allowed:   []string{"internal/domain"},
	},
	{
		name:      "app-no-infra-transport",
		desc:      "app 层不得 import infra/transport 及其现状形态（repository/handler/server/service）",
		appliesTo: []string{"internal/app"},
		forbidden: []string{
			"internal/infra", "internal/transport",
			"internal/repository", "internal/handler", "internal/server",
			"internal/service", "internal/middleware", "internal/setup", "internal/web",
		},
	},
	{
		name:      "infra-no-app",
		desc:      "infra 层（ports 实现）禁止 import app 层及其现状形态（service/handler/server）",
		appliesTo: []string{"internal/infra"},
		forbidden: []string{
			"internal/app", "internal/transport",
			"internal/service", "internal/handler", "internal/server",
		},
	},
	{
		name:      "transport-via-app",
		desc:      "transport 层只经由 app 访问下层，不得直接 import infra/repository",
		appliesTo: []string{"internal/transport"},
		forbidden: []string{"internal/infra", "internal/repository"},
	},

	// ---- 当前成立的依赖不变量（冻结现状，禁止回退） ----
	{
		name:      "config-standalone",
		desc:      "config 不依赖任何 internal 包",
		appliesTo: []string{"internal/config"},
		forbidden: []string{"internal"},
	},
	{
		name:      "util-minimal",
		desc:      "util 只允许依赖 config",
		appliesTo: []string{"internal/util"},
		forbidden: []string{"internal"},
		allowed:   []string{"internal/config"},
	},
	{
		name:      "model-minimal",
		desc:      "model 只允许依赖 pkg",
		appliesTo: []string{"internal/model"},
		forbidden: []string{"internal"},
		allowed:   []string{"internal/pkg"},
	},
	{
		name:      "pkg-no-upper-layers",
		desc:      "pkg（无状态协议/工具库）不得 import 业务与接入层",
		appliesTo: []string{"internal/pkg"},
		forbidden: []string{
			"internal/service", "internal/repository", "internal/handler",
			"internal/server", "internal/middleware", "internal/model",
			"internal/payment", "internal/setup", "internal/web", "internal/domain",
		},
	},
	{
		name:      "middleware-standalone",
		desc:      "internal/middleware（基础中间件）不得 import 业务与接入层",
		appliesTo: []string{"internal/middleware"},
		forbidden: []string{
			"internal/service", "internal/repository", "internal/handler",
			"internal/server", "internal/setup", "internal/web",
		},
	},
	{
		name:      "payment-no-upper-layers",
		desc:      "payment（支付协议库）不得 import 业务与接入层",
		appliesTo: []string{"internal/payment"},
		forbidden: []string{
			"internal/service", "internal/repository", "internal/handler",
			"internal/server", "internal/setup", "internal/web",
		},
	},
	{
		name:      "repository-layering",
		desc:      "repository 不得 import 接入层与应用装配层（对 service 的反向依赖为已知拆包死锁，白名单管理，P1 ports 下沉后清零）",
		appliesTo: []string{"internal/repository"},
		forbidden: []string{
			"internal/service", "internal/handler", "internal/server",
			"internal/setup", "internal/web", "internal/payment", "internal/middleware",
		},
	},
	{
		name:      "service-layering",
		desc:      "service 不得 import 接入层（handler/server/middleware）、装配层（setup）与 repository",
		appliesTo: []string{"internal/service"},
		forbidden: []string{
			"internal/repository", "internal/handler", "internal/server",
			"internal/middleware", "internal/setup", "internal/web", "internal/integration",
		},
	},
	{
		name:      "handler-no-lower-bypass",
		desc:      "handler 不得绕过 service 直接依赖 repository，也不得反向依赖路由注册层",
		appliesTo: []string{"internal/handler"},
		forbidden: []string{
			"internal/repository", "internal/server/routes",
			"internal/setup", "internal/web",
		},
	},
	{
		name:      "server-no-repository",
		desc:      "server（路由/HTTP 装配）不得直接 import repository",
		appliesTo: []string{"internal/server"},
		forbidden: []string{"internal/repository"},
	},
	{
		name:      "setup-no-transport",
		desc:      "setup（初始化向导）不得 import 常规接入层",
		appliesTo: []string{"internal/setup"},
		forbidden: []string{"internal/handler", "internal/server", "internal/web"},
	},
}

// TestImportBoundaries 加载 backend/internal/... 的 import 图（默认构建上下文，
// 不含测试文件），校验依赖方向规则；现存违例以 baseline_imports.txt 白名单管理。
func TestImportBoundaries(t *testing.T) {
	root := moduleRoot(t)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports,
		Dir:  root,
	}
	pkgs, err := packages.Load(cfg, modulePrefix+"internal/...")
	if err != nil {
		t.Fatalf("加载 internal 包失败: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("未加载到任何 internal 包")
	}
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			t.Errorf("包 %s 加载错误: %v", pkg.PkgPath, e)
		}
	}
	if t.Failed() {
		t.Fatalf("包加载存在错误，无法继续架构检查")
	}

	// violation key: "from -> to"；value: 命中的规则说明（仅用于报错展示）。
	violations := make(map[string]string)
	for _, pkg := range pkgs {
		from := shortPath(pkg.PkgPath)
		for impPath := range pkg.Imports {
			if !strings.HasPrefix(impPath, modulePrefix) {
				continue
			}
			to := shortPath(impPath)
			for _, rule := range importRules {
				if !matchAny(from, rule.appliesTo) {
					continue
				}
				if !matchAny(to, rule.forbidden) || matchAny(to, rule.allowed) {
					continue
				}
				key := fmt.Sprintf("%s -> %s", from, to)
				if _, ok := violations[key]; !ok {
					violations[key] = fmt.Sprintf("违反规则 %s：%s", rule.name, rule.desc)
				}
			}
		}
	}

	baseline := loadBaseline(t, "baseline_imports.txt")
	reportBaselineDiff(t, "internal/archtest/baseline_imports.txt", violations, baseline,
		"请优先调整依赖方向（见 openspec/changes/refactor-architecture/design.md D1）；\n"+
			"若确认为过渡期必须保留的依赖，将上述 \"from -> to\" 行加入 internal/archtest/baseline_imports.txt 并在评审中说明。\n",
		"")
}

func matchAny(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if hasPathPrefix(path, p) {
			return true
		}
	}
	return false
}
