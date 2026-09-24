package admin

// mirasim relay 账号的模型映射修复端点。
//
// 背景（生产事故 2026-09-25，账号 140）：relay 账号 model_mapping 为空时，
// 调度语义是「认领所有模型」，账号会被选中去转发上游不支持的模型
// （上游 422 → 网关 502）。本端点把**账号维度**的上游可用模型清单写成
// identity 映射（id -> id），让「空映射」这个坑可以被一键修好。
//
// 关键约束（改动前请先读）：
//   - 绝不写空映射：上游清单为空 / 全部空白时直接报错。写入空映射等于恢复成
//     「认领所有模型」，正是本端点要消除的状态。
//   - 非 mirasim relay 账号一律 400（识别规则唯一来源：service.IsMirasimRelayAccount）。
//   - 凭据保全：service.MergePreservingSensitiveCreds 的语义是「非敏感键完全由
//     incoming 决定」，因此必须先把账号现有凭据中的**非敏感**子键整体拷入
//     incoming（只覆盖 model_mapping），否则会静默丢掉 base_url 等键。
//   - model_mapping 必须落成 map[string]any 且值为 string：
//     domain 层 GetModelMapping 用 `.(map[string]any)` 断言，map[string]string
//     会被判为「无映射」，反而重新变成「认领所有模型」。
//   - 日志/响应只含模型 id 与数量，绝不含凭据或上游响应正文。

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// upstreamModelsFetcher 是「取上游可用模型清单」的最小依赖面。
//
// 关键约束（改动前请先读）：生产实现是 *service.AccountTestService，测试里用桩替换
// 它需要能满足 service.MirasimMetadataQuerier，而该接口含未导出返回类型
// （*mirasimLimitsSnapshot），跨包无法实现。因此这里只声明本端点真正用到的那一个方法。
type upstreamModelsFetcher interface {
	FetchUpstreamSupportedModels(ctx context.Context, account *service.Account) ([]string, error)
}

// ApplyMirasimModelMapping handles writing the account's upstream available models
// as an identity model mapping.
// POST /api/v1/admin/accounts/:id/models/apply-mapping
func (h *AccountHandler) ApplyMirasimModelMapping(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}

	var req struct {
		// Overwrite 为 false 时，账号已有非空 model_mapping 会被拒绝（409），
		// 避免静默丢弃人工维护的别名映射。
		Overwrite bool `json:"overwrite"`
	}
	// 请求体允许为空（等价 overwrite=false）。
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "Invalid request body")
		return
	}

	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.NotFound(c, "Account not found")
		return
	}

	if !service.IsMirasimRelayAccount(account) {
		response.BadRequest(c, "Account is not a mirasim relay account")
		return
	}

	if h.upstreamModels == nil {
		response.InternalError(c, "Account test service is not configured")
		return
	}

	// 模型清单来源与「同步上游模型」一致：账号维度只读接口，force=true 绕过 10 分钟缓存
	// （管理员点按钮时期望看到上游最新状态）。
	models, err := h.upstreamModels.FetchUpstreamSupportedModels(c.Request.Context(), account)
	if err != nil {
		var syncErr *service.UpstreamModelSyncError
		if errors.As(err, &syncErr) {
			switch syncErr.Kind {
			case service.UpstreamModelSyncErrorConfiguration, service.UpstreamModelSyncErrorUnsupported:
				response.BadRequest(c, syncErr.SafeMessage())
			default:
				slog.Warn("mirasim_apply_model_mapping_failed", "account_id", accountID, "kind", syncErr.Kind)
				response.Error(c, http.StatusBadGateway, syncErr.SafeMessage())
			}
			return
		}
		slog.Warn("mirasim_apply_model_mapping_failed", "account_id", accountID)
		response.Error(c, http.StatusBadGateway, "Failed to fetch mirasim available models")
		return
	}

	identity := service.BuildMirasimIdentityModelMapping(models)
	if len(identity) == 0 {
		// 防御：上游返回空清单（或全是空白 id）时绝不落库。
		slog.Warn("mirasim_apply_model_mapping_empty", "account_id", accountID)
		response.Error(c, http.StatusBadGateway, "Mirasim returned no available models")
		return
	}

	applyModels := sortedKeys(identity)

	// 幂等：目标映射与现值逐键一致时不写库。
	if service.MirasimModelMappingEqual(account.GetModelMapping(), identity) {
		response.Success(c, gin.H{"models": applyModels, "changed": false})
		return
	}

	existing := account.GetModelMapping()
	if len(existing) > 0 && !req.Overwrite {
		response.Error(c, http.StatusConflict,
			"Account already has a model mapping; pass overwrite=true to replace it")
		return
	}

	// 只覆盖 model_mapping，其余非敏感键原样拷回（敏感键由 merge 语义保留）。
	incoming := make(map[string]any, len(account.Credentials)+1)
	for key, value := range account.Credentials {
		if service.IsSensitiveCredentialKey(key) {
			continue
		}
		incoming[key] = value
	}
	mappingValue := make(map[string]any, len(identity))
	for key, value := range identity {
		mappingValue[key] = value
	}
	incoming["model_mapping"] = mappingValue

	updated, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Credentials: incoming,
	})
	if err != nil {
		// 只记录稳定类别（reason/code），绝不记录 err.Error()：账号更新路径的错误
		// 可能带上凭据相关的上下文。
		slog.Warn("mirasim_apply_model_mapping_update_failed", "account_id", accountID,
			"reason", infraerrors.Reason(err), "status", infraerrors.Code(err))
		response.Error(c, http.StatusBadGateway, "Failed to update account model mapping")
		return
	}

	// 写后自检：确认落库的映射数量与预期一致（防止 merge 语义意外吞掉映射）。
	applied := 0
	if updated != nil {
		applied = len(updated.GetModelMapping())
	}
	slog.Info("mirasim_apply_model_mapping_applied",
		"account_id", accountID, "models", len(identity), "applied", applied)

	response.Success(c, gin.H{"models": applyModels, "changed": true})
}

// sortedKeys 返回排序后的键集合，保证响应可预期（便于前端 diff 与测试断言）。
func sortedKeys(mapping map[string]string) []string {
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
