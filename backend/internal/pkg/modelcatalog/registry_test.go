package modelcatalog

import (
	"reflect"
	"testing"
)

func TestEmbeddedCatalogLoads(t *testing.T) {
	t.Parallel()

	if err := Load(nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := Get()
	if c == nil || c.Version < 1 {
		t.Fatalf("expected catalog version >= 1")
	}
	if len(OpenAIModels()) == 0 {
		t.Fatalf("expected openai models")
	}
	if len(AnthropicModels()) == 0 {
		t.Fatalf("expected anthropic models")
	}
	if len(GrokModels()) == 0 {
		t.Fatalf("expected grok models")
	}
	if GrokDefaultChatModel() == "" {
		t.Fatalf("expected grok default chat model")
	}
	if m := AntigravityDefaultMapping(); len(m) == 0 {
		t.Fatalf("expected antigravity mapping")
	}
	if GrokDefaultChatModel() != "grok-4.7" {
		t.Fatalf("GrokDefaultChatModel() = %q, want grok-4.7", GrokDefaultChatModel())
	}
	entry, ok := ResolvePriceEntry("grok-4.7")
	if !ok || entry.InputCostPerToken != 2e-6 || entry.OutputCostPerToken != 6e-6 || entry.CacheReadInputTokenCost != 0.5e-6 {
		t.Fatalf("expected official grok-4.7 fallback pricing, got %+v ok=%v", entry, ok)
	}
	if entry.LongContextInputTokenThreshold != 200000 || entry.LongContextInputCostMultiplier != 2 || entry.LongContextOutputCostMultiplier != 2 {
		t.Fatalf("expected grok-4.7 long-context pricing, got %+v", entry)
	}
	if entry.InputCostPerTokenPriority != 4e-6 || entry.OutputCostPerTokenPriority != 12e-6 || entry.CacheReadInputTokenCostPriority != 1e-6 {
		t.Fatalf("expected grok-4.7 priority pricing, got %+v", entry)
	}
	entry, ok = ResolvePriceEntry("grok-4.5")
	if !ok || entry.InputCostPerToken <= 0 {
		t.Fatalf("expected grok-4.5 fallback pricing, got %+v ok=%v", entry, ok)
	}
	entry, ok = ResolvePriceEntry("grok-4.6")
	if !ok || entry.InputCostPerToken != 2e-6 || entry.OutputCostPerToken != 6e-6 || entry.CacheReadInputTokenCost != 0.5e-6 {
		t.Fatalf("expected official grok-4.6 fallback pricing, got %+v ok=%v", entry, ok)
	}
	if entry.LongContextInputTokenThreshold != 200000 || entry.LongContextInputCostMultiplier != 2 || entry.LongContextOutputCostMultiplier != 2 {
		t.Fatalf("expected grok-4.6 long-context pricing, got %+v", entry)
	}
	deepseek := Get().Platforms["deepseek"]
	if deepseek.DefaultChatModel != "deepseek-v4-pro" || len(deepseek.Models) != 5 || deepseek.Models[0].ID != "deepseek-flash" {
		t.Fatalf("expected current DeepSeek V4 catalog, got %+v", deepseek)
	}
	vision, ok := deepseek.DefaultMapping["deepseek-v4-flash-vision-exp"]
	if !ok || vision != "deepseek-v4-flash-vision-exp" || !deepseek.Models[4].Media["image_input"] {
		t.Fatalf("expected DeepSeek V4 Flash Vision Exp catalog mapping/media, got %+v", deepseek)
	}
	entry, ok = ResolvePriceEntry("deepseek-v4-flash-vision-exp")
	if !ok || entry.InputCostPerToken != 2e-6 || entry.OutputCostPerToken != 8e-6 {
		t.Fatalf("expected DeepSeek V4 Flash Vision Exp pricing, got %+v ok=%v", entry, ok)
	}
	entry, ok = ResolvePriceEntry("deepseek-v4-pro")
	if !ok || entry.Currency != "CNY" || entry.InputCostPerToken != 9e-6 || entry.OutputCostPerToken != 27e-6 || entry.CacheReadInputTokenCost != 0.30e-6 {
		t.Fatalf("expected official deepseek-v4-pro fallback pricing, got %+v ok=%v", entry, ok)
	}
	// alias chain
	entry, ok = ResolvePriceEntry("gpt-5.5")
	if !ok || entry.InputCostPerToken <= 0 {
		t.Fatalf("expected gpt-5.5 alias pricing, got %+v ok=%v", entry, ok)
	}
	entry, ok = ResolvePriceEntry("gpt-6-astra")
	if !ok || entry.InputCostPerToken != 1e-5 || entry.OutputCostPerToken != 5e-5 {
		t.Fatalf("expected gpt-6-astra fallback pricing, got %+v ok=%v", entry, ok)
	}
	entry, ok = ResolvePriceEntry("swe-2-high")
	if !ok || entry.InputCostPerToken != 3e-6 || entry.OutputCostPerToken != 15e-6 || entry.CacheReadInputTokenCost != 0.30e-6 {
		t.Fatalf("expected swe-2-high list pricing, got %+v ok=%v", entry, ok)
	}
	for _, id := range []string{"swe-2-medium", "swe-2-max"} {
		entry, ok = ResolvePriceEntry(id)
		if !ok || entry.InputCostPerToken != 3e-6 || entry.OutputCostPerToken != 15e-6 {
			t.Fatalf("expected %s to alias swe-2 list pricing, got %+v ok=%v", id, entry, ok)
		}
	}
	models := OpenAIModels()
	foundAstra := false
	for _, model := range models {
		if model.ID == "gpt-6-astra" {
			foundAstra = true
			break
		}
	}
	if !foundAstra {
		t.Fatalf("expected gpt-6-astra in OpenAI catalog")
	}
	astra := Get().Platforms["openai"].Models[0]
	if astra.ID != "gpt-6-astra" || astra.MaxContextWindow != 872000 || len(astra.SupportedReasoningLevels) != 6 || !astra.UseResponsesLite {
		t.Fatalf("expected official Astra capability metadata, got %+v", astra)
	}
	if !IsOpenAIRetired("gpt-5.2") {
		t.Fatalf("expected gpt-5.2 retired")
	}
}

func TestKimiCatalogUsesOfficialCodingModelIDs(t *testing.T) {
	models := KimiModels()
	got := make([]string, 0, len(models))
	for _, model := range models {
		got = append(got, model.ID)
	}
	want := []string{"k3", "k3-256k", "kimi-for-coding", "kimi-for-coding-highspeed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("KimiModels() = %v, want %v", got, want)
	}
	if KimiDefaultTestModel() != "k3" {
		t.Fatalf("KimiDefaultTestModel() = %q, want k3", KimiDefaultTestModel())
	}
}
