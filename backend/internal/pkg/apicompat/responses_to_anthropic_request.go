package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ResponsesToAnthropicRequest converts a Responses API request into an
// Anthropic Messages request. This is the reverse of AnthropicToResponses and
// enables Anthropic platform groups to accept OpenAI Responses API requests
// by converting them to the native /v1/messages format before forwarding upstream.
func ResponsesToAnthropicRequest(req *ResponsesRequest) (*AnthropicRequest, error) {
	// 先把 input 里的 additional_tools 输入项取出来并从消息流中剔除：
	// 它是工具载体，不是对话消息。**必须在** convertResponsesInputToAnthropic 之前完成，
	// 否则该输入项会被当成未知类型跳过，连同里面的工具一起丢掉——模型于是拿不到
	// 工具定义，只能按系统提示编造调用文本，客户端报 “incompatible payload”。
	ingestedTools, filteredInput := extractAdditionalTools(req.Input)

	system, messages, err := convertResponsesInputToAnthropic(req.Instructions, filteredInput)
	if err != nil {
		return nil, err
	}

	out := &AnthropicRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}

	if len(system) > 0 {
		out.System = system
	}

	// max_output_tokens → max_tokens
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		out.MaxTokens = *req.MaxOutputTokens
	}
	if out.MaxTokens == 0 {
		// Anthropic requires max_tokens; default to a sensible value.
		out.MaxTokens = 8192
	}

	// Convert tools。顺序：顶层 tools 在前、additional_tools 摄取项在后，
	// 与 responses-lite 降级（demoteOpenAIResponsesLiteTools）保持一致，
	// 避免同一条请求在 OpenAI 上游与 Anthropic 上游上得到不同的工具顺序。
	allTools := make([]ResponsesTool, 0, len(req.Tools)+len(ingestedTools))
	allTools = append(allTools, req.Tools...)
	allTools = append(allTools, ingestedTools...)
	if len(allTools) > 0 {
		flatTools, flatErr := prepareResponsesTools(allTools)
		if flatErr != nil {
			return nil, flatErr
		}
		// custom/freeform 工具（Codex 的 exec 即此类）必须降级为 Anthropic 能表达的
		// function + 单一 input 字符串 schema，否则 exec 会以「零参数工具」下发，
		// 模型无法表达「执行这条命令」。回程由响应桥按 custom 名单还原成
		// custom_tool_call（codex 只按该类型路由）。
		out.Tools = convertResponsesToAnthropicTools(downgradeResponsesCustomTools(flatTools))
	}

	// Convert tool_choice (reverse of convertAnthropicToolChoiceToResponses)
	if len(req.ToolChoice) > 0 {
		tc, err := convertResponsesToAnthropicToolChoice(req.ToolChoice)
		if err != nil {
			return nil, fmt.Errorf("convert tool_choice: %w", err)
		}
		out.ToolChoice = tc
	}

	// reasoning.effort → output_config.effort + thinking
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		effort := mapResponsesEffortToAnthropic(req.Reasoning.Effort)
		out.OutputConfig = &AnthropicOutputConfig{Effort: effort}
		// Enable thinking for non-low efforts
		if effort != "low" {
			out.Thinking = &AnthropicThinking{
				Type:         "enabled",
				BudgetTokens: defaultThinkingBudget(effort),
			}
		}
	}

	return out, nil
}

// defaultThinkingBudget returns a sensible thinking budget based on effort level.
func defaultThinkingBudget(effort string) int {
	switch effort {
	case "low":
		return 1024
	case "medium":
		return 4096
	case "high":
		return 10240
	case "max":
		return 32768
	default:
		return 10240
	}
}

// mapResponsesEffortToAnthropic converts OpenAI Responses reasoning effort to
// Anthropic effort levels. Reverse of mapAnthropicEffortToResponses.
//
//	low    → low
//	medium → medium
//	high   → high
//	xhigh  → max
func mapResponsesEffortToAnthropic(effort string) string {
	if effort == "xhigh" {
		return "max"
	}
	return effort // low→low, medium→medium, high→high, unknown→passthrough
}

// convertResponsesInputToAnthropic extracts system prompt and messages from
// a Responses API instructions + input array. Returns the system as raw JSON
// (for Anthropic's polymorphic system field) and a list of Anthropic messages.
func convertResponsesInputToAnthropic(instructions string, inputRaw json.RawMessage) (json.RawMessage, []AnthropicMessage, error) {
	var systemParts []string
	if strings.TrimSpace(instructions) != "" {
		systemParts = append(systemParts, strings.TrimSpace(instructions))
	}

	// Try as plain string input.
	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		content, _ := json.Marshal(inputStr)
		var system json.RawMessage
		if len(systemParts) > 0 {
			system, _ = json.Marshal(strings.Join(systemParts, "\n\n"))
		}
		return system, []AnthropicMessage{{Role: "user", Content: content}}, nil
	}

	var items []ResponsesInputItem
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return nil, nil, fmt.Errorf("parse responses input: %w", err)
	}

	var messages []AnthropicMessage

	for _, item := range items {
		switch {
		case item.Role == "system" || item.Role == "developer":
			text := extractTextFromContent(item.Content)
			if text != "" {
				systemParts = append(systemParts, text)
			}

		case item.Type == "function_call":
			// function_call → assistant message with tool_use block
			input := json.RawMessage("{}")
			if item.Arguments != "" {
				input = json.RawMessage(item.Arguments)
			}
			block := AnthropicContentBlock{
				Type:  "tool_use",
				ID:    fromResponsesCallIDToAnthropic(item.CallID),
				Name:  item.Name,
				Input: input,
			}
			blockJSON, _ := json.Marshal([]AnthropicContentBlock{block})
			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: blockJSON,
			})

		case item.Type == "function_call_output" || item.Type == "custom_tool_call_output":
			// function_call_output / custom_tool_call_output → user message with tool_result block
			//
			// custom_tool_call_output 必须一并处理：我们把 custom/freeform 工具（exec 即此类）
			// 在响应方向还原成了 custom_tool_call，客户端就会回 custom_tool_call_output。
			// 只处理 function_call_output 会让工具结果被静默丢弃——模型看不到结果，
			// 于是无限重调同一个命令（实测同一命令被连续执行 74 次）。
			contentJSON := responsesFunctionOutputToAnthropicContent(item)
			block := AnthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: fromResponsesCallIDToAnthropic(item.CallID),
				Content:   contentJSON,
			}
			blockJSON, _ := json.Marshal([]AnthropicContentBlock{block})
			messages = append(messages, AnthropicMessage{
				Role:    "user",
				Content: blockJSON,
			})

		case item.Role == "user":
			content, err := convertResponsesUserToAnthropicContent(item.Content)
			if err != nil {
				return nil, nil, err
			}
			messages = append(messages, AnthropicMessage{
				Role:    "user",
				Content: content,
			})

		case item.Role == "assistant":
			content, err := convertResponsesAssistantToAnthropicContent(item.Content)
			if err != nil {
				return nil, nil, err
			}
			messages = append(messages, AnthropicMessage{
				Role:    "assistant",
				Content: content,
			})

		default:
			// Unknown role/type — attempt as user message
			if item.Content != nil {
				messages = append(messages, AnthropicMessage{
					Role:    "user",
					Content: item.Content,
				})
			}
		}
	}

	// Repair tool_use/tool_result pairing, then merge consecutive same-role
	// messages (Anthropic requires alternating roles). The first merge groups
	// parallel calls (and their results) so the pairing pass sees them together;
	// the pairing pass may re-split a user turn (e.g. when an injected message
	// sat between a call and its output), so a second merge restores alternation.
	messages = mergeConsecutiveMessages(messages)
	messages = normalizeAnthropicToolPairing(messages)
	messages = mergeConsecutiveMessages(messages)

	var system json.RawMessage
	if len(systemParts) > 0 {
		system, _ = json.Marshal(strings.Join(systemParts, "\n\n"))
	}

	return system, messages, nil
}

// responsesFunctionOutputToAnthropicContent 把 function_call_output 的 output
// 转成 Anthropic tool_result 的 content。兼容三种线上形态：
//   - 字符串（旧版 codex）→ 原样字符串
//   - 内容块数组（新版 codex 会发 []{type:input_text/output_text/input_image}）
//     → 转成 text/image 块列表
//   - 其它对象/自由 JSON（outputRaw 有值但非块数组）→ 整体序列化回字符串
//
// 空输出记 "(empty)"，避免 tool_result content 缺失触发 Anthropic 校验错误。
func responsesFunctionOutputToAnthropicContent(item ResponsesInputItem) json.RawMessage {
	if len(item.outputRaw) == 0 {
		output := item.Output
		if output == "" {
			output = "(empty)"
		}
		content, _ := json.Marshal(output)
		return content
	}

	var parts []ResponsesContentPart
	if err := json.Unmarshal(item.outputRaw, &parts); err == nil {
		blocks := make([]AnthropicContentBlock, 0, len(parts))
		for _, part := range parts {
			switch part.Type {
			case "input_text", "output_text", "text":
				if part.Text != "" {
					blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: part.Text})
				}
			case "input_image":
				if source := dataURIToAnthropicImageSource(part.ImageURL); source != nil {
					blocks = append(blocks, AnthropicContentBlock{Type: "image", Source: source})
				}
			}
		}
		if len(blocks) > 0 {
			content, _ := json.Marshal(blocks)
			return content
		}
		if len(parts) == 0 {
			content, _ := json.Marshal("(empty)")
			return content
		}
	}

	content, _ := json.Marshal(item.Output)
	return content
}

// normalizeAnthropicToolPairing rebuilds the message sequence so it satisfies
// Anthropic's tool_use/tool_result invariants, which the naive item-by-item
// conversion violates whenever the Responses history interleaves anything
// between a function_call and its function_call_output:
//
//   - every tool_result block must have a matching tool_use in the immediately
//     preceding assistant message ("tool_result ... must have a corresponding
//     tool_use block in the previous message");
//   - every tool_use block must be answered by a tool_result in the immediately
//     following user message (Anthropic rejects unanswered tool_use ids);
//   - user/assistant turns must alternate.
//
// codex (Responses, store:false) re-sends the whole history each turn and
// frequently injects items between a call and its output — a developer/approval
// notice, or a sibling parallel call whose output never arrived. The unrepaired
// converter emits each function_call as its own assistant message and each
// output as its own user message, so any such interleaving breaks
// tool_use↔tool_result adjacency and yields an upstream 400.
//
// The repair indexes every tool_result by its tool_use id, then for each
// assistant message carrying tool_use blocks keeps only the answered ones
// (dropping unanswered/dangling calls — and the assistant message entirely if it
// has no other content) and emits the matching tool_result blocks, in call
// order, as the very next user message. Standalone tool_result blocks are
// dropped from their original position (re-emitted adjacent to their call);
// orphan tool_results with no announcing tool_use are dropped. Non-tool content
// passes through in place. This mirrors normalizeChatMessages on the
// Responses→Chat path.
func normalizeAnthropicToolPairing(messages []AnthropicMessage) []AnthropicMessage {
	// Index every tool_result block by its tool_use id (last wins on dup).
	results := make(map[string]AnthropicContentBlock)
	for _, m := range messages {
		if m.Role != "user" {
			continue
		}
		for _, b := range parseContentBlocks(m.Content) {
			if b.Type == "tool_result" && b.ToolUseID != "" {
				results[b.ToolUseID] = b
			}
		}
	}

	out := make([]AnthropicMessage, 0, len(messages))
	for _, m := range messages {
		blocks := parseContentBlocks(m.Content)
		switch m.Role {
		case "assistant":
			var toolUses, others []AnthropicContentBlock
			for _, b := range blocks {
				if b.Type == "tool_use" {
					toolUses = append(toolUses, b)
				} else {
					others = append(others, b)
				}
			}
			if len(toolUses) == 0 {
				out = append(out, m)
				continue
			}
			kept := make([]AnthropicContentBlock, 0, len(toolUses))
			for _, tu := range toolUses {
				if _, ok := results[tu.ID]; ok {
					kept = append(kept, tu)
				}
			}
			if len(kept) == 0 {
				// No answered calls: keep any non-tool content, else drop.
				if len(others) > 0 {
					out = append(out, anthropicMessageFromBlocks("assistant", others))
				}
				continue
			}
			asstBlocks := make([]AnthropicContentBlock, 0, len(others)+len(kept))
			asstBlocks = append(asstBlocks, others...)
			asstBlocks = append(asstBlocks, kept...)
			out = append(out, anthropicMessageFromBlocks("assistant", asstBlocks))

			resBlocks := make([]AnthropicContentBlock, 0, len(kept))
			for _, tu := range kept {
				resBlocks = append(resBlocks, results[tu.ID])
			}
			out = append(out, anthropicMessageFromBlocks("user", resBlocks))

		case "user":
			var nonResult []AnthropicContentBlock
			hasResult := false
			for _, b := range blocks {
				if b.Type == "tool_result" {
					hasResult = true
					continue
				}
				nonResult = append(nonResult, b)
			}
			if !hasResult {
				out = append(out, m)
				continue
			}
			// The tool_result blocks are re-emitted next to their call; keep any
			// other content of this user turn in place, drop it if there is none.
			if len(nonResult) > 0 {
				out = append(out, anthropicMessageFromBlocks("user", nonResult))
			}

		default:
			out = append(out, m)
		}
	}
	return out
}

// anthropicMessageFromBlocks builds an AnthropicMessage whose content is the
// marshaled block array.
func anthropicMessageFromBlocks(role string, blocks []AnthropicContentBlock) AnthropicMessage {
	content, _ := json.Marshal(blocks)
	return AnthropicMessage{Role: role, Content: content}
}

// extractTextFromContent extracts text from a content field that may be a
// plain string or an array of content parts.
func extractTextFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var texts []string
		for _, p := range parts {
			if (p.Type == "input_text" || p.Type == "output_text" || p.Type == "text") && p.Text != "" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n\n")
	}
	return ""
}

// convertResponsesUserToAnthropicContent converts a Responses user message
// content field into Anthropic content blocks JSON.
func convertResponsesUserToAnthropicContent(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal("") // empty string content
	}

	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal(s)
	}

	// Array of content parts → Anthropic content blocks.
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		// Pass through as-is if we can't parse
		return raw, nil
	}

	var blocks []AnthropicContentBlock
	for _, p := range parts {
		switch p.Type {
		case "input_text", "text":
			if p.Text != "" {
				blocks = append(blocks, AnthropicContentBlock{
					Type: "text",
					Text: p.Text,
				})
			}
		case "input_image":
			src := dataURIToAnthropicImageSource(p.ImageURL)
			if src != nil {
				blocks = append(blocks, AnthropicContentBlock{
					Type:   "image",
					Source: src,
				})
			}
		}
	}

	if len(blocks) == 0 {
		return json.Marshal("")
	}
	return json.Marshal(blocks)
}

// convertResponsesAssistantToAnthropicContent converts a Responses assistant
// message content field into Anthropic content blocks JSON.
func convertResponsesAssistantToAnthropicContent(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal([]AnthropicContentBlock{{Type: "text", Text: ""}})
	}

	// Try plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal([]AnthropicContentBlock{{Type: "text", Text: s}})
	}

	// Array of content parts → Anthropic content blocks.
	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return raw, nil
	}

	var blocks []AnthropicContentBlock
	for _, p := range parts {
		switch p.Type {
		case "output_text", "text":
			if p.Text != "" {
				blocks = append(blocks, AnthropicContentBlock{
					Type: "text",
					Text: p.Text,
				})
			}
		}
	}

	if len(blocks) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: ""})
	}
	return json.Marshal(blocks)
}

// fromResponsesCallIDToAnthropic converts an OpenAI function call ID back to
// Anthropic format. Reverses toResponsesCallID.
func fromResponsesCallIDToAnthropic(id string) string {
	// If it has our "fc_" prefix wrapping a known Anthropic prefix, strip it
	if after, ok := strings.CutPrefix(id, "fc_"); ok {
		if strings.HasPrefix(after, "toolu_") || strings.HasPrefix(after, "call_") {
			return after
		}
	}
	// Generate a synthetic Anthropic tool ID
	if !strings.HasPrefix(id, "toolu_") && !strings.HasPrefix(id, "call_") {
		return "toolu_" + id
	}
	return id
}

// dataURIToAnthropicImageSource parses a data URI into an AnthropicImageSource.
func dataURIToAnthropicImageSource(dataURI string) *AnthropicImageSource {
	if !strings.HasPrefix(dataURI, "data:") {
		return nil
	}
	// Format: data:<media_type>;base64,<data>
	rest := strings.TrimPrefix(dataURI, "data:")
	semicolonIdx := strings.Index(rest, ";")
	if semicolonIdx < 0 {
		return nil
	}
	mediaType := rest[:semicolonIdx]
	rest = rest[semicolonIdx+1:]
	if !strings.HasPrefix(rest, "base64,") {
		return nil
	}
	data := strings.TrimPrefix(rest, "base64,")
	return &AnthropicImageSource{
		Type:      "base64",
		MediaType: mediaType,
		Data:      data,
	}
}

// mergeConsecutiveMessages merges consecutive messages with the same role
// because Anthropic requires alternating user/assistant turns.
func mergeConsecutiveMessages(messages []AnthropicMessage) []AnthropicMessage {
	if len(messages) <= 1 {
		return messages
	}

	var merged []AnthropicMessage
	for _, msg := range messages {
		if len(merged) == 0 || merged[len(merged)-1].Role != msg.Role {
			merged = append(merged, msg)
			continue
		}

		// Same role — merge content arrays
		last := &merged[len(merged)-1]
		lastBlocks := parseContentBlocks(last.Content)
		newBlocks := parseContentBlocks(msg.Content)
		combined := append(lastBlocks, newBlocks...)
		last.Content, _ = json.Marshal(combined)
	}
	return merged
}

// parseContentBlocks attempts to parse content as []AnthropicContentBlock.
// If it's a string, wraps it in a text block.
func parseContentBlocks(raw json.RawMessage) []AnthropicContentBlock {
	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []AnthropicContentBlock{{Type: "text", Text: s}}
	}
	return nil
}

// extractAdditionalTools 从 Responses 的 input 原始 JSON 数组里取出 additional_tools 载体
// 携带的工具，并把该载体剔除；其余输入项**原始字节保持不变**（不重建结构）。
//
// 关键约束（改动前请先读）：
//   - 载体是**工具声明**，不是对话消息，必须剔除；否则上游会看到一条莫名其妙的
//     developer 消息，而且里面的工具也到不了 tools 字段；
//   - tools 为空数组时仍要剔除载体，但绝不能因此清空调用方的顶层 tools；
//   - 没有任何载体时**原样返回入参**，保证无此形状的请求逐字节不变；
//   - 在 RawMessage 层做而不是先反序列化成 []ResponsesInputItem，是为了不重建
//     其它输入项的字节：content/arguments/output 的原形状必须原样传给下游。
func extractAdditionalTools(inputRaw json.RawMessage) ([]ResponsesTool, json.RawMessage) {
	trimmed := strings.TrimSpace(string(inputRaw))
	if trimmed == "" || trimmed == "null" {
		return nil, inputRaw
	}

	var items []json.RawMessage
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		// 非数组：交给下游的原有校验去报错，这里不改变行为。
		return nil, inputRaw
	}

	modified := false
	for _, raw := range items {
		if rawResponsesInputItemType(raw) == "additional_tools" {
			modified = true
			break
		}
	}
	if !modified {
		return nil, inputRaw
	}

	var tools []ResponsesTool
	kept := make([]json.RawMessage, 0, len(items))
	for _, raw := range items {
		if rawResponsesInputItemType(raw) != "additional_tools" {
			kept = append(kept, raw)
			continue
		}
		var carrier struct {
			Tools []ResponsesTool `json:"tools"`
		}
		if err := json.Unmarshal(raw, &carrier); err == nil {
			tools = append(tools, carrier.Tools...)
		}
	}

	filtered, err := json.Marshal(kept)
	if err != nil {
		return tools, inputRaw
	}
	return tools, filtered
}

// rawResponsesInputItemType 读出单个输入项的 type（只解一个字段，不重建整项）。
func rawResponsesInputItemType(raw json.RawMessage) string {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}
	return strings.TrimSpace(probe.Type)
}

// prepareResponsesTools 把工具集整理成可交给 convertResponsesToAnthropicTools 的平铺形态：
// 递归展开 namespace 包装，并检测展平后的同名冲突。
//
// 为什么要展平：Anthropic Messages 没有命名空间概念；客户端（Codex）请求里的
// namespace 语义是**分组**，不是名字的一部分——子工具名必须**原样保留**
// （如 exec_command 就叫 exec_command），因为客户端按 name 匹配工具回执，
// 改名会让 function_call 对不上任何工具（“incompatible payload”）。
//
// 为什么要检测冲突：两个命名空间下可能有同名工具，静默覆盖会让其中一个永远call不到。
func prepareResponsesTools(tools []ResponsesTool) ([]ResponsesTool, error) {
	flat := make([]ResponsesTool, 0, len(tools))
	seen := make(map[string]string, len(tools))
	if err := flattenResponsesTools(tools, &flat, seen, ""); err != nil {
		return nil, err
	}
	return flat, nil
}

// flattenResponsesTools 递归展开 namespace；owner 用于冲突报错时指出归属命名空间。
func flattenResponsesTools(tools []ResponsesTool, out *[]ResponsesTool, seen map[string]string, owner string) error {
	for _, t := range tools {
		if strings.TrimSpace(t.Type) == "namespace" {
			children := t.Tools
			if len(children) == 0 {
				// 两个子键语义相同，二选一。
				children = t.Children
			}
			childOwner := strings.TrimSpace(t.Name)
			if childOwner == "" {
				childOwner = owner
			}
			if err := flattenResponsesTools(children, out, seen, childOwner); err != nil {
				return err
			}
			continue
		}

		name := strings.TrimSpace(t.Name)
		if name != "" {
			if prev, dup := seen[name]; dup {
				return fmt.Errorf("spaces: duplicate tool name %q (already declared in namespace %q)", name, prev)
			}
			seen[name] = owner
		}
		*out = append(*out, t)
	}
	return nil
}

// downgradeResponsesCustomTools 把 custom/freeform 工具改写为 Anthropic 能表达的形态：
// type "custom" → "function"，并在缺少 parameters 时补上单一字符串参数 input。
//
// 为什么必需：Codex 的 exec 以 {"type":"custom","name":"exec","format":{…lark…}}
// 声明，**没有 parameters**。不降级就会走 normalizeAnthropicInputSchema("") 得到
// {"type":"object","properties":{}}——模型看到一个不吃参数的工具，无法表达命令。
//
// 刻意**不**把 format.definition（lark 语法）拼进 description：
//  1. 它很长，会把 description 撞到上游上限（仓内另有 truncateExternalBridgeToolDescriptions 正因如此）；
//  2. 工具本身的 description 已说明用途（“Runs a command…”）。
//
// 若实测发现模型产出的命令不符合语法，再考虑带截断地追加。
func downgradeResponsesCustomTools(tools []ResponsesTool) []ResponsesTool {
	out := make([]ResponsesTool, 0, len(tools))
	for _, t := range tools {
		if strings.TrimSpace(t.Type) != "custom" {
			out = append(out, t)
			continue
		}
		degraded := t
		degraded.Type = "function"
		if len(strings.TrimSpace(string(degraded.Parameters))) == 0 || strings.TrimSpace(string(degraded.Parameters)) == "null" {
			degraded.Parameters = json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}},"required":["input"]}`)
		}
		out = append(out, degraded)
	}
	return out
}

// CustomToolNamesFromResponsesBody 从 Responses **原始请求体**里收集 custom/freeform 工具名。
//
// 为何从 body 而不是从转换结果：服务层需要在**出站**（响应方向）知道哪些工具是 custom，
// 才能把模型对它们的 tool_use 还原成 custom_tool_call；而入站与出站是两个函数。
//
// 入参形状与入站摄取保持一致：顶层 tools + input[].additional_tools 载体（含 namespace 包装）。
// 展平失败（如同名冲突）时返回已收集到的部分——出站只需要名字，不值得因此中断请求。
func CustomToolNamesFromResponsesBody(body []byte) map[string]bool {
	if len(body) == 0 {
		return nil
	}
	var req struct {
		Tools []ResponsesTool   `json:"tools"`
		Input []json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil
	}
	tools := append([]ResponsesTool{}, req.Tools...)
	for _, raw := range req.Input {
		if rawResponsesInputItemType(raw) != "additional_tools" {
			continue
		}
		var carrier struct {
			Tools []ResponsesTool `json:"tools"`
		}
		if err := json.Unmarshal(raw, &carrier); err == nil {
			tools = append(tools, carrier.Tools...)
		}
	}
	if len(tools) == 0 {
		return nil
	}
	flat, err := prepareResponsesTools(tools)
	if err != nil {
		flat = tools
	}
	return CustomToolNames(flat)
}

// convertResponsesToAnthropicTools maps Responses API tools to Anthropic format.
//
// 调用方应先跑 prepareResponsesTools（展平 namespace + 冲突检测）；此处对
// namespace 再做一次递归兑底，避免直接调用本函数时把命名空间包装当成一个
// 空 schema 的工具发上去。
// Reverse of convertAnthropicToolsToResponses.
func convertResponsesToAnthropicTools(tools []ResponsesTool) []AnthropicTool {
	var out []AnthropicTool
	for _, t := range tools {
		switch t.Type {
		case "namespace":
			// 兑底：子工具优先在 prepareResponsesTools 里展平（带冲突检测），
			// 这里直接递归只是为了不把命名空间当成可用工具发出去。
			children := t.Tools
			if len(children) == 0 {
				children = t.Children
			}
			out = append(out, convertResponsesToAnthropicTools(children)...)
		case "web_search", "google_search", "web_search_20250305":
			out = append(out, AnthropicTool{
				Type: "web_search_20250305",
				Name: "web_search",
			})
		case "function":
			out = append(out, AnthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
			})
		case "custom":
			out = append(out, AnthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
			})
		default:
			// Pass through unknown tool types
			out = append(out, AnthropicTool{
				Type:        t.Type,
				Name:        t.Name,
				Description: t.Description,
				InputSchema: normalizeAnthropicInputSchema(t.Parameters),
			})
		}
	}
	return out
}

// normalizeAnthropicInputSchema ensures input_schema is a valid object schema.
func normalizeAnthropicInputSchema(schema json.RawMessage) json.RawMessage {
	const emptyObjectSchema = `{"type":"object","properties":{}}`

	trimmed := strings.TrimSpace(string(schema))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(emptyObjectSchema)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(schema, &m); err != nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}

	typeRaw, ok := m["type"]
	if !ok || strings.TrimSpace(string(typeRaw)) == "" || string(typeRaw) == "null" {
		m["type"] = json.RawMessage(`"object"`)
	} else {
		var typ string
		if err := json.Unmarshal(typeRaw, &typ); err != nil || typ != "object" {
			return json.RawMessage(emptyObjectSchema)
		}
	}

	if _, ok := m["properties"]; !ok {
		m["properties"] = json.RawMessage(`{}`)
	}

	out, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(emptyObjectSchema)
	}
	return out
}

// convertResponsesToAnthropicToolChoice maps Responses tool_choice to Anthropic format.
// Reverse of convertAnthropicToolChoiceToResponses.
//
//	"auto"                                     → {"type":"auto"}
//	"required"                                 → {"type":"any"}
//	"none"                                     → {"type":"none"}
//	{"type":"function","name":"X"}                 → {"type":"tool","name":"X"}
//	{"type":"function","function":{"name":"X"}}     → {"type":"tool","name":"X"} // legacy
func convertResponsesToAnthropicToolChoice(raw json.RawMessage) (json.RawMessage, error) {
	// Try as string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			return json.Marshal(map[string]string{"type": "auto"})
		case "required":
			return json.Marshal(map[string]string{"type": "any"})
		case "none":
			return json.Marshal(map[string]string{"type": "none"})
		default:
			return raw, nil
		}
	}

	// Try as object with type=function
	var tc struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &tc); err == nil && tc.Type == "function" {
		name := strings.TrimSpace(tc.Name)
		if name == "" {
			name = strings.TrimSpace(tc.Function.Name)
		}
		if name == "" {
			return raw, nil
		}
		return json.Marshal(map[string]string{
			"type": "tool",
			"name": name,
		})
	}

	// Pass through unknown
	return raw, nil
}
