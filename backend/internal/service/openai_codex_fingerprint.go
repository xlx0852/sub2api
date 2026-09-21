package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// codexFingerprintMode 控制 OAuth 账号出站请求的设备指纹收敛强度。
// 多人共享同一 OAuth 账号时，每个用户的 Codex 客户端会携带各自不同的
// installation_id / session_id / thread_id，上游据此判定设备数和会话数。
// 收敛模式将这些标识改写为账号级恒定值，减少上游可见的设备/会话指纹。
type codexFingerprintMode string

const (
	// codexFingerprintOff 不做任何收敛，原样透传客户端标识（默认行为）。
	codexFingerprintOff codexFingerprintMode = "off"
	// codexFingerprintDevice 仅收敛 installation_id 为账号级恒定值。
	// 上游看到 1 台设备 + 多会话（每用户各自的 session）。
	codexFingerprintDevice codexFingerprintMode = "device"
	// codexFingerprintSession 收敛 installation_id + session_id，
	// thread_id 按客户端原始 session-id 确定性派生（每个真实 Codex 会话一个独立线程）。
	// 上游看到 1 台设备 + 1 会话 + N 线程，最接近正常用户 spawn 子代理的模式。
	codexFingerprintSession codexFingerprintMode = "session"
	// codexFingerprintFull 收敛所有标识：installation_id + session_id + thread_id。
	// 上游看到 1 台设备 + 1 会话 + 1 线程，最激进。
	codexFingerprintFull codexFingerprintMode = "full"
)

// codexFingerprintModeExtraKey 指纹收敛模式（off/device/session/full）。
const codexFingerprintModeExtraKey = "codex_fingerprint_mode"

// codexOverloadRotationExtraKey 按账号覆盖「容量过载时重抽会话身份」。
// 取值 on/off；缺失或非法时跟随全局配置
// gateway.disable_codex_overload_identity_rotation。
//
// 为什么要按账号可覆盖：该行为只在 session/full 指纹模式下有对象（device 模式
// 不改写 thread-id）。灰度时需要把实验组与对照组都放在同一指纹模式下，
// 只用本开关区分，否则「改指纹模式」与「开轮换」两个变量会混在一起。
const codexOverloadRotationExtraKey = "codex_overload_identity_rotation"

// GetCodexOverloadRotationOverride 返回账号级覆盖值：
//   - true/false：显式开/关
//   - ok=false：未配置或非法，调用方应回退全局配置
func GetCodexOverloadRotationOverride(a *Account) (enabled bool, ok bool) {
	if a == nil || !a.IsOpenAIOAuth() {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(a.GetExtraString(codexOverloadRotationExtraKey))) {
	case "on", "true", "1", "yes":
		return true, true
	case "off", "false", "0", "no":
		return false, true
	default:
		return false, false
	}
}

const codexFingerprintIDsContextKey = "codex_fingerprint_ids"
const codexFingerprintAttemptContextKey = "codex_fingerprint_attempt"

// GetCodexFingerprintMode 从账号 extra JSON 读取指纹收敛模式。
// 未设置或值无效时保持 off，避免升级后静默改变历史账号的出站指纹。
func GetCodexFingerprintMode(a *Account) codexFingerprintMode {
	if a == nil || !a.IsOpenAIOAuth() {
		return codexFingerprintOff
	}
	raw := strings.TrimSpace(a.GetExtraString(codexFingerprintModeExtraKey))
	switch codexFingerprintMode(raw) {
	case codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull:
		return codexFingerprintMode(raw)
	default:
		return codexFingerprintOff
	}
}

// deriveStableUUIDv4 从种子确定性派生一个 UUIDv4 格式的字符串。
// 同一种子永远返回同一值。
// 派生时混入进程级随机盐：即使种子里的 accountID 自增可被枚举，
// 不同部署（不同进程/不同实例）也派生不出可跨部署关联的相同值，
// 且外部无法用公开算法反推/验证种子，避免上游将多个账号关联到同一网关。
var codexFingerprintSalt = func() string {
	// 进程启动时生成一次，进程内稳定；多实例 = 多"设备"，符合真实分布。
	return uuid.NewString()
}()

func deriveStableUUIDv4(seed string) string {
	h := sha256.Sum256([]byte(codexFingerprintSalt + "|" + seed))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16])
}

// resolveConvergedInstallationID 返回账号级恒定的 installation_id。
// 优先使用管理员配置的真实 device_id，无则从 accountID 确定性派生。
func resolveConvergedInstallationID(account *Account) string {
	if account == nil {
		return ""
	}
	if deviceID := account.GetOpenAIDeviceID(); deviceID != "" {
		return deviceID
	}
	return deriveStableUUIDv4(fmt.Sprintf("codex:install-id:v1:%d", account.ID))
}

// resolveConvergedSessionID 返回账号级恒定的 session_id。
func resolveConvergedSessionID(account *Account) string {
	if account == nil {
		return ""
	}
	return deriveStableUUIDv4(fmt.Sprintf("codex:session-id:v1:%d", account.ID))
}

// resolveConvergedThreadID 按客户端原始 session-id 确定性派生 thread_id。
// 每个真实 Codex 会话（不同客户端启动实例）获得一个独立线程，
// 模拟正常用户 spawn 子代理或开多窗口的模式。
//
// attempt 为请求内已发生的失败重试轮次（首次尝试为 0）；
// epoch 为该 (账号, 客户端会话) 的过载 epoch（未发生过载为 0，见 openai_codex_overload_burst.go）。
//
//   - attempt<=0 且 epoch<=0 时派生结果与历史实现逐字节一致（防回归）；
//   - 否则在种子里追加重试轮次 / 过载 epoch，使重试与过载后的下一条请求拿到不同的 thread_id。
//
// 这样做的理由：thread-id 即上游的会话路由提示，同一个 thread_id 会被反复送回
// 同一个已饱和的后端；换新值才能重新路由。session_id 仍是账号级恒定值
// （见 resolveConvergedSessionID），不随重试/过载变化。
func resolveConvergedThreadID(account *Account, clientSessionID string, attempt, epoch int) string {
	if account == nil || clientSessionID == "" {
		return ""
	}
	// 过载 epoch 单独一个种子版本：即使 epoch 与 attempt 数值碰撞也不会派生同值。
	if epoch > 0 {
		if attempt > 0 {
			return deriveStableUUIDv4(fmt.Sprintf("codex:thread-id:v2:%d:%s:%d:%d", account.ID, clientSessionID, epoch, attempt))
		}
		return deriveStableUUIDv4(fmt.Sprintf("codex:thread-id:v2:%d:%s:%d", account.ID, clientSessionID, epoch))
	}
	if attempt <= 0 {
		return deriveStableUUIDv4(fmt.Sprintf("codex:thread-id:v1:%d:%s", account.ID, clientSessionID))
	}
	return deriveStableUUIDv4(fmt.Sprintf("codex:thread-id:v1:%d:%s:%d", account.ID, clientSessionID, attempt))
}

// codexFingerprintIDs 收敛后的完整 ID 集合。
// 由 resolveCodexFingerprintIDs 一次性生成，同一个实例在头改写和体改写之间共享，
// 确保所有载体中的 turn_id 等随机字段一致。
type codexFingerprintIDs struct {
	accountID      int64
	mode           codexFingerprintMode
	installationID string
	sessionID      string
	threadID       string
	turnID         string
	windowID       string
}

// resolveCodexFingerprintIDs 按收敛模式计算出站 ID 集合。
// clientSessionID 是客户端原始的 session-id 头值（连字符形式），用于 session 模式下
// 的 thread_id 派生——每个真实 Codex 会话得到一个独立线程。
// attempt 是请求内已发生的失败重试轮次，epoch 是该会话的过载 epoch；
// 两者都只影响派生的 thread_id（见 resolveConvergedThreadID）。
// 返回 nil 表示 off 模式，不需要改写。
// 注意：包含随机生成的 turn_id，调用方必须只调用一次并共享结果给头改写和体改写。
func resolveCodexFingerprintIDs(account *Account, clientSessionID string, mode codexFingerprintMode, attempt, epoch int) *codexFingerprintIDs {
	if mode == codexFingerprintOff {
		return nil
	}

	ids := &codexFingerprintIDs{accountID: account.ID, mode: mode}

	ids.installationID = resolveConvergedInstallationID(account)
	if ids.installationID == "" {
		return nil
	}

	switch mode {
	case codexFingerprintDevice:
		return ids

	case codexFingerprintSession:
		ids.sessionID = resolveConvergedSessionID(account)
		ids.threadID = resolveConvergedThreadID(account, clientSessionID, attempt, epoch)
		if ids.threadID == "" {
			ids.threadID = ids.sessionID
		}
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.windowID = ids.threadID + ":0"
		return ids

	case codexFingerprintFull:
		ids.sessionID = resolveConvergedSessionID(account)
		ids.threadID = ids.sessionID
		ids.turnID = uuid.Must(uuid.NewV7()).String()
		ids.windowID = ids.threadID + ":0"
		return ids
	}

	return nil
}

// extractClientSessionID 从请求头中提取客户端原始的会话标识。
// 优先取 session-id（连字符形式，Codex CLI 标准），回退到 session_id（下划线形式）。
// 返回的值尚未被 isolateOpenAISessionID 改写，是客户端的真实标识。
func extractClientSessionID(h http.Header) string {
	if v := strings.TrimSpace(h.Get("session-id")); v != "" {
		return v
	}
	return strings.TrimSpace(h.Get("session_id"))
}

// resolveCodexFingerprintIDsFromRequest 从客户端原始请求头中提取 session-id，
// 结合账号配置一次性解析收敛 ID 集合。调用方应将返回的 ids 同时传给
// applyCodexFingerprintHeaders 和 applyCodexFingerprintClientMetadata。
// 不带重试轮次与过载 epoch（等价于 attempt=0/epoch=0）。
func resolveCodexFingerprintIDsFromRequest(account *Account, clientHeaders http.Header) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	mode := GetCodexFingerprintMode(account)
	if mode == codexFingerprintOff {
		return nil
	}
	clientSessionID := ""
	if clientHeaders != nil {
		clientSessionID = extractClientSessionID(clientHeaders)
	}
	return resolveCodexFingerprintIDs(account, clientSessionID, mode, 0, 0)
}

// codexFingerprintIDsEntry 缓存一次解析结果，并记录它对应的 (重试轮次, 过载 epoch)。
// 同一组内头改写与体改写必须共享同一实例（turn_id 等随机字段一致性）；
// 任一分量变化时必须重新解析，否则重试或过载后的请求会带着与失败尝试完全相同的
// thread_id 出站。
type codexFingerprintIDsEntry struct {
	attempt int
	epoch   int
	ids     *codexFingerprintIDs
}

// CodexFingerprintAttempt 返回当前请求已发生的失败重试轮次（首次尝试为 0）。
func CodexFingerprintAttempt(c *gin.Context) int {
	if c == nil {
		return 0
	}
	if v, ok := c.Get(codexFingerprintAttemptContextKey); ok {
		if n, ok := v.(int); ok && n > 0 {
			return n
		}
	}
	return 0
}

// BumpCodexFingerprintAttempt 记录一次「不换账号的失败重试」：递增轮次并使已缓存的
// 指纹失效，使下一次出站请求以新的轮次重新派生会话身份。
// 返回递增后的轮次（>=1），供调用方做结构化观测。
//
// 调用点必须在确认「本次尝试会重新发同一个账号的上游请求」之后调用，
// 换账号的 failover 不需要它（account.ID 变化本身就会改变 thread_id）。
func BumpCodexFingerprintAttempt(c *gin.Context) int {
	if c == nil {
		return 0
	}
	next := CodexFingerprintAttempt(c) + 1
	c.Set(codexFingerprintAttemptContextKey, next)
	// 显式清除缓存，让下一次解析按新轮次重新派生。
	c.Set(codexFingerprintIDsContextKey, nil)
	return next
}

// resolveCodexFingerprintIDsFromContext 返回本次尝试的收敛 ID 集合，按 (重试轮次, 过载 epoch) 缓存。
// attempt 来自 BumpCodexFingerprintAttempt；epoch 来自账号+客户端会话的过载窗口。
// 两者变化时自动重新派生 thread_id。
func resolveCodexFingerprintIDsFromContext(c *gin.Context, account *Account) *codexFingerprintIDs {
	if c == nil || account == nil {
		return nil
	}
	attempt := CodexFingerprintAttempt(c)
	var headers http.Header
	if c.Request != nil {
		headers = c.Request.Header
	}
	clientSessionID := extractClientSessionID(headers)
	// 过载 epoch：客户端跨请求重试时，轮次归零但 epoch 仍在窗口内，
	// 保证下一条请求不会拿到与失败尝试相同的 thread-id。
	epoch := openAICodexOverloadSessionEpoch(account.ID, clientSessionID)
	if existing, ok := c.Get(codexFingerprintIDsContextKey); ok {
		if entry, ok := existing.(*codexFingerprintIDsEntry); ok && entry != nil && entry.attempt == attempt && entry.epoch == epoch {
			return entry.ids
		}
	}
	mode := GetCodexFingerprintMode(account)
	// attempt/epoch 只参与 thread_id 派生；两者均为 0 时与 resolveCodexFingerprintIDsFromRequest 等价。
	ids := resolveCodexFingerprintIDs(account, clientSessionID, mode, attempt, epoch)
	c.Set(codexFingerprintIDsContextKey, &codexFingerprintIDsEntry{attempt: attempt, epoch: epoch, ids: ids})
	return ids
}

// applyCodexFingerprintJSONBytes 用于原生透传和 WebSocket payload；
// JSON 无效时保持原始字节，不把收敛功能变成新的请求失败点。
func applyCodexFingerprintJSONBytes(body []byte, ids *codexFingerprintIDs) []byte {
	if len(body) == 0 || ids == nil {
		return body
	}
	decoded := make(map[string]any)
	if err := json.Unmarshal(body, &decoded); err != nil {
		return body
	}
	if !applyCodexFingerprintClientMetadata(decoded, ids) {
		return body
	}
	rebuilt, err := json.Marshal(decoded)
	if err != nil {
		return body
	}
	return rebuilt
}

func nextCodexFingerprintTurnIDs(base *codexFingerprintIDs) *codexFingerprintIDs {
	if base == nil || base.mode == codexFingerprintDevice {
		return base
	}
	next := *base
	next.turnID = uuid.Must(uuid.NewV7()).String()
	return &next
}

// applyCodexFingerprintHeaders 按预计算的收敛 ID 改写出站 HTTP 头中的设备指纹。
// 在 buildUpstreamRequest 的白名单透传之后、enforceCodexIdentityHeaders 之前调用。
func applyCodexFingerprintHeaders(h http.Header, ids *codexFingerprintIDs) {
	if h == nil || ids == nil {
		return
	}

	// installation_id **只写进 x-codex-turn-metadata**，不再单独发 x-codex-installation-id 头：
	// 实测（2026-09-22 全量抓包）App / TUI / exec 三个入口都把它放在 turn-metadata 里，
	// 没有任何一个客户端发这个独立头。单独发它会凭空多出一个客户端从不产生的头。
	if ids.mode == codexFingerprintDevice {
		rewriteCodexTurnMetadata(h, ids, map[string]any{
			"installation_id": ids.installationID,
		})
		return
	}

	// session / full 模式：改写所有相关头
	if parentThreadID := strings.TrimSpace(h.Get("x-codex-parent-thread-id")); parentThreadID != "" {
		h.Set("x-codex-parent-thread-id", rewriteCodexThreadRelationshipID("parent_thread_id", parentThreadID, ids))
	}
	h.Set("x-codex-window-id", ids.windowID)
	h.Set("x-client-request-id", ids.threadID)
	// 对齐官方 Codex：只写连字符形式，不再写下划线兼容头。
	h.Set("session-id", ids.sessionID)
	h.Set("thread-id", ids.threadID)

	rewriteCodexTurnMetadata(h, ids, map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": time.Now().UnixMilli(),
	})
}

func rewriteCodexTurnMetadata(h http.Header, ids *codexFingerprintIDs, fields map[string]any) {
	raw := strings.TrimSpace(h.Get("x-codex-turn-metadata"))
	if raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return
	}
	rewriteCodexRelationshipFields(metadata, ids)
	for k, v := range fields {
		metadata[k] = v
	}
	rebuilt, err := json.Marshal(metadata)
	if err == nil {
		h.Set("x-codex-turn-metadata", string(rebuilt))
	}
}

// rewriteCodexRelationshipFields 保留 Codex 的父子/fork 拓扑，但将关系 ID
// 改写为账号命名空间内的稳定假名。仅改写已存在且非空的字段。
func rewriteCodexRelationshipFields(metadata map[string]any, ids *codexFingerprintIDs) {
	if metadata == nil || ids == nil || ids.mode == codexFingerprintDevice {
		return
	}
	for _, key := range []string{"parent_thread_id", "forked_from_thread_id"} {
		original, _ := metadata[key].(string)
		if strings.TrimSpace(original) == "" {
			continue
		}
		metadata[key] = rewriteCodexThreadRelationshipID(key, original, ids)
	}
	for _, key := range []string{"root_turn_id", "parent_turn_id"} {
		original, _ := metadata[key].(string)
		if strings.TrimSpace(original) != "" {
			metadata[key] = deriveStableUUIDv4(fmt.Sprintf("codex:%s:v1:%d:%s", key, ids.accountID, original))
		}
	}
}

func rewriteCodexThreadRelationshipID(key string, original string, ids *codexFingerprintIDs) string {
	if ids == nil || strings.TrimSpace(original) == "" {
		return original
	}
	if ids.mode == codexFingerprintFull {
		return ids.threadID
	}
	return deriveStableUUIDv4(fmt.Sprintf("codex:%s:v1:%d:%s", key, ids.accountID, original))
}

// applyCodexFingerprintClientMetadata 按预计算的收敛 ID 改写请求体中的 client_metadata。
// 使用与头改写相同的 ids 实例，确保 turn_id 等随机字段一致。
func applyCodexFingerprintClientMetadata(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil || ids == nil {
		return false
	}

	existing, _ := reqBody["client_metadata"].(map[string]any)
	if existing == nil {
		existing = make(map[string]any)
	}

	modified := false

	if ids.installationID != "" {
		existing["x-codex-installation-id"] = ids.installationID
		modified = true
	}

	if ids.mode == codexFingerprintDevice {
		rewriteClientMetadataEmbeddedTurnMetadata(existing, ids, map[string]any{
			"installation_id": ids.installationID,
		})
		if modified {
			reqBody["client_metadata"] = existing
		}
		return modified
	}

	// session / full 模式
	existing["session_id"] = ids.sessionID
	existing["thread_id"] = ids.threadID
	existing["turn_id"] = ids.turnID
	existing["x-codex-window-id"] = ids.windowID
	rewriteCodexRelationshipFields(existing, ids)

	rewriteClientMetadataEmbeddedTurnMetadata(existing, ids, map[string]any{
		"installation_id":         ids.installationID,
		"session_id":              ids.sessionID,
		"thread_id":               ids.threadID,
		"turn_id":                 ids.turnID,
		"window_id":               ids.windowID,
		"turn_started_at_unix_ms": time.Now().UnixMilli(),
	})

	reqBody["client_metadata"] = existing
	return true
}

// rewriteClientMetadataEmbeddedTurnMetadata 改写 client_metadata 中内嵌的
// x-codex-turn-metadata JSON 字符串里的指定字段。
func rewriteClientMetadataEmbeddedTurnMetadata(clientMetadata map[string]any, ids *codexFingerprintIDs, fields map[string]any) {
	raw, ok := clientMetadata["x-codex-turn-metadata"].(string)
	if !ok || raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return
	}
	rewriteCodexRelationshipFields(metadata, ids)
	for k, v := range fields {
		metadata[k] = v
	}
	if rebuilt, err := json.Marshal(metadata); err == nil {
		clientMetadata["x-codex-turn-metadata"] = string(rebuilt)
	}
}

// requestClientSessionID 从 gin 请求头提取客户端原始会话标识，供过载 epoch 使用。
// 与 extractClientSessionID 同义，但接受 *gin.Context 以简化调用点。
func requestClientSessionID(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return extractClientSessionID(c.Request.Header)
}

// codexWindowIDSuffix 返回 window-id 中 `<thread>:` 之后的部分（如 `:3`）。
// 无法解析为 `:<数字>` 时回退 `:0`，与官方客户端的首窗口形态一致。
func codexWindowIDSuffix(raw string) string {
	idx := strings.LastIndexByte(strings.TrimSpace(raw), ':')
	if idx < 0 {
		return ":0"
	}
	suffix := strings.TrimSpace(raw[idx+1:])
	if suffix == "" {
		return ":0"
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return ":0"
		}
	}
	return ":" + suffix
}

// alignCodexTurnMetadataWithIsolatedSession 把会话元数据对齐到刚刚写进会话头的隔离值。
//
// isolatedSession / isolatedThread 就是调用方写入 session-id / thread-id 头的值；
// isolatedThread 为空时回退到 isolatedSession（仅隔离 session 的场景）。
// isolate 用于以同一套派生规则隔离父子线程字段，与头保持一致。
//
// 为什么必须做（2026-09-22 实测抓包）：
//   - 隔离逻辑只改写 session-id / thread-id / x-client-request-id 头，而
//     x-codex-turn-metadata 里仍留着**客户端原始** session_id / thread_id / window_id，
//     x-codex-window-id 头也仍是客户端原值。结果是：
//     ① 上游能从元数据头读到已被“隔离”掉的原始 thread，隔离被绕过；
//     ② 同一请求的头与元数据互相矛盾（一个说隔离值，一个说原值）。
//   - 该矛盾在 off / device 指纹模式下出现；session / full 会整体重写，不受影响。
func alignCodexTurnMetadataWithIsolatedSession(h http.Header, isolatedSession, isolatedThread string, isolate func(string) string) {
	if h == nil {
		return
	}
	isolatedSession = strings.TrimSpace(isolatedSession)
	isolatedThread = strings.TrimSpace(isolatedThread)
	if isolatedSession == "" && isolatedThread == "" {
		return
	}
	if isolatedThread == "" {
		isolatedThread = isolatedSession
	}
	if isolatedSession == "" {
		isolatedSession = isolatedThread
	}

	// window-id 与 thread 同源：保留客户端原有窗口序号后缀。
	if raw := strings.TrimSpace(h.Get("x-codex-window-id")); raw != "" {
		h.Set("x-codex-window-id", isolatedThread+codexWindowIDSuffix(raw))
	}

	raw := strings.TrimSpace(h.Get("x-codex-turn-metadata"))
	if raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		// 元数据不可解析时不破坏原始字节：宁可不改，也不要把身份改写变成新的失败点。
		return
	}

	changed := false
	for key, value := range map[string]string{"session_id": isolatedSession, "thread_id": isolatedThread} {
		if original, _ := metadata[key].(string); strings.TrimSpace(original) != "" {
			metadata[key] = value
			changed = true
		}
	}
	if original, _ := metadata["window_id"].(string); strings.TrimSpace(original) != "" {
		metadata["window_id"] = isolatedThread + codexWindowIDSuffix(original)
		changed = true
	}
	// 父子线程同样指向客户端真实线程；不隔离就等于从头元数据反推出原始线程。
	if isolate != nil {
		for _, key := range []string{"parent_thread_id", "forked_from_thread_id"} {
			original, _ := metadata[key].(string)
			if strings.TrimSpace(original) == "" {
				continue
			}
			if isolated := strings.TrimSpace(isolate(original)); isolated != "" {
				metadata[key] = isolated
				changed = true
			}
		}
	}
	if !changed {
		return
	}
	if rebuilt, err := json.Marshal(metadata); err == nil {
		h.Set("x-codex-turn-metadata", string(rebuilt))
	}
}
