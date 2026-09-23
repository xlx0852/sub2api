package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Reasoning degradation probe.
//
// Diagnostics used to send "hi", which proves transport but says nothing about whether the
// upstream still reasons like the model it claims to be. This probe sends one pigeonhole
// problem with a single integer answer instead, and the runner records whether the answer
// matches. The expected value is kept here, server side only — it is never part of the prompt.
//
// The problem (bag of sweets, three flavours x two shapes, draw the minimum that guarantees a
// cross-shape apple+peach pair) has answer 29:
//
//	to avoid the property you must have no star-apple or no star-peach etc.; the largest
//	avoiding selection is 7 round apple + 9 round peach + 8 round watermelon + 4 star
//	watermelon = 28, and 29 always forces one of the two pairings.
//
// 21 is a known wrong distractor produced by only following one branch (12 watermelons +
// 9 round peach, then "one more star apple suffices"), so a model answering 21 is exactly the
// degradation signal this probe is meant to catch. Do not "fix" the expected value to 21.
const reasoningProbeExpectedAnswer = 29

// reasoningProbePrompt is sent verbatim as the test request body.
const reasoningProbePrompt = `袋子里有糖果，口味与形状数量如下：
苹果味：圆形 7 个，五角星形 7 个
桃子味：圆形 9 个，五角星形 6 个
西瓜味：圆形 8 个，五角星形 4 个
所有糖果都在一个黑色袋子中，只能靠手感区分形状（能区分圆和五角星），无法凭手感区分口味。
现在要事先决定一次摸出多少个糖果，才能保证摸完之后手中一定同时拥有「不同形状的苹果味和桃子味」——
也就是（圆形苹果味 且 五角星形桃子味）或者（圆形桃子味 且 五角星形苹果味）至少满足一组。
请问最少需要摸出多少个糖果才能保证满足要求？
直接给出最终结论：一个整数，最多再用一句话说明理由，不要复述题目，不要长篇推理。`

var (
	// Answers like "答案：29", "最少 29 个", "answer is 29", "= 29".
	probeAnswerWithMarker = regexp.MustCompile(`(?i)(?:答案|最少|至少|需要|是|answer|total|minimum|min)\s*(?:为|是|:|：|=|is)?\s*\**\s*(\d{1,4})`)
	// Any standalone integer in the answer text.
	probeAnyInteger = regexp.MustCompile(`\d{1,4}`)
	probeCodeFence  = regexp.MustCompile("(?s)```[a-zA-Z]*\\n(.*?)```")
)

// probeVerdict is the outcome of judging one diagnostic answer.
type probeVerdict struct {
	// Parsed is the integer taken from the answer, when one could be read.
	Parsed int
	// Found reports whether any integer was parsed at all (false = refusal/empty/truncated).
	Found bool
	// Correct reports whether the parsed value matches the expected answer.
	Correct bool
}

// expectedProbeAnswer returns the configured answer key, falling back to the verified default.
func (s *AccountTestService) expectedProbeAnswer() int {
	if s != nil && s.cfg != nil && s.cfg.Diagnostics.ReasoningProbeExpectedAnswer > 0 {
		return s.cfg.Diagnostics.ReasoningProbeExpectedAnswer
	}
	return reasoningProbeExpectedAnswer
}

// judgeReasoningProbe extracts the model's final number and compares it with the expectation.
func judgeReasoningProbe(answer string) probeVerdict {
	return judgeReasoningProbeWith(answer, reasoningProbeExpectedAnswer)
}

// judgeReasoningProbeWith judges against an explicit answer key.
func judgeReasoningProbeWith(answer string, expected int) probeVerdict {
	text := strings.TrimSpace(answer)
	if text == "" {
		return probeVerdict{}
	}
	// Prefer the content of a fenced block (models often wrap the final number), then any
	// "answer:" style marker, then the last standalone integer in the whole reply, which is
	// where a normal answer sentence puts it.
	candidates := make([]string, 0, 4)
	if match := probeCodeFence.FindStringSubmatch(text); len(match) == 2 {
		candidates = append(candidates, match[1])
	}
	if match := probeAnswerWithMarker.FindStringSubmatch(text); len(match) == 2 {
		candidates = append(candidates, match[1])
	}
	all := probeAnyInteger.FindAllString(text, -1)
	if len(all) > 0 {
		candidates = append(candidates, all[len(all)-1])
	}
	for _, candidate := range candidates {
		value, err := strconv.Atoi(strings.TrimSpace(candidate))
		if err != nil {
			continue
		}
		return probeVerdict{Parsed: value, Found: true, Correct: value == expected}
	}
	return probeVerdict{}
}

// isProbeStreamTimeout reports whether an error is only the probe running out of time rather
// than an upstream failure. The reasoning answer is far longer than a greeting, so a slow
// upstream can hit the test budget while still being perfectly healthy.
func isProbeStreamTimeout(errText string) bool {
	lowered := strings.ToLower(strings.TrimSpace(errText))
	if lowered == "" {
		return false
	}
	for _, marker := range []string{"context deadline exceeded", "context canceled", "timeout", "timed out", "stream read error"} {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

// probeTruncatedVerdict marks an answer that was cut short, so the reader knows the probe could
// not judge this sample instead of reading it as a wrong answer.
func probeTruncatedVerdict() string {
	return "[降智检测] 上游在限定时间内未给完答案（已收到部分内容）→ 本样本未判定"
}

// probeNoAnswerVerdict marks a sample that produced no answer text at all. Such a sample must be
// labelled explicitly: an empty response otherwise reads exactly like a passing test.
// The most common cause is a reasoning model burning the whole max_tokens budget on thinking.
func probeNoAnswerVerdict() string {
	return "[降智检测] 未收到答案正文 → 本样本未判定（推理模型常见原因：max_tokens 预算被 thinking 耗尽）"
}

// formatProbeVerdict renders the one-line report appended to a diagnostic answer, so both the
// diagnostics panel and the account usage drawer show the judgement without a schema change.
func formatProbeVerdict(verdict probeVerdict) string {
	return formatProbeVerdictWith(verdict, reasoningProbeExpectedAnswer)
}

func formatProbeVerdictWith(verdict probeVerdict, expected int) string {
	switch {
	case !verdict.Found:
		return "[降智检测] 未从回答中解析出整数（疑似拒答、截断或答非所问）"
	case verdict.Correct:
		return fmt.Sprintf("[降智检测] 期望 %d，模型给出 %d → 正确", expected, verdict.Parsed)
	default:
		// One wrong answer is a lead, not a verdict: the same upstream was observed answering 21
		// and 29 on consecutive runs, so the report asks for repeat samples instead of accusing.
		line := fmt.Sprintf("[降智检测] 期望 %d，模型给出 %d → 不正确（疑似降智，建议多次复测确认）",
			expected, verdict.Parsed)
		// If the configured key differs from the strictly derived answer and the model produced
		// that derived answer, say so instead of hiding the discrepancy behind the key.
		if expected != reasoningProbeExpectedAnswer && verdict.Parsed == reasoningProbeExpectedAnswer {
			line += fmt.Sprintf("（注：按题面严格推导应为 %d，当前判定键为 %d）",
				reasoningProbeExpectedAnswer, expected)
		}
		return line
	}
}
