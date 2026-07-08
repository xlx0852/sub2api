package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/tidwall/gjson"
)

// grokObserveSpan carries per-turn correlation fields for Grok pipeline logs.
type grokObserveSpan struct {
	AccountID          int64
	Turn               int
	Transport          string
	SessionHash        string
	PromptCacheKey     string
	PreviousResponseID string
	UpstreamModel      string
}

type grokResponseAnalysis struct {
	OutputTextChars     int
	ReasoningTextChars  int
	OutputItemCount     int
	ReasoningItemCount  int
	SequenceNumber      int64
	HasEncryptedReasoning bool
	Anomalies           []string
	OutputPreview       string
	ReasoningPreview    string
}

var grokObserveCounter uint64

func grokObserveEnabled() bool {
	if strings.TrimSpace(os.Getenv("SUB2API_GROK_OBSERVE")) == "1" ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("SUB2API_GROK_OBSERVE")), "true") {
		return true
	}
	return grokCaptureDir() != ""
}

func grokObserveDir() string {
	if dir := strings.TrimSpace(os.Getenv("SUB2API_GROK_OBSERVE_DIR")); dir != "" {
		return dir
	}
	if dir := grokCaptureDir(); dir != "" {
		return filepath.Join(dir, "observe")
	}
	return ""
}

func observeGrokEvent(span *grokObserveSpan, event string, fields map[string]string) {
	if !grokObserveEnabled() {
		return
	}
	event = strings.TrimSpace(event)
	if event == "" {
		return
	}
	seq := atomic.AddUint64(&grokObserveCounter, 1)
	record := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"seq":       seq,
		"event":     event,
	}
	if span != nil {
		if span.AccountID > 0 {
			record["account_id"] = span.AccountID
		}
		if span.Turn > 0 {
			record["turn"] = span.Turn
		}
		if v := strings.TrimSpace(span.Transport); v != "" {
			record["transport"] = v
		}
		if v := strings.TrimSpace(span.SessionHash); v != "" {
			record["session_hash"] = v
		}
		if v := strings.TrimSpace(span.PromptCacheKey); v != "" {
			record["prompt_cache_key"] = v
		}
		if v := strings.TrimSpace(span.PreviousResponseID); v != "" {
			record["previous_response_id"] = v
		}
		if v := strings.TrimSpace(span.UpstreamModel); v != "" {
			record["upstream_model"] = v
		}
	}
	for key, value := range fields {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		record[key] = value
	}

	if dir := grokObserveDir(); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		line, err := json.Marshal(record)
		if err == nil {
			f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				_, _ = f.Write(append(line, '\n'))
				_ = f.Close()
			}
		}
	}

	parts := make([]string, 0, len(record)+1)
	parts = append(parts, "grok_observe", "event="+event, fmt.Sprintf("seq=%d", seq))
	for _, key := range []string{
		"account_id", "turn", "transport", "stage", "upstream_model",
		"prompt_cache_key", "previous_response_id", "session_hash",
		"inbound_bytes", "outbound_bytes", "input_items_before", "input_items_after",
		"collapse_strategy", "collapse_reason", "collapse_changed",
		"cache_key_before", "cache_key_after", "rotation_reason",
		"replay_action", "replay_session_key", "replay_items", "replay_injected",
		"encrypted_dropped", "encrypted_preserved", "encrypted_codex_rejected",
		"output_text_chars", "reasoning_text_chars", "anomalies", "anomaly_count",
		"response_id", "sequence_number", "output_preview", "user_text_preview",
		"dropped_assistant_chars", "recall_detected", "preserve_assistants",
	} {
		if value, ok := record[key]; ok {
			parts = append(parts, fmt.Sprintf("%s=%s", key, truncateGrokObserveValue(fmt.Sprint(value))))
		}
	}
	for key, value := range fields {
		if _, listed := record[key]; listed {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, truncateGrokObserveValue(value)))
	}
	logOpenAIWSModeInfo(strings.Join(parts, " "))
}

func truncateGrokObserveValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", "\\n")
	return truncateOpenAIWSLogValue(value, 160)
}

func grokObserveSpanFromPayload(span *grokObserveSpan, payload []byte, upstreamModel string) *grokObserveSpan {
	out := &grokObserveSpan{}
	if span != nil {
		*out = *span
	}
	if strings.TrimSpace(upstreamModel) != "" {
		out.UpstreamModel = strings.TrimSpace(upstreamModel)
	}
	if len(payload) == 0 {
		return out
	}
	if out.PromptCacheKey == "" {
		out.PromptCacheKey = strings.TrimSpace(gjson.GetBytes(payload, "prompt_cache_key").String())
	}
	if out.PreviousResponseID == "" {
		out.PreviousResponseID = strings.TrimSpace(gjson.GetBytes(payload, "previous_response_id").String())
	}
	return out
}

func observeGrokPatchPipeline(span *grokObserveSpan, stage string, before, after []byte, extra map[string]string) {
	if !grokObserveEnabled() {
		return
	}
	fields := map[string]string{
		"stage":              stage,
		"inbound_bytes":      fmt.Sprintf("%d", len(before)),
		"outbound_bytes":     fmt.Sprintf("%d", len(after)),
		"input_items_before": fmt.Sprintf("%d", len(gjson.GetBytes(before, "input").Array())),
		"input_items_after":  fmt.Sprintf("%d", len(gjson.GetBytes(after, "input").Array())),
		"input_summary_before": truncateGrokObserveValue(
			summarizeGrokResponsesInputItems(gjson.GetBytes(before, "input")),
		),
		"input_summary_after": truncateGrokObserveValue(
			summarizeGrokResponsesInputItems(gjson.GetBytes(after, "input")),
		),
	}
	for key, value := range extra {
		fields[key] = value
	}
	observeGrokEvent(span, "patch_stage", fields)
}

func observeGrokCollapseDecision(span *grokObserveSpan, strategy string, before, after []any, preserveAssistants, recallDetected bool, droppedAssistantChars int) {
	if !grokObserveEnabled() {
		return
	}
	userPreview := truncateGrokObserveValue(grokLatestUserMessageText(after))
	observeGrokEvent(span, "collapse_decision", map[string]string{
		"collapse_strategy":       strategy,
		"input_items_before":      fmt.Sprintf("%d", len(before)),
		"input_items_after":       fmt.Sprintf("%d", len(after)),
		"preserve_assistants":     fmt.Sprintf("%v", preserveAssistants),
		"recall_detected":         fmt.Sprintf("%v", recallDetected),
		"dropped_assistant_chars": fmt.Sprintf("%d", droppedAssistantChars),
		"user_text_preview":       userPreview,
		"input_summary_before": truncateGrokObserveValue(
			summarizeGrokResponsesInputItems(mustJSONArrayGrokObserve(before)),
		),
		"input_summary_after": truncateGrokObserveValue(
			summarizeGrokResponsesInputItems(mustJSONArrayGrokObserve(after)),
		),
	})
}

func grokCollapseStrategyName(input []any) string {
	if len(input) == 0 {
		return "empty"
	}
	switch {
	case grokUserMessageReferencesPriorContext(grokLatestUserMessageText(input)):
		return "recall_dialogue"
	case isGrokTrailingUserContinuation(input):
		return "trailing_user_only"
	case needsGrokTrailingToolContinuation(input):
		return "tool_continuation"
	default:
		lastAssistantIdx := -1
		for i, rawItem := range input {
			if isGrokAssistantInputItem(rawItem) {
				lastAssistantIdx = i
			}
		}
		if lastAssistantIdx >= 0 && lastAssistantIdx+1 < len(input) {
			return "after_last_assistant"
		}
		return "trailing_fallback"
	}
}

func mustJSONArrayGrokObserve(items []any) gjson.Result {
	raw, err := json.Marshal(items)
	if err != nil {
		return gjson.Result{}
	}
	return gjson.GetBytes(raw, "#")
}

func analyzeGrokCompletedResponse(completed []byte) grokResponseAnalysis {
	analysis := grokResponseAnalysis{}
	if len(completed) == 0 {
		return analysis
	}
	analysis.SequenceNumber = gjson.GetBytes(completed, "sequence_number").Int()
	output := gjson.GetBytes(completed, "response.output")
	if !output.IsArray() {
		output = gjson.GetBytes(completed, "output")
	}
	for _, item := range output.Array() {
		itemType := strings.TrimSpace(item.Get("type").String())
		switch itemType {
		case "reasoning":
			analysis.ReasoningItemCount++
			if enc := strings.TrimSpace(item.Get("encrypted_content").String()); enc != "" {
				analysis.HasEncryptedReasoning = true
			}
			for _, part := range item.Get("content").Array() {
				if text := strings.TrimSpace(part.Get("text").String()); text != "" {
					analysis.ReasoningTextChars += len(text)
					if analysis.ReasoningPreview == "" {
						analysis.ReasoningPreview = truncateGrokObserveValue(text)
					}
				}
			}
		case "message":
			for _, part := range item.Get("content").Array() {
				if text := strings.TrimSpace(part.Get("text").String()); text != "" {
					analysis.OutputTextChars += len(text)
					if analysis.OutputPreview == "" {
						analysis.OutputPreview = truncateGrokObserveValue(text)
					}
				}
			}
		default:
			for _, part := range item.Get("content").Array() {
				if text := strings.TrimSpace(part.Get("text").String()); text != "" {
					analysis.OutputTextChars += len(text)
					if analysis.OutputPreview == "" {
						analysis.OutputPreview = truncateGrokObserveValue(text)
					}
				}
			}
		}
		analysis.OutputItemCount++
	}
	analysis.Anomalies = detectGrokOutputAnomalies(completed)
	return analysis
}

func detectGrokOutputAnomalies(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	text := extractGrokObserveCombinedText(raw)
	if text == "" {
		return nil
	}
	anomalies := make([]string, 0, 8)
	checks := []struct {
		name    string
		matcher func(string) bool
	}{
		{"eval_step_rollout", func(s string) bool { return strings.Contains(s, "【步骤") }},
		{"reasoning_live_rollout", func(s string) bool {
			return strings.Contains(s, "REASONING_LIVE_2") ||
				(strings.Contains(s, "REASONING_LIVE_") && strings.Contains(s, "【步骤"))
		}},
		{"agent_step_json", func(s string) bool {
			return strings.Contains(s, "current_step") && strings.Contains(s, "\"history\"")
		}},
		{"think_token_leak", func(s string) bool {
			return strings.Contains(s, "</think>") || strings.Contains(s, "</think>") || strings.Contains(s, "｜Assistant｜")
		}},
		{"large_output_text", func(s string) bool { return len(s) >= 4096 }},
		{"very_large_output_text", func(s string) bool { return len(s) >= 16384 }},
		{"codex_encrypted_in_output", func(s string) bool { return strings.Contains(s, "gAAAA") }},
		{"multi_turn_eval_banner", func(s string) bool {
			return strings.Contains(s, "============================================================")
		}},
	}
	for _, check := range checks {
		if check.matcher(text) {
			anomalies = append(anomalies, check.name)
		}
	}
	return anomalies
}

func extractGrokObserveCombinedText(raw []byte) string {
	var parts []string
	output := gjson.GetBytes(raw, "response.output")
	if !output.IsArray() {
		output = gjson.GetBytes(raw, "output")
	}
	for _, item := range output.Array() {
		for _, part := range item.Get("content").Array() {
			if text := part.Get("text").String(); text != "" {
				parts = append(parts, text)
			}
		}
		if text := item.Get("text").String(); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func observeGrokUpstreamCompleted(span *grokObserveSpan, requestPayload, completed []byte, responseID string) {
	if !grokObserveEnabled() {
		return
	}
	analysis := analyzeGrokCompletedResponse(completed)
	fields := map[string]string{
		"response_id":           responseID,
		"sequence_number":     fmt.Sprintf("%d", analysis.SequenceNumber),
		"output_text_chars":   fmt.Sprintf("%d", analysis.OutputTextChars),
		"reasoning_text_chars": fmt.Sprintf("%d", analysis.ReasoningTextChars),
		"output_item_count":   fmt.Sprintf("%d", analysis.OutputItemCount),
		"reasoning_item_count": fmt.Sprintf("%d", analysis.ReasoningItemCount),
		"has_encrypted_reasoning": fmt.Sprintf("%v", analysis.HasEncryptedReasoning),
		"anomaly_count":       fmt.Sprintf("%d", len(analysis.Anomalies)),
		"output_preview":      analysis.OutputPreview,
		"reasoning_preview":   analysis.ReasoningPreview,
		"request_bytes":       fmt.Sprintf("%d", len(requestPayload)),
	}
	if len(analysis.Anomalies) > 0 {
		fields["anomalies"] = strings.Join(analysis.Anomalies, ",")
		fields["root_cause_hint"] = grokAnomalyRootCauseHint(analysis.Anomalies, analysis.OutputTextChars, analysis.ReasoningTextChars)
	}
	observeGrokEvent(span, "upstream_response", fields)
}

func grokAnomalyRootCauseHint(anomalies []string, outputChars, reasoningChars int) string {
	set := make(map[string]struct{}, len(anomalies))
	for _, anomaly := range anomalies {
		set[anomaly] = struct{}{}
	}
	if len(set) == 0 {
		return "clean"
	}
	if _, ok := set["eval_step_rollout"]; ok {
		return "upstream_eval_rollout_regurgitation"
	}
	if _, ok := set["reasoning_live_rollout"]; ok {
		return "upstream_eval_rollout_regurgitation"
	}
	if _, ok := set["agent_step_json"]; ok {
		return "upstream_agent_json_leak"
	}
	if _, ok := set["think_token_leak"]; ok {
		return "upstream_format_token_leak"
	}
	if _, ok := set["very_large_output_text"]; ok {
		return "upstream_oversized_output_text"
	}
	if _, ok := set["large_output_text"]; ok {
		return "upstream_oversized_output_text"
	}
	if outputChars > 1024 && reasoningChars < 256 {
		return "output_text_oversized_vs_reasoning"
	}
	return "upstream_output_anomaly"
}

func countGrokEncryptedContentStats(body []byte) (codexRejected, preserved, dropped int) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return 0, 0, 0
	}
	for _, item := range input.Array() {
		itemType := strings.TrimSpace(item.Get("type").String())
		if itemType != "reasoning" && itemType != "compaction" {
			continue
		}
		encrypted := item.Get("encrypted_content")
		if !encrypted.Exists() {
			continue
		}
		if encrypted.Type == gjson.Null {
			dropped++
			continue
		}
		if encrypted.Type != gjson.String {
			dropped++
			continue
		}
		value := encrypted.String()
		if strings.HasPrefix(value, "gAAAA") {
			codexRejected++
			continue
		}
		if _, err := xai.InspectGrokEncryptedContent(value); err == nil {
			preserved++
		} else {
			dropped++
		}
	}
	return codexRejected, preserved, dropped
}