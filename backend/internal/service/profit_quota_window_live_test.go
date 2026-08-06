package service

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFillProfitQuotaWindows_LiveAccount69Extra(t *testing.T) {
	raw, err := os.ReadFile("/tmp/acc69_extra.json")
	if err != nil {
		t.Skip("no live extra dump")
	}
	var extra map[string]any
	require.NoError(t, json.Unmarshal(raw, &extra))
	now := time.Date(2026, 8, 6, 17, 30, 0, 0, time.FixedZone("CST", 8*3600))
	summary := &AccountProfitSummary{}
	fillProfitQuotaWindows(summary, &Account{ID: 69, Platform: PlatformOpenAI, Extra: extra}, now)
	require.NotEmpty(t, summary.QuotaWindows)
	t.Logf("windows=%+v", summary.QuotaWindows)
	found := false
	for _, w := range summary.QuotaWindows {
		t.Logf("kind=%s label=%s mins=%v used=%v start=%v end=%v", w.Kind, w.Label, w.WindowMinutes, w.UsedPercent, w.StartAt, w.EndAt)
		if w.WindowMinutes != nil && *w.WindowMinutes == 43200 {
			found = true
			require.Equal(t, "30d", w.Kind)
			require.Equal(t, "30d", w.Label)
		}
	}
	require.True(t, found, "expected 43200-minute window")
}
