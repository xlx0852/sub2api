package service

// Mirasim relay 的 thinking 形态与 effort 归一（对齐官方客户端 0.0.354 行为规格）。
//
// 关键约束（改动前请先读）：
//   - **形态只有两种**：自适应型（`thinking.type=adaptive` + `output_config.effort`）与
//     预算型（`thinking.type=enabled` + `thinking.budget_tokens`）。形态由账号 roster 的
//     `adaptive` 标记决定，**绝不按模型名猜**；roster 无该模型（含未来新模型）时保持
//     默认的自适应型——relay 上所有 Claude 模型都是这一形态；
//   - effort 阶梯按模型族校验：Claude/GPT `low/medium/high/xhigh/max`；DeepSeek 额外接受
//     `off`，实际允许 `low/high/max`；GLM/Kimi 只允许 `low/high/max`；
//   - 官方客户端的 `ultra` = `max` + 它自己在多轮里编排的工作流。本仓只发单次请求，
//     因此把 `ultra` 折成 `max` 后照发（与官方单请求字节一致），不是拒绝它；
//   - 请求体里**没有** thinking/effort 控制时原样放行（不得凭空造金额或形态）；
//   - 不支持的 effort 返回 400（MirasimThinkingConfigError），而不是转给上游再被拒。

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// mirasimThinkingShape 是模型接受的上游 thinking 形态。
type mirasimThinkingShape int

const (
	// mirasimShapeUnknown：roster 无该模型（或未拉到 roster）→ 用默认的自适应型。
	mirasimShapeUnknown mirasimThinkingShape = iota
	// mirasimShapeAdaptive：thinking.type=adaptive + output_config.effort。
	mirasimShapeAdaptive
	// mirasimShapeBudget：thinking.type=enabled + thinking.budget_tokens。
	mirasimShapeBudget
)

// mirasimShapeAdaptiveDefault 返回该形态是否走自适应型。
func (s mirasimThinkingShape) adaptive() bool { return s != mirasimShapeBudget }

// mirasimEffortLadder 是 Claude/Responses 两端口共用的 effort 阶梯说明（错误信息可操作）。
const mirasimEffortLadder = "Mirasim accepts low, medium, high, xhigh, max and ultra"

// MirasimThinkingConfigError 表示「请求里的 thinking/effort 控制无法映射到该模型的形态」。
//
// 关键约束（改动前请先读）：这类错误必须在**本地**以 400 返回，不得转给上游；
// 消息里只能出现模型名与阶梯说明，不得包含凭据或上游正文。
type MirasimThinkingConfigError struct {
	Code    string
	Message string
}

func (e *MirasimThinkingConfigError) Error() string {
	if e == nil {
		return "mirasim thinking: invalid configuration"
	}
	return e.Message
}

// StatusCode 让 handler 侧统一按 400 处理（与 BetaBlockedError 同一模式）。
func (e *MirasimThinkingConfigError) StatusCode() int { return http.StatusBadRequest }

// mirasimThinkingConfig 是请求体里已有的 thinking 控制（归一后的中间表示）。
type mirasimThinkingConfig struct {
	Mode   string // none | auto | level | budget
	Level  string
	Budget int
}

// normalizeMirasimRelayThinkingBody 把 relay 账号请求体里的 thinking 控制改写成
// 该模型接受的形态；无控制时原样返回。model 必须是**最终上游模型名**（映射之后）。
func normalizeMirasimRelayThinkingBody(body []byte, model string, shape mirasimThinkingShape) ([]byte, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, nil
	}
	// 1) ultra 只是官方客户端的编排标记：单请求形态与 max 一致。
	out := mirasimFoldWorkflowEffort(body)
	// 2) 取请求体里已有的控制；没有任何控制就不动。
	cfg, ok := mirasimBodyThinkingConfig(out)
	if !ok {
		return out, nil
	}
	// 3) 自适应型收到固定预算时，把预算折成最接近的档位而不是拒绝。
	if cfg.Mode == "budget" && shape.adaptive() {
		level := mirasimRelayEffortForBudget(cfg.Budget)
		if !mirasimIsRelayEffort(level) {
			return out, nil
		}
		cfg = mirasimThinkingConfig{Mode: "level", Level: level}
	}
	normalized, err := mirasimApplyThinkingConfig(out, model, cfg, shape)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

// mirasimFoldWorkflowEffort 把 ultra 折成 max（三个可能携带 effort 的路径）。
func mirasimFoldWorkflowEffort(body []byte) []byte {
	for _, path := range []string{"reasoning.effort", "reasoning_effort", "output_config.effort"} {
		if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(body, path).String()), "ultra") {
			body = mirasimSetString(body, path, "max")
		}
	}
	return body
}

// mirasimBodyThinkingConfig 读取请求体里的 thinking/effort 控制。
func mirasimBodyThinkingConfig(body []byte) (mirasimThinkingConfig, bool) {
	effort := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "output_config.effort").String()))
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "thinking.type").String())) {
	case "disabled":
		return mirasimThinkingConfig{Mode: "none"}, true
	case "adaptive":
		if effort != "" {
			return mirasimThinkingConfig{Mode: "level", Level: effort}, true
		}
		return mirasimThinkingConfig{Mode: "auto", Budget: -1}, true
	case "enabled":
		if budget := int(gjson.GetBytes(body, "thinking.budget_tokens").Int()); budget > 0 {
			return mirasimThinkingConfig{Mode: "budget", Budget: budget}, true
		}
		return mirasimThinkingConfig{Mode: "auto", Budget: -1}, true
	}
	if effort != "" {
		return mirasimThinkingConfig{Mode: "level", Level: effort}, true
	}
	return mirasimThinkingConfig{}, false
}

// mirasimApplyThinkingConfig 按形态落实控制。
func mirasimApplyThinkingConfig(body []byte, model string, cfg mirasimThinkingConfig, shape mirasimThinkingShape) ([]byte, error) {
	model = strings.ToLower(strings.TrimSpace(model))
	adaptive := shape.adaptive()
	switch cfg.Mode {
	case "none":
		if strings.HasPrefix(model, "deepseek-") {
			body = mirasimDeletePath(body, "thinking")
			return mirasimSetString(body, "output_config.effort", "off"), nil
		}
		if strings.HasPrefix(model, "glm-") || strings.HasPrefix(model, "kimi-") {
			return body, &MirasimThinkingConfigError{
				Code:    "mirasim_effort_invalid",
				Message: fmt.Sprintf("%s does not offer an off effort", model),
			}
		}
		body = mirasimSetString(body, "thinking.type", "disabled")
		body = mirasimDeletePath(body, "thinking.budget_tokens")
		return mirasimDeleteClaudeEffort(body), nil

	case "auto":
		if adaptive {
			body = mirasimSetString(body, "thinking.type", "adaptive")
			body = mirasimDeletePath(body, "thinking.budget_tokens")
			return mirasimDeleteClaudeEffort(body), nil
		}
		return mirasimApplyManualThinking(body, 1024)

	case "level":
		if !adaptive {
			budget, ok := mirasimLevelToBudget(cfg.Level)
			if !ok {
				return body, &MirasimThinkingConfigError{
					Code:    "mirasim_thinking_level_invalid",
					Message: fmt.Sprintf("unsupported thinking level %q; %s", cfg.Level, mirasimEffortLadder),
				}
			}
			return mirasimApplyManualThinking(body, budget)
		}
		if cfg.Level == "off" && strings.HasPrefix(model, "deepseek-") {
			body = mirasimDeletePath(body, "thinking")
			return mirasimSetString(body, "output_config.effort", "off"), nil
		}
		if !mirasimModelAcceptsEffort(model, cfg.Level) {
			return body, &MirasimThinkingConfigError{
				Code:    "mirasim_claude_effort_invalid",
				Message: fmt.Sprintf("unsupported thinking effort %q for %s; %s", cfg.Level, model, mirasimEffortLadder),
			}
		}
		body = mirasimSetString(body, "thinking.type", "adaptive")
		body = mirasimDeletePath(body, "thinking.budget_tokens")
		return mirasimSetString(body, "output_config.effort", cfg.Level), nil

	case "budget":
		if adaptive {
			return body, &MirasimThinkingConfigError{
				Code:    "mirasim_claude_budget_unsupported",
				Message: fmt.Sprintf("Claude model %s requires adaptive thinking and does not accept a fixed token budget", model),
			}
		}
		return mirasimApplyManualThinking(body, cfg.Budget)
	}
	return body, nil
}

// mirasimModelAcceptsEffort 判定该模型是否接受某档 effort（模型族阶梯）。
func mirasimModelAcceptsEffort(model, level string) bool {
	switch {
	case strings.HasPrefix(model, "deepseek-"):
		return level == "low" || level == "high" || level == "max"
	case strings.HasPrefix(model, "glm-"), strings.HasPrefix(model, "kimi-"):
		return level == "low" || level == "high" || level == "max"
	default:
		return mirasimIsRelayEffort(level)
	}
}

// mirasimIsRelayEffort 判定是否属于 relay 的 effort 阶梯（ultra 已折成 max）。
func mirasimIsRelayEffort(level string) bool {
	switch level {
	case "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

// mirasimRelayEffortForBudget 把 token 预算折成阶梯档位（最小预算落在最底档而非之下）。
func mirasimRelayEffortForBudget(budget int) string {
	level := mirasimBudgetToLevel(budget)
	if level == "minimal" {
		return "low"
	}
	return level
}

func mirasimLevelToBudget(level string) (int, bool) {
	switch level {
	case "low":
		return 1024, true
	case "medium":
		return 8192, true
	case "high":
		return 24576, true
	case "xhigh":
		return 32768, true
	case "max":
		return 128000, true
	default:
		return 0, false
	}
}

func mirasimBudgetToLevel(budget int) string {
	switch {
	case budget < 0:
		return ""
	case budget == 0:
		return "none"
	case budget <= 512:
		return "minimal"
	case budget <= 1024:
		return "low"
	case budget <= 8192:
		return "medium"
	case budget <= 24576:
		return "high"
	default:
		return "xhigh"
	}
}

// mirasimApplyManualThinking 写预算型形态：budget ≥1024，且必须小于 max_tokens。
func mirasimApplyManualThinking(body []byte, budget int) ([]byte, error) {
	if budget < 1024 {
		budget = 1024
	}
	if maxTokens := int(gjson.GetBytes(body, "max_tokens").Int()); maxTokens > 0 && budget >= maxTokens {
		budget = maxTokens - 1
		if budget < 1024 {
			return body, &MirasimThinkingConfigError{
				Code:    "mirasim_claude_budget_out_of_range",
				Message: "Claude thinking budget must be at least 1024 and lower than max_tokens",
			}
		}
	}
	body = mirasimSetString(body, "thinking.type", "enabled")
	body = mirasimSetInt(body, "thinking.budget_tokens", budget)
	return mirasimDeleteClaudeEffort(body), nil
}

// mirasimDeleteClaudeEffort 删除 output_config.effort；若 output_config 变空则一并删除。
func mirasimDeleteClaudeEffort(body []byte) []byte {
	body = mirasimDeletePath(body, "output_config.effort")
	outputConfig := gjson.GetBytes(body, "output_config")
	if outputConfig.Exists() && outputConfig.IsObject() && len(outputConfig.Map()) == 0 {
		body = mirasimDeletePath(body, "output_config")
	}
	return body
}

func mirasimSetString(body []byte, path, value string) []byte {
	updated, err := sjson.SetBytes(body, path, value)
	if err != nil {
		return body
	}
	return updated
}

func mirasimSetInt(body []byte, path string, value int) []byte {
	updated, err := sjson.SetBytes(body, path, value)
	if err != nil {
		return body
	}
	return updated
}

func mirasimDeletePath(body []byte, path string) []byte {
	updated, err := sjson.DeleteBytes(body, path)
	if err != nil {
		return body
	}
	return updated
}
