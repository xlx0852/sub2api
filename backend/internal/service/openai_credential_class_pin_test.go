package service

import (
	"testing"
)

// 构造带模型映射的 OpenAI 账号。
func pinTestAccount(id int64, typ string, models ...string) *Account {
	creds := map[string]any{}
	if len(models) > 0 {
		m := map[string]any{}
		for _, mm := range models {
			m[mm] = mm
		}
		creds["model_mapping"] = m
	}
	return &Account{ID: id, Platform: PlatformOpenAI, Type: typ, Credentials: creds}
}

// sticky 钉到不支持目标模型的 apikey 号时，不得把 class 钉成 api_key，
// 否则会把唯一支持模型的 oauth 号全按 credential_class 拒掉 → 空池 503。
func TestResolvePinnedCredentialClass_PinSkippedWhenModelUnsupported(t *testing.T) {
	sticky := pinTestAccount(89, AccountTypeAPIKey, "gpt-5.6-sol") // 不支持 astra
	got := resolvePinnedCredentialClass([]*Account{sticky}, "gpt-6-astra")
	if got != OpenAICredentialClassUnspecified {
		t.Fatalf("expected Unspecified when pin source lacks model, got %q", got)
	}
}

// sticky 支持目标模型时仍钉其 class（行为不变）。
func TestResolvePinnedCredentialClass_PinsWhenModelSupported(t *testing.T) {
	sticky := pinTestAccount(89, AccountTypeAPIKey, "gpt-6-astra")
	got := resolvePinnedCredentialClass([]*Account{sticky}, "gpt-6-astra")
	if got != OpenAICredentialClassAPIKey {
		t.Fatalf("expected api_key pin, got %q", got)
	}
}

// 多个钉选来源：第一个不支持模型跳过，第二个支持则钉其 class。
func TestResolvePinnedCredentialClass_FallsToNextSupportedSource(t *testing.T) {
	sticky := pinTestAccount(89, AccountTypeAPIKey, "gpt-5.6-sol")              // 不支持 astra
	prev := pinTestAccount(69, AccountTypeOAuth, "gpt-6-astra", "gpt-5.6-luna") // 支持
	got := resolvePinnedCredentialClass([]*Account{sticky, prev}, "gpt-6-astra")
	if got != OpenAICredentialClassOAuth {
		t.Fatalf("expected oauth pin from supported source, got %q", got)
	}
}

// 全部来源都不支持 → Unspecified，放开全类竞争。
func TestResolvePinnedCredentialClass_AllUnsupported(t *testing.T) {
	a := pinTestAccount(89, AccountTypeAPIKey, "gpt-5.6-sol")
	b := pinTestAccount(126, AccountTypeAPIKey, "swe-2")
	got := resolvePinnedCredentialClass([]*Account{a, b}, "gpt-6-astra")
	if got != OpenAICredentialClassUnspecified {
		t.Fatalf("expected Unspecified, got %q", got)
	}
}

// requestedModel 为空时不做模型校验，按原逻辑钉第一个非空 class。
func TestResolvePinnedCredentialClass_EmptyModelPinsFirst(t *testing.T) {
	sticky := pinTestAccount(89, AccountTypeAPIKey, "gpt-5.6-sol")
	got := resolvePinnedCredentialClass([]*Account{sticky}, "")
	if got != OpenAICredentialClassAPIKey {
		t.Fatalf("expected api_key pin on empty model, got %q", got)
	}
}
