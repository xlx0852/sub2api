package service

// Mirasim 模型清单（model roster）：解析上游 `GET /v1/model-roster` 的响应，
// 按 spec 的选择顺序挑出账号对应的 agent，并产出可用的模型 id 集合。
//
// 关键约束（改动前请先读）：
//   - 归一化口径逐条对齐桌面客户端 `server.cjs` 的 `znr` / `U1n` / `hdo` / `mdo`
//     （含取证到的 `L1n = ^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`）。字段名、trim、
//     lowercase 的每一步都必须与客户端一致，否则管理端「同步上游模型」给出的白名单
//     会与客户端实际可用的模型名对不上；
//   - `contextWindow` 必须是 JSON number 且 > 0：**刻意不做字符串强转**，
//     对齐客户端 `typeof === 'number'` 的严格判定（`"200000"` 这种条目要丢弃）；
//   - agent 选择顺序：`extra.mirasim_roster_agent` → `default: true` 的首个 agent
//     → 名称为 `claude` 的 agent → 字典序首个 agent。`default` 在客户端里是
//     **条目级**标记（组内首个 `default: true` 条目胜出），这里保持同样语义，
//     组级 `Default` 只表示「该 agent 含 default 条目」；
//   - agent 的文档顺序有意义（决定“首个 default agent”），因此 `agents` 用
//     `json.Decoder` 顺序解析，不能退化成 map 遍历；
//   - 本文件不发起任何网络请求：拉取在 `mirasim_relay.go` 的只读元数据通道里，
//     缓存与服务门面在 `mirasim_metadata.go`。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// mirasimRosterCLIMinVersionPattern 来自客户端 `L1n`，用于校验顶层 `cli` 的版本号。
const mirasimRosterCLIMinVersionPattern = `^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`

var mirasimRosterCLIMinVersionRegexp = regexp.MustCompile(mirasimRosterCLIMinVersionPattern)

// mirasimRosterModel 是一个通过校验的模型条目。
type mirasimRosterModel struct {
	ID            string  // trim + lowercase
	Label         string  // trim + lowercase；缺失时回退 ID
	ContextWindow float64 // JSON number 且 > 0；保持原值不做整型化（同客户端）
	Default       bool    // 所在 agent 内首个 default: true
}

// mirasimRosterAgent 是一个通过校验的 agent 分组。
type mirasimRosterAgent struct {
	Name    string
	Models  []mirasimRosterModel
	Default bool // 组内是否存在 default: true 条目
}

// mirasimRosterModelMeta 是顶层 `models` 条目的元数据子集。
// 客户端 `U1n` 还会解析 maxOutput / autoCompactRatio / effort / adaptive / pricing，
// 本实现没有消费方，按需只保留 label 与 contextWindow。
type mirasimRosterModelMeta struct {
	Model         string
	Label         string
	ContextWindow float64
}

// mirasimRosterCLIEntry 是顶层 `cli` 条目（客户端 `mdo` 的子集）。
type mirasimRosterCLIEntry struct {
	MinVersion string
	Platforms  map[string]string
}

// mirasimModelRoster 是归一化后的模型清单。
type mirasimModelRoster struct {
	Version string
	Agents  map[string]*mirasimRosterAgent // key 为原始 agent 名（客户端同样不改写 key）
	Order   []string                       // agent 的文档顺序
	Models  map[string]mirasimRosterModelMeta
	CLI     map[string]mirasimRosterCLIEntry
}

// parseMirasimModelRoster 解析并校验 roster 响应。
// 没有任何有效 agent 条目时返回错误（对应 spec 的“无有效条目即失败”）。
func parseMirasimModelRoster(body []byte) (*mirasimModelRoster, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	token, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("mirasim roster: 响应解析失败: %w", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("mirasim roster: 响应根不是 JSON 对象")
	}

	roster := &mirasimModelRoster{Agents: map[string]*mirasimRosterAgent{}}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("mirasim roster: 响应解析失败: %w", err)
		}
		key, _ := keyToken.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("mirasim roster: 响应解析失败: %w", err)
		}
		switch key {
		case "version":
			// 客户端要求 version 为非空字符串，否则整份 roster 视为无效；本 change 的
			// spec 未把 version 列为校验项，这里只作展示信息（缺失不影响解析）。
			if value, ok := mirasimJSONStringValue(raw); ok {
				roster.Version = strings.TrimSpace(value)
			}
		case "agents":
			agents, order := mirasimDecodeRosterAgents(raw)
			roster.Agents = agents
			roster.Order = order
		case "models":
			roster.Models = mirasimDecodeRosterModelsMeta(raw)
		case "cli":
			roster.CLI = mirasimDecodeRosterCLI(raw)
		}
	}
	// 吃掉收尾的 '}'；失败说明响应被截断。
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("mirasim roster: 响应解析失败: %w", err)
	}
	if len(roster.Agents) == 0 {
		return nil, errors.New("mirasim roster: 响应中没有有效的 agent 模型条目")
	}
	return roster, nil
}

// mirasimDecodeRosterAgents 顺序解析 `agents`：key 保持原样，值必须是数组。
// 非法 agent（值不是数组、或组内没有有效条目）直接丢弃。
func mirasimDecodeRosterAgents(raw json.RawMessage) (map[string]*mirasimRosterAgent, []string) {
	agents := map[string]*mirasimRosterAgent{}
	var order []string

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	token, err := dec.Token()
	if err != nil {
		return agents, order
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return agents, order
	}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return agents, order
		}
		name, _ := keyToken.(string)
		var entries json.RawMessage
		if err := dec.Decode(&entries); err != nil {
			return agents, order
		}
		models := mirasimDecodeRosterModels(entries)
		if len(models) == 0 {
			continue
		}
		if _, exists := agents[name]; exists {
			// 重复 key：首个胜出，保持文档顺序稳定（对齐客户端“首个 default 胜出”）。
			continue
		}
		agent := &mirasimRosterAgent{Name: name, Models: models}
		for _, model := range models {
			if model.Default {
				agent.Default = true
				break
			}
		}
		agents[name] = agent
		order = append(order, name)
	}
	return agents, order
}

// mirasimDecodeRosterModels 解析一个模型数组，逐条校验并归一化。
// 非数组（含 JSON null）返回 nil。
func mirasimDecodeRosterModels(raw json.RawMessage) []mirasimRosterModel {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil
	}
	models := make([]mirasimRosterModel, 0, len(entries))
	defaultSeen := false
	for _, entryRaw := range entries {
		model, ok := mirasimDecodeRosterModel(entryRaw)
		if !ok {
			continue
		}
		// 客户端只在组内第一个 default 条目上保留标记。
		if model.Default {
			if defaultSeen {
				model.Default = false
			} else {
				defaultSeen = true
			}
		}
		models = append(models, model)
	}
	return models
}

// mirasimDecodeRosterModel 校验单条条目：id 非空字符串、contextWindow 为 >0 有限数。
func mirasimDecodeRosterModel(raw json.RawMessage) (mirasimRosterModel, bool) {
	var entry mirasimRosterModel
	fields := mirasimJSONObject(raw)
	if fields == nil {
		return entry, false
	}
	id, ok := mirasimJSONStringField(fields, "id")
	id = strings.ToLower(strings.TrimSpace(id))
	if !ok || id == "" {
		return entry, false
	}
	contextWindow, ok := mirasimJSONNumberField(fields, "contextWindow")
	if !ok || contextWindow <= 0 {
		return entry, false
	}
	label := id
	if rawLabel, ok := mirasimJSONStringField(fields, "label"); ok {
		if trimmed := strings.ToLower(strings.TrimSpace(rawLabel)); trimmed != "" {
			label = trimmed
		}
	}
	defaultFlag, _ := mirasimJSONBoolField(fields, "default")
	return mirasimRosterModel{
		ID:            id,
		Label:         label,
		ContextWindow: contextWindow,
		Default:       defaultFlag,
	}, true
}

// mirasimDecodeRosterModelsMeta 解析顶层 `models`：key trim + lowercase，
// 值只保留 label / contextWindow；两个字段都不可用时丢弃该条。
func mirasimDecodeRosterModelsMeta(raw json.RawMessage) map[string]mirasimRosterModelMeta {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	token, err := dec.Token()
	if err != nil {
		return nil
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil
	}
	models := map[string]mirasimRosterModelMeta{}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return nil
		}
		name, _ := keyToken.(string)
		var entryRaw json.RawMessage
		if err := dec.Decode(&entryRaw); err != nil {
			return nil
		}
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		fields := mirasimJSONObject(entryRaw)
		if fields == nil {
			continue
		}
		meta := mirasimRosterModelMeta{Model: key}
		if rawLabel, ok := mirasimJSONStringField(fields, "label"); ok {
			meta.Label = strings.ToLower(strings.TrimSpace(rawLabel))
		}
		if contextWindow, ok := mirasimJSONNumberField(fields, "contextWindow"); ok && contextWindow > 0 {
			meta.ContextWindow = contextWindow
		}
		if meta.Label == "" && meta.ContextWindow <= 0 {
			continue
		}
		models[key] = meta
	}
	if len(models) == 0 {
		return nil
	}
	return models
}

// mirasimDecodeRosterCLI 解析顶层 `cli`：key 保持原样（同客户端 `mdo`），
// `minVersion` 必须匹配版本号正则；`platforms` 的 key 小写、值同样要匹配正则。
func mirasimDecodeRosterCLI(raw json.RawMessage) map[string]mirasimRosterCLIEntry {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	token, err := dec.Token()
	if err != nil {
		return nil
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil
	}
	entries := map[string]mirasimRosterCLIEntry{}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return nil
		}
		name, _ := keyToken.(string)
		var entryRaw json.RawMessage
		if err := dec.Decode(&entryRaw); err != nil {
			return nil
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		fields := mirasimJSONObject(entryRaw)
		if fields == nil {
			continue
		}
		entry := mirasimRosterCLIEntry{}
		if minVersion, ok := mirasimJSONStringField(fields, "minVersion"); ok {
			if trimmed := strings.TrimSpace(minVersion); mirasimRosterCLIMinVersionRegexp.MatchString(trimmed) {
				entry.MinVersion = trimmed
			}
		}
		if platformsRaw, ok := fields["platforms"]; ok {
			if platforms := mirasimJSONObject(platformsRaw); platforms != nil {
				for platform, versionRaw := range platforms {
					version, ok := mirasimJSONStringValue(versionRaw)
					if !ok {
						continue
					}
					version = strings.TrimSpace(version)
					if !mirasimRosterCLIMinVersionRegexp.MatchString(version) {
						continue
					}
					if entry.Platforms == nil {
						entry.Platforms = map[string]string{}
					}
					entry.Platforms[strings.ToLower(strings.TrimSpace(platform))] = version
				}
			}
		}
		if entry.MinVersion == "" && len(entry.Platforms) == 0 {
			continue
		}
		entries[name] = entry
	}
	if len(entries) == 0 {
		return nil
	}
	return entries
}

// selectAgent 按 spec 选择顺序挑出 agent；解析成功过的 roster 必然能选出一个
// （Agents 非空即 Order 非空），返回 error 只用于防御性兜底。
func (r *mirasimModelRoster) selectAgent(configured string) (*mirasimRosterAgent, error) {
	if r == nil || len(r.Agents) == 0 {
		return nil, errors.New("mirasim roster: 模型清单为空")
	}
	if wanted := strings.ToLower(strings.TrimSpace(configured)); wanted != "" {
		if agent := r.agentByName(wanted); agent != nil {
			return agent, nil
		}
		// 显式配置不匹配时静默回退：账号配置错误不应让同步整体失败。
	}
	for _, name := range r.Order {
		if agent := r.Agents[name]; agent != nil && agent.Default {
			return agent, nil
		}
	}
	if agent := r.agentByName("claude"); agent != nil {
		return agent, nil
	}
	names := r.agentNames()
	return r.Agents[names[0]], nil
}

// agentByName 用 trim + lowercase 后的名字查找 agent（agent key 原样保留）。
func (r *mirasimModelRoster) agentByName(name string) *mirasimRosterAgent {
	if r == nil {
		return nil
	}
	wanted := strings.ToLower(strings.TrimSpace(name))
	if wanted == "" {
		return nil
	}
	for _, candidate := range r.Order {
		if strings.ToLower(strings.TrimSpace(candidate)) == wanted {
			return r.Agents[candidate]
		}
	}
	return nil
}

// agentNames 返回 agent 名的字典序列表（稳定，用于“字典序首个”）。
func (r *mirasimModelRoster) agentNames() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.Agents))
	for name := range r.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// mirasimRosterAgentOverride 读取账号上的 agent 覆盖配置（extra.mirasim_roster_agent）。
// 缺失/非字符串时返回空串，交给 selectAgent 走 default/claude/字典序回退。
func mirasimRosterAgentOverride(account *Account) string {
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.GetExtraString(mirasimExtraRosterAgent))
}

// mirasimRosterModelIDsForAccount 是「同步上游模型」的产出：选出 agent 后给出
// 去重升序的模型 id 列表。所选 agent 没有任何可用模型时返回错误。
//
// 关键约束（改动前请先读）：这里只回模型 id，**不**回 agent 内的 label /
// contextWindow，也不改写调用方的账号配置——同步动作必须是只读的。
func mirasimRosterModelIDsForAccount(roster *mirasimModelRoster, account *Account) ([]string, error) {
	if roster == nil {
		return nil, errors.New("mirasim roster: 模型清单为空")
	}
	agent, err := roster.selectAgent(mirasimRosterAgentOverride(account))
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, errors.New("mirasim roster: 没有可用的 agent")
	}
	ids := mirasimRosterModelIDs(agent.Models)
	if len(ids) == 0 {
		return nil, errors.New("mirasim roster: 所选 agent 没有可用模型")
	}
	return ids, nil
}

// modelIDs 返回去重后的模型 id 升序列表（解析阶段已 trim + lowercase）。
func mirasimRosterModelIDs(models []mirasimRosterModel) []string {
	seen := make(map[string]struct{}, len(models))
	ids := make([]string, 0, len(models))
	for _, model := range models {
		if _, ok := seen[model.ID]; ok {
			continue
		}
		seen[model.ID] = struct{}{}
		ids = append(ids, model.ID)
	}
	sort.Strings(ids)
	return ids
}

// mirasimJSONObject 把原始 JSON 解成字段表；非对象（含 null/数组）返回 nil。
func mirasimJSONObject(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	return fields
}

// mirasimJSONStringField 只在值确为 JSON 字符串时返回 ok（不做数字/布尔强转）。
func mirasimJSONStringField(fields map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := fields[key]
	if !ok {
		return "", false
	}
	return mirasimJSONStringValue(raw)
}

// mirasimJSONStringValue 只在原始值确为 JSON 字符串时返回 ok。
func mirasimJSONStringValue(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

// mirasimJSONNumberField 只在值确为有限数字时返回 ok（字符串 "1" 不算）。
func mirasimJSONNumberField(fields map[string]json.RawMessage, key string) (float64, bool) {
	raw, ok := fields[key]
	if !ok {
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// mirasimJSONBoolField 只在值确为 JSON 布尔时返回 ok。
func mirasimJSONBoolField(fields map[string]json.RawMessage, key string) (bool, bool) {
	raw, ok := fields[key]
	if !ok {
		return false, false
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false
	}
	return value, true
}
