package archtest

import (
	"fmt"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// globalScanTargets 是包级可变全局检查的目标包（上帝包拆解重点区域）。
// 随迁移推进（P1+ 新增 app/infra 模块）应将新包加入此列表。
var globalScanTargets = []string{
	modulePrefix + "internal/service",
	modulePrefix + "internal/repository",
}

// immutableArtifactPointerTypes 是"初始化后只读"的惯用工件类型：
// 指向这些类型的包级指针变量不视为可变全局（否则错误哨兵/编译期正则/Lua 脚本
// 会带来数百条噪音，掩盖真实的全局状态）。
var immutableArtifactPointerTypes = map[string]bool{
	"regexp.Regexp":                       true, // 编译期正则
	"strings.Replacer":                    true, // 编译期替换器
	"github.com/redis/go-redis/v9.Script": true, // Lua 脚本句柄
	"github.com/Wei-Shaw/sub2api/internal/pkg/errors.ApplicationError": true, // 错误哨兵（ErrXxx）
}

// TestNoNewPackageLevelMutableGlobals 扫描目标包中的包级可变全局状态：
//
//  1. 可变类型的包级 var：指针、函数值、channel、sync/atomic 原子类型、
//     sync.Map、singleflight.Group（含其组合的泛型实例，如 atomic.Pointer[T]）。
//     只读查表用的 map/slice/struct 字面量不在此列（该类误报太多且通常无害）；
//     指向 immutableArtifactPointerTypes 中类型的指针同理豁免。
//  2. 包级导出的全局 Set* 函数（装配旁路，绕过 Wire 注入）。
//     参数带 context.Context / *gin.Context 的请求级 Set* 助手不算全局状态，予以放行。
//
// 现存条目已记录在 baseline_globals.txt（现状白名单），此测试只禁止新增；
// design.md D6 计划将它们收归构造注入后逐条从基线移除。
//
// 已知局限（P0 护栏定位，能接受）：包内自定义 struct 类型若内嵌可变字段
// （如 struct{ v atomic.Value }）不会被识别为可变全局。
func TestNoNewPackageLevelMutableGlobals(t *testing.T) {
	root := moduleRoot(t)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo,
		Dir: root,
	}
	pkgs, err := packages.Load(cfg, globalScanTargets...)
	if err != nil {
		t.Fatalf("加载目标包失败: %v", err)
	}
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			t.Errorf("包 %s 加载错误: %v", pkg.PkgPath, e)
		}
	}
	if t.Failed() {
		t.Fatalf("包加载存在错误，无法继续全局状态检查")
	}

	// finding key: "<pkg>: var <name>" 或 "<pkg>: func <name>"；value: 位置与类别说明。
	findings := make(map[string]string)
	for _, pkg := range pkgs {
		short := shortPath(pkg.PkgPath)
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			pos := pkg.Fset.Position(obj.Pos())
			loc := fmt.Sprintf("%s:%d", pos.Filename, pos.Line)

			switch o := obj.(type) {
			case *types.Var:
				if name == "_" {
					continue
				}
				if kind := classifyMutableType(o.Type()); kind != "" {
					key := fmt.Sprintf("%s: var %s", short, name)
					findings[key] = fmt.Sprintf("类别: %s，位置: %s", kind, loc)
				}
			case *types.Func:
				if !isGlobalSetter(o) {
					continue
				}
				key := fmt.Sprintf("%s: func %s", short, name)
				findings[key] = fmt.Sprintf("类别: 全局 setter，位置: %s", loc)
			}
		}
	}

	baseline := loadBaseline(t, "baseline_globals.txt")
	reportBaselineDiff(t, "internal/archtest/baseline_globals.txt", findings, baseline,
		"新代码请将依赖收归构造函数注入（Wire），不要新增包级可变全局/全局 Set* 装配旁路\n"+
			"（见 openspec/changes/refactor-architecture/design.md D6）；\n"+
			"确属现状过渡产物的，将上述条目加入 internal/archtest/baseline_globals.txt 并在评审中说明。\n",
		"")
}

// classifyMutableType 判断类型是否属于关注的可变全局类别，返回类别名；不属于则返回空串。
func classifyMutableType(t types.Type) string {
	switch tt := types.Unalias(t).(type) {
	case *types.Pointer:
		if named, ok := types.Unalias(tt.Elem()).(*types.Named); ok {
			obj := named.Obj()
			if obj != nil && obj.Pkg() != nil && immutableArtifactPointerTypes[obj.Pkg().Path()+"."+obj.Name()] {
				return ""
			}
		}
		return "指针"
	case *types.Signature:
		return "函数值"
	case *types.Chan:
		return "channel"
	case *types.Named:
		obj := tt.Obj()
		if obj == nil || obj.Pkg() == nil {
			return ""
		}
		pkgPath := obj.Pkg().Path()
		qualified := pkgPath + "." + obj.Name()
		switch qualified {
		case "sync.Map":
			return "sync.Map"
		case "golang.org/x/sync/singleflight.Group":
			return "singleflight.Group"
		}
		if pkgPath == "sync/atomic" || pkgPath == "go.uber.org/atomic" {
			return "atomic"
		}
	}
	return ""
}

// isGlobalSetter 判断包级函数是否为全局状态 setter：
// 导出、命名为 SetXxx，且参数不含 context.Context / *gin.Context（请求级助手放行）。
func isGlobalSetter(fn *types.Func) bool {
	name := fn.Name()
	if !fn.Exported() || !strings.HasPrefix(name, "Set") || len(name) <= 3 {
		return false
	}
	if c := name[3]; c < 'A' || c > 'Z' {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() != nil {
		return false
	}
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		typeName := params.At(i).Type().String()
		if typeName == "context.Context" || strings.Contains(typeName, "github.com/gin-gonic/gin.Context") {
			return false
		}
	}
	return true
}
