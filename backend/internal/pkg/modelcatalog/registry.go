package modelcatalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed catalog.json
var embeddedCatalog []byte

var (
	globalMu sync.RWMutex
	global   *Catalog
)

func init() {
	if err := Load(nil); err != nil {
		// Fail closed at package init so misconfigured binaries don't boot silently
		// with empty catalogs. Tests can still call Load with fixtures.
		panic("modelcatalog: load default catalog: " + err.Error())
	}
}

// LoadOptions configures catalog loading.
type LoadOptions struct {
	// File is an optional filesystem path override. Empty uses embed only.
	File string
}

// Load loads the catalog from File if set and readable, otherwise the embedded document.
func Load(opts *LoadOptions) error {
	var raw []byte
	var err error
	if opts != nil && strings.TrimSpace(opts.File) != "" {
		path := strings.TrimSpace(opts.File)
		raw, err = os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read model catalog %q: %w", path, err)
		}
	} else {
		// Prefer resources path relative to working directory when present (ops override without rebuild).
		candidates := []string{
			filepath.Join("resources", "model-catalog", "catalog.json"),
			filepath.Join("backend", "resources", "model-catalog", "catalog.json"),
		}
		for _, c := range candidates {
			if b, readErr := os.ReadFile(c); readErr == nil {
				raw = b
				break
			}
		}
		if raw == nil {
			raw = embeddedCatalog
		}
	}

	var cat Catalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		return fmt.Errorf("parse model catalog: %w", err)
	}
	if cat.Platforms == nil {
		cat.Platforms = map[string]PlatformConfig{}
	}
	if cat.FallbackPricing == nil {
		cat.FallbackPricing = map[string]PriceEntry{}
	}
	if cat.UIPresets == nil {
		cat.UIPresets = map[string][]UIPreset{}
	}

	// Expand identity mappings for grok models when only aliases are listed.
	if p, ok := cat.Platforms["grok"]; ok {
		if p.DefaultMapping == nil {
			p.DefaultMapping = map[string]string{}
		}
		for _, m := range p.Models {
			if m.ID == "" {
				continue
			}
			if _, exists := p.DefaultMapping[m.ID]; !exists {
				p.DefaultMapping[m.ID] = m.ID
			}
		}
		cat.Platforms["grok"] = p
	}

	globalMu.Lock()
	global = &cat
	globalMu.Unlock()
	return nil
}

// Get returns the loaded catalog (never nil after successful init).
func Get() *Catalog {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return cloneCatalog(global)
}

func platform(name string) PlatformConfig {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if global == nil {
		return PlatformConfig{}
	}
	return clonePlatform(global.Platforms[name])
}

// --- OpenAI ---

func OpenAIModels() []ModelEntry {
	return append([]ModelEntry(nil), platform("openai").Models...)
}

func OpenAIRetiredIDs() map[string]struct{} {
	out := make(map[string]struct{})
	for _, id := range platform("openai").RetiredIDs {
		out[strings.ToLower(strings.TrimSpace(id))] = struct{}{}
	}
	return out
}

func IsOpenAIRetired(id string) bool {
	_, ok := OpenAIRetiredIDs()[strings.ToLower(strings.TrimSpace(id))]
	return ok
}

func OpenAIDefaultTestModel() string {
	if v := platform("openai").DefaultTestModel; v != "" {
		return v
	}
	return "gpt-5.4"
}

// --- Anthropic ---

func AnthropicModels() []ModelEntry {
	return append([]ModelEntry(nil), platform("anthropic").Models...)
}

func AnthropicDefaultTestModel() string {
	if v := platform("anthropic").DefaultTestModel; v != "" {
		return v
	}
	return "claude-sonnet-4-5-20250929"
}

func AnthropicIDOverrides() map[string]string {
	return copyStringMap(platform("anthropic").IDOverrides)
}

func AnthropicIDReverseOverrides() map[string]string {
	return copyStringMap(platform("anthropic").IDReverseOverrides)
}

// --- Gemini ---

func GeminiModels() []ModelEntry {
	return append([]ModelEntry(nil), platform("gemini").Models...)
}

func GeminiDefaultTestModel() string {
	if v := platform("gemini").DefaultTestModel; v != "" {
		return v
	}
	return "gemini-3.5-flash"
}

// --- Antigravity ---

func AntigravityModels() []ModelEntry {
	return append([]ModelEntry(nil), platform("antigravity").Models...)
}

func AntigravityDefaultMapping() map[string]string {
	return copyStringMap(platform("antigravity").DefaultMapping)
}

// --- Grok ---

func GrokModels() []ModelEntry {
	return append([]ModelEntry(nil), platform("grok").Models...)
}

func GrokDefaultChatModel() string {
	if v := platform("grok").DefaultChatModel; v != "" {
		return v
	}
	if v := platform("grok").DefaultTestModel; v != "" {
		return v
	}
	return "grok-4.5"
}

func GrokDefaultMapping() map[string]string {
	m := copyStringMap(platform("grok").DefaultMapping)
	if m == nil {
		m = map[string]string{}
	}
	for _, model := range platform("grok").Models {
		if model.ID == "" {
			continue
		}
		if _, ok := m[model.ID]; !ok {
			m[model.ID] = model.ID
		}
	}
	return m
}

// --- Kimi ---

func KimiModels() []ModelEntry {
	return append([]ModelEntry(nil), platform("kimi").Models...)
}

func KimiDefaultTestModel() string {
	if v := platform("kimi").DefaultTestModel; v != "" {
		return v
	}
	return "k3"
}

// --- Bedrock ---

func BedrockDefaultMapping() map[string]string {
	return copyStringMap(platform("bedrock").DefaultMapping)
}

// --- Pricing ---

func FallbackPricing() map[string]PriceEntry {
	c := Get()
	if c == nil {
		return map[string]PriceEntry{}
	}
	out := make(map[string]PriceEntry, len(c.FallbackPricing))
	for k, v := range c.FallbackPricing {
		out[k] = v
	}
	return out
}

// ResolvePriceEntry follows alias_of chains (max depth 8).
func ResolvePriceEntry(key string) (PriceEntry, bool) {
	c := Get()
	if c == nil {
		return PriceEntry{}, false
	}
	seen := map[string]struct{}{}
	cur := key
	for i := 0; i < 8; i++ {
		if _, ok := seen[cur]; ok {
			return PriceEntry{}, false
		}
		seen[cur] = struct{}{}
		entry, ok := c.FallbackPricing[cur]
		if !ok {
			return PriceEntry{}, false
		}
		if strings.TrimSpace(entry.AliasOf) == "" {
			return entry, true
		}
		cur = strings.TrimSpace(entry.AliasOf)
	}
	return PriceEntry{}, false
}

func ImageDefaultsConfig() ImageDefaults {
	c := Get()
	if c == nil {
		return ImageDefaults{BasePriceUSD: 0.134, SizeMultipliers: map[string]float64{"1K": 1, "2K": 1.5, "4K": 2}}
	}
	return c.ImageDefaults
}

func UIPresetsFor(platform string) []UIPreset {
	c := Get()
	if c == nil {
		return nil
	}
	return append([]UIPreset(nil), c.UIPresets[platform]...)
}

// PublicView returns a JSON-safe catalog subset for admin/UI (no internal secrets).
func PublicView() *Catalog {
	c := Get()
	if c == nil {
		return &Catalog{Version: 0, Platforms: map[string]PlatformConfig{}}
	}
	return c
}

func cloneCatalog(in *Catalog) *Catalog {
	if in == nil {
		return nil
	}
	out := *in
	out.Platforms = make(map[string]PlatformConfig, len(in.Platforms))
	for key, value := range in.Platforms {
		out.Platforms[key] = clonePlatform(value)
	}
	out.FallbackPricing = make(map[string]PriceEntry, len(in.FallbackPricing))
	for key, value := range in.FallbackPricing {
		out.FallbackPricing[key] = value
	}
	out.ImageDefaults.SizeMultipliers = copyFloatMap(in.ImageDefaults.SizeMultipliers)
	out.UIPresets = make(map[string][]UIPreset, len(in.UIPresets))
	for key, value := range in.UIPresets {
		out.UIPresets[key] = append([]UIPreset(nil), value...)
	}
	return &out
}

func clonePlatform(in PlatformConfig) PlatformConfig {
	out := in
	out.Models = append([]ModelEntry(nil), in.Models...)
	for i := range out.Models {
		out.Models[i].SupportedGenerationMethods = append([]string(nil), in.Models[i].SupportedGenerationMethods...)
		out.Models[i].Media = copyBoolMap(in.Models[i].Media)
		out.Models[i].Flags = copyBoolMap(in.Models[i].Flags)
	}
	out.Aliases = copyStringMap(in.Aliases)
	out.RetiredIDs = append([]string(nil), in.RetiredIDs...)
	out.DefaultMapping = copyStringMap(in.DefaultMapping)
	out.IDOverrides = copyStringMap(in.IDOverrides)
	out.IDReverseOverrides = copyStringMap(in.IDReverseOverrides)
	return out
}

func copyBoolMap(in map[string]bool) map[string]bool {
	if in == nil {
		return nil
	}
	out := make(map[string]bool, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyFloatMap(in map[string]float64) map[string]float64 {
	if in == nil {
		return nil
	}
	out := make(map[string]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
