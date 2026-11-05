package modelcatalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RegistrySnapshot 是数据库注册表（models / model_aliases / 默认价）的一次性快照。
//
// 它只承载"从文件搬进库"的那部分：模型身份与能力、别名与上游映射、平台默认价。
// ui_presets 与 image_defaults 仍来自文件，Overlay 会保留它们。
type RegistrySnapshot struct {
	Models        []RegistryModel
	Aliases       []RegistryAlias
	DefaultPrices map[string]PriceEntry
}

// RegistryModel 对应 models 表的一行。
type RegistryModel struct {
	ID               string
	Platform         string
	DisplayName      string
	Family           string
	OwnedBy          string
	ContextWindow    int
	MaxContextWindow int
	Media            map[string]bool
	IsReasoning      bool
	Enabled          bool
	Retired          bool
	IsDefaultChat    bool
	IsDefaultTest    bool
	UpstreamID       string
}

// RegistryAlias 对应 model_aliases 表的一行；Kind 决定它落到 catalog 的哪个映射里。
type RegistryAlias struct {
	Alias    string
	Platform string
	ModelID  string
	Kind     string // alias | id_override | id_reverse | retired
}

// ApplyRegistry 用注册表快照重建全局 catalog 的 models / aliases / 默认价，并返回与
// 覆盖前文件的差异（供 dual 模式打点）。ui_presets 与 image_defaults 保持文件值。
func ApplyRegistry(snap *RegistrySnapshot) ([]string, error) {
	if snap == nil {
		return nil, fmt.Errorf("empty registry snapshot")
	}
	globalMu.RLock()
	current := global
	globalMu.RUnlock()
	if current == nil {
		return nil, fmt.Errorf("catalog not loaded")
	}

	nextPtr, err := buildOverlay(current, snap)
	if err != nil {
		return nil, err
	}
	diffs := diffCatalogs(current, nextPtr)

	globalMu.Lock()
	global = nextPtr
	globalMu.Unlock()
	return diffs, nil
}

// buildOverlay 根据当前 catalog + 注册表快照构造一份新的 catalog（不触碰全局状态）。
func buildOverlay(current *Catalog, snap *RegistrySnapshot) (*Catalog, error) {
	if current == nil || snap == nil {
		return nil, fmt.Errorf("nil catalog or snapshot")
	}
	next := *current
	next.Platforms = make(map[string]PlatformConfig, len(current.Platforms))
	for name, cfg := range current.Platforms {
		next.Platforms[name] = cfg
	}

	// 1) 按平台重建模型清单与默认值
	byPlatform := map[string][]RegistryModel{}
	retiredByPlatform := map[string][]string{}
	mappingByPlatform := map[string]map[string]string{}
	defaults := map[string]string{} // platform → default chat / test
	defaultTests := map[string]string{}
	for _, m := range snap.Models {
		platform := strings.TrimSpace(m.Platform)
		if platform == "" {
			// 只有价格的兼容 id：不进入模型清单，但保留默认价
			continue
		}
		if m.Retired {
			retiredByPlatform[platform] = append(retiredByPlatform[platform], m.ID)
			continue
		}
		if !m.Enabled {
			continue
		}
		byPlatform[platform] = append(byPlatform[platform], m)
		if m.IsDefaultChat {
			defaults[platform] = m.ID
		}
		if m.IsDefaultTest {
			defaultTests[platform] = m.ID
		}
		if strings.TrimSpace(m.UpstreamID) != "" && m.UpstreamID != m.ID {
			if mappingByPlatform[platform] == nil {
				mappingByPlatform[platform] = map[string]string{}
			}
			mappingByPlatform[platform][m.ID] = m.UpstreamID
		}
	}

	for platform, models := range byPlatform {
		cfg := next.Platforms[platform]
		sort.SliceStable(models, func(i, j int) bool { return models[i].ID < models[j].ID })
		entries := make([]ModelEntry, 0, len(models))
		for _, m := range models {
			entries = append(entries, ModelEntry{
				ID:               m.ID,
				DisplayName:      m.DisplayName,
				Object:           "model",
				OwnedBy:          m.OwnedBy,
				Family:           m.Family,
				IsReasoning:      m.IsReasoning,
				Media:            m.Media,
				ContextWindow:    m.ContextWindow,
				MaxContextWindow: m.MaxContextWindow,
			})
		}
		cfg.Models = entries
		cfg.RetiredIDs = retiredByPlatform[platform]
		cfg.DefaultMapping = mappingByPlatform[platform]
		if v := defaults[platform]; v != "" {
			cfg.DefaultChatModel = v
		}
		if v := defaultTests[platform]; v != "" {
			cfg.DefaultTestModel = v
		}
		next.Platforms[platform] = cfg
	}

	// 2) 别名 → aliases / id_overrides / id_reverse_overrides
	for platform, cfg := range next.Platforms {
		cfg.Aliases = map[string]string{}
		cfg.IDOverrides = map[string]string{}
		cfg.IDReverseOverrides = map[string]string{}
		next.Platforms[platform] = cfg
	}
	for _, a := range snap.Aliases {
		cfg, ok := next.Platforms[a.Platform]
		if !ok {
			continue
		}
		switch a.Kind {
		case "id_override":
			cfg.IDOverrides[a.Alias] = a.ModelID
		case "id_reverse":
			cfg.IDReverseOverrides[a.Alias] = a.ModelID
		case "retired":
			// retired id 已在 models.retired 处理；这里只补一个自映射，便于解析
			cfg.Aliases[a.Alias] = a.ModelID
		default:
			cfg.Aliases[a.Alias] = a.ModelID
		}
		next.Platforms[a.Platform] = cfg
	}

	// 3) 平台默认价 → fallback_pricing
	if len(snap.DefaultPrices) > 0 {
		prices := make(map[string]PriceEntry, len(snap.DefaultPrices))
		for model, entry := range snap.DefaultPrices {
			prices[model] = entry
		}
		next.FallbackPricing = prices
	}

	return &next, nil
}

// DiffRegistry 只比较、不改动全局 catalog，供 dual 模式使用。
func DiffRegistry(snap *RegistrySnapshot) ([]string, error) {
	if snap == nil {
		return nil, fmt.Errorf("empty registry snapshot")
	}
	globalMu.RLock()
	current := global
	globalMu.RUnlock()
	if current == nil {
		return nil, fmt.Errorf("catalog not loaded")
	}
	next, err := buildOverlay(current, snap)
	if err != nil {
		return nil, err
	}
	return diffCatalogs(current, next), nil
}

func diffCatalogs(old, next *Catalog) []string {
	var diffs []string
	platforms := map[string]struct{}{}
	for name := range old.Platforms {
		platforms[name] = struct{}{}
	}
	for name := range next.Platforms {
		platforms[name] = struct{}{}
	}
	names := make([]string, 0, len(platforms))
	for name := range platforms {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		a, b := old.Platforms[name], next.Platforms[name]
		aIDs := modelIDSet(a.Models)
		bIDs := modelIDSet(b.Models)
		for id := range bIDs {
			if _, ok := aIDs[id]; !ok {
				diffs = append(diffs, fmt.Sprintf("models.%s: registry has %s, file does not", name, id))
			}
		}
		for id := range aIDs {
			if _, ok := bIDs[id]; !ok {
				diffs = append(diffs, fmt.Sprintf("models.%s: file has %s, registry does not", name, id))
			}
		}
		if a.DefaultChatModel != b.DefaultChatModel {
			diffs = append(diffs, fmt.Sprintf("default_chat.%s: file=%q registry=%q", name, a.DefaultChatModel, b.DefaultChatModel))
		}
		if a.DefaultTestModel != b.DefaultTestModel {
			diffs = append(diffs, fmt.Sprintf("default_test.%s: file=%q registry=%q", name, a.DefaultTestModel, b.DefaultTestModel))
		}
		for alias, target := range b.Aliases {
			if a.Aliases[alias] != target {
				diffs = append(diffs, fmt.Sprintf("aliases.%s: %q → file=%q registry=%q", name, alias, a.Aliases[alias], target))
			}
		}
		for alias := range a.Aliases {
			if _, ok := b.Aliases[alias]; !ok {
				diffs = append(diffs, fmt.Sprintf("aliases.%s: %q only in file", name, alias))
			}
		}
	}

	for model, entry := range next.FallbackPricing {
		oldEntry, ok := old.FallbackPricing[model]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("fallback_pricing: registry has %s, file does not", model))
			continue
		}
		if oldEntry.InputCostPerToken != entry.InputCostPerToken || oldEntry.OutputCostPerToken != entry.OutputCostPerToken {
			diffs = append(diffs, fmt.Sprintf("fallback_pricing.%s: file=%g/%g registry=%g/%g", model,
				oldEntry.InputCostPerToken, oldEntry.OutputCostPerToken, entry.InputCostPerToken, entry.OutputCostPerToken))
		}
	}
	return diffs
}

func modelIDSet(models []ModelEntry) map[string]struct{} {
	out := make(map[string]struct{}, len(models))
	for _, m := range models {
		out[m.ID] = struct{}{}
	}
	return out
}

// DecodeMediaFlags 解析 models.media 的 JSON（{"image":true,...}）。
// 解析失败返回空 map —— 能力标记缺失不应影响启动。
func DecodeMediaFlags(raw []byte) map[string]bool {
	out := map[string]bool{}
	if len(raw) == 0 {
		return out
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]bool{}
	}
	return out
}
