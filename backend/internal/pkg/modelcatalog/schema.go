package modelcatalog

// Catalog is the top-level model registry document.
type Catalog struct {
	Version         int                       `json:"version"`
	UpdatedAt       string                    `json:"updated_at,omitempty"`
	Platforms       map[string]PlatformConfig `json:"platforms"`
	FallbackPricing map[string]PriceEntry     `json:"fallback_pricing"`
	ImageDefaults   ImageDefaults             `json:"image_defaults"`
	UIPresets       map[string][]UIPreset     `json:"ui_presets"`
}

// PlatformConfig holds models and mappings for one platform.
type PlatformConfig struct {
	DefaultTestModel   string            `json:"default_test_model,omitempty"`
	DefaultChatModel   string            `json:"default_chat_model,omitempty"`
	Models             []ModelEntry      `json:"models,omitempty"`
	Aliases            map[string]string `json:"aliases,omitempty"`
	RetiredIDs         []string          `json:"retired_ids,omitempty"`
	DefaultMapping     map[string]string `json:"default_mapping,omitempty"`
	IDOverrides        map[string]string `json:"id_overrides,omitempty"`
	IDReverseOverrides map[string]string `json:"id_reverse_overrides,omitempty"`
}

// ModelEntry is a single model in the catalog.
type ModelEntry struct {
	ID                          string          `json:"id"`
	Name                        string          `json:"name,omitempty"` // Gemini native "models/..." form
	DisplayName                 string          `json:"display_name,omitempty"`
	Object                      string          `json:"object,omitempty"`
	OwnedBy                     string          `json:"owned_by,omitempty"`
	Type                        string          `json:"type,omitempty"`
	Created                     int64           `json:"created,omitempty"`
	CreatedAt                   string          `json:"created_at,omitempty"`
	Family                      string          `json:"family,omitempty"` // antigravity: claude|gemini
	IsReasoning                 bool            `json:"is_reasoning,omitempty"`
	SupportedGenerationMethods  []string        `json:"supported_generation_methods,omitempty"`
	Media                       map[string]bool `json:"media,omitempty"`
	Flags                       map[string]bool `json:"flags,omitempty"`
	ContextWindow               int             `json:"context_window,omitempty"`
	MaxContextWindow            int             `json:"max_context_window,omitempty"`
	MaxOutputTokens             int             `json:"max_output_tokens,omitempty"`
	SupportedReasoningLevels    []string        `json:"supported_reasoning_levels,omitempty"`
	DefaultReasoningLevel       string          `json:"default_reasoning_level,omitempty"`
	SupportsVerbosity           bool            `json:"supports_verbosity,omitempty"`
	DefaultVerbosity            string          `json:"default_verbosity,omitempty"`
	UseResponsesLite            bool            `json:"use_responses_lite,omitempty"`
	ToolMode                    string          `json:"tool_mode,omitempty"`
	MultiAgentVersion           string          `json:"multi_agent_version,omitempty"`
	MultiAgentReasoningEffort   string          `json:"multi_agent_reasoning_effort,omitempty"`
	SupportsParallelToolCalls   bool            `json:"supports_parallel_tool_calls,omitempty"`
	SupportsImageDetailOriginal bool            `json:"supports_image_detail_original,omitempty"`
	TruncationLimit             int             `json:"truncation_limit,omitempty"`
	PreferWebsockets            bool            `json:"prefer_websockets,omitempty"`
	MinimalClientVersion        string          `json:"minimal_client_version,omitempty"`
	SupportedInAPI              bool            `json:"supported_in_api,omitempty"`
}

// PriceEntry is LiteLLM-compatible fallback pricing. Currency defaults to USD.
type PriceEntry struct {
	AliasOf                     string  `json:"alias_of,omitempty"`
	Currency                    string  `json:"currency,omitempty"`
	InputCostPerToken           float64 `json:"input_cost_per_token,omitempty"`
	InputCostPerTokenPriority   float64 `json:"input_cost_per_token_priority,omitempty"`
	ImageInputCostPerToken      float64 `json:"image_input_cost_per_token,omitempty"`
	OutputCostPerToken          float64 `json:"output_cost_per_token,omitempty"`
	OutputCostPerTokenPriority  float64 `json:"output_cost_per_token_priority,omitempty"`
	CacheCreationInputTokenCost float64 `json:"cache_creation_input_token_cost,omitempty"`
	// CacheCreationInputTokenCostPriority 是 Fast mode（service_tier=priority/fast）下的
	// 缓存写入单价。官方 Fast 档对 cache writes 与 input 同倍率，缺此字段时
	// priceEntryToModelPricing 无法表达快速档 cache-write 价，会按标准价少收。
	CacheCreationInputTokenCostPriority float64 `json:"cache_creation_input_token_cost_priority,omitempty"`
	CacheReadInputTokenCost             float64 `json:"cache_read_input_token_cost,omitempty"`
	CacheReadInputTokenCostPriority     float64 `json:"cache_read_input_token_cost_priority,omitempty"`
	SupportsCacheBreakdown              bool    `json:"supports_cache_breakdown,omitempty"`
	LongContextInputTokenThreshold      int     `json:"long_context_input_token_threshold,omitempty"`
	LongContextInputCostMultiplier      float64 `json:"long_context_input_cost_multiplier,omitempty"`
	LongContextOutputCostMultiplier     float64 `json:"long_context_output_cost_multiplier,omitempty"`
	OutputCostPerImage                  float64 `json:"output_cost_per_image,omitempty"`
}

// ImageDefaults holds default image unit pricing.
type ImageDefaults struct {
	BasePriceUSD    float64            `json:"base_price_usd"`
	SizeMultipliers map[string]float64 `json:"size_multipliers"`
}

// UIPreset is an admin UI mapping chip.
type UIPreset struct {
	Label string `json:"label"`
	From  string `json:"from"`
	To    string `json:"to"`
	Color string `json:"color,omitempty"`
}
