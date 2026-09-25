package apicompat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Non-streaming: AnthropicResponse → ResponsesResponse
// ---------------------------------------------------------------------------

// AnthropicToResponsesResponse converts an Anthropic Messages response into a
// Responses API response. This is the reverse of ResponsesToAnthropic and
// enables Anthropic upstream responses to be returned in OpenAI Responses format.
func AnthropicToResponsesResponse(resp *AnthropicResponse) *ResponsesResponse {
	id := resp.ID
	if id == "" {
		id = generateResponsesID()
	}

	out := &ResponsesResponse{
		ID:     id,
		Object: "response",
		Model:  resp.Model,
	}

	var outputs []ResponsesOutput
	var msgParts []ResponsesContentPart

	for _, block := range resp.Content {
		switch block.Type {
		case "thinking":
			if block.Thinking != "" {
				outputs = append(outputs, ResponsesOutput{
					Type: "reasoning",
					ID:   generateItemID(),
					Summary: []ResponsesSummary{{
						Type: "summary_text",
						Text: block.Thinking,
					}},
				})
			}
		case "text":
			if block.Text != "" {
				msgParts = append(msgParts, ResponsesContentPart{
					Type: "output_text",
					Text: block.Text,
				})
			}
		case "tool_use":
			args := "{}"
			if len(block.Input) > 0 {
				args = string(block.Input)
			}
			outputs = append(outputs, ResponsesOutput{
				Type:      "function_call",
				ID:        generateItemID(),
				CallID:    toResponsesCallID(block.ID),
				Name:      block.Name,
				Arguments: args,
				Status:    "completed",
			})
		}
	}

	// Assemble message output item from text parts
	if len(msgParts) > 0 {
		outputs = append(outputs, ResponsesOutput{
			Type:    "message",
			ID:      generateItemID(),
			Role:    "assistant",
			Content: msgParts,
			Status:  "completed",
		})
	}

	if len(outputs) == 0 {
		outputs = append(outputs, ResponsesOutput{
			Type:    "message",
			ID:      generateItemID(),
			Role:    "assistant",
			Content: []ResponsesContentPart{{Type: "output_text", Text: ""}},
			Status:  "completed",
		})
	}
	out.Output = outputs

	// Map stop_reason → status
	out.Status = anthropicStopReasonToResponsesStatus(resp.StopReason, resp.Content)
	if out.Status == "incomplete" {
		out.IncompleteDetails = &ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	}

	// Usage
	// Anthropic's input_tokens excludes cache_read/cache_creation, while OpenAI
	// Responses' input_tokens is the total including cached tokens. Add them back
	// when converting so downstream consumers see OpenAI semantics.
	totalInputTokens := resp.Usage.InputTokens +
		resp.Usage.CacheReadInputTokens +
		resp.Usage.CacheCreationInputTokens
	out.Usage = &ResponsesUsage{
		InputTokens:  totalInputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		TotalTokens:  totalInputTokens + resp.Usage.OutputTokens,
	}
	if resp.Usage.CacheReadInputTokens > 0 {
		out.Usage.InputTokensDetails = &ResponsesInputTokensDetails{
			CachedTokens: resp.Usage.CacheReadInputTokens,
		}
	}

	return out
}

// anthropicStopReasonToResponsesStatus maps Anthropic stop_reason to Responses status.
func anthropicStopReasonToResponsesStatus(stopReason string, blocks []AnthropicContentBlock) string {
	switch stopReason {
	case "max_tokens":
		return "incomplete"
	case "end_turn", "tool_use", "stop_sequence":
		return "completed"
	default:
		return "completed"
	}
}

// ---------------------------------------------------------------------------
// Streaming: AnthropicStreamEvent → []ResponsesStreamEvent (stateful converter)
// ---------------------------------------------------------------------------

// AnthropicEventToResponsesState tracks state for converting a sequence of
// Anthropic SSE events into Responses SSE events.
type AnthropicEventToResponsesState struct {
	ResponseID     string
	Model          string
	Created        int64
	SequenceNumber int

	// CreatedSent tracks whether response.created has been emitted.
	CreatedSent bool
	// CompletedSent tracks whether the terminal event has been emitted.
	CompletedSent bool

	// Current output tracking
	OutputIndex     int
	CurrentItemID   string
	CurrentItemType string // "message" | "function_call" | "reasoning"

	// For message output: accumulate text parts
	ContentIndex int
	// text accumulates message output_text deltas. Without it the terminal
	// artifacts (output_text.done / output_item.done / response.completed.output)
	// are emitted empty, which is fatal for clients that finalize from those
	// rather than from deltas — Codex CLI reads response.completed.output and
	// reports an empty assistant message otherwise.
	text strings.Builder
	// reasoning accumulates reasoning summary deltas (thinking_delta).
	reasoning strings.Builder
	// currentArguments accumulates function_call argument deltas.
	currentArguments strings.Builder
	// OutputItems accumulates every closed output item in order so the terminal
	// event can carry a complete response.output. Mirrors
	// ChatCompletionsToResponsesStreamState.chatOutput().
	OutputItems []ResponsesOutput

	// For function_call: track per-output info
	CurrentCallID string
	CurrentName   string

	// CustomTools 是本请求里以 custom/freeform 声明的工具名集合。
	//
	// 这些工具（Codex 的 exec 即此类）在**请求向**已降级为「单一 input 字符串参数」，
	// 因此回程必须把模型的 tool_use 还原成 custom_tool_call +
	// response.custom_tool_call_input.* —— codex 只按声明的类型路由：
	// 对 custom 工具回 function_call 会被判为 payload 不兼容。
	CustomTools map[string]bool
	// CurrentIsCustom 记录当前打开的 tool_use 是否属于 CustomTools。
	CurrentIsCustom bool

	// Usage from message_start / message_delta. InputTokens here follows
	// Anthropic semantics (excludes cached tokens); they are added back when
	// emitting the OpenAI Responses usage.
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int

	StopReason string
}

// NewAnthropicEventToResponsesState returns an initialised stream state.
func NewAnthropicEventToResponsesState() *AnthropicEventToResponsesState {
	return &AnthropicEventToResponsesState{
		Created: time.Now().Unix(),
	}
}

// AnthropicEventToResponsesEvents converts a single Anthropic SSE event into
// zero or more Responses SSE events, updating state as it goes.
func AnthropicEventToResponsesEvents(
	evt *AnthropicStreamEvent,
	state *AnthropicEventToResponsesState,
) []ResponsesStreamEvent {
	switch evt.Type {
	case "message_start":
		return anthToResHandleMessageStart(evt, state)
	case "content_block_start":
		return anthToResHandleContentBlockStart(evt, state)
	case "content_block_delta":
		return anthToResHandleContentBlockDelta(evt, state)
	case "content_block_stop":
		return anthToResHandleContentBlockStop(evt, state)
	case "message_delta":
		return anthToResHandleMessageDelta(evt, state)
	case "message_stop":
		return anthToResHandleMessageStop(state)
	default:
		return nil
	}
}

// FinalizeAnthropicResponsesStream emits synthetic termination events if the
// stream ended without a proper message_stop.
func FinalizeAnthropicResponsesStream(state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if !state.CreatedSent || state.CompletedSent {
		return nil
	}

	var events []ResponsesStreamEvent

	// Close any open item
	events = append(events, closeCurrentResponsesItem(state)...)

	// Emit response.completed
	events = append(events, makeResponsesCompletedEvent(state, "completed", nil))
	state.CompletedSent = true
	return events
}

// ResponsesEventToSSE formats a ResponsesStreamEvent as an SSE data line.
func ResponsesEventToSSE(evt ResponsesStreamEvent) (string, error) {
	data, err := json.Marshal(evt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Type, data), nil
}

// --- internal handlers ---

func anthToResHandleMessageStart(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.Message != nil {
		state.ResponseID = evt.Message.ID
		if state.Model == "" {
			state.Model = evt.Message.Model
		}
		if evt.Message.Usage.InputTokens > 0 {
			state.InputTokens = evt.Message.Usage.InputTokens
		}
		if evt.Message.Usage.CacheReadInputTokens > 0 {
			state.CacheReadInputTokens = evt.Message.Usage.CacheReadInputTokens
		}
		if evt.Message.Usage.CacheCreationInputTokens > 0 {
			state.CacheCreationInputTokens = evt.Message.Usage.CacheCreationInputTokens
		}
	}

	if state.CreatedSent {
		return nil
	}
	state.CreatedSent = true

	// Emit response.created
	return []ResponsesStreamEvent{makeResponsesCreatedEvent(state)}
}

func anthToResHandleContentBlockStart(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.ContentBlock == nil {
		return nil
	}

	var events []ResponsesStreamEvent

	switch evt.ContentBlock.Type {
	case "thinking":
		state.CurrentItemID = generateItemID()
		state.CurrentItemType = "reasoning"
		state.ContentIndex = 0

		events = append(events, makeResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex,
			Item: &ResponsesOutput{
				Type: "reasoning",
				ID:   state.CurrentItemID,
			},
		}))

	case "text":
		// If we don't have an open message item, open one
		if state.CurrentItemType != "message" {
			state.CurrentItemID = generateItemID()
			state.CurrentItemType = "message"
			state.ContentIndex = 0

			events = append(events, makeResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
				OutputIndex: state.OutputIndex,
				Item: &ResponsesOutput{
					Type:   "message",
					ID:     state.CurrentItemID,
					Role:   "assistant",
					Status: "in_progress",
				},
			}))
		}

	case "tool_use":
		// Close previous item if any
		events = append(events, closeCurrentResponsesItem(state)...)

		state.CurrentItemID = generateItemID()
		state.CurrentItemType = "function_call"
		state.CurrentCallID = toResponsesCallID(evt.ContentBlock.ID)
		state.CurrentName = evt.ContentBlock.Name
		state.CurrentIsCustom = state.CustomTools[strings.TrimSpace(evt.ContentBlock.Name)]

		events = append(events, makeResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex,
			Item: &ResponsesOutput{
				Type:   responsesItemTypeForTool(state.CurrentIsCustom),
				ID:     state.CurrentItemID,
				CallID: state.CurrentCallID,
				Name:   state.CurrentName,
				Status: "in_progress",
			},
		}))
	}

	return events
}

func anthToResHandleContentBlockDelta(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.Delta == nil {
		return nil
	}

	switch evt.Delta.Type {
	case "text_delta":
		if evt.Delta.Text == "" {
			return nil
		}
		state.text.WriteString(evt.Delta.Text)
		return []ResponsesStreamEvent{makeResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
			OutputIndex:  state.OutputIndex,
			ContentIndex: state.ContentIndex,
			Delta:        evt.Delta.Text,
			ItemID:       state.CurrentItemID,
		})}

	case "thinking_delta":
		if evt.Delta.Thinking == "" {
			return nil
		}
		state.reasoning.WriteString(evt.Delta.Thinking)
		return []ResponsesStreamEvent{makeResponsesEvent(state, "response.reasoning_summary_text.delta", &ResponsesStreamEvent{
			OutputIndex:  state.OutputIndex,
			SummaryIndex: 0,
			Delta:        evt.Delta.Thinking,
			ItemID:       state.CurrentItemID,
		})}

	case "input_json_delta":
		if evt.Delta.PartialJSON == "" {
			return nil
		}
		state.currentArguments.WriteString(evt.Delta.PartialJSON)
		// custom 工具不发增量：codex 走 custom_tool_call_input.delta/done 生命周期，
		// 且 input 在收尾时一次性下发（同 CC 桥的做法）。
		if state.CurrentIsCustom {
			return nil
		}
		return []ResponsesStreamEvent{makeResponsesEvent(state, "response.function_call_arguments.delta", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex,
			Delta:       evt.Delta.PartialJSON,
			ItemID:      state.CurrentItemID,
			CallID:      state.CurrentCallID,
			Name:        state.CurrentName,
		})}

	case "signature_delta":
		// Anthropic signature deltas have no Responses equivalent; skip
		return nil
	}

	return nil
}

func anthToResHandleContentBlockStop(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	switch state.CurrentItemType {
	case "reasoning":
		// Emit reasoning summary done + output item done
		events := []ResponsesStreamEvent{
			makeResponsesEvent(state, "response.reasoning_summary_text.done", &ResponsesStreamEvent{
				OutputIndex:  state.OutputIndex,
				SummaryIndex: 0,
				ItemID:       state.CurrentItemID,
				Text:         state.reasoning.String(),
			}),
		}
		events = append(events, closeCurrentResponsesItem(state)...)
		return events

	case "function_call":
		accumulated := state.currentArguments.String()
		if state.CurrentIsCustom {
			// custom 工具：input 在此处一次性下发（delta + done），与 CC 桥一致。
			input := extractResponsesCustomToolInput(accumulated)
			events := []ResponsesStreamEvent{
				makeResponsesEvent(state, "response.custom_tool_call_input.delta", &ResponsesStreamEvent{
					OutputIndex: state.OutputIndex,
					ItemID:      state.CurrentItemID,
					Delta:       input,
				}),
			}
			events = append(events, makeResponsesEvent(state, "response.custom_tool_call_input.done", &ResponsesStreamEvent{
				OutputIndex: state.OutputIndex,
				ItemID:      state.CurrentItemID,
				Input:       input,
			}))
			events = append(events, closeCurrentResponsesItem(state)...)
			return events
		}
		// Emit function_call_arguments.done + output item done
		events := []ResponsesStreamEvent{
			makeResponsesEvent(state, "response.function_call_arguments.done", &ResponsesStreamEvent{
				OutputIndex: state.OutputIndex,
				ItemID:      state.CurrentItemID,
				CallID:      state.CurrentCallID,
				Name:        state.CurrentName,
				Arguments:   accumulated,
			}),
		}
		events = append(events, closeCurrentResponsesItem(state)...)
		return events

	case "message":
		// Emit output_text.done (text block is done, but message item stays open for potential more blocks)
		return []ResponsesStreamEvent{
			makeResponsesEvent(state, "response.output_text.done", &ResponsesStreamEvent{
				OutputIndex:  state.OutputIndex,
				ContentIndex: state.ContentIndex,
				ItemID:       state.CurrentItemID,
				Text:         state.text.String(),
			}),
		}
	}

	return nil
}

func anthToResHandleMessageDelta(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.Usage != nil {
		state.OutputTokens = evt.Usage.OutputTokens
		if evt.Usage.InputTokens > 0 {
			state.InputTokens = evt.Usage.InputTokens
		}
		if evt.Usage.CacheReadInputTokens > 0 {
			state.CacheReadInputTokens = evt.Usage.CacheReadInputTokens
		}
		if evt.Usage.CacheCreationInputTokens > 0 {
			state.CacheCreationInputTokens = evt.Usage.CacheCreationInputTokens
		}
	}
	if evt.Delta != nil && evt.Delta.StopReason != "" {
		state.StopReason = evt.Delta.StopReason
	}

	return nil
}

func anthToResHandleMessageStop(state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if state.CompletedSent {
		return nil
	}

	var events []ResponsesStreamEvent
	events = append(events, closeCurrentResponsesItem(state)...)

	status := "completed"
	var incompleteDetails *ResponsesIncompleteDetails
	if state.StopReason == "max_tokens" {
		status = "incomplete"
		incompleteDetails = &ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	}

	events = append(events, makeResponsesCompletedEvent(state, status, incompleteDetails))
	state.CompletedSent = true
	return events
}

// --- helper functions ---

func closeCurrentResponsesItem(state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if state.CurrentItemType == "" {
		return nil
	}

	itemType := state.CurrentItemType
	itemID := state.CurrentItemID
	isCustom := state.CurrentIsCustom

	// Build the completed item from the accumulated deltas before resetting the
	// buffers. Emitting only Type/ID/Status (the previous behaviour) left every
	// item with no text, so clients reading output_item.done saw empty content.
	item := ResponsesOutput{
		Type:   itemType,
		ID:     itemID,
		Status: "completed",
	}
	if itemType == "function_call" && isCustom {
		// custom 工具回 custom_tool_call + 自由文本 input（不是 JSON arguments）。
		item.Type = "custom_tool_call"
		item.CallID = state.CurrentCallID
		item.Name = state.CurrentName
		item.Input = extractResponsesCustomToolInput(state.currentArguments.String())
		state.OutputItems = append(state.OutputItems, item)
		state.CurrentItemType = ""
		state.CurrentItemID = ""
		state.CurrentCallID = ""
		state.CurrentName = ""
		state.CurrentIsCustom = false
		state.currentArguments.Reset()
		state.text.Reset()
		state.reasoning.Reset()
		state.OutputIndex++
		state.ContentIndex = 0
		return []ResponsesStreamEvent{makeResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex - 1,
			Item:        &item,
		})}
	}
	switch itemType {
	case "message":
		item.Role = "assistant"
		item.Content = []ResponsesContentPart{{Type: "output_text", Text: state.text.String()}}
	case "reasoning":
		item.Summary = []ResponsesSummary{{Type: "summary_text", Text: state.reasoning.String()}}
	case "function_call":
		item.CallID = state.CurrentCallID
		item.Name = state.CurrentName
		item.Arguments = state.currentArguments.String()
	}
	state.OutputItems = append(state.OutputItems, item)

	// Reset
	state.CurrentItemType = ""
	state.CurrentItemID = ""
	state.CurrentCallID = ""
	state.CurrentName = ""
	state.CurrentIsCustom = false
	state.text.Reset()
	state.reasoning.Reset()
	state.currentArguments.Reset()
	state.OutputIndex++
	state.ContentIndex = 0

	return []ResponsesStreamEvent{makeResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
		OutputIndex: state.OutputIndex - 1, // Use the index before increment
		Item:        &item,
	})}
}

func makeResponsesCreatedEvent(state *AnthropicEventToResponsesState) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++
	return ResponsesStreamEvent{
		Type:           "response.created",
		SequenceNumber: seq,
		Response: &ResponsesResponse{
			ID:     state.ResponseID,
			Object: "response",
			Model:  state.Model,
			Status: "in_progress",
			Output: []ResponsesOutput{},
		},
	}
}

func makeResponsesCompletedEvent(
	state *AnthropicEventToResponsesState,
	status string,
	incompleteDetails *ResponsesIncompleteDetails,
) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++

	// Anthropic's input_tokens excludes cache_read/cache_creation; add them
	// back to match OpenAI Responses semantics where input_tokens is the total.
	totalInputTokens := state.InputTokens + state.CacheReadInputTokens + state.CacheCreationInputTokens
	usage := &ResponsesUsage{
		InputTokens:  totalInputTokens,
		OutputTokens: state.OutputTokens,
		TotalTokens:  totalInputTokens + state.OutputTokens,
	}
	if state.CacheReadInputTokens > 0 {
		usage.InputTokensDetails = &ResponsesInputTokensDetails{
			CachedTokens: state.CacheReadInputTokens,
		}
	}

	eventType := "response.completed"
	if status == "incomplete" {
		eventType = "response.incomplete"
	}

	// Carry every closed output item so the terminal event is complete. Clients
	// such as Codex CLI finalize the assistant message from this payload; an
	// always-empty slice produced empty answers.
	output := state.OutputItems
	if output == nil {
		output = []ResponsesOutput{}
	}

	return ResponsesStreamEvent{
		Type:           eventType,
		SequenceNumber: seq,
		Response: &ResponsesResponse{
			ID:                state.ResponseID,
			Object:            "response",
			Model:             state.Model,
			Status:            status,
			Output:            output,
			Usage:             usage,
			IncompleteDetails: incompleteDetails,
		},
	}
}

// responsesItemTypeForTool 返回该工具对应的 Responses 项类型：
// custom/freeform 工具用 custom_tool_call —— codex 只按声明的类型路由。
func responsesItemTypeForTool(isCustom bool) string {
	if isCustom {
		return "custom_tool_call"
	}
	return "function_call"
}

// extractResponsesCustomToolInput 从降级后的单一参数 JSON（{"input":"…"}）里取出自由文本。
// 解不出时退回原始字符串（宁可把原文交给客户端，也不要丢内容）。
func extractResponsesCustomToolInput(arguments string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return ""
	}
	var v struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal([]byte(trimmed), &v); err == nil && v.Input != "" {
		return v.Input
	}
	return trimmed
}

func makeResponsesEvent(state *AnthropicEventToResponsesState, eventType string, template *ResponsesStreamEvent) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++

	evt := *template
	evt.Type = eventType
	evt.SequenceNumber = seq
	return evt
}

func generateResponsesID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "resp_" + hex.EncodeToString(b)
}

func generateItemID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "item_" + hex.EncodeToString(b)
}
