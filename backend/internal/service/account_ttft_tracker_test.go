package service

import (
	"testing"
	"time"
)

func TestAccountTTFTTrackerMedianAndThreshold(t *testing.T) {
	tracker := NewAccountTTFTTracker()
	fixed := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return fixed }

	for _, value := range []int{1000, 3000, 2000, 4000, 5000} {
		tracker.Observe(7, "gpt-5.6-luna", value)
	}
	median, samples, ok := tracker.AccountTTFT(7, "gpt-5.6-luna")
	if !ok || samples != 5 || median != 3000 {
		t.Fatalf("median=%d samples=%d ok=%v, want 3000/5/true", median, samples, ok)
	}
	// 模型大小写与空格归一
	if _, _, ok := tracker.AccountTTFT(7, " GPT-5.6-Luna "); !ok {
		t.Fatal("model key must be normalized")
	}
	// 样本不足：4 个样本不参与比较
	tracker.Observe(8, "gpt-5.6-luna", 100)
	if _, _, ok := tracker.AccountTTFT(8, "gpt-5.6-luna"); ok {
		t.Fatal("insufficient samples must not report ok")
	}
	// 非正值忽略
	tracker.Observe(8, "gpt-5.6-luna", 0)
	tracker.Observe(8, "gpt-5.6-luna", -5)
	if _, samples, _ := tracker.AccountTTFT(8, "gpt-5.6-luna"); samples != 1 {
		t.Fatalf("invalid samples must be ignored, samples=%d", samples)
	}
}

func TestAccountTTFTTrackerDropsStaleSamples(t *testing.T) {
	tracker := NewAccountTTFTTracker()
	base := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	current := base
	tracker.now = func() time.Time { return current }
	for i := 0; i < 6; i++ {
		tracker.Observe(9, "m", 1000)
	}
	current = base.Add(31 * time.Minute)
	if _, samples, ok := tracker.AccountTTFT(9, "m"); ok || samples != 0 {
		t.Fatalf("stale samples must be dropped, samples=%d ok=%v", samples, ok)
	}
}

func TestAccountTTFTTrackerCapacity(t *testing.T) {
	tracker := NewAccountTTFTTracker()
	tracker.now = func() time.Time { return time.Now() }
	for i := 0; i < 30; i++ {
		tracker.Observe(10, "m", 1000+i)
	}
	if _, samples, _ := tracker.AccountTTFT(10, "m"); samples != defaultAccountTTFTCapacity {
		t.Fatalf("samples=%d, want capacity %d", samples, defaultAccountTTFTCapacity)
	}
}

func TestTTFTLatencyPreferenceHonorsConfigFlag(t *testing.T) {
	var nilOpenAI *OpenAIGatewayService
	if pref := nilOpenAI.ttftLatencyPreference("m"); pref.Enabled {
		t.Fatal("nil service must not enable latency preference")
	}
	var nilGateway *GatewayService
	if pref := nilGateway.ttftLatencyPreference("m"); pref.Enabled {
		t.Fatal("nil gateway service must not enable latency preference")
	}
}
