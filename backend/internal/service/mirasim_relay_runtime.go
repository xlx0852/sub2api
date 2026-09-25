package service

// Mirasim relay 运行时客户端缓存 + 凭据落盘。
//
// 每个 mirasim 账号一个 client（承载 Ed25519 设备身份、access token 与 device
// ticket 的内存缓存）。账号对象本身是只读快照，token 轮换只能写回数据库，所以
// 这里用 (fingerprint, baseToken) 双条件失效：
//
//   - fingerprint 覆盖 relay/auth URL、client version、seal 公钥、设备私钥、代理；
//     这些字段变了 → 重建 client；
//   - baseToken 记录"上次构建时账号快照里的 refresh token"。账号快照更新后可能
//     仍是旧值（上游刚轮换过），此时若 client 内部 token 已等于快照值就直接复用并
//     推进 baseToken，避免轮换后立刻缓存穿透、重复换 token。

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type mirasimRelayEntry struct {
	fingerprint string
	baseToken   string
	lastUsed    time.Time
	client      *mirasimRelayClient
}

type MirasimRelayRegistry struct {
	mu      sync.Mutex
	entries map[int64]*mirasimRelayEntry
}

// NewMirasimRelayRegistry creates the account-scoped relay client registry.
// The application must share one instance across gateway, metadata and rate-limit services.
func NewMirasimRelayRegistry() *MirasimRelayRegistry {
	return &MirasimRelayRegistry{}
}

// mirasimRelayRuntimeAcquire 取（或构建）账号对应的 client。
func mirasimRelayRuntimeAcquire(
	cfg mirasimRelayConfig,
	upstream HTTPUpstream,
	persistRefresh func(newToken string),
	persistDeviceKey func(pem string),
	registries ...*MirasimRelayRegistry,
) (*mirasimRelayClient, error) {
	var registry *MirasimRelayRegistry
	if len(registries) > 0 {
		registry = registries[0]
	}
	if registry == nil {
		registry = NewMirasimRelayRegistry()
	}
	return registry.acquire(cfg, upstream, persistRefresh, persistDeviceKey)
}

func (rt *MirasimRelayRegistry) acquire(
	cfg mirasimRelayConfig,
	upstream HTTPUpstream,
	persistRefresh func(newToken string),
	persistDeviceKey func(pem string),
) (*mirasimRelayClient, error) {
	fingerprint := cfg.fingerprint()
	now := time.Now()

	rt.mu.Lock()
	if rt.entries == nil {
		rt.entries = make(map[int64]*mirasimRelayEntry)
	}
	if entry, ok := rt.entries[cfg.AccountID]; ok && entry != nil && entry.client != nil {
		if entry.fingerprint == fingerprint && entry.baseToken == cfg.RefreshToken {
			entry.lastUsed = now
			client := entry.client
			rt.mu.Unlock()
			return client, nil
		}
		if entry.client.currentRefreshToken() == cfg.RefreshToken {
			// 快照仍是轮换前的 token：沿用现有 client（其内部已是最新 token）。
			entry.baseToken = cfg.RefreshToken
			entry.lastUsed = now
			client := entry.client
			rt.mu.Unlock()
			return client, nil
		}
	}
	rt.pruneLocked(now)
	rt.mu.Unlock()

	// client 构建不做网络请求（仅解析/生成设备密钥），无需 single-flight。
	client, err := newMirasimRelayClient(cfg, upstream, persistRefresh, persistDeviceKey)
	if err != nil {
		return nil, err
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if entry, ok := rt.entries[cfg.AccountID]; ok && entry != nil && entry.client != nil &&
		entry.fingerprint == fingerprint && entry.baseToken == cfg.RefreshToken {
		// 并发构建时保留先到者。
		entry.lastUsed = now
		return entry.client, nil
	}
	rt.entries[cfg.AccountID] = &mirasimRelayEntry{
		fingerprint: fingerprint,
		baseToken:   cfg.RefreshToken,
		lastUsed:    now,
		client:      client,
	}
	return client, nil
}

// invalidateMirasimRelayClientTokens 处理上游 401/403：只清内存里的 access token
// 与 ticket，保留 client 本身。
//
// 关键约束（改动前请先读）：这里刻意不删除条目。client 内可能持有比数据库快照
// 更新的 refresh token（轮换落盘是异步的），丢弃条目会让下一次请求退回旧 token，
// 触发 401 → 再丢弃的死循环。凭据本身变化时由 acquire 的 fingerprint/baseToken
// 比对负责重建，不需要在这里删。
func invalidateMirasimRelayClientTokens(accountID int64, registries ...*MirasimRelayRegistry) {
	if len(registries) == 0 || registries[0] == nil {
		return
	}
	registries[0].invalidateTokens(accountID)
}

func (rt *MirasimRelayRegistry) invalidateTokens(accountID int64) {
	rt.mu.Lock()
	entry, ok := rt.entries[accountID]
	rt.mu.Unlock()
	if !ok || entry == nil || entry.client == nil {
		return
	}
	entry.client.invalidateTokens()
}

// pruneLocked 防止已删除账号的条目长期驻留。
func (rt *MirasimRelayRegistry) pruneLocked(now time.Time) {
	if len(rt.entries) <= mirasimRelayEntrySoftLimit {
		return
	}
	cutoff := now.Add(-mirasimRelayEntryTTL)
	for id, entry := range rt.entries {
		if entry == nil || entry.lastUsed.Before(cutoff) {
			delete(rt.entries, id)
		}
	}
}

const (
	mirasimRelayEntrySoftLimit = 512
	mirasimRelayEntryTTL       = time.Hour
)

// mirasimPersistCallbacks 返回两个"写回账号凭据"的回调：
//   - refresh token 轮换（上游每次 refresh 都可能换发新 refresh token）；
//   - 新建设备密钥（账号第一次使用时生成）。
//
// 关键约束（改动前请先读）：
//   - 回调是在 client 的互斥锁内、模型请求路径上被调用的，必须立即返回，
//     所以落盘放到独立 goroutine；否则同账号并发请求会一起卡在 DB 写回上；
//   - 落盘前先 GetByID 重读，避免用过期快照覆盖其他字段（账号快照写回有窄竞态）；
//   - last 表防止轮换风暴下重复写同一个值；写失败时清除，让下一次请求重试。
func mirasimPersistCallbacks(accountID int64, repo AccountRepository) (func(string), func(string)) {
	var (
		mu   sync.Mutex
		last = make(map[string]string, 2)
	)
	persist := func(key, value string) {
		value = strings.TrimSpace(value)
		if repo == nil || accountID <= 0 || value == "" {
			return
		}
		mu.Lock()
		if last[key] == value {
			mu.Unlock()
			return
		}
		last[key] = value
		mu.Unlock()

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), mirasimPersistTimeout)
			defer cancel()
			failed := func() bool {
				account, err := repo.GetByID(ctx, accountID)
				if err != nil || account == nil {
					slog.Warn("mirasim_relay_persist_reload_failed", "account_id", accountID, "key", key, "error", err)
					return true
				}
				if strings.TrimSpace(account.GetCredential(key)) == value {
					return false
				}
				credentials := shallowCopyMap(account.Credentials)
				credentials[key] = value
				if err := persistAccountCredentials(ctx, repo, account, credentials); err != nil {
					slog.Warn("mirasim_relay_persist_failed", "account_id", accountID, "key", key, "error", err)
					return true
				}
				slog.Info("mirasim_relay_credential_updated", "account_id", accountID, "key", key)
				return false
			}()
			if failed {
				mu.Lock()
				if last[key] == value {
					delete(last, key)
				}
				mu.Unlock()
			}
		}()
	}
	return func(newToken string) { persist(mirasimCredentialRefreshToken, newToken) },
		func(pem string) { persist(mirasimCredentialDeviceKey, pem) }
}
