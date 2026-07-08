package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func runStrictSuite() []checkResult {
	baseURL := strings.TrimRight(envOr("SUB2API_BASE_URL", defaultBaseURL), "/")
	apiKey := strings.TrimSpace(envOr("SUB2API_API_KEY", defaultAPIKey))
	if apiKey == "" {
		return []checkResult{{Name: "strict_suite", OK: false, Skipped: true, Detail: "SUB2API_API_KEY empty"}}
	}

	results := make([]checkResult, 0, 32)
	results = append(results, checkHealth(baseURL))

	results = append(results, runStrictMultiTurn(baseURL, apiKey)...)
	results = append(results, runStrictLongDialog(baseURL, apiKey)...)
	results = append(results, runStrictComplexTools(baseURL, apiKey)...)
	results = append(results, runStrictMismatchAfterLong(baseURL, apiKey)...)

	return results
}

func runStrictMultiTurn(baseURL, apiKey string) []checkResult {
	sessionID := fmt.Sprintf("strict-multi-%d", time.Now().Unix())
	questions := []string{
		"请只回复 exactly: STRICT_MULTI_1，不要多说任何字。",
		"17+25等于几？只回答数字，不要解释。",
		"月亮是什么形状？只答一个词，不要调用工具。",
		"我刚才问的加法结果是多少？只回答数字，不要任何解释。",
	}
	turns, err := runCodexWSMultiTurnSession(baseURL, apiKey, sessionID, questions, 180*time.Second)
	if err != nil {
		if isSkippedDetail(err.Error()) {
			return []checkResult{{Name: "strict_multi_session", OK: false, Skipped: true, Detail: err.Error()}}
		}
		return []checkResult{{Name: "strict_multi_session", OK: false, Detail: err.Error()}}
	}

	fmt.Printf("\n[strict-multi] session_id=%s turns=%d\n", sessionID, len(turns))
	for i, turn := range turns {
		fmt.Printf("[strict-multi] turn%d ok=%v text=%s\n", i+1, turn.Result.OK, truncate(turn.AssistantText, 160))
	}

	results := make([]checkResult, 0, 8)
	for i, turn := range turns {
		results = append(results, checkResult{
			Name:   fmt.Sprintf("strict_multi_turn_%d", i+1),
			OK:     turn.Result.OK && strings.TrimSpace(turn.ResponseID) != "",
			Detail: truncate(turn.AssistantText, 120),
		})
	}
	if len(turns) < 4 {
		return results
	}

	t1 := strings.ToUpper(turns[0].AssistantText)
	results = append(results, checkResult{
		Name:   "strict_multi_t1_exact",
		OK:     strings.Contains(t1, "STRICT_MULTI_1"),
		Detail: truncate(turns[0].AssistantText, 80),
	})
	results = append(results, checkResult{
		Name:   "strict_multi_t2_math",
		OK:     strictNumericAnswer(turns[1].AssistantText, "42"),
		Detail: strings.TrimSpace(turns[1].AssistantText),
	})
	results = append(results, checkResult{
		Name:   "strict_multi_t3_topic",
		OK:     strictContainsAny(turns[2].AssistantText, "圆", "球", "round", "circle", "月", "D形", "d形", "半圆"),
		Detail: truncate(turns[2].AssistantText, 80),
	})
	results = append(results, checkResult{
		Name:   "strict_multi_t4_recall",
		OK:     strictNumericAnswer(turns[3].AssistantText, "42"),
		Detail: strings.TrimSpace(turns[3].AssistantText),
	})
	return results
}

func runStrictLongDialog(baseURL, apiKey string) []checkResult {
	sessionID := fmt.Sprintf("strict-long-%d", time.Now().Unix())
	questions := []string{
		"用不超过40字说明 bridge_wechat 项目里 JSON 双重转义的主要原因。不要调用工具，直接文字回答。",
		"上一问里最关键的一个词是什么？只答一个词。",
		"如果 content 字段必须是字符串，最推荐的改法是什么？不超过30字。",
		"给一行 Python 示例说明只 dumps 一次。不超过60字。",
		"彩虹七种颜色的英文单词有哪些？只列7个英文单词，用逗号分隔，不要解释。",
		"我们第一个问题讨论的是哪个项目？只答项目名。",
	}
	turns, err := runCodexWSMultiTurnSession(baseURL, apiKey, sessionID, questions, 200*time.Second)
	if err != nil {
		if isSkippedDetail(err.Error()) {
			return []checkResult{{Name: "strict_long_session", OK: false, Skipped: true, Detail: err.Error()}}
		}
		return []checkResult{{Name: "strict_long_session", OK: false, Detail: err.Error()}}
	}

	fmt.Printf("\n[strict-long] session_id=%s turns=%d\n", sessionID, len(turns))
	for i, turn := range turns {
		fmt.Printf("[strict-long] turn%d ok=%v text=%s\n", i+1, turn.Result.OK, truncate(turn.AssistantText, 200))
	}

	results := make([]checkResult, 0, 10)
	for i, turn := range turns {
		results = append(results, checkResult{
			Name:   fmt.Sprintf("strict_long_turn_%d", i+1),
			OK:     turn.Result.OK && strings.TrimSpace(turn.ResponseID) != "",
			Detail: truncate(turn.AssistantText, 120),
		})
	}
	if len(turns) < 6 {
		return results
	}

	results = append(results, checkResult{
		Name:   "strict_long_t1_bridge_topic",
		OK:     strictContainsAny(turns[0].AssistantText, "bridge_wechat", "JSON", "转义", "dumps"),
		Detail: truncate(turns[0].AssistantText, 100),
	})
	results = append(results, checkResult{
		Name:   "strict_long_t5_color_switch",
		OK:     strictLongColorSwitchOK(turns[4].AssistantText),
		Detail: truncate(turns[4].AssistantText, 120),
	})
	results = append(results, checkResult{
		Name:   "strict_long_t6_recall_project",
		OK:     strings.Contains(strings.ToLower(turns[5].AssistantText), "bridge_wechat"),
		Detail: truncate(turns[5].AssistantText, 80),
	})
	return results
}

func runStrictComplexTools(baseURL, apiKey string) []checkResult {
	sessionID := fmt.Sprintf("strict-tools-%d", time.Now().Unix())
	seedQuestions := []string{"请只回复 exactly: STRICT_TOOL_SEED，不要多说任何字。"}
	seedTurns, err := runCodexWSMultiTurnSession(baseURL, apiKey, sessionID, seedQuestions, 120*time.Second)
	if err != nil {
		if isSkippedDetail(err.Error()) {
			return []checkResult{{Name: "strict_tools_seed", OK: false, Skipped: true, Detail: err.Error()}}
		}
		return []checkResult{{Name: "strict_tools_seed", OK: false, Detail: err.Error()}}
	}
	if len(seedTurns) == 0 || strings.TrimSpace(seedTurns[0].ResponseID) == "" {
		return []checkResult{{Name: "strict_tools_seed", OK: false, Detail: "missing seed response_id"}}
	}

	captureDir := strings.TrimSpace(envOr("SUB2API_GROK_CAPTURE_DIR", filepath.Join(".local-demo", "codex-capture", "strict-tools")))
	payload := map[string]any{
		"type":                 "response.create",
		"model":                "gpt-5.4",
		"stream":               true,
		"prompt_cache_key":     sessionID,
		"previous_response_id": seedTurns[0].ResponseID,
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "请只回复 exactly: STRICT_TOOL_SEED，不要多说任何字。"}},
			},
			map[string]any{
				"type":    "message",
				"role":    "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": seedTurns[0].AssistantText}},
			},
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "我已经执行了两个命令，请把两个输出按顺序拼起来，只回复 STRICT_A_STRICT_B"}},
			},
			map[string]any{
				"type":    "local_shell_call",
				"call_id": "strict_tool_call_1",
				"name":    "shell",
				"input":   map[string]any{"command": "echo STRICT_A"},
			},
			map[string]any{
				"type":    "mcp_tool_call_output",
				"call_id": "strict_tool_call_1",
				"output":  "STRICT_A",
			},
			map[string]any{
				"type":    "local_shell_call",
				"call_id": "strict_tool_call_2",
				"name":    "shell",
				"input":   map[string]any{"command": "echo STRICT_B"},
			},
			map[string]any{
				"type":    "mcp_tool_call_output",
				"call_id": "strict_tool_call_2",
				"output":  "STRICT_B",
			},
		},
	}
	payloadBytes, _ := json.Marshal(payload)
	writeCaptureArtifact(captureDir, "tool_turn_request.json", payloadBytes)

	toolTurn, responseID := runCodexWSTurn(baseURL, apiKey, payloadBytes, 180*time.Second)
	writeCaptureArtifact(captureDir, "tool_turn_result.json", marshalCaptureResult(toolTurn, responseID))

	fmt.Printf("\n[strict-tools] session_id=%s seed_id=%s tool_text=%s\n",
		sessionID, seedTurns[0].ResponseID, truncate(toolTurn.Detail, 200))

	text := strings.ToUpper(toolTurn.Detail)
	joined := strings.Contains(text, "STRICT_A") && strings.Contains(text, "STRICT_B")
	return []checkResult{
		{Name: "strict_tools_seed", OK: strings.Contains(strings.ToUpper(seedTurns[0].AssistantText), "STRICT_TOOL_SEED"), Detail: truncate(seedTurns[0].AssistantText, 80)},
		{Name: "strict_tools_dual_shell", OK: toolTurn.OK, Detail: truncate(toolTurn.Detail, 120)},
		{Name: "strict_tools_dual_output_merge", OK: joined, Detail: truncate(toolTurn.Detail, 120)},
	}
}

func runStrictMismatchAfterLong(baseURL, apiKey string) []checkResult {
	sessionID := fmt.Sprintf("strict-mismatch-%d", time.Now().Unix())
	longQuestions := []string{
		"详细分析 bridge_wechat 项目里 JSON 响应中 \\n 被双重转义的原因，至少列出3个要点。",
		"再补充一个最常见的修复建议，不超过50字。",
	}
	longTurns, err := runCodexWSMultiTurnSession(baseURL, apiKey, sessionID, longQuestions, 200*time.Second)
	if err != nil {
		if isSkippedDetail(err.Error()) {
			return []checkResult{{Name: "strict_mismatch_long", OK: false, Skipped: true, Detail: err.Error()}}
		}
		return []checkResult{{Name: "strict_mismatch_long", OK: false, Detail: err.Error()}}
	}
	if len(longTurns) < 2 || strings.TrimSpace(longTurns[1].ResponseID) == "" {
		return []checkResult{{Name: "strict_mismatch_long", OK: false, Detail: "long preamble incomplete"}}
	}

	assistant := map[string]any{
		"type":    "message",
		"role":    "assistant",
		"content": []any{map[string]any{"type": "output_text", "text": longTurns[1].AssistantText}},
	}
	turnReq := map[string]any{
		"type":                 "response.create",
		"model":                "gpt-5.4",
		"stream":               true,
		"prompt_cache_key":     sessionID,
		"previous_response_id": longTurns[1].ResponseID,
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": longQuestions[0]}}},
			map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": longTurns[0].AssistantText}}},
			assistant,
			map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "看下桌面都有什么文件，只列文件名，不要分析 JSON。"}}},
		},
	}
	turnBytes, _ := json.Marshal(turnReq)
	switchTurn, _ := runCodexWSTurn(baseURL, apiKey, turnBytes, 180*time.Second)

	fmt.Printf("\n[strict-mismatch] session_id=%s switch_text=%s\n", sessionID, truncate(switchTurn.Detail, 240))

	lower := strings.ToLower(switchTurn.Detail)
	bleed := strings.Contains(lower, "bridge_wechat") ||
		strings.Contains(switchTurn.Detail, "双重转义") ||
		strings.Contains(switchTurn.Detail, "json.dumps") ||
		strings.Contains(lower, "zygisk")
	desktop := strings.Contains(switchTurn.Detail, "桌面") ||
		strings.Contains(lower, "desktop") ||
		strings.Contains(switchTurn.Detail, "文件") ||
		strings.Contains(lower, "tool_call") ||
		strings.Contains(lower, "powershell") ||
		strings.Contains(lower, "shell(")

	return []checkResult{
		{Name: "strict_mismatch_long_preamble", OK: longTurns[1].Result.OK, Detail: truncate(longTurns[1].AssistantText, 100)},
		{Name: "strict_mismatch_switch_ok", OK: switchTurn.OK, Detail: truncate(switchTurn.Detail, 120)},
		{Name: "strict_mismatch_no_bleed", OK: switchTurn.OK && !bleed, Detail: truncate(switchTurn.Detail, 120)},
		{Name: "strict_mismatch_desktop_aligned", OK: switchTurn.OK && desktop, Detail: truncate(switchTurn.Detail, 120)},
	}
}

func strictNumericAnswer(text, want string) bool {
	text = strings.TrimSpace(text)
	if text == want {
		return true
	}
	re := regexp.MustCompile(`(?i)^\s*` + regexp.QuoteMeta(want) + `\s*[。.!！]?\s*$`)
	return re.MatchString(text)
}

func strictContainsAny(text string, markers ...string) bool {
	lower := strings.ToLower(text)
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func strictLongColorSwitchOK(text string) bool {
	lower := strings.ToLower(text)
	colorHits := 0
	for _, color := range []string{"red", "orange", "yellow", "green", "blue", "indigo", "violet", "purple"} {
		if strings.Contains(lower, color) {
			colorHits++
		}
	}
	if colorHits >= 4 {
		return true
	}
	for _, zh := range []string{"红", "橙", "黄", "绿", "蓝", "靛", "紫"} {
		if strings.Contains(text, zh) {
			colorHits++
		}
	}
	return colorHits >= 4 && !strings.Contains(lower, "bridge_wechat")
}