package service

// Mirasim 额度窗口：解析上游 `GET /v1/limits` 响应、归一化窗口、推断顶层状态，
// 并把 `5h` / `7d` 映射到既有展示字段（`five_hour` / `seven_day`）。
//
// 关键约束（改动前请先读）：
//   - 百分比口径逐字对齐桌面客户端 `server.cjs` 的 `Nir` / `Qir`：
//     `usedPercent = clamp(round(used/budget*100*10)/10, 0, 100)`（**保留 1 位小数**），
//     且 `usedPercent >= 99` 时上取为 `100`；随后
//     `remainingPercent = max(0, round((100-usedPercent)*10)/10)`。
//     注意 spec 正文把 `clamp(round(...))` 写成了不带小数的形式，实现按客户端口径；
//   - `budget` / `used` 与客户端一致地做 `Number()` 强转（数字字符串也接受），
//     `name` 则严格要求是非空字符串且**不** trim / 转小写（`label` 保留原值）；
//   - `budget === 0` 表示不限量：不产百分比、状态固定 `allowed`、**不**参与
//     `five_hour` / `seven_day` 映射；
//   - `model_scoped: true` 的窗口只进 `mirasim_windows`，MUST NOT 参与映射，
//     MUST NOT 决定顶层状态（顶层状态先看非 model_scoped 窗口，全为空才回退全部）；
//   - `reset_at` 是 unix 秒；`<= 0` / 非法值一律输出空（客户端 `uBn` 同口径）；
//   - 本文件不发起网络请求（拉取在 `mirasim_relay.go`，缓存与门面在
//     `mirasim_metadata.go`）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	// mirasimUnlimitedWindow 是 budget == 0 的窗口状态。
	mirasimWindowStatusAllowed = "allowed"
	// mirasimWindowStatusWarning 触发阈值（客户端 `pBn`：>= 80）。
	mirasimWindowWarningPercent = 80
	// mirasimWindowStatusLimitReached 触发阈值（客户端 `pBn`：>= 100）。
	mirasimWindowLimitReachedPercent = 100
	// mirasimWindowCapPercent 是客户端 `Qir` 的上取阈值（Vpo = 99）。
	mirasimWindowCapPercent = 99

	mirasimWindowKeyFiveHour = "five_hour"
	mirasimWindowKeySevenDay = "seven_day"
)

// MirasimQuotaWindow 是归一化后的单个额度窗口。
type MirasimQuotaWindow struct {
	Name             string   `json:"name"`                        // 上游原始 name（不做 trim / lowercase）
	Label            string   `json:"label"`                       // 展示用标签，等于 Name
	Budget           float64  `json:"budget"`                      // 额度上限；0 表示不限量
	Used             float64  `json:"used"`                        // 已用量
	Unlimited        bool     `json:"unlimited,omitempty"`         // budget == 0
	Utilization      *float64 `json:"utilization,omitempty"`       // 使用率百分比（1 位小数）
	RemainingPercent *float64 `json:"remaining_percent,omitempty"` // 剩余百分比
	Status           string   `json:"status"`                      // allowed / warning / limit_reached
	ResetAt          string   `json:"reset_at,omitempty"`          // RFC3339（UTC）
	ModelScoped      bool     `json:"model_scoped,omitempty"`      // 仅模型级窗口
	WindowKey        string   `json:"window_key,omitempty"`        // five_hour / seven_day（未映射时为空）
}

// mirasimLimitsSnapshot 是一次 `/v1/limits` 归一化结果。
type mirasimLimitsSnapshot struct {
	Windows    []MirasimQuotaWindow
	Paid       *bool // 上游必须显式给布尔；否则 nil
	Degraded   bool  // 上游显式标 degraded 时为 true
	Status     string
	CapturedAt time.Time
}

// parseMirasimLimits 解析 `/v1/limits` 响应；没有任何有效窗口时返回错误
// （对应 spec 的“降级错误”）。
func parseMirasimLimits(body []byte) (*mirasimLimitsSnapshot, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("mirasim limits: 响应解析失败: %w", err)
	}
	if root == nil {
		return nil, errors.New("mirasim limits: 响应根不是 JSON 对象")
	}

	snapshot := &mirasimLimitsSnapshot{CapturedAt: time.Now().UTC()}
	if rawPaid, ok := root["paid"]; ok {
		// 客户端：typeof body.paid === 'boolean' ? body.paid : null
		var paid bool
		if err := json.Unmarshal(rawPaid, &paid); err == nil {
			snapshot.Paid = &paid
		}
	}
	if rawDegraded, ok := root["degraded"]; ok {
		// 客户端用真值判定；上游实际只发布尔，这里只认 true。
		var degraded bool
		if err := json.Unmarshal(rawDegraded, &degraded); err == nil && degraded {
			snapshot.Degraded = true
		}
	}

	var entries []json.RawMessage
	if rawWindows, ok := root["windows"]; ok {
		if err := json.Unmarshal(rawWindows, &entries); err != nil {
			entries = nil
		}
	}
	for _, entryRaw := range entries {
		window, ok := mirasimDecodeQuotaWindow(entryRaw)
		if !ok {
			continue
		}
		snapshot.Windows = append(snapshot.Windows, window)
	}
	if len(snapshot.Windows) == 0 {
		return nil, errors.New("mirasim limits: 响应中没有有效的额度窗口")
	}
	snapshot.Status = mirasimTopLevelWindowStatus(snapshot.Windows)
	return snapshot, nil
}

// mirasimDecodeQuotaWindow 校验并归一化单个窗口。
func mirasimDecodeQuotaWindow(raw json.RawMessage) (MirasimQuotaWindow, bool) {
	var window MirasimQuotaWindow
	fields := mirasimJSONObject(raw)
	if fields == nil {
		return window, false
	}
	name, ok := mirasimJSONStringField(fields, "name")
	if !ok || name == "" {
		return window, false
	}
	budget, ok := mirasimJSONNumberCoerce(fields["budget"])
	if !ok || budget < 0 {
		return window, false
	}
	used, ok := mirasimJSONNumberCoerce(fields["used"])
	if !ok {
		return window, false
	}
	modelScoped, _ := mirasimJSONBoolField(fields, "model_scoped")

	window = MirasimQuotaWindow{
		Name:        name,
		Label:       name,
		Budget:      budget,
		Used:        used,
		Unlimited:   budget == 0,
		Status:      mirasimWindowStatusAllowed,
		ModelScoped: modelScoped,
	}
	if usedPercent, ok := mirasimWindowUsedPercent(used, budget); ok {
		remaining := math.Max(0, math.Round((100-usedPercent)*10)/10)
		window.Utilization = &usedPercent
		window.RemainingPercent = &remaining
		window.Status = mirasimWindowStatusFor(usedPercent)
	}
	if resetAt, ok := mirasimWindowResetAt(fields["reset_at"]); ok {
		window.ResetAt = resetAt
	}
	return window, true
}

// mirasimWindowUsedPercent 实现客户端 `Nir` + `Qir`（1 位小数，[0,100]，>= 99 上取 100）。
// budget == 0（不限量）或数值非有限时返回 ok=false。
func mirasimWindowUsedPercent(used, budget float64) (float64, bool) {
	if budget <= 0 || math.IsNaN(budget) || math.IsNaN(used) {
		return 0, false
	}
	value := used / budget * 100
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	percent := math.Max(0, math.Min(100, math.Round(value*10)/10))
	if percent >= mirasimWindowCapPercent {
		percent = 100
	}
	return percent, true
}

func mirasimWindowStatusFor(usedPercent float64) string {
	switch {
	case usedPercent >= mirasimWindowLimitReachedPercent:
		return "limit_reached"
	case usedPercent >= mirasimWindowWarningPercent:
		return "warning"
	default:
		return mirasimWindowStatusAllowed
	}
}

// mirasimTopLevelWindowStatus 与客户端 `pBn` 一致：先看非 model_scoped 窗口，
// 只有在没有任何非 model_scoped 窗口时才回退使用全部窗口。
func mirasimTopLevelWindowStatus(windows []MirasimQuotaWindow) string {
	scoped := windows
	unscoped := make([]MirasimQuotaWindow, 0, len(windows))
	for _, window := range windows {
		if !window.ModelScoped {
			unscoped = append(unscoped, window)
		}
	}
	if len(unscoped) > 0 {
		scoped = unscoped
	}
	status := mirasimWindowStatusAllowed
	for _, window := range scoped {
		if window.Status == "limit_reached" {
			return "limit_reached"
		}
		if window.Status == "warning" {
			status = "warning"
		}
	}
	return status
}

// mirasimWindowResetAt 把 unix 秒（或数字字符串）转成 RFC3339 UTC；<= 0 / 非法 → ok=false。
func mirasimWindowResetAt(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	// 客户端先 String() 再 Number()，因此数字与数字字符串都能解析。
	var asString string
	if err := json.Unmarshal(raw, &asString); err != nil {
		asString = strings.TrimSpace(string(raw))
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(asString), 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return "", false
	}
	return time.Unix(int64(seconds), 0).UTC().Format(time.RFC3339), true
}

// mirasimJSONNumberCoerce 对齐客户端 `Number()` 强转：JSON 数字与可解析的数字
// 字符串都接受，其余（含 null/布尔/对象）返回 ok=false。
func mirasimJSONNumberCoerce(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err == nil {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}
		return value, true
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err != nil {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(asString), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

// mirasimApplyLimitsSnapshot 把快照写进 UsageInfo：全部窗口进 `mirasim_windows`，
// `5h` / `7d`（budget > 0 且非 model_scoped）另外映射到 five_hour / seven_day。
func mirasimApplyLimitsSnapshot(info *UsageInfo, snapshot *mirasimLimitsSnapshot) {
	if info == nil || snapshot == nil {
		return
	}
	capturedAt := snapshot.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}
	info.UpdatedAt = &capturedAt
	info.Source = "active"
	info.MirasimWindows = snapshot.Windows
	info.MirasimPaid = snapshot.Paid
	info.MirasimDegraded = snapshot.Degraded

	now := time.Now()
	for i := range info.MirasimWindows {
		window := &info.MirasimWindows[i]
		if window.Unlimited || window.ModelScoped {
			// 不限量窗口不映射（spec）；model_scoped 窗口不参与映射。
			continue
		}
		switch window.Name {
		case "5h":
			window.WindowKey = mirasimWindowKeyFiveHour
			info.FiveHour = mirasimUsageProgressFromWindow(window, now)
		case "7d":
			window.WindowKey = mirasimWindowKeySevenDay
			info.SevenDay = mirasimUsageProgressFromWindow(window, now)
		}
	}
}

// mirasimUsageProgressFromWindow 把窗口映射到既有展示结构。
func mirasimUsageProgressFromWindow(window *MirasimQuotaWindow, now time.Time) *UsageProgress {
	if window == nil {
		return nil
	}
	progress := &UsageProgress{
		WindowLabel: window.Label,
	}
	if window.Utilization != nil {
		progress.Utilization = *window.Utilization
	}
	if window.ResetAt != "" {
		if parsed, err := time.Parse(time.RFC3339, window.ResetAt); err == nil {
			progress.ResetsAt = &parsed
			if remaining := int(parsed.Sub(now).Seconds()); remaining > 0 {
				progress.RemainingSeconds = remaining
			}
		}
	}
	return progress
}

// getMirasimUsage 拉取 mirasim 账号的额度窗口。
//
// 关键约束（改动前请先读）：只读元数据失败一律**降级**为 HTTP 200 + `error` 字段，
// 不返回 5xx，也绝不改动账号调度状态（不写 rate_limit_reset_at、不置
// 临时不可调度）——上游抖动不得让账号掉出调度池。
func (s *AccountUsageService) getMirasimUsage(ctx context.Context, account *Account, forceProbe bool) (*UsageInfo, error) {
	info := &UsageInfo{Source: "active"}
	if s == nil || s.mirasimMetadataService == nil {
		info.Error = "mirasim metadata query is not configured"
		slog.Warn("mirasim_limits_unavailable", "account_id", mirasimAccountID(account), "reason", "querier_not_configured")
		return info, nil
	}
	snapshot, err := s.mirasimMetadataService.Limits(ctx, account, forceProbe)
	if err != nil {
		info.Error = "failed to fetch mirasim usage: " + mirasimMetadataErrorCategory(err)
		slog.Warn("mirasim_limits_degraded", "account_id", mirasimAccountID(account), "category", mirasimMetadataErrorCategory(err))
		return info, nil
	}
	mirasimApplyLimitsSnapshot(info, snapshot)
	return info, nil
}

// mirasimAccountID 让日志在 account 为空时也不 panic。
func mirasimAccountID(account *Account) int64 {
	if account == nil {
		return 0
	}
	return account.ID
}
