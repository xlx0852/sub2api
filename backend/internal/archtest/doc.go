// Package archtest 承载架构基线测试（OpenSpec change: refactor-architecture, P0 边界冻结）。
//
// 本包只包含测试代码，不参与生产构建：
//
//   - imports_test.go  校验 internal 包间依赖方向（design.md D1），
//     现存违例记录在 baseline_imports.txt 中作为白名单，禁止新增、只允许收敛。
//   - globals_test.go  扫描 internal/service 与 internal/repository 的包级可变全局
//     （指针/atomic/sync.Map/func 变量与全局 Set* 导出函数），
//     现存条目记录在 baseline_globals.txt 中，禁止新增、只允许收敛。
//
// 该 doc.go 仅用于让本目录成为可构建的 Go 包（go build ./... 不因纯测试目录报错）。
package archtest
