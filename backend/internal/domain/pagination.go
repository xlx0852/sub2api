// pagination 类型再导出：ports 接口签名经 domain 引用分页类型，
// 保持 ports 只依赖 domain（archtest ports-only-domain）。类型别名，完全同一。
package domain

import "github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

type PaginationParams = pagination.PaginationParams
type PaginationResult = pagination.PaginationResult
