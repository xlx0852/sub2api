package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
)

var grokCaptureCounter uint64

type grokCaptureRecord struct {
	Timestamp          string          `json:"timestamp"`
	Seq                uint64          `json:"seq"`
	Event              string          `json:"event"`
	AccountID          int64           `json:"account_id,omitempty"`
	Transport          string          `json:"transport,omitempty"`
	Turn               int             `json:"turn,omitempty"`
	SessionHash        string          `json:"session_hash,omitempty"`
	PromptCacheKey     string          `json:"prompt_cache_key,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
	InputItemsBefore   int             `json:"input_items_before,omitempty"`
	InputItemsAfter    int             `json:"input_items_after,omitempty"`
	InputSummaryBefore string          `json:"input_summary_before,omitempty"`
	InputSummaryAfter  string          `json:"input_summary_after,omitempty"`
	CollapseReason     string          `json:"collapse_reason,omitempty"`
	CollapseChanged    bool            `json:"collapse_changed,omitempty"`
	InboundBytes       int             `json:"inbound_bytes,omitempty"`
	OutboundBytes      int             `json:"outbound_bytes,omitempty"`
	Inbound            json.RawMessage `json:"inbound,omitempty"`
	Outbound           json.RawMessage `json:"outbound,omitempty"`
}

func grokCaptureDir() string {
	return strings.TrimSpace(os.Getenv("SUB2API_GROK_CAPTURE_DIR"))
}

func captureGrokTransform(event string, accountID int64, transport string, turn int, sessionHash string, inbound, outbound []byte, meta map[string]string) {
	dir := grokCaptureDir()
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o755)

	seq := atomic.AddUint64(&grokCaptureCounter, 1)
	rec := grokCaptureRecord{
		Timestamp:      time.Now().Format(time.RFC3339Nano),
		Seq:            seq,
		Event:          event,
		AccountID:      accountID,
		Transport:      transport,
		Turn:           turn,
		SessionHash:    sessionHash,
		InboundBytes:   len(inbound),
		OutboundBytes:  len(outbound),
		CollapseReason: metaValue(meta, "collapse_reason"),
	}
	if v := metaValue(meta, "collapse_changed"); v == "true" {
		rec.CollapseChanged = true
	}
	rec.PromptCacheKey = firstNonEmpty(
		metaValue(meta, "prompt_cache_key"),
		strings.TrimSpace(gjson.GetBytes(outbound, "prompt_cache_key").String()),
		strings.TrimSpace(gjson.GetBytes(inbound, "prompt_cache_key").String()),
	)
	rec.PreviousResponseID = firstNonEmpty(
		metaValue(meta, "previous_response_id"),
		strings.TrimSpace(gjson.GetBytes(inbound, "previous_response_id").String()),
	)
	rec.InputSummaryBefore = firstNonEmpty(
		metaValue(meta, "input_summary_before"),
		summarizeGrokResponsesInputItems(gjson.GetBytes(inbound, "input")),
	)
	rec.InputSummaryAfter = firstNonEmpty(
		metaValue(meta, "input_summary_after"),
		summarizeGrokResponsesInputItems(gjson.GetBytes(outbound, "input")),
	)
	rec.InputItemsBefore = grokCaptureInputItemCount(inbound, metaValue(meta, "input_items_before"))
	rec.InputItemsAfter = grokCaptureInputItemCount(outbound, metaValue(meta, "input_items_after"))
	if json.Valid(inbound) {
		rec.Inbound = json.RawMessage(inbound)
	}
	if json.Valid(outbound) {
		rec.Outbound = json.RawMessage(outbound)
	}

	safeEvent := strings.NewReplacer("/", "_", " ", "_").Replace(event)
	filename := filepath.Join(dir, fmt.Sprintf("%06d_%s.json", seq, safeEvent))
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		logOpenAIWSModeInfo(
			"grok_capture_write_failed seq=%d event=%s cause=%s",
			seq,
			event,
			truncateOpenAIWSLogValue(err.Error(), openAIWSLogValueMaxLen),
		)
		return
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		logOpenAIWSModeInfo(
			"grok_capture_write_failed seq=%d event=%s cause=%s",
			seq,
			event,
			truncateOpenAIWSLogValue(err.Error(), openAIWSLogValueMaxLen),
		)
		return
	}
	logOpenAIWSModeInfo(
		"grok_capture_written seq=%d event=%s file=%s transport=%s turn=%d account_id=%d inbound_bytes=%d outbound_bytes=%d input_items_before=%d input_items_after=%d prompt_cache_key=%s previous_response_id=%s collapse_changed=%v",
		seq,
		event,
		filename,
		transport,
		turn,
		accountID,
		len(inbound),
		len(outbound),
		rec.InputItemsBefore,
		rec.InputItemsAfter,
		truncateOpenAIWSLogValue(rec.PromptCacheKey, 64),
		truncateOpenAIWSLogValue(rec.PreviousResponseID, 64),
		rec.CollapseChanged,
	)
}

func metaValue(meta map[string]string, key string) string {
	if meta == nil {
		return ""
	}
	return strings.TrimSpace(meta[key])
}

func grokCaptureInputItemCount(payload []byte, override string) int {
	if override != "" {
		var count int
		if _, err := fmt.Sscanf(override, "%d", &count); err == nil {
			return count
		}
	}
	if !json.Valid(payload) {
		return 0
	}
	return len(gjson.GetBytes(payload, "input").Array())
}