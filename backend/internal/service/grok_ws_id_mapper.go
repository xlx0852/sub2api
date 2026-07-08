package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var globalGrokWSIDStateStore grokWSIDStateStore

type grokWSIDStateStore struct {
	mu       sync.Mutex
	sessions map[string]*grokWSIDState
}

type grokWSIDState struct {
	mu                   sync.Mutex
	downstreamToUpstream map[string]string
	sequence             int
	transcriptInput      []json.RawMessage
}

type grokWSRequestIDMapper struct {
	state                *grokWSIDState
	downstreamPreviousID string
	upstreamPreviousID   string
	upstreamResponseID   string
	downstreamResponseID string
}

func getGrokWSIDState(sessionID string) *grokWSIDState {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	globalGrokWSIDStateStore.mu.Lock()
	defer globalGrokWSIDStateStore.mu.Unlock()
	if globalGrokWSIDStateStore.sessions == nil {
		globalGrokWSIDStateStore.sessions = make(map[string]*grokWSIDState)
	}
	if state := globalGrokWSIDStateStore.sessions[sessionID]; state != nil {
		return state
	}
	state := &grokWSIDState{
		downstreamToUpstream: make(map[string]string),
	}
	globalGrokWSIDStateStore.sessions[sessionID] = state
	return state
}

func deleteGrokWSIDState(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	globalGrokWSIDStateStore.mu.Lock()
	delete(globalGrokWSIDStateStore.sessions, sessionID)
	globalGrokWSIDStateStore.mu.Unlock()
}

func newGrokWSRequestIDMapper(sessionID string, downstreamRequest []byte) *grokWSRequestIDMapper {
	state := getGrokWSIDState(sessionID)
	if state == nil {
		return nil
	}
	downstreamPreviousID := strings.TrimSpace(gjson.GetBytes(downstreamRequest, "previous_response_id").String())
	upstreamPreviousID := downstreamPreviousID
	if downstreamPreviousID != "" {
		upstreamPreviousID = state.upstreamIDForDownstream(downstreamPreviousID)
	}
	return &grokWSRequestIDMapper{
		state:                state,
		downstreamPreviousID: downstreamPreviousID,
		upstreamPreviousID:   upstreamPreviousID,
	}
}

func (s *grokWSIDState) upstreamIDForDownstream(downstreamID string) string {
	downstreamID = strings.TrimSpace(downstreamID)
	if s == nil || downstreamID == "" {
		return downstreamID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if upstreamID, ok := s.downstreamToUpstream[downstreamID]; ok {
		return strings.TrimSpace(upstreamID)
	}
	return downstreamID
}

func (s *grokWSIDState) mapDownstreamToUpstream(downstreamID string, upstreamID string) {
	downstreamID = strings.TrimSpace(downstreamID)
	upstreamID = strings.TrimSpace(upstreamID)
	if s == nil || downstreamID == "" || upstreamID == "" {
		return
	}
	s.mu.Lock()
	if s.downstreamToUpstream == nil {
		s.downstreamToUpstream = make(map[string]string)
	}
	s.downstreamToUpstream[downstreamID] = upstreamID
	s.mu.Unlock()
}

func (s *grokWSIDState) prependTranscriptInput(payload []byte) []byte {
	if s == nil || len(payload) == 0 {
		return payload
	}
	s.mu.Lock()
	prefix := make([]json.RawMessage, 0, len(s.transcriptInput))
	for _, item := range s.transcriptInput {
		prefix = append(prefix, bytes.Clone(item))
	}
	s.mu.Unlock()
	if len(prefix) == 0 {
		return payload
	}
	current := grokWSJSONRawMessages(gjson.GetBytes(payload, "input"))
	merged := append(prefix, current...)
	out, err := sjson.SetRawBytes(payload, "input", grokWSMarshalRawMessages(merged))
	if err != nil {
		return payload
	}
	return out
}

func (s *grokWSIDState) recordTranscriptTurn(requestPayload []byte, completedPayload []byte) {
	if s == nil || len(requestPayload) == 0 || len(completedPayload) == 0 {
		return
	}
	inputItems := grokWSJSONRawMessages(gjson.GetBytes(requestPayload, "input"))
	outputItems := grokWSJSONRawMessages(gjson.GetBytes(completedPayload, "response.output"))
	if len(inputItems) == 0 && len(outputItems) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(gjson.GetBytes(requestPayload, "previous_response_id").String()) == "" {
		s.transcriptInput = nil
	}
	s.transcriptInput = append(s.transcriptInput, inputItems...)
	s.transcriptInput = append(s.transcriptInput, outputItems...)
}

func grokWSJSONRawMessages(result gjson.Result) []json.RawMessage {
	if !result.Exists() || !result.IsArray() {
		return nil
	}
	items := result.Array()
	out := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw := bytes.TrimSpace([]byte(item.Raw))
		if len(raw) == 0 || !json.Valid(raw) {
			continue
		}
		out = append(out, bytes.Clone(raw))
	}
	return out
}

func grokWSMarshalRawMessages(items []json.RawMessage) []byte {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(bytes.TrimSpace(item))
	}
	buf.WriteByte(']')
	return buf.Bytes()
}

func (m *grokWSRequestIDMapper) upstreamRequestPayload(payload []byte) []byte {
	if m == nil || len(payload) == 0 || m.downstreamPreviousID == m.upstreamPreviousID {
		return payload
	}
	if m.upstreamPreviousID == "" {
		out, err := sjson.DeleteBytes(payload, "previous_response_id")
		if err != nil {
			return payload
		}
		if m.downstreamPreviousID != "" && m.state != nil {
			out = m.state.prependTranscriptInput(out)
		}
		return out
	}
	out, err := sjson.SetBytes(payload, "previous_response_id", m.upstreamPreviousID)
	if err != nil {
		return payload
	}
	return out
}

func (m *grokWSRequestIDMapper) downstreamResponsePayload(payload []byte) []byte {
	if m == nil || len(payload) == 0 {
		return payload
	}
	upstreamResponseID := extractOpenAIResponseIDFromJSONBytes(payload)
	downstreamResponseID := m.downstreamIDForUpstreamResponse(upstreamResponseID)
	if downstreamResponseID == "" {
		return payload
	}
	return rewriteGrokWSDownstreamIDs(payload, m.upstreamResponseID, downstreamResponseID, m.upstreamPreviousID, m.downstreamPreviousID)
}

func (m *grokWSRequestIDMapper) recordTranscriptTurn(requestPayload []byte, completedPayload []byte) {
	if m == nil || m.state == nil {
		return
	}
	m.state.recordTranscriptTurn(requestPayload, completedPayload)
}

func (m *grokWSRequestIDMapper) downstreamIDForUpstreamResponse(upstreamResponseID string) string {
	upstreamResponseID = strings.TrimSpace(upstreamResponseID)
	if m == nil || m.state == nil {
		return upstreamResponseID
	}
	if m.downstreamResponseID != "" {
		return m.downstreamResponseID
	}
	if upstreamResponseID == "" {
		return ""
	}

	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	m.upstreamResponseID = upstreamResponseID
	m.downstreamResponseID = upstreamResponseID
	if m.state.downstreamToUpstream == nil {
		m.state.downstreamToUpstream = make(map[string]string)
	}
	_, upstreamResponseIDSeen := m.state.downstreamToUpstream[upstreamResponseID]
	if (m.downstreamPreviousID != "" && m.upstreamPreviousID != "" && upstreamResponseID == m.upstreamPreviousID) || upstreamResponseIDSeen {
		m.state.sequence++
		m.downstreamResponseID = fmt.Sprintf("%s-xai-%d", upstreamResponseID, m.state.sequence)
	}
	m.state.downstreamToUpstream[upstreamResponseID] = upstreamResponseID
	m.state.downstreamToUpstream[m.downstreamResponseID] = upstreamResponseID
	return m.downstreamResponseID
}

func rewriteGrokWSDownstreamIDs(payload []byte, upstreamResponseID string, downstreamResponseID string, upstreamPreviousID string, downstreamPreviousID string) []byte {
	upstreamResponseID = strings.TrimSpace(upstreamResponseID)
	downstreamResponseID = strings.TrimSpace(downstreamResponseID)
	upstreamPreviousID = strings.TrimSpace(upstreamPreviousID)
	downstreamPreviousID = strings.TrimSpace(downstreamPreviousID)
	if len(payload) == 0 || (upstreamResponseID == downstreamResponseID && upstreamPreviousID == downstreamPreviousID) {
		return payload
	}

	var value any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return payload
	}
	if !rewriteGrokWSDownstreamIDValue(value, upstreamResponseID, downstreamResponseID, upstreamPreviousID, downstreamPreviousID, "") {
		return payload
	}
	out, err := json.Marshal(value)
	if err != nil {
		return payload
	}
	return out
}

func rewriteGrokWSDownstreamIDValue(value any, upstreamResponseID string, downstreamResponseID string, upstreamPreviousID string, downstreamPreviousID string, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		for childKey, childValue := range typed {
			if childString, ok := childValue.(string); ok {
				replaced := rewriteGrokWSDownstreamIDString(childString, childKey, upstreamResponseID, downstreamResponseID, upstreamPreviousID, downstreamPreviousID)
				if replaced != childString {
					typed[childKey] = replaced
					changed = true
				}
				continue
			}
			if rewriteGrokWSDownstreamIDValue(childValue, upstreamResponseID, downstreamResponseID, upstreamPreviousID, downstreamPreviousID, childKey) {
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for i := range typed {
			if rewriteGrokWSDownstreamIDValue(typed[i], upstreamResponseID, downstreamResponseID, upstreamPreviousID, downstreamPreviousID, key) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

func rewriteGrokWSDownstreamIDString(value string, key string, upstreamResponseID string, downstreamResponseID string, upstreamPreviousID string, downstreamPreviousID string) string {
	switch key {
	case "id", "item_id":
		if upstreamResponseID != "" && downstreamResponseID != "" && downstreamResponseID != upstreamResponseID && strings.Contains(value, upstreamResponseID) {
			return strings.ReplaceAll(value, upstreamResponseID, downstreamResponseID)
		}
	case "previous_response_id":
		if upstreamPreviousID != "" && downstreamPreviousID != "" && value == upstreamPreviousID {
			return downstreamPreviousID
		}
	}
	return value
}