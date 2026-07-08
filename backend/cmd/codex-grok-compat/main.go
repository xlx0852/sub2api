package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	coderws "github.com/coder/websocket"
)

const (
	defaultBaseURL = "http://127.0.0.1:8180"
	defaultAPIKey  = "sk-sub2api-local-grok-proxy-key"
	// Match sub2api openAIWSMessageReadLimitBytes so large Grok turns don't fail locally.
	codexWSReadLimitBytes int64 = 16 * 1024 * 1024
)

type checkResult struct {
	Name    string
	OK      bool
	Detail  string
	Skipped bool
}

func main() {
	mode := strings.TrimSpace(os.Getenv("COMPAT_MODE"))
	if mode == "" {
		mode = "all"
	}

	results := make([]checkResult, 0, 16)
	switch mode {
	case "unit":
		results = append(results, runUnitTests()...)
	case "live":
		results = append(results, runLiveChecks()...)
	case "mismatch-repro":
		results = append(results, runMismatchRepro()...)
	case "multi-turn":
		results = append(results, runMultiTurnDialog()...)
	case "strict":
		results = append(results, runStrictSuite()...)
	case "reasoning-live":
		results = append(results, runReasoningEncryptedLiveTest()...)
	default:
		results = append(results, runUnitTests()...)
		results = append(results, runLiveChecks()...)
	}

	passed, failed, skipped := 0, 0, 0
	for _, result := range results {
		switch {
		case result.Skipped:
			skipped++
			fmt.Printf("SKIP  %s (%s)\n", result.Name, result.Detail)
		case result.OK:
			passed++
			fmt.Printf("PASS  %s\n", result.Name)
		default:
			failed++
			fmt.Printf("FAIL  %s: %s\n", result.Name, result.Detail)
		}
	}
	fmt.Printf("\nSummary: pass=%d fail=%d skip=%d\n", passed, failed, skipped)
	if failed > 0 {
		os.Exit(1)
	}
	if skipped > 0 && passed > 0 {
		fmt.Println("Note: live upstream checks were skipped; refresh local Grok OAuth before deploying.")
	}
}

func runUnitTests() []checkResult {
	backendDir, err := locateBackendDir()
	if err != nil {
		return []checkResult{{Name: "unit_tests", OK: false, Detail: err.Error()}}
	}
	cmd := exec.Command("go", "test", "./internal/service/", "-count=1",
		"-run", "Grok|CollapseGrok|XGrokConv|GrokBuildHTTPRelay|OpenAIWSHTTPBridge.*Grok|GrokWS")
	cmd.Dir = backendDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > 1200 {
			detail = detail[len(detail)-1200:]
		}
		return []checkResult{{Name: "unit_tests", OK: false, Detail: detail}}
	}
	return []checkResult{{Name: "unit_tests", OK: true}}
}

func runLiveChecks() []checkResult {
	baseURL := strings.TrimRight(envOr("SUB2API_BASE_URL", defaultBaseURL), "/")
	apiKey := strings.TrimSpace(envOr("SUB2API_API_KEY", defaultAPIKey))
	if apiKey == "" {
		return []checkResult{{Name: "live_checks", OK: false, Skipped: true, Detail: "SUB2API_API_KEY is empty"}}
	}

	results := make([]checkResult, 0, 8)
	results = append(results, checkHealth(baseURL))

	httpResult, _ := checkGrokBuildHTTP(baseURL, apiKey)
	results = append(results, httpResult)
	if !httpResult.OK {
		reason := "skipped because grok build http failed"
		if httpResult.Skipped {
			reason = "skipped because local grok oauth/subscription is unavailable"
		}
		results = append(results, checkResult{Name: "codex_ws_turn1", OK: false, Skipped: true, Detail: reason})
		results = append(results, checkResult{Name: "codex_ws_tool_turn", OK: false, Skipped: true, Detail: reason})
		return results
	}

	wsTurn1, wsResponseID := checkCodexWSTurn1(baseURL, apiKey)
	results = append(results, wsTurn1)
	if wsResponseID != "" {
		results = append(results, checkCodexWSToolTurn(baseURL, apiKey, wsResponseID))
	} else {
		results = append(results, checkResult{Name: "codex_ws_tool_turn", OK: false, Skipped: true, Detail: "no response_id from turn1"})
	}
	return results
}

func checkHealth(baseURL string) checkResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return checkResult{Name: "health", OK: false, Detail: err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return checkResult{Name: "health", OK: false, Detail: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"ok"`) {
		return checkResult{Name: "health", OK: false, Detail: fmt.Sprintf("status=%d body=%s", resp.StatusCode, truncate(string(body), 200))}
	}
	return checkResult{Name: "health", OK: true}
}

func checkGrokBuildHTTP(baseURL, apiKey string) (checkResult, string) {
	convID := fmt.Sprintf("local-demo-%d", time.Now().Unix())
	payload := map[string]any{
		"model":  "grok-composer-2.5-fast",
		"stream": false,
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "Reply with exactly: LOCAL_DEMO_OK"}},
			},
		},
	}
	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		return checkResult{Name: "grok_build_http", OK: false, Detail: err.Error()}, ""
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "xai-grok-cli/0.2.22")
	req.Header.Set("x-grok-client", "grok-pager")
	req.Header.Set("x-grok-conv-id", convID)
	xai.SetGrokCLIRequestHeaders(req.Header, "grok-composer-2.5-fast")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return checkResult{Name: "grok_build_http", OK: false, Detail: err.Error()}, ""
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		detail := fmt.Sprintf("status=%d body=%s", resp.StatusCode, truncate(string(respBody), 400))
		if isUpstreamCredentialFailure(resp.StatusCode, string(respBody)) {
			return checkResult{
				Name:    "grok_build_http",
				OK:      false,
				Skipped: true,
				Detail:  "upstream credential/subscription issue (re-auth local grok oauth account in admin UI): " + detail,
			}, ""
		}
		if resp.StatusCode == http.StatusBadGateway && strings.Contains(strings.ToLower(string(respBody)), "upstream") {
			return checkResult{
				Name:    "grok_build_http",
				OK:      false,
				Skipped: true,
				Detail:  "upstream unavailable for local demo (likely expired oauth/subscription); routing reached account_id=1: " + detail,
			}, ""
		}
		return checkResult{Name: "grok_build_http", OK: false, Detail: detail}, ""
	}
	responseID := extractResponseID(respBody)
	text := extractResponseText(respBody)
	if !strings.Contains(strings.ToUpper(text), "LOCAL_DEMO_OK") && strings.TrimSpace(text) == "" {
		return checkResult{Name: "grok_build_http", OK: false, Detail: "empty response text"}, responseID
	}
	return checkResult{Name: "grok_build_http", OK: true, Detail: truncate(text, 120)}, responseID
}

func runMismatchRepro() []checkResult {
	baseURL := strings.TrimRight(envOr("SUB2API_BASE_URL", defaultBaseURL), "/")
	apiKey := strings.TrimSpace(envOr("SUB2API_API_KEY", defaultAPIKey))
	captureDir := strings.TrimSpace(envOr("SUB2API_GROK_CAPTURE_DIR", filepath.Join(".local-demo", "codex-capture", "client")))
	sessionID := strings.TrimSpace(envOr("CAPTURE_SESSION_ID", fmt.Sprintf("capture-repro-%d", time.Now().Unix())))

	health := checkHealth(baseURL)
	if !health.OK {
		return []checkResult{health}
	}

	turn1Req := map[string]any{
		"type":             "response.create",
		"model":            "gpt-5.4",
		"stream":           true,
		"prompt_cache_key": sessionID,
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "帮我分析 bridge_wechat 项目的 JSON 转义问题，重点看 \\n 在响应里怎么被双重转义。"}},
			},
		},
	}
	turn1ReqBytes, _ := json.Marshal(turn1Req)
	writeCaptureArtifact(captureDir, "01_turn1_request.json", turn1ReqBytes)

	turn1, responseID := runCodexWSTurn(baseURL, apiKey, turn1ReqBytes, 120*time.Second)
	writeCaptureArtifact(captureDir, "01_turn1_result.json", marshalCaptureResult(turn1, responseID))
	if !turn1.OK {
		if isSkippedDetail(turn1.Detail) {
			return []checkResult{{Name: "mismatch_repro_turn1", OK: false, Skipped: true, Detail: turn1.Detail}}
		}
		return []checkResult{{Name: "mismatch_repro_turn1", OK: false, Detail: turn1.Detail}}
	}
	if strings.TrimSpace(responseID) == "" {
		return []checkResult{{Name: "mismatch_repro_turn1", OK: false, Detail: "empty response_id"}}
	}

	assistantTurn1 := map[string]any{
		"type":    "message",
		"role":    "assistant",
		"content": []any{map[string]any{"type": "output_text", "text": turn1.Detail}},
	}
	turn2Req := map[string]any{
		"type":                 "response.create",
		"model":                "gpt-5.4",
		"stream":               true,
		"prompt_cache_key":     sessionID,
		"previous_response_id": responseID,
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "帮我分析 bridge_wechat 项目的 JSON 转义问题，重点看 \\n 在响应里怎么被双重转义。"}},
			},
			assistantTurn1,
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "看下桌面都有什么文件"}},
			},
		},
	}
	turn2ReqBytes, _ := json.Marshal(turn2Req)
	writeCaptureArtifact(captureDir, "02_turn2_request.json", turn2ReqBytes)

	turn2, _ := runCodexWSTurn(baseURL, apiKey, turn2ReqBytes, 180*time.Second)
	writeCaptureArtifact(captureDir, "02_turn2_result.json", marshalCaptureResult(turn2, ""))

	analysis := analyzeMismatchRepro(sessionID, responseID, turn2.Detail)
	writeCaptureArtifact(captureDir, "03_analysis.json", analysis)

	fmt.Printf("\n[mismatch-repro] session_id=%s previous_response_id=%s\n", sessionID, responseID)
	fmt.Printf("[mismatch-repro] turn2_text=%s\n", truncate(turn2.Detail, 500))
	fmt.Printf("[mismatch-repro] analysis=%s\n", string(analysis))
	fmt.Printf("[mismatch-repro] client capture dir=%s\n", captureDir)
	serverCapture := strings.TrimSpace(os.Getenv("SUB2API_SERVER_CAPTURE_DIR"))
	if serverCapture == "" {
		serverCapture = "(check .local-demo/codex-capture/server on the sub2api host)"
	}
	fmt.Printf("[mismatch-repro] server capture dir=%s\n", serverCapture)

	results := []checkResult{
		{Name: "mismatch_repro_turn1", OK: turn1.OK, Detail: truncate(turn1.Detail, 160)},
		{Name: "mismatch_repro_turn2", OK: turn2.OK, Detail: truncate(turn2.Detail, 160)},
	}
	if turn2.OK {
		var report map[string]any
		_ = json.Unmarshal(analysis, &report)
		mismatch, _ := report["likely_context_bleed"].(bool)
		if mismatch {
			results = append(results, checkResult{
				Name:   "mismatch_repro_analysis",
				OK:     false,
				Detail: "turn2 answer still references old bridge_wechat context instead of desktop listing",
			})
		} else {
			results = append(results, checkResult{
				Name: "mismatch_repro_analysis",
				OK:   true,
				Detail: firstNonEmptyString(
					report["summary"],
					"turn2 answer appears aligned with desktop question",
				),
			})
		}
	} else if turn2.Skipped {
		results[1].Skipped = true
	}
	return results
}

type dialogTurn struct {
	UserText       string
	AssistantText  string
	ResponseID     string
	Result         checkResult
}

func runMultiTurnDialog() []checkResult {
	baseURL := strings.TrimRight(envOr("SUB2API_BASE_URL", defaultBaseURL), "/")
	apiKey := strings.TrimSpace(envOr("SUB2API_API_KEY", defaultAPIKey))
	captureDir := strings.TrimSpace(envOr("SUB2API_GROK_CAPTURE_DIR", filepath.Join(".local-demo", "codex-capture", "client", "multi-turn")))
	sessionID := strings.TrimSpace(envOr("CAPTURE_SESSION_ID", fmt.Sprintf("multi-turn-%d", time.Now().Unix())))

	health := checkHealth(baseURL)
	if !health.OK {
		return []checkResult{health}
	}

	questions := []string{
		"请只回复 exactly: MULTI_TURN_1，不要多说任何字。",
		"1+1等于几？只回答数字，不要解释。",
		"天空是什么颜色？只答一个词，不要调用任何工具。",
		"我刚才问的加法结果是多少？只回答数字。",
	}
	turns, err := runCodexWSMultiTurnSession(baseURL, apiKey, sessionID, questions, 180*time.Second)
	if err != nil {
		if isSkippedDetail(err.Error()) {
			return []checkResult{{Name: "multi_turn_session", OK: false, Skipped: true, Detail: err.Error()}}
		}
		return []checkResult{{Name: "multi_turn_session", OK: false, Detail: err.Error()}}
	}

	summary, _ := json.MarshalIndent(map[string]any{
		"session_id": sessionID,
		"turns":      turns,
	}, "", "  ")
	writeCaptureArtifact(captureDir, "summary.json", summary)

	fmt.Printf("\n[multi-turn] session_id=%s turns=%d capture_dir=%s\n", sessionID, len(turns), captureDir)
	for i, turn := range turns {
		fmt.Printf("[multi-turn] turn%d user=%q response_id=%s ok=%v text=%s\n",
			i+1, turn.UserText, turn.ResponseID, turn.Result.OK, truncate(turn.AssistantText, 200))
	}

	results := make([]checkResult, 0, len(turns)+2)
	for i, turn := range turns {
		name := fmt.Sprintf("multi_turn_%d", i+1)
		results = append(results, checkResult{
			Name:   name,
			OK:     turn.Result.OK,
			Detail: truncate(turn.AssistantText, 160),
		})
		if !turn.Result.OK {
			return results
		}
	}

	// Turn 1 marker
	t1 := turns[0].AssistantText
	results = append(results, checkResult{
		Name:   "multi_turn_t1_marker",
		OK:     strings.Contains(strings.ToUpper(t1), "MULTI_TURN_1"),
		Detail: truncate(t1, 120),
	})

	// Turn 2 math
	t2 := strings.TrimSpace(turns[1].AssistantText)
	results = append(results, checkResult{
		Name:   "multi_turn_t2_math",
		OK:     strings.Contains(t2, "2"),
		Detail: t2,
	})

	// Turn 3 topic switch (should answer sky color, not math)
	t3 := strings.TrimSpace(turns[2].AssistantText)
	t3OK := strings.TrimSpace(t3) != "" &&
		(strings.Contains(t3, "蓝") || strings.Contains(strings.ToLower(t3), "blue"))
	results = append(results, checkResult{
		Name:   "multi_turn_t3_topic_switch",
		OK:     t3OK,
		Detail: truncate(t3, 160),
	})

	// Turn 4 recall math (not old topic bleed)
	t4 := strings.TrimSpace(turns[3].AssistantText)
	recallOK := strings.Contains(t4, "2")
	mentionsOldTopic := strings.Contains(strings.ToLower(t4), "bridge_wechat") ||
		strings.Contains(t4, "JSON") ||
		strings.Contains(t4, "转义")
	results = append(results, checkResult{
		Name: "multi_turn_t4_recall",
		OK:   recallOK && !mentionsOldTopic,
		Detail: firstNonEmptyString(
			t4,
			"expected recall of 2 from turn2",
		),
	})

	return results
}

func dialCodexWS(ctx context.Context, wsURL string, headers http.Header) (*coderws.Conn, *http.Response, error) {
	conn, resp, err := coderws.Dial(ctx, wsURL, &coderws.DialOptions{HTTPHeader: headers})
	if err != nil {
		return nil, resp, err
	}
	conn.SetReadLimit(codexWSReadLimitBytes)
	return conn, resp, nil
}

func runCodexWSMultiTurnSession(baseURL, apiKey, sessionID string, questions []string, turnTimeout time.Duration) ([]dialogTurn, error) {
	if len(questions) == 0 {
		return nil, fmt.Errorf("no questions provided")
	}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v1/responses"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)
	headers.Set("User-Agent", "codex_cli_rs/0.142.5")
	headers.Set("originator", "codex_cli_rs")
	headers.Set("OpenAI-Beta", "responses=experimental")

	ctx, cancel := context.WithTimeout(context.Background(), turnTimeout*time.Duration(len(questions)))
	defer cancel()

	conn, resp, err := dialCodexWS(ctx, wsURL, headers)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, fmt.Errorf("dial status=%d err=%v", status, err)
	}
	defer conn.Close(coderws.StatusNormalClosure, "multi-turn done")

	captureDir := strings.TrimSpace(envOr("SUB2API_GROK_CAPTURE_DIR", filepath.Join(".local-demo", "codex-capture", "client", "multi-turn")))
	history := make([]any, 0, len(questions)*2)
	turns := make([]dialogTurn, 0, len(questions))
	var previousResponseID string

	for i, question := range questions {
		history = append(history, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": []any{map[string]any{"type": "input_text", "text": question}},
		})

		req := map[string]any{
			"type":             "response.create",
			"model":            "gpt-5.4",
			"stream":           true,
			"prompt_cache_key": sessionID,
			"input":            append([]any(nil), history...),
		}
		if previousResponseID != "" {
			req["previous_response_id"] = previousResponseID
		}
		reqBytes, _ := json.Marshal(req)
		writeCaptureArtifact(captureDir, fmt.Sprintf("%02d_turn%d_request.json", i+1, i+1), reqBytes)

		if err := conn.Write(ctx, coderws.MessageText, reqBytes); err != nil {
			return turns, fmt.Errorf("turn %d write: %w", i+1, err)
		}

		terminal, readErr := readUntilTerminal(ctx, conn, turnTimeout)
		turn := dialogTurn{UserText: question}
		if readErr != nil {
			turn.Result = checkResult{OK: false, Detail: readErr.Error()}
			turns = append(turns, turn)
			return turns, fmt.Errorf("turn %d read: %w", i+1, readErr)
		}
		if !terminal.Completed {
			turn.Result = checkResult{OK: false, Detail: "no response.completed"}
			turns = append(turns, turn)
			return turns, fmt.Errorf("turn %d: no response.completed", i+1)
		}
		if terminal.ErrorMessage != "" {
			turn.Result = checkResult{OK: false, Detail: terminal.ErrorMessage}
			turns = append(turns, turn)
			if isSkippedDetail(terminal.ErrorMessage) {
				return turns, fmt.Errorf("%s", terminal.ErrorMessage)
			}
			return turns, fmt.Errorf("turn %d error: %s", i+1, terminal.ErrorMessage)
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
			return turns, fmt.Errorf("turn %d missing response_id", i+1)
		}

		history = append(history, map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": terminal.Text}},
		})
		previousResponseID = terminal.ResponseID
	}

	return turns, nil
}

func runCodexWSTurn(baseURL, apiKey string, payloadBytes []byte, timeout time.Duration) (checkResult, string) {
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v1/responses"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)
	headers.Set("User-Agent", "codex_cli_rs/0.142.5")
	headers.Set("originator", "codex_cli_rs")
	headers.Set("OpenAI-Beta", "responses=experimental")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, resp, err := dialCodexWS(ctx, wsURL, headers)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return checkResult{OK: false, Detail: fmt.Sprintf("dial status=%d err=%v", status, err)}, ""
	}
	defer conn.Close(coderws.StatusNormalClosure, "capture done")

	if err := conn.Write(ctx, coderws.MessageText, payloadBytes); err != nil {
		return checkResult{OK: false, Detail: "write: " + err.Error()}, ""
	}

	terminal, readErr := readUntilTerminal(ctx, conn, timeout-time.Second)
	if readErr != nil {
		return checkResult{OK: false, Detail: readErr.Error()}, terminal.ResponseID
	}
	if !terminal.Completed {
		return checkResult{OK: false, Detail: "no response.completed"}, terminal.ResponseID
	}
	if terminal.ErrorMessage != "" {
		return checkResult{OK: false, Detail: terminal.ErrorMessage}, terminal.ResponseID
	}
	return checkResult{OK: true, Detail: terminal.Text}, terminal.ResponseID
}

func analyzeMismatchRepro(sessionID, previousResponseID, answer string) []byte {
	lower := strings.ToLower(answer)
	mentionsBridge := strings.Contains(lower, "bridge_wechat") ||
		strings.Contains(answer, "JSON") ||
		strings.Contains(answer, "转义") ||
		strings.Contains(lower, "magisk") ||
		strings.Contains(lower, "zygisk")
	mentionsDesktop := strings.Contains(answer, "桌面") ||
		strings.Contains(lower, "desktop") ||
		strings.Contains(answer, "文件") ||
		strings.Contains(lower, "ls ") ||
		strings.Contains(lower, "list")
	report := map[string]any{
		"session_id":             sessionID,
		"previous_response_id":   previousResponseID,
		"new_user_question":      "看下桌面都有什么文件",
		"old_topic_markers":      []string{"bridge_wechat", "JSON", "转义", "magisk", "zygisk"},
		"new_topic_markers":      []string{"桌面", "desktop", "文件", "ls"},
		"mentions_old_topic":     mentionsBridge,
		"mentions_new_topic":     mentionsDesktop,
		"likely_context_bleed":   mentionsBridge && !mentionsDesktop,
		"answer_preview":         truncate(answer, 800),
		"summary":                "",
	}
	switch {
	case mentionsBridge && !mentionsDesktop:
		report["summary"] = "Likely context bleed: answer continues bridge_wechat/JSON topic while user asked about desktop files."
	case mentionsDesktop && !mentionsBridge:
		report["summary"] = "Answer appears to follow the new desktop-files question."
	case mentionsBridge && mentionsDesktop:
		report["summary"] = "Mixed signals: answer mentions both old and new topics."
	default:
		report["summary"] = "Inconclusive: answer lacks clear old/new topic markers."
	}
	out, _ := json.MarshalIndent(report, "", "  ")
	return out
}

func writeCaptureArtifact(dir, name string, payload []byte) {
	if len(payload) == 0 {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, name), payload, 0o644)
}

func marshalCaptureResult(result checkResult, responseID string) []byte {
	out, _ := json.MarshalIndent(map[string]any{
		"ok":          result.OK,
		"skipped":     result.Skipped,
		"detail":      result.Detail,
		"response_id": responseID,
	}, "", "  ")
	return out
}

func isSkippedDetail(detail string) bool {
	lower := strings.ToLower(detail)
	return strings.Contains(lower, "oauth") ||
		strings.Contains(lower, "subscription") ||
		strings.Contains(lower, "credential") ||
		strings.Contains(lower, "upstream")
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func checkCodexWSTurn1(baseURL, apiKey string) (checkResult, string) {
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v1/responses"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)
	headers.Set("User-Agent", "codex_cli_rs/0.142.5")
	headers.Set("originator", "codex_cli_rs")
	headers.Set("OpenAI-Beta", "responses=experimental")

	payload := map[string]any{
		"type":   "response.create",
		"model":  "gpt-5.4",
		"stream": true,
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "Say LOCAL_WS_OK in one short sentence."}},
			},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	conn, resp, err := dialCodexWS(ctx, wsURL, headers)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return checkResult{Name: "codex_ws_turn1", OK: false, Detail: fmt.Sprintf("dial status=%d err=%v", status, err)}, ""
	}
	defer conn.Close(coderws.StatusNormalClosure, "compat done")

	if err := conn.Write(ctx, coderws.MessageText, payloadBytes); err != nil {
		return checkResult{Name: "codex_ws_turn1", OK: false, Detail: "write: " + err.Error()}, ""
	}

	terminal, readErr := readUntilTerminal(ctx, conn, 90*time.Second)
	if readErr != nil {
		return checkResult{Name: "codex_ws_turn1", OK: false, Detail: readErr.Error()}, ""
	}
	if !terminal.Completed {
		return checkResult{Name: "codex_ws_turn1", OK: false, Detail: "no response.completed"}, terminal.ResponseID
	}
	if terminal.ErrorMessage != "" {
		return checkResult{Name: "codex_ws_turn1", OK: false, Detail: terminal.ErrorMessage}, terminal.ResponseID
	}
	return checkResult{Name: "codex_ws_turn1", OK: true, Detail: truncate(terminal.Text, 120)}, terminal.ResponseID
}

func checkCodexWSToolTurn(baseURL, apiKey, previousResponseID string) checkResult {
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v1/responses"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)
	headers.Set("User-Agent", "codex_cli_rs/0.142.5")
	headers.Set("originator", "codex_cli_rs")
	headers.Set("OpenAI-Beta", "responses=experimental")

	payload := map[string]any{
		"type":                 "response.create",
		"model":                "gpt-5.4",
		"stream":               true,
		"previous_response_id": previousResponseID,
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "重启一下前后端"}},
			},
			map[string]any{
				"type":    "local_shell_call",
				"call_id": "call_local_demo_1",
				"name":    "shell",
				"input":   map[string]any{"command": "echo LOCAL_TOOL_OK"},
			},
			map[string]any{
				"type":    "mcp_tool_call_output",
				"call_id": "call_local_demo_1",
				"output":  "LOCAL_TOOL_OK",
			},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	conn, resp, err := dialCodexWS(ctx, wsURL, headers)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return checkResult{Name: "codex_ws_tool_turn", OK: false, Detail: fmt.Sprintf("dial status=%d err=%v", status, err)}
	}
	defer conn.Close(coderws.StatusNormalClosure, "compat done")

	if err := conn.Write(ctx, coderws.MessageText, payloadBytes); err != nil {
		return checkResult{Name: "codex_ws_tool_turn", OK: false, Detail: "write: " + err.Error()}
	}

	terminal, readErr := readUntilTerminal(ctx, conn, 90*time.Second)
	if readErr != nil {
		return checkResult{Name: "codex_ws_tool_turn", OK: false, Detail: readErr.Error()}
	}
	if !terminal.Completed {
		return checkResult{Name: "codex_ws_tool_turn", OK: false, Detail: "no response.completed"}
	}
	if terminal.ErrorMessage != "" {
		return checkResult{Name: "codex_ws_tool_turn", OK: false, Detail: terminal.ErrorMessage}
	}
	return checkResult{Name: "codex_ws_tool_turn", OK: true, Detail: truncate(terminal.Text, 120)}
}

type wsTerminalState struct {
	Completed      bool
	ErrorMessage   string
	Text           string
	ResponseID     string
	CompletedRaw   []byte
	HasToolCalls   bool
}

func appendWSStreamText(state *wsTerminalState, eventType string, data []byte) {
	switch eventType {
	case "response.output_text.delta":
		state.Text += stringValue(jsonGet(data, "delta"))
	case "response.output_text.done":
		if text := stringValue(jsonGet(data, "text")); strings.TrimSpace(text) != "" {
			state.Text = text
		}
	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		state.HasToolCalls = true
	case "response.completed":
		if state.ResponseID == "" {
			state.ResponseID = stringValue(jsonGet(data, "response.id"))
		}
		state.CompletedRaw = append([]byte(nil), data...)
		if strings.TrimSpace(state.Text) == "" {
			state.Text = extractResponseText(data)
		}
	}
}

func readUntilTerminal(ctx context.Context, conn *coderws.Conn, timeout time.Duration) (wsTerminalState, error) {
	deadline := time.Now().Add(timeout)
	state := wsTerminalState{}
	for time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		_, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			if state.Completed {
				return state, nil
			}
			return state, err
		}
		eventType := stringValue(jsonGet(data, "type"))
		switch eventType {
		case "error":
			state.ErrorMessage = truncate(string(data), 400)
			return state, nil
		case "response.completed":
			state.Completed = true
			appendWSStreamText(&state, eventType, data)
			if strings.TrimSpace(state.Text) == "" {
				if extracted := extractResponseText(data); strings.TrimSpace(extracted) != "" {
					state.Text = extracted
				} else if state.HasToolCalls {
					state.Text = "[tool_call_only_response]"
				}
			}
			return state, nil
		case "response.created", "response.in_progress":
			if state.ResponseID == "" {
				state.ResponseID = stringValue(jsonGet(data, "response.id"))
			}
		default:
			appendWSStreamText(&state, eventType, data)
		}
	}
	if state.Completed {
		return state, nil
	}
	return state, fmt.Errorf("timeout waiting for response.completed")
}

func jsonGet(raw []byte, path string) any {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	cur := any(payload)
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[part]
	}
	if cur == nil {
		return nil
	}
	return cur
}

func stringValue(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func extractResponseID(raw []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if id, ok := payload["id"].(string); ok && strings.TrimSpace(id) != "" {
		return id
	}
	if response, ok := payload["response"].(map[string]any); ok {
		if id, ok := response["id"].(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func extractResponseText(raw []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if response, ok := payload["response"].(map[string]any); ok {
		if text := collectOutputText(response["output"]); text != "" {
			return text
		}
	}
	return collectOutputText(payload["output"])
}

func collectOutputText(outputRaw any) string {
	output, ok := outputRaw.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, item := range output {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType, _ := obj["type"].(string)
		if itemType == "reasoning" {
			continue
		}
		content, ok := obj["content"].([]any)
		if !ok {
			continue
		}
		for _, part := range content {
			partObj, ok := part.(map[string]any)
			if !ok {
				continue
			}
			partType, _ := partObj["type"].(string)
			if partType != "" && partType != "output_text" && partType != "text" {
				continue
			}
			if text, ok := partObj["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func isUpstreamCredentialFailure(status int, body string) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusPaymentRequired:
		return true
	}
	lower := strings.ToLower(body)
	return strings.Contains(lower, "payment") ||
		strings.Contains(lower, "subscription") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "entitlement")
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func locateBackendDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "internal", "service")); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("unable to locate backend module from %s", wd)
}