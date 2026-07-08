package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	coderws "github.com/coder/websocket"
)

const codexFakeEncryptedContentPrefix = "gAAAAABkZWFkYmVlZi1mYWtlLWNvZGV4LXJlYXNvbmluZy1zaWduYXR1cmU="

func runReasoningEncryptedLiveTest() []checkResult {
	baseURL := strings.TrimRight(envOr("SUB2API_BASE_URL", defaultBaseURL), "/")
	apiKey := strings.TrimSpace(envOr("SUB2API_API_KEY", defaultAPIKey))
	if apiKey == "" {
		return []checkResult{{Name: "reasoning_live_suite", OK: false, Skipped: true, Detail: "SUB2API_API_KEY empty"}}
	}

	results := make([]checkResult, 0, 16)
	health := checkHealth(baseURL)
	results = append(results, health)
	if !health.OK {
		return results
	}

	sessionID := strings.TrimSpace(envOr("CAPTURE_SESSION_ID", fmt.Sprintf("reasoning-live-%d", time.Now().Unix())))
	clientCaptureDir := strings.TrimSpace(envOr("SUB2API_GROK_CAPTURE_DIR", filepath.Join(".local-demo", "codex-capture", "client", "reasoning-live")))
	serverCaptureDir := strings.TrimSpace(os.Getenv("SUB2API_SERVER_CAPTURE_DIR"))

	questions := []string{
		"请只回复 exactly: REASONING_LIVE_1，不要多说任何字。",
		"23+19等于几？只回答数字，不要解释。",
		"草是什么颜色？只答一个词，不要调用任何工具。",
		"我刚才问的加法结果是多少？只回答数字。",
	}

	turns, diagnostics, err := runCodexWSReasoningEncryptedSession(baseURL, apiKey, sessionID, questions, clientCaptureDir, 200*time.Second)
	if err != nil {
		if isSkippedDetail(err.Error()) {
			return append(results, checkResult{Name: "reasoning_live_session", OK: false, Skipped: true, Detail: err.Error()})
		}
		return append(results, checkResult{Name: "reasoning_live_session", OK: false, Detail: err.Error()})
	}

	summary, _ := json.MarshalIndent(map[string]any{
		"session_id":  sessionID,
		"turns":       turns,
		"diagnostics": diagnostics,
	}, "", "  ")
	writeCaptureArtifact(clientCaptureDir, "summary.json", summary)

	fmt.Printf("\n[reasoning-live] session_id=%s turns=%d capture_dir=%s\n", sessionID, len(turns), clientCaptureDir)
	for i, turn := range turns {
		fmt.Printf("[reasoning-live] turn%d ok=%v invalid_encrypted=%v upstream_reasoning=%v text=%s\n",
			i+1,
			turn.Result.OK,
			turn.HadInvalidEncryptedError,
			turn.UpstreamHadReasoning,
			truncate(turn.AssistantText, 160),
		)
	}

	for i, turn := range turns {
		results = append(results, checkResult{
			Name:   fmt.Sprintf("reasoning_live_turn_%d", i+1),
			OK:     turn.Result.OK && strings.TrimSpace(turn.ResponseID) != "",
			Detail: truncate(turn.AssistantText, 120),
		})
		if turn.HadInvalidEncryptedError {
			results = append(results, checkResult{
				Name:   fmt.Sprintf("reasoning_live_turn_%d_no_invalid_encrypted", i+1),
				OK:     false,
				Detail: "upstream returned invalid_encrypted_content",
			})
		} else {
			results = append(results, checkResult{
				Name: fmt.Sprintf("reasoning_live_turn_%d_no_invalid_encrypted", i+1),
				OK:   true,
			})
		}
	}

	if len(turns) < 4 {
		return results
	}

	results = append(results, checkResult{
		Name: "reasoning_live_t1_marker",
		OK: strings.Contains(strings.ToUpper(turns[0].AssistantText), "REASONING_LIVE_1") &&
			len(strings.TrimSpace(turns[0].AssistantText)) <= 64,
		Detail: truncate(turns[0].AssistantText, 80),
	})
	results = append(results, checkResult{
		Name:   "reasoning_live_t2_math",
		OK:     strictNumericAnswer(turns[1].AssistantText, "42"),
		Detail: strings.TrimSpace(turns[1].AssistantText),
	})
	results = append(results, checkResult{
		Name:   "reasoning_live_t3_topic",
		OK:     strictContainsAny(turns[2].AssistantText, "绿", "green", "青"),
		Detail: truncate(turns[2].AssistantText, 80),
	})
	results = append(results, checkResult{
		Name:   "reasoning_live_t4_recall",
		OK:     strictNumericAnswer(turns[3].AssistantText, "42"),
		Detail: strings.TrimSpace(turns[3].AssistantText),
	})

	if diagnostics.Turn1CapturedGrokReasoning {
		results = append(results, checkResult{
			Name:   "reasoning_live_turn1_upstream_reasoning",
			OK:     true,
			Detail: "turn1 response.completed included reasoning output",
		})
	} else {
		results = append(results, checkResult{
			Name:   "reasoning_live_turn1_upstream_reasoning",
			OK:     true,
			Detail: "turn1 had no upstream reasoning item (replay may still work via cache miss path)",
		})
	}

	if serverCaptureDir != "" {
		ok, detail := verifyServerCaptureOutboundNoCodexEncrypted(serverCaptureDir, sessionID)
		results = append(results, checkResult{
			Name:   "reasoning_live_server_outbound_sanitized",
			OK:     ok,
			Detail: detail,
		})
	} else {
		results = append(results, checkResult{
			Name:    "reasoning_live_server_outbound_sanitized",
			OK:      true,
			Skipped: true,
			Detail:  "set SUB2API_SERVER_CAPTURE_DIR to verify outbound stripping",
		})
	}

	return results
}

type reasoningLiveDiagnostics struct {
	SessionID                  string `json:"session_id"`
	Turn1CapturedGrokReasoning bool   `json:"turn1_captured_grok_reasoning"`
	Turn2InjectedCodexBlobs    int    `json:"turn2_injected_codex_blobs"`
}

type reasoningDialogTurn struct {
	dialogTurn
	HadInvalidEncryptedError bool
	UpstreamHadReasoning     bool
}

func runCodexWSReasoningEncryptedSession(
	baseURL, apiKey, sessionID string,
	questions []string,
	captureDir string,
	turnTimeout time.Duration,
) ([]reasoningDialogTurn, reasoningLiveDiagnostics, error) {
	diagnostics := reasoningLiveDiagnostics{SessionID: sessionID}
	if len(questions) == 0 {
		return nil, diagnostics, fmt.Errorf("no questions provided")
	}

	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v1/responses"
	headers := buildCodexWSHeaders(apiKey)

	ctx, cancel := context.WithTimeout(context.Background(), turnTimeout*time.Duration(len(questions)))
	defer cancel()

	conn, resp, err := dialCodexWS(ctx, wsURL, headers)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, diagnostics, fmt.Errorf("dial status=%d err=%v", status, err)
	}
	defer conn.Close(coderws.StatusNormalClosure, "reasoning-live done")

	history := make([]any, 0, len(questions)*4)
	turns := make([]reasoningDialogTurn, 0, len(questions))
	var previousResponseID string

	for i, question := range questions {
		history = append(history, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": []any{map[string]any{"type": "input_text", "text": question}},
		})

		input := append([]any(nil), history...)
		if i > 0 {
			var injected int
			input, injected = injectCodexEncryptedBeforeTrailingUser(input, i)
			diagnostics.Turn2InjectedCodexBlobs = injected
		}

		req := map[string]any{
			"type":             "response.create",
			"model":            "gpt-5.4",
			"stream":           true,
			"prompt_cache_key": sessionID,
			"input":            input,
		}
		if previousResponseID != "" {
			req["previous_response_id"] = previousResponseID
		}
		reqBytes, _ := json.Marshal(req)
		writeCaptureArtifact(captureDir, fmt.Sprintf("%02d_turn%d_request.json", i+1, i+1), reqBytes)

		if err := conn.Write(ctx, coderws.MessageText, reqBytes); err != nil {
			return turns, diagnostics, fmt.Errorf("turn %d write: %w", i+1, err)
		}

		terminal, readErr := readUntilTerminal(ctx, conn, turnTimeout)
		turn := reasoningDialogTurn{dialogTurn: dialogTurn{UserText: question}}
		if readErr != nil {
			turn.Result = checkResult{OK: false, Detail: readErr.Error()}
			turns = append(turns, turn)
			return turns, diagnostics, fmt.Errorf("turn %d read: %w", i+1, readErr)
		}
		turn.HadInvalidEncryptedError = strings.Contains(strings.ToLower(terminal.ErrorMessage), "invalid_encrypted_content")
		turn.UpstreamHadReasoning = completedHasReasoningOutput(terminal.CompletedRaw)
		if i == 0 && turn.UpstreamHadReasoning {
			diagnostics.Turn1CapturedGrokReasoning = true
		}

		if !terminal.Completed {
			turn.Result = checkResult{OK: false, Detail: "no response.completed"}
			turns = append(turns, turn)
			return turns, diagnostics, fmt.Errorf("turn %d: no response.completed", i+1)
		}
		if terminal.ErrorMessage != "" {
			turn.Result = checkResult{OK: false, Detail: terminal.ErrorMessage}
			turns = append(turns, turn)
			if isSkippedDetail(terminal.ErrorMessage) {
				return turns, diagnostics, fmt.Errorf("%s", terminal.ErrorMessage)
			}
			return turns, diagnostics, fmt.Errorf("turn %d error: %s", i+1, terminal.ErrorMessage)
		}

		turn.AssistantText = terminal.Text
		turn.ResponseID = terminal.ResponseID
		hasText := strings.TrimSpace(terminal.Text) != ""
		turn.Result = checkResult{OK: hasText, Detail: terminal.Text}
		if !hasText && terminal.ResponseID != "" {
			turn.Result = checkResult{
				OK:     true,
				Detail: "[empty_text_but_response_completed response_id=" + terminal.ResponseID + "]",
			}
		}
		turns = append(turns, turn)
		writeCaptureArtifact(captureDir, fmt.Sprintf("%02d_turn%d_result.json", i+1, i+1), marshalCaptureResult(turn.Result, turn.ResponseID))
		if len(terminal.CompletedRaw) > 0 {
			writeCaptureArtifact(captureDir, fmt.Sprintf("%02d_turn%d_completed_raw.json", i+1, i+1), terminal.CompletedRaw)
		}

		if strings.TrimSpace(terminal.ResponseID) == "" {
			return turns, diagnostics, fmt.Errorf("turn %d missing response_id", i+1)
		}

		if reasoningItem := extractReasoningReplayItemFromCompleted(terminal.CompletedRaw); reasoningItem != nil {
			history = append(history, reasoningItem)
		} else if i == 0 {
			history = append(history, codexFakeReasoningItem("rs_turn1_echo", "seed turn reasoning"))
		}
		history = append(history, map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": terminal.Text}},
		})
		previousResponseID = terminal.ResponseID
	}

	return turns, diagnostics, nil
}

func buildCodexWSHeaders(apiKey string) http.Header {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)
	headers.Set("User-Agent", "codex_cli_rs/0.142.5")
	headers.Set("originator", "codex_cli_rs")
	headers.Set("OpenAI-Beta", "responses=experimental")
	headers.Set("X-Codex-Window-Id", "reasoning-live-window")
	return headers
}

func injectCodexEncryptedBeforeTrailingUser(input []any, turnIndex int) ([]any, int) {
	if len(input) == 0 {
		return input, 0
	}
	lastIdx := len(input) - 1
	reasoning := codexFakeReasoningItem(fmt.Sprintf("rs_replay_%d", turnIndex), fmt.Sprintf("prior reasoning turn %d", turnIndex))
	compaction := map[string]any{
		"type":              "compaction",
		"id":                fmt.Sprintf("cmp_codex_%d", turnIndex),
		"encrypted_content": codexFakeEncryptedContentPrefix + "compaction-blob",
	}
	out := make([]any, 0, len(input)+2)
	out = append(out, input[:lastIdx]...)
	out = append(out, compaction, reasoning)
	out = append(out, input[lastIdx])
	return out, 2
}

func codexFakeReasoningItem(id, summary string) map[string]any {
	return map[string]any{
		"type":              "reasoning",
		"id":                id,
		"encrypted_content": codexFakeEncryptedContentPrefix + strings.Repeat("YWJj", 24),
		"summary": []any{
			map[string]any{"type": "summary_text", "text": summary},
		},
	}
}

func completedHasReasoningOutput(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	response, _ := payload["response"].(map[string]any)
	output, _ := response["output"].([]any)
	for _, item := range output {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(stringValue(obj["type"])) == "reasoning" {
			return true
		}
	}
	return false
}

func extractReasoningReplayItemFromCompleted(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	response, _ := payload["response"].(map[string]any)
	output, _ := response["output"].([]any)
	for _, item := range output {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(stringValue(obj["type"])) != "reasoning" {
			continue
		}
		encrypted := stringValue(obj["encrypted_content"])
		if strings.TrimSpace(encrypted) == "" || strings.HasPrefix(encrypted, "gAAAA") {
			continue
		}
		replay := map[string]any{
			"type":    "reasoning",
			"summary": obj["summary"],
		}
		if replay["summary"] == nil {
			replay["summary"] = []any{}
		}
		replay["encrypted_content"] = encrypted
		return replay
	}
	return nil
}

func verifyServerCaptureOutboundNoCodexEncrypted(captureDir, sessionID string) (bool, string) {
	entries, err := os.ReadDir(captureDir)
	if err != nil {
		return false, "read capture dir: " + err.Error()
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	checked := 0
	violations := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_ws_upstream_prepare.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(captureDir, entry.Name()))
		if err != nil {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(raw, &record); err != nil {
			continue
		}
		promptCacheKey := stringValue(record["prompt_cache_key"])
		if !strings.Contains(promptCacheKey, sessionID) && stringValue(record["session_hash"]) != sessionID {
			continue
		}
		outbound, _ := record["outbound"].(map[string]any)
		if outbound == nil {
			continue
		}
		checked++
		if hits := findCodexEncryptedInInput(outbound["input"]); len(hits) > 0 {
			violations = append(violations, fmt.Sprintf("%s:%s", entry.Name(), strings.Join(hits, ",")))
		}
	}
	if checked == 0 {
		return false, "no ws_upstream_prepare capture matched session " + sessionID
	}
	if len(violations) > 0 {
		return false, "outbound still contains Codex encrypted_content in " + strings.Join(violations, "; ")
	}
	return true, fmt.Sprintf("checked %d capture(s); outbound has no gAAAA encrypted_content", checked)
}

func findCodexEncryptedInInput(inputRaw any) []string {
	input, ok := inputRaw.([]any)
	if !ok {
		return nil
	}
	hits := make([]string, 0)
	for i, item := range input {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		encrypted := stringValue(obj["encrypted_content"])
		if strings.HasPrefix(encrypted, "gAAAA") {
			hits = append(hits, fmt.Sprintf("input[%d].%s", i, stringValue(obj["type"])))
		}
	}
	return hits
}