package service

// Mirasim 只读元数据门面：账号维度可用模型（/v1/models）与额度窗口（limits）的拉取 + 缓存。
//
// 本文件把 mirasim_relay.go 的只读通道（凭据选择 / 签名头 / 超时 / 401 兜底）、
// mirasim_available_models.go 的可用模型解析、mirasim_limits.go 的窗口归一化串成一个可注入的
// 服务，供两条消费路径复用同一份缓存：
//   - 管理端「同步上游模型」→ AccountTestService.FetchUpstreamSupportedModels；
//   - 管理端用量抽屉 → AccountUsageService.GetUsage（降级为 200 + error 字段）。
//
// 关键约束（改动前请先读）：
//   - 模型清单的来源是**账号维度**的 `GET /v1/models`（只返回本账号当前可用的模型）。
//     全局目录 `/v1/model-roster` 不描述「本账号能用什么」，不再用于白名单；
//   - 缓存按 account id，并用 mirasimRelayConfig.fingerprint 做失效判定：relay/auth
//     URL、client version、seal 公钥、设备私钥、代理任一变化都会让旧缓存失效，
//     避免把 A 账号（或 A 端点）的清单/额度喂给 B；
//   - 失败**不写入负缓存**：调用方（管理端）每次点击都应看到真实的上游状态，
//     而不是被 30 秒前的失败粘住；
//   - 错误分类是「唯一」对外可暴露的信息：`mirasimMetadataErrorCategory` 只返回
//     固定串（configuration / timeout / canceled / upstream / upstream_http_<code>）。
//     上游响应正文可能含凭据链路细节，**绝不**进入错误消息、日志或响应体；
//   - 该服务不做任何调度副作用：不写 rate_limit_reset_at、不置临时不可调度。

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// MirasimMetadataQuerier 是 AccountUsageService / AccountTestService 依赖的最小接口。
//
// 形态对标 KimiQuotaQuerier：只暴露两个读取入口与一个缓存失效钩子，便于测试注入
// 假实现，也避免把 HTTP 细节泄漏到调用方。
type MirasimMetadataQuerier interface {
	// AvailableModels 返回账号维度的可用模型 id 列表（去重、升序）；force 为 true 时绕过缓存。
	AvailableModels(ctx context.Context, account *Account, force bool) ([]string, error)
	// Limits 返回归一化后的额度窗口快照；force 为 true 时绕过缓存。
	Limits(ctx context.Context, account *Account, force bool) (*mirasimLimitsSnapshot, error)
	// InvalidateMirasimMetadata 清空该账号的 available-models/limits 缓存（账号配置变更后调用）。
	InvalidateMirasimMetadata(accountID int64)
}

// MirasimMetadataConfigError 标注「账号/凭据配置缺失」这类不可重试的失败。
//
// 关键约束（改动前请先读）：只有这一类别可以被模型同步映射成
// UpstreamModelSyncErrorConfiguration；其余（网络、超时、非 2xx、解析失败）统一
// 映射成 upstream。凭据缺失时**不得**发起任何上游请求。
type MirasimMetadataConfigError struct {
	Err error
}

func (e *MirasimMetadataConfigError) Error() string {
	if e == nil || e.Err == nil {
		return "mirasim metadata: 配置缺失"
	}
	return e.Err.Error()
}

func (e *MirasimMetadataConfigError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// mirasimMetadataIsConfigError 判定错误是否属于「配置缺失」类别。
func mirasimMetadataIsConfigError(err error) bool {
	var target *MirasimMetadataConfigError
	return errors.As(err, &target)
}

// mirasimMetadataErrorCategory 把错误压成固定类别串。
//
// 关键约束（改动前请先读）：返回值会写进管理端可见的响应体（用量降级 error 字段），
// 因此这里**只**输出常量与状态码。`*mirasimHTTPError` 携带上游正文，绝不能把它的
// Error() 透出去——这里只取状态码。
func mirasimMetadataErrorCategory(err error) string {
	switch {
	case err == nil:
		return ""
	case mirasimMetadataIsConfigError(err):
		return "configuration"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	var metadataErr *mirasimMetadataHTTPError
	if errors.As(err, &metadataErr) {
		return fmt.Sprintf("upstream_http_%d", metadataErr.StatusCode)
	}
	var authErr *mirasimHTTPError
	if errors.As(err, &authErr) {
		return fmt.Sprintf("upstream_http_%d", authErr.StatusCode)
	}
	return "upstream"
}

// mirasimModelsCacheEntry / mirasimLimitsCacheEntry 按 account id 缓存，附带配置指纹。
type mirasimModelsCacheEntry struct {
	fingerprint string
	models      []string
	capturedAt  time.Time
}

type mirasimLimitsCacheEntry struct {
	fingerprint string
	snapshot    *mirasimLimitsSnapshot
	capturedAt  time.Time
}

// MirasimMetadataService 是只读元数据客户端 + 进程内缓存。
type MirasimMetadataService struct {
	accountRepo  AccountRepository
	httpUpstream HTTPUpstream

	// now 可被测试替换；生产恒为 time.Now。
	now func() time.Time

	mu     sync.Mutex
	models map[int64]mirasimModelsCacheEntry
	limits map[int64]mirasimLimitsCacheEntry
}

// NewMirasimMetadataService 创建只读元数据服务。
func NewMirasimMetadataService(accountRepo AccountRepository, httpUpstream HTTPUpstream) *MirasimMetadataService {
	return &MirasimMetadataService{
		accountRepo:  accountRepo,
		httpUpstream: httpUpstream,
		now:          time.Now,
	}
}

// AvailableModels 拉取（或读缓存）账号维度的可用模型列表。
func (s *MirasimMetadataService) AvailableModels(ctx context.Context, account *Account, force bool) ([]string, error) {
	if s == nil {
		return nil, &MirasimMetadataConfigError{Err: errors.New("mirasim metadata: 服务未初始化")}
	}
	cfg, err := s.resolveConfig(account)
	if err != nil {
		return nil, err
	}
	if !force {
		if cached, ok := s.cachedAvailableModels(cfg); ok {
			return cached, nil
		}
	}
	client, err := s.client(cfg)
	if err != nil {
		return nil, err
	}
	body, err := client.fetchAvailableModels(ctx)
	if err != nil {
		slog.Warn("mirasim_models_fetch_failed", "account_id", cfg.AccountID, "category", mirasimMetadataErrorCategory(err))
		return nil, err
	}
	models, err := parseMirasimAvailableModels(body)
	if err != nil {
		slog.Warn("mirasim_models_parse_failed", "account_id", cfg.AccountID, "category", mirasimMetadataErrorCategory(err))
		return nil, err
	}
	s.storeAvailableModels(cfg, models)
	return models, nil
}

// Limits 拉取（或读缓存）额度窗口快照。
func (s *MirasimMetadataService) Limits(ctx context.Context, account *Account, force bool) (*mirasimLimitsSnapshot, error) {
	if s == nil {
		return nil, &MirasimMetadataConfigError{Err: errors.New("mirasim metadata: 服务未初始化")}
	}
	cfg, err := s.resolveConfig(account)
	if err != nil {
		return nil, err
	}
	if !force {
		if cached, ok := s.cachedLimits(cfg); ok {
			return cached, nil
		}
	}
	client, err := s.client(cfg)
	if err != nil {
		return nil, err
	}
	body, err := client.fetchLimits(ctx)
	if err != nil {
		slog.Warn("mirasim_limits_fetch_failed", "account_id", cfg.AccountID, "category", mirasimMetadataErrorCategory(err))
		return nil, err
	}
	snapshot, err := parseMirasimLimits(body)
	if err != nil {
		slog.Warn("mirasim_limits_parse_failed", "account_id", cfg.AccountID, "category", mirasimMetadataErrorCategory(err))
		return nil, err
	}
	s.storeLimits(cfg, snapshot)
	return snapshot, nil
}

// InvalidateMirasimMetadata 清空账号的 available-models / limits 缓存。
func (s *MirasimMetadataService) InvalidateMirasimMetadata(accountID int64) {
	if s == nil || accountID <= 0 {
		return
	}
	s.mu.Lock()
	delete(s.models, accountID)
	delete(s.limits, accountID)
	s.mu.Unlock()
}

func (s *MirasimMetadataService) resolveConfig(account *Account) (mirasimRelayConfig, error) {
	if account == nil {
		return mirasimRelayConfig{}, &MirasimMetadataConfigError{Err: errors.New("mirasim metadata: account 为空")}
	}
	cfg, err := mirasimRelayConfigForAccount(account)
	if err != nil {
		// 凭据缺失属配置问题：调用方应给出 configuration 级提示，且不得发起请求。
		return mirasimRelayConfig{}, &MirasimMetadataConfigError{Err: err}
	}
	return cfg, nil
}

func (s *MirasimMetadataService) client(cfg mirasimRelayConfig) (*mirasimRelayClient, error) {
	if s.httpUpstream == nil {
		return nil, &MirasimMetadataConfigError{Err: errors.New("mirasim metadata: upstream HTTP 客户端未初始化")}
	}
	persistRefresh, persistDeviceKey := mirasimPersistCallbacks(cfg.AccountID, s.accountRepo)
	return mirasimRelayRuntimeAcquire(cfg, s.httpUpstream, persistRefresh, persistDeviceKey)
}

func (s *MirasimMetadataService) cachedAvailableModels(cfg mirasimRelayConfig) ([]string, bool) {
	fingerprint := cfg.fingerprint()
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.models[cfg.AccountID]
	if !ok || len(entry.models) == 0 || entry.fingerprint != fingerprint {
		return nil, false
	}
	if now.Sub(entry.capturedAt) >= mirasimModelsCacheTTL {
		return nil, false
	}
	return entry.models, true
}

func (s *MirasimMetadataService) cachedLimits(cfg mirasimRelayConfig) (*mirasimLimitsSnapshot, bool) {
	fingerprint := cfg.fingerprint()
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.limits[cfg.AccountID]
	if !ok || entry.snapshot == nil || entry.fingerprint != fingerprint {
		return nil, false
	}
	if now.Sub(entry.capturedAt) >= mirasimLimitsCacheTTL {
		return nil, false
	}
	return entry.snapshot, true
}

func (s *MirasimMetadataService) storeAvailableModels(cfg mirasimRelayConfig, models []string) {
	if len(models) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.models == nil {
		s.models = make(map[int64]mirasimModelsCacheEntry)
	}
	s.models[cfg.AccountID] = mirasimModelsCacheEntry{
		fingerprint: cfg.fingerprint(),
		models:      models,
		capturedAt:  s.clock(),
	}
}

func (s *MirasimMetadataService) storeLimits(cfg mirasimRelayConfig, snapshot *mirasimLimitsSnapshot) {
	if snapshot == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.limits == nil {
		s.limits = make(map[int64]mirasimLimitsCacheEntry)
	}
	s.limits[cfg.AccountID] = mirasimLimitsCacheEntry{
		fingerprint: cfg.fingerprint(),
		snapshot:    snapshot,
		capturedAt:  s.clock(),
	}
}

func (s *MirasimMetadataService) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// mirasimRelayAccountCandidate 是「看起来像 mirasim relay 账号」的宽松判定。
//
// 与 isMirasimRelayAccount 的区别：不要求 refresh token 已存在。只读元数据与
// 模型同步用这个判定，好处是**凭据缺失时给出 configuration 级错误**，
// 而不是掉进「API Key 账号不支持用量查询 / 上游没有模型列表」这类误导性结论。
//
// 关键约束（改动前请先读）：模型转发路径**必须**继续使用 isMirasimRelayAccount，
// 不能让没有凭据的账号走 relay 改写。
func mirasimRelayAccountCandidate(account *Account) bool {
	if account == nil || account.Platform != PlatformAnthropic || account.Type != AccountTypeAPIKey {
		return false
	}
	if _, ok := account.Extra[mirasimExtraRelayEnabled]; ok && !account.GetExtraBool(mirasimExtraRelayEnabled) {
		return false
	}
	if strings.TrimSpace(account.GetCredential(mirasimCredentialRefreshToken)) != "" {
		return true
	}
	// 没有 refresh token 时只有显式开启 relay 才视作候选（用于报告配置缺失）。
	return account.GetExtraBool(mirasimExtraRelayEnabled)
}

var _ MirasimMetadataQuerier = (*MirasimMetadataService)(nil)
