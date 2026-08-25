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
	entry, ok := ResolvePriceEntry("grok-4.5")
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
	if deepseek.DefaultChatModel != "deepseek-v4-pro" || len(deepseek.Models) != 3 || deepseek.Models[0].ID != "deepseek-v4-pro" {
		t.Fatalf("expected current DeepSeek V4 catalog, got %+v", deepseek)
	}
	vision, ok := deepseek.DefaultMapping["deepseek-v4-flash-vision-exp"]
	if !ok || vision != "deepseek-v4-flash-vision-exp" || !deepseek.Models[2].Media["image_input"] {
		t.Fatalf("expected DeepSeek V4 Flash Vision Exp catalog mapping/media, got %+v", deepseek)
	}
	entry, ok = ResolvePriceEntry("deepseek-v4-flash-vision-exp")
	if !ok || entry.InputCostPerToken != 3e-6 || entry.OutputCostPerToken != 9e-6 {
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
