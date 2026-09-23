package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

func TestReasoningProbePromptNeverLeaksTheAnswer(t *testing.T) {
	// The whole point is that the model has to work it out; leaking either the correct answer
	// or the popular wrong distractor would invalidate the probe.
	if strings.Contains(reasoningProbePrompt, "29") {
		t.Error("prompt contains the correct answer")
	}
	if strings.Contains(reasoningProbePrompt, "21") {
		t.Error("prompt contains the common wrong answer, which would anchor the model")
	}
	if !strings.Contains(reasoningProbePrompt, "苹果味") || !strings.Contains(reasoningProbePrompt, "五角星") {
		t.Error("prompt lost the problem statement")
	}
}

func TestReasoningProbeExpectedAnswerIsTheVerifiedValue(t *testing.T) {
	// Verified by exhaustive case analysis: 7 round apple + 9 round peach + 8 round watermelon
	// + 4 star watermelon = 28 sweets without any valid pairing, and 29 always forces one.
	// 21 is the distractor produced by following only the "round peach" branch.
	if reasoningProbeExpectedAnswer != 29 {
		t.Fatalf("expected answer changed to %d; redo the case analysis before trusting the probe", reasoningProbeExpectedAnswer)
	}
}

func TestJudgeReasoningProbeParsesRealAnswerShapes(t *testing.T) {
	cases := []struct {
		name      string
		answer    string
		wantFound bool
		wantValue int
		wantOK    bool
	}{
		{"plain integer", "29", true, 29, true},
		{"chinese sentence", "最少需要取出 29 个糖果。", true, 29, true},
		{"marker with colon", "答案：29", true, 29, true},
		{"english", "The minimum is 29 candies.", true, 29, true},
		{"fenced block", "```\n29\n```", true, 29, true},
		{"bold", "**29** 个", true, 29, true},
		{"wrong answer 21", "最少需要 21 个糖果。", true, 21, false},
		{"wrong answer 28", "需要 28 个", true, 28, false},
		{"reasoning then answer", "星形必须取到，所以答案是 29 个。", true, 29, true},
		{"restates the table then answers", "圆形 7 9 8，星形 7 6 4，因此最少 29 个。", true, 29, true},
		{"refusal", "抱歉，我无法回答这个问题。", false, 0, false},
		{"empty", "   ", false, 0, false},
	}
	for _, testCase := range cases {
		got := judgeReasoningProbe(testCase.answer)
		if got.Found != testCase.wantFound {
			t.Errorf("%s: found=%v want %v (%q)", testCase.name, got.Found, testCase.wantFound, testCase.answer)
			continue
		}
		if testCase.wantFound {
			if got.Parsed != testCase.wantValue {
				t.Errorf("%s: parsed %d want %d (%q)", testCase.name, got.Parsed, testCase.wantValue, testCase.answer)
			}
			if got.Correct != testCase.wantOK {
				t.Errorf("%s: correct=%v want %v (%q)", testCase.name, got.Correct, testCase.wantOK, testCase.answer)
			}
		}
	}
}

func TestDiagnosticPayloadsUseTheProbe(t *testing.T) {
	chat := createOpenAIChatCompletionsTestPayload("gpt-5.5", "")
	messages, _ := chat["messages"].([]map[string]any)
	if len(messages) != 1 || messages[0]["content"] != reasoningProbePrompt {
		t.Errorf("chat completions default prompt is not the probe: %+v", chat)
	}

	responses := createOpenAIDiagnosticPayload("gpt-5.5", true)
	if !strings.Contains(strings.Join([]string{stringifyPayload(t, responses)}, ""), "苹果味") {
		t.Errorf("responses diagnostic payload is not the probe: %+v", responses)
	}

	// The usage/quota probe must stay cheap.
	usage := createOpenAITestPayload("gpt-5.5", true)
	if strings.Contains(stringifyPayload(t, usage), "苹果味") {
		t.Error("usage probe must keep the trivial prompt")
	}
}

func stringifyPayload(t *testing.T, payload map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(encoded)
}

func TestEmitReasoningProbeVerdictAppendsJudgement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &AccountTestService{}

	render := func(body string) string {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
		service.emitReasoningProbeVerdict(c, body)
		return recorder.Body.String()
	}

	out := render("data: {\"type\":\"content\",\"text\":\"最少需要 21 个糖果\"}\n")
	if !strings.Contains(out, "降智检测") || !strings.Contains(out, "不正确") {
		t.Fatalf("verdict not appended for a wrong answer: %q", out)
	}

	if got := render("data: {\"type\":\"content\",\"text\":\"data:image/png;base64,AAA\"}\n"); strings.Contains(got, "降智检测") {
		t.Fatalf("image answers must not be judged: %q", got)
	}

	if got := render("data: {\"type\":\"error\",\"error\":\"boom\"}\n"); strings.Contains(got, "降智检测") {
		t.Fatalf("failed tests must not be judged: %q", got)
	}
}

func TestDiagnosticVerdictIsRecordedExactlyOnce(t *testing.T) {
	// The SSE wrapper appends the verdict; the background runner must not annotate again, or the
	// diagnostics history shows the judgement twice (this happened in production).
	body := "data: {\"type\":\"content\",\"text\":\"29\"}\n" +
		"data: {\"type\":\"content\",\"text\":\"\\n" + formatProbeVerdict(judgeReasoningProbe("29")) + "\"}\n"
	responseText, errMsg := parseTestSSEOutput(body)
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if got := strings.Count(responseText, "[降智检测]"); got != 1 {
		t.Fatalf("verdict recorded %d times, want 1: %q", got, responseText)
	}
	if _, errMsg := parseTestSSEOutput(body); errMsg != "" {
		t.Fatal(errMsg)
	}
	// The source of the runner must not re-annotate the parsed text.
	source, err := os.ReadFile("account_test_service.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if strings.Contains(string(source), "annotateDiagnosticAnswer") {
		t.Error("background runner still annotates; the verdict would be duplicated")
	}
}

func TestProbeAnswerKeyIsConfigurable(t *testing.T) {
	// The answer key can be overridden by configuration (env DIAGNOSTICS_REASONING_PROBE_EXPECTED_ANSWER)
	// so an operator is not blocked on a rebuild. The default stays the verified value, and the
	// report always states which key was used, so a wrong key is visible rather than silent.
	if got := (&AccountTestService{}).expectedProbeAnswer(); got != 29 {
		t.Fatalf("default key = %d, want the verified 29", got)
	}
	configured := &AccountTestService{cfg: &config.Config{Diagnostics: config.DiagnosticsConfig{ReasoningProbeExpectedAnswer: 21}}}
	if got := configured.expectedProbeAnswer(); got != 21 {
		t.Fatalf("configured key = %d, want 21", got)
	}

	verdict := judgeReasoningProbeWith("21", 21)
	if !verdict.Correct {
		t.Error("with a 21 key, a 21 answer must count as correct")
	}
	line := formatProbeVerdictWith(verdict, 21)
	if !strings.Contains(line, "期望 21") || !strings.Contains(line, "正确") {
		t.Errorf("report must name the key actually used: %q", line)
	}
	// Sanity: the same answer against the verified key is wrong, which is exactly why the key matters.
	wrong := judgeReasoningProbeWith("21", 29)
	if wrong.Correct {
		t.Error("21 must not count as correct against the verified key")
	}
}

func TestProbeTimeoutWithPartialContentIsNotAFailure(t *testing.T) {
	// Regression: the reasoning answer is slow, and a slow upstream used to turn the whole
	// diagnostics run into "failed" (feeding quota-wall/demote logic and dropping the
	// availability rate). Partial content proves the account answers.
	cases := []struct {
		err  string
		want bool
	}{
		{"Chat Completions stream read error from /v1/chat/completions: context deadline exceeded", true},
		{"context canceled", true},
		{"API returned 401: invalid_api_key", false},
		{"上游返回 403: INSUFFICIENT_BALANCE", false},
		{"", false},
	}
	for _, testCase := range cases {
		if got := isProbeStreamTimeout(testCase.err); got != testCase.want {
			t.Errorf("isProbeStreamTimeout(%q) = %v, want %v", testCase.err, got, testCase.want)
		}
	}
	if !strings.Contains(probeTruncatedVerdict(), "未判定") {
		t.Error("truncated answers must be reported as unjudged, not wrong")
	}
}

func TestProbePromptAsksForABriefAnswer(t *testing.T) {
	// Latency is the probe's main cost; the prompt must ask for a short answer.
	if !strings.Contains(reasoningProbePrompt, "不要长篇推理") {
		t.Error("prompt no longer asks for a brief answer, which raises diagnostics latency")
	}
}

func TestConfiguredKeyDoesNotHideTheStrictAnswer(t *testing.T) {
	// The operator may set their own answer key. When it differs from the strictly derived value
	// and the model produced that value, the report must say so rather than silently calling a
	// correct derivation wrong.
	line := formatProbeVerdictWith(judgeReasoningProbeWith("29", 21), 21)
	if !strings.Contains(line, "期望 21") || !strings.Contains(line, "模型给出 29") {
		t.Errorf("report must state the configured key and the observed answer: %q", line)
	}
	if !strings.Contains(line, "严格推导应为 29") {
		t.Errorf("report must disclose the strict derivation: %q", line)
	}
	// With the default key the note is unnecessary noise and must not appear.
	if got := formatProbeVerdictWith(judgeReasoningProbeWith("29", 29), 29); strings.Contains(got, "严格推导") {
		t.Errorf("unexpected note with the default key: %q", got)
	}
}
