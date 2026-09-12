package domain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

const (
	ModelRateLimitsKey                 = "model_rate_limits"
	AntigravityGeminiModelRateLimitKey = "antigravity:gemini"
	OpenAIImageGenerationRateLimitKey  = "openai:image_generation"
	AnthropicFableRateLimitKey         = "claude-fable-5"
	CreditsExhaustedKey                = "AICredits"

	AnthropicAPIKeyAuthSchemeExtraKey            = "anthropic_apikey_auth_scheme"
	AnthropicAPIKeyAuthSchemeXAPIKey             = "x_api_key"
	AnthropicAPIKeyAuthSchemeAuthorizationBearer = "authorization_bearer"

	OpenAIAuthModeAgentIdentity = "agentIdentity"
	vertexDefaultLocation       = "us-central1"

	featureKeyCodexImageGenerationBridge             = "codex_image_generation_bridge"
	featureKeyCodexImageGenerationExplicitToolPolicy = "codex_image_generation_explicit_tool_policy"
	codexImageGenerationExplicitToolPolicyAllow      = "allow"
	codexImageGenerationExplicitToolPolicyStrip      = "strip"

	defaultOpenAIMessagesDispatchOpusMappedModel   = "gpt-5.4"
	defaultOpenAIMessagesDispatchSonnetMappedModel = "gpt-5.3-codex"
	defaultOpenAIMessagesDispatchHaikuMappedModel  = "gpt-5.4-mini"
)

var errVertexServiceAccountMissing = errors.New("service account credentials not configured")

func NormalizeAccountNotes(value *string) *string {
	return normalizeAccountNotes(value)
}

func (a *Account) GetAnthropicAPIKeyAuthScheme() string {
	if a == nil || a.Platform != PlatformAnthropic || a.Type != AccountTypeAPIKey {
		return AnthropicAPIKeyAuthSchemeXAPIKey
	}
	switch strings.TrimSpace(a.GetExtraString(AnthropicAPIKeyAuthSchemeExtraKey)) {
	case AnthropicAPIKeyAuthSchemeAuthorizationBearer:
		return AnthropicAPIKeyAuthSchemeAuthorizationBearer
	default:
		return AnthropicAPIKeyAuthSchemeXAPIKey
	}
}

func (a *Account) IsOpenAIAgentIdentity() bool {
	if a == nil || !a.IsOpenAIOAuth() {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(a.GetCredential(openAIAuthModeCredentialKey)), OpenAIAuthModeAgentIdentity)
}

func (a *Account) IsVertexServiceAccount() bool {
	return a != nil && a.Type == AccountTypeServiceAccount
}

func (a *Account) VertexProjectID() string {
	if a == nil {
		return ""
	}
	if v := strings.TrimSpace(a.GetCredential("project_id")); v != "" {
		return v
	}
	if key, err := parseVertexServiceAccountKeyLite(a); err == nil {
		return strings.TrimSpace(key.ProjectID)
	}
	return ""
}

func (a *Account) VertexLocation(model string) string {
	if a == nil {
		return vertexDefaultLocation
	}
	if model != "" && a.Credentials != nil {
		if raw, ok := a.Credentials["vertex_model_locations"].(map[string]any); ok {
			if loc, ok := raw[model].(string); ok && strings.TrimSpace(loc) != "" {
				return strings.TrimSpace(loc)
			}
		}
	}
	if v := strings.TrimSpace(a.GetCredential("location")); v != "" {
		return v
	}
	if v := strings.TrimSpace(a.GetCredential("vertex_location")); v != "" {
		return v
	}
	return vertexDefaultLocation
}

type vertexServiceAccountKeyLite struct {
	ProjectID string `json:"project_id"`
}

func parseVertexServiceAccountKeyLite(account *Account) (*vertexServiceAccountKeyLite, error) {
	if account == nil || account.Credentials == nil {
		return nil, errVertexServiceAccountMissing
	}
	if raw := strings.TrimSpace(account.GetCredential("service_account_json")); raw != "" {
		return parseVertexServiceAccountJSONLite([]byte(raw))
	}
	if raw := strings.TrimSpace(account.GetCredential("service_account")); raw != "" {
		return parseVertexServiceAccountJSONLite([]byte(raw))
	}
	if nested, ok := account.Credentials["service_account_json"].(map[string]any); ok {
		b, _ := json.Marshal(nested)
		return parseVertexServiceAccountJSONLite(b)
	}
	if nested, ok := account.Credentials["service_account"].(map[string]any); ok {
		b, _ := json.Marshal(nested)
		return parseVertexServiceAccountJSONLite(b)
	}
	return nil, errVertexServiceAccountMissing
}

func parseVertexServiceAccountJSONLite(raw []byte) (*vertexServiceAccountKeyLite, error) {
	var key vertexServiceAccountKeyLite
	if err := json.Unmarshal(raw, &key); err != nil {
		return nil, err
	}
	return &key, nil
}

func (a *Account) CodexImageGenerationBridgeOverride() *bool {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return nil
	}
	if override := boolOverrideFromMap(a.Extra, featureKeyCodexImageGenerationBridge, "codex_image_generation_bridge_enabled"); override != nil {
		return override
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	return boolOverrideFromMap(openaiConfig, featureKeyCodexImageGenerationBridge, "codex_image_generation_bridge_enabled")
}

func (a *Account) CodexImageGenerationExplicitToolPolicy() string {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return codexImageGenerationExplicitToolPolicyAllow
	}
	if policy, ok := stringOverrideFromMap(a.Extra, featureKeyCodexImageGenerationExplicitToolPolicy); ok {
		return normalizeCodexImageGenerationExplicitToolPolicy(policy)
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	if policy, ok := stringOverrideFromMap(openaiConfig, featureKeyCodexImageGenerationExplicitToolPolicy); ok {
		return normalizeCodexImageGenerationExplicitToolPolicy(policy)
	}
	return codexImageGenerationExplicitToolPolicyAllow
}

func (c *Channel) CodexImageGenerationBridgeOverride(platform string) *bool {
	if c == nil {
		return nil
	}
	return platformBoolOverride(c.FeaturesConfig, featureKeyCodexImageGenerationBridge, platform)
}

func boolOverridePtr(v bool) *bool { return &v }

func boolOverrideFromMap(values map[string]any, keys ...string) *bool {
	if values == nil {
		return nil
	}
	for _, key := range keys {
		if v, ok := values[key].(bool); ok {
			return boolOverridePtr(v)
		}
	}
	return nil
}

func stringOverrideFromMap(values map[string]any, keys ...string) (string, bool) {
	if values == nil {
		return "", false
	}
	for _, key := range keys {
		if v, ok := values[key].(string); ok {
			return v, true
		}
	}
	return "", false
}

func normalizeCodexImageGenerationExplicitToolPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case codexImageGenerationExplicitToolPolicyStrip, "remove", "drop":
		return codexImageGenerationExplicitToolPolicyStrip
	default:
		return codexImageGenerationExplicitToolPolicyAllow
	}
}

func platformBoolOverride(values map[string]any, key string, platform string) *bool {
	if values == nil {
		return nil
	}
	if v, ok := values[key].(bool); ok {
		return boolOverridePtr(v)
	}
	raw, ok := values[key].(map[string]any)
	if !ok {
		return nil
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil
	}
	if v, ok := raw[platform].(bool); ok {
		return boolOverridePtr(v)
	}
	return nil
}

func (a *Account) IsRateLimitActiveForKey(key string) bool {
	resetAt := a.modelRateLimitResetAt(key)
	return resetAt != nil && time.Now().Before(*resetAt)
}

func (a *Account) GetRateLimitRemainingForKey(key string) time.Duration {
	resetAt := a.modelRateLimitResetAt(key)
	if resetAt == nil {
		return 0
	}
	remaining := time.Until(*resetAt)
	if remaining > 0 {
		return remaining
	}
	return 0
}

func (a *Account) IsModelRateLimitedWithContext(ctx context.Context, requestedModel string) bool {
	for _, key := range a.modelRateLimitKeysForRequest(ctx, requestedModel) {
		if a.IsRateLimitActiveForKey(key) {
			return true
		}
	}
	return false
}

func (a *Account) GetModelRateLimitRemainingTime(requestedModel string) time.Duration {
	return a.GetModelRateLimitRemainingTimeWithContext(context.Background(), requestedModel)
}

func (a *Account) GetModelRateLimitRemainingTimeWithContext(ctx context.Context, requestedModel string) time.Duration {
	remaining := time.Duration(0)
	for _, key := range a.modelRateLimitKeysForRequest(ctx, requestedModel) {
		if keyRemaining := a.GetRateLimitRemainingForKey(key); keyRemaining > remaining {
			remaining = keyRemaining
		}
	}
	return remaining
}

func (a *Account) modelRateLimitKeysForRequest(ctx context.Context, requestedModel string) []string {
	if a == nil {
		return nil
	}

	modelKey := a.GetMappedModel(requestedModel)
	if a.Platform == PlatformAntigravity {
		modelKey = ResolveFinalAntigravityModelKey(ctx, a, requestedModel)
	}
	modelKey = strings.TrimSpace(modelKey)
	if modelKey == "" {
		return nil
	}

	keys := []string{modelKey}
	switch a.Platform {
	case PlatformAntigravity:
		if IsAntigravityGeminiModel(modelKey) && modelKey != AntigravityGeminiModelRateLimitKey {
			keys = append(keys, AntigravityGeminiModelRateLimitKey)
		}
	case PlatformOpenAI:
		if openAIImageGenerationRateLimitApplies(ctx, requestedModel, modelKey) && modelKey != OpenAIImageGenerationRateLimitKey {
			keys = append(keys, OpenAIImageGenerationRateLimitKey)
		}
	case PlatformAnthropic:
		if IsAnthropicFableModel(modelKey) && modelKey != AnthropicFableRateLimitKey {
			keys = append(keys, AnthropicFableRateLimitKey)
		}
	}
	return keys
}

func (a *Account) modelRateLimitResetAt(scope string) *time.Time {
	if a == nil || a.Extra == nil || scope == "" {
		return nil
	}
	rawLimits, ok := a.Extra[ModelRateLimitsKey].(map[string]any)
	if !ok {
		return nil
	}
	rawLimit, ok := rawLimits[scope].(map[string]any)
	if !ok {
		return nil
	}
	resetAtRaw, ok := rawLimit["rate_limit_reset_at"].(string)
	if !ok || strings.TrimSpace(resetAtRaw) == "" {
		return nil
	}
	resetAt, err := time.Parse(time.RFC3339, resetAtRaw)
	if err != nil {
		return nil
	}
	return &resetAt
}

func IsAnthropicFableModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "fable")
}

func openAIImageGenerationRateLimitApplies(ctx context.Context, requestedModel, modelKey string) bool {
	if isOpenAIImageGenerationModelName(requestedModel) || isOpenAIImageGenerationModelName(modelKey) {
		return true
	}
	return OpenAIImageGenerationIntentFromContext(ctx)
}

func isOpenAIImageGenerationModelName(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(model, "gpt-image-") {
		return true
	}
	return model == "grok-imagine" ||
		model == "grok-imagine-edit" ||
		strings.HasPrefix(model, "grok-imagine-image")
}

func ResolveFinalAntigravityModelKey(ctx context.Context, account *Account, requestedModel string) string {
	modelKey := MapAntigravityModel(account, requestedModel)
	if modelKey == "" {
		return ""
	}
	if enabled, ok := ThinkingEnabledFromContext(ctx); ok {
		modelKey = ApplyThinkingModelSuffix(modelKey, enabled)
	}
	return modelKey
}

func IsAntigravityGeminiModel(model string) bool {
	return strings.HasPrefix(NormalizeAntigravityModelName(model), "gemini-")
}

func AntigravityModelRateLimitKeys(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	keys := []string{model}
	if IsAntigravityGeminiModel(model) && model != AntigravityGeminiModelRateLimitKey {
		keys = append(keys, AntigravityGeminiModelRateLimitKey)
	}
	return keys
}

func NormalizeAntigravityModelName(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if idx := strings.LastIndex(normalized, "/publishers/google/models/"); idx != -1 {
		normalized = normalized[idx+len("/publishers/google/models/"):]
	} else if idx := strings.LastIndex(normalized, "/publishers/anthropic/models/"); idx != -1 {
		normalized = normalized[idx+len("/publishers/anthropic/models/"):]
	} else if idx := strings.LastIndex(normalized, "/models/"); idx != -1 {
		normalized = normalized[idx+len("/models/"):]
	} else {
		normalized = strings.TrimPrefix(normalized, "publishers/google/models/")
		normalized = strings.TrimPrefix(normalized, "publishers/anthropic/models/")
		normalized = strings.TrimPrefix(normalized, "models/")
	}
	return normalized
}

func MapAntigravityModel(account *Account, requestedModel string) string {
	if account == nil {
		return ""
	}
	requestedModel = strings.TrimPrefix(requestedModel, "models/")
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return ""
	}
	mapped := account.GetMappedModel(requestedModel)
	if mapped != requestedModel {
		return mapped
	}
	if account.IsModelSupported(requestedModel) {
		return requestedModel
	}
	return ""
}

func ApplyThinkingModelSuffix(mappedModel string, thinkingEnabled bool) string {
	if !thinkingEnabled {
		return mappedModel
	}
	if mappedModel == "claude-sonnet-4-5" {
		return "claude-sonnet-4-5-thinking"
	}
	return mappedModel
}

func (a *Account) IsSchedulableForModel(requestedModel string) bool {
	return a.IsSchedulableForModelWithContext(context.Background(), requestedModel)
}

func (a *Account) IsSchedulableForModelWithContext(ctx context.Context, requestedModel string) bool {
	if a == nil {
		return false
	}
	if !a.IsSchedulable() {
		return false
	}
	if a.IsModelRateLimitedWithContext(ctx, requestedModel) {
		if a.Platform == PlatformAntigravity && a.IsOveragesEnabled() && !a.IsCreditsExhausted() {
			return true
		}
		return false
	}
	return true
}

func (a *Account) GetRateLimitRemainingTime(requestedModel string) time.Duration {
	return a.GetRateLimitRemainingTimeWithContext(context.Background(), requestedModel)
}

func (a *Account) GetRateLimitRemainingTimeWithContext(ctx context.Context, requestedModel string) time.Duration {
	if a == nil {
		return 0
	}
	return a.GetModelRateLimitRemainingTimeWithContext(ctx, requestedModel)
}

func (a *Account) IsCreditsExhausted() bool {
	if a == nil {
		return false
	}
	return a.IsRateLimitActiveForKey(CreditsExhaustedKey)
}

func (g *Group) CustomModelsListEnabled() bool {
	return g != nil && g.ModelsListConfig.Enabled && len(g.ModelsListConfig.Models) > 0
}

func (g *Group) ResolveMessagesDispatchModel(requestedModel string) string {
	if g == nil {
		return ""
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return ""
	}

	if g.Platform == PlatformGrok {
		if claudeMessagesDispatchFamily(requestedModel) != "" {
			return xai.DefaultModelMapping()["grok"]
		}
		return ""
	}
	if g.Platform == PlatformKimi {
		if claudeMessagesDispatchFamily(requestedModel) != "" {
			return "k3"
		}
		return ""
	}
	cfg := normalizeOpenAIMessagesDispatchModelConfig(g.MessagesDispatchModelConfig)
	if mappedModel := strings.TrimSpace(cfg.ExactModelMappings[requestedModel]); mappedModel != "" {
		return mappedModel
	}

	switch claudeMessagesDispatchFamily(requestedModel) {
	case "opus":
		if mappedModel := strings.TrimSpace(cfg.OpusMappedModel); mappedModel != "" {
			return mappedModel
		}
		return defaultOpenAIMessagesDispatchOpusMappedModel
	case "sonnet":
		if mappedModel := strings.TrimSpace(cfg.SonnetMappedModel); mappedModel != "" {
			return mappedModel
		}
		return defaultOpenAIMessagesDispatchSonnetMappedModel
	case "haiku":
		if mappedModel := strings.TrimSpace(cfg.HaikuMappedModel); mappedModel != "" {
			return mappedModel
		}
		return defaultOpenAIMessagesDispatchHaikuMappedModel
	default:
		return ""
	}
}

func normalizeOpenAIMessagesDispatchMappedModel(model string) string {
	return strings.TrimSpace(model)
}

func normalizeOpenAIMessagesDispatchModelConfig(cfg OpenAIMessagesDispatchModelConfig) OpenAIMessagesDispatchModelConfig {
	out := OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:   normalizeOpenAIMessagesDispatchMappedModel(cfg.OpusMappedModel),
		SonnetMappedModel: normalizeOpenAIMessagesDispatchMappedModel(cfg.SonnetMappedModel),
		HaikuMappedModel:  normalizeOpenAIMessagesDispatchMappedModel(cfg.HaikuMappedModel),
	}
	if len(cfg.ExactModelMappings) > 0 {
		out.ExactModelMappings = make(map[string]string, len(cfg.ExactModelMappings))
		for requestedModel, mappedModel := range cfg.ExactModelMappings {
			requestedModel = strings.TrimSpace(requestedModel)
			mappedModel = normalizeOpenAIMessagesDispatchMappedModel(mappedModel)
			if requestedModel == "" || mappedModel == "" {
				continue
			}
			out.ExactModelMappings[requestedModel] = mappedModel
		}
		if len(out.ExactModelMappings) == 0 {
			out.ExactModelMappings = nil
		}
	}
	return out
}

func claudeMessagesDispatchFamily(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if !strings.HasPrefix(normalized, "claude") {
		return ""
	}
	switch {
	case strings.Contains(normalized, "opus"):
		return "opus"
	case strings.Contains(normalized, "sonnet"):
		return "sonnet"
	case strings.Contains(normalized, "haiku"):
		return "haiku"
	default:
		return ""
	}
}
