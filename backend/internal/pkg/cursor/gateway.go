package cursor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// GatewayClientVersion is the Grok Bot / Sand host version used for EnsureSandBox.
	GatewayClientVersion = "0.44.0"

	HeaderAnyrunNetworkToken = "x-anyrun-network-token"

	defaultSandboxTTL       = 10 * time.Minute
	defaultSandboxSkew      = 30 * time.Second
	defaultGatewayTimeout   = 120 * time.Second
	defaultPollInterval     = 500 * time.Millisecond
	defaultHealthTimeout    = 8 * time.Second
	defaultMaxEnsureRetries = 3

	cursorSandboxEndpoint = "/aiserver.v1.GrokBotService/EnsureSandBox"
	cursorGatewayEndpoint = "sand-gateway"
)

// SandboxSession holds credentials returned by EnsureSandBox.
type SandboxSession struct {
	GatewayURL   string
	GatewayToken string
	NetworkToken string
	Cluster      string
	TenantID     string
	PodID        string
	ExpiresAt    time.Time
}

// GatewayEvent is one SSE frame from GET {gateway}/events.
type GatewayEvent struct {
	Channel string          `json:"channel"`
	Payload json.RawMessage `json:"payload"`
}

// TranscriptEntry is a normalized agent transcript row.
type TranscriptEntry struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	TimestampMs int64           `json:"timestampMs"`
	Seq         int64           `json:"seq"`
	Role        string          `json:"role,omitempty"`
	Content     json.RawMessage `json:"content,omitempty"`
	IsStreaming bool            `json:"isStreaming,omitempty"`
	Name        string          `json:"name,omitempty"`
	Status      string          `json:"status,omitempty"`
	Args        json.RawMessage `json:"args,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	ToolCallID  string          `json:"toolCallId,omitempty"`
	Message     *TranscriptMsg  `json:"message,omitempty"`
	Raw         json.RawMessage `json:"-"`
}

// TranscriptMsg is the nested message object on send-message entries.
type TranscriptMsg struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// ChatDelta is an incremental unit emitted while waiting for a gateway reply.
type ChatDelta struct {
	Text         string
	Reasoning    string
	ToolCallID   string
	ToolName     string
	ToolArgs     string
	ToolIndex    int
	FinishReason string // stop | tool_calls | "" (still running)
	Done         bool
	Usage        *ChatUsageEstimate
}

// ChatUsageEstimate is a best-effort token usage for billing when upstream is silent.
type ChatUsageEstimate struct {
	InputTokens  int
	OutputTokens int
}

// GatewayChatRequest is the input for a sandbox-gateway chat turn.
type GatewayChatRequest struct {
	Messages       []OpenAIMessage
	Model          string // desired agentDefaultModel.modelId (best-effort bind)
	ConversationID string
	Tools          []OpenAITool
	// Stream when true prefers /events SSE; falls back to transcript polling with deltas.
	Stream bool
	// Timeout bounds the whole turn. Zero uses defaultGatewayTimeout.
	Timeout time.Duration
	// PollInterval for transcript polling fallback.
	PollInterval time.Duration
}

// GatewayChatResult is the final aggregated reply from a non-stream or drained stream.
type GatewayChatResult struct {
	Text         string
	Reasoning    string
	ToolCalls    []OpenAIToolCall
	FinishReason string
	AgentID      string
	Model        string
	Usage        ChatUsageEstimate
	FirstTokenAt time.Time
}

var (
	sandboxSessionMu    sync.Mutex
	sandboxSessionCache = map[string]*SandboxSession{} // token hash -> session
	agentIDMu           sync.Mutex
	agentIDCache        = map[string]string{} // tokenHash:conversation -> agentId
)

func tokenCacheKey(token string) string {
	// Short stable key; full token must not be logged.
	sum := 0
	for i := 0; i < len(token); i++ {
		sum = sum*31 + int(token[i])
	}
	if sum < 0 {
		sum = -sum
	}
	return fmt.Sprintf("%x:%d", sum, len(token))
}

// InvalidateSandboxCache drops cached sandbox sessions (e.g. after 401).
func InvalidateSandboxCache(token string) {
	if token == "" {
		return
	}
	key := tokenCacheKey(token)
	sandboxSessionMu.Lock()
	delete(sandboxSessionCache, key)
	sandboxSessionMu.Unlock()
	agentIDMu.Lock()
	prefix := key + ":"
	for k := range agentIDCache {
		if strings.HasPrefix(k, prefix) {
			delete(agentIDCache, k)
		}
	}
	agentIDMu.Unlock()
}

// EnsureSandBox calls GrokBotService/EnsureSandBox and caches the gateway session.
func (c *Client) EnsureSandBox(ctx context.Context, wake bool) (*SandboxSession, error) {
	key := tokenCacheKey(c.token)
	sandboxSessionMu.Lock()
	if cached := sandboxSessionCache[key]; cached != nil && time.Now().Before(cached.ExpiresAt.Add(-defaultSandboxSkew)) {
		sess := *cached
		sandboxSessionMu.Unlock()
		return &sess, nil
	}
	sandboxSessionMu.Unlock()

	var lastErr error
	for attempt := 0; attempt < defaultMaxEnsureRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
		sess, err := c.ensureSandBoxOnce(ctx, wake || attempt > 0)
		if err == nil {
			sandboxSessionMu.Lock()
			sandboxSessionCache[key] = sess
			sandboxSessionMu.Unlock()
			return sess, nil
		}
		lastErr = err
		if up, ok := err.(*UpstreamError); ok && up.StatusCode == http.StatusUnauthorized {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("EnsureSandBox failed")
	}
	return nil, lastErr
}

func (c *Client) ensureSandBoxOnce(ctx context.Context, wake bool) (*SandboxSession, error) {
	bodyObj := map[string]any{}
	if wake {
		bodyObj["wake"] = true
	}
	body, _ := json.Marshal(bodyObj)
	url := c.baseURL + cursorSandboxEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.applyGatewayBackendHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("EnsureSandBox request: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       string(data),
			Headers:    resp.Header.Clone(),
		}
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("EnsureSandBox decode: %w", err)
	}
	gatewayURL := strings.TrimRight(firstString(raw, "gatewayUrl", "gateway_url"), "/")
	gatewayToken := firstString(raw, "gatewayToken", "gateway_token")
	networkToken := firstString(raw, "networkToken", "network_token", "anyrunNetworkToken", "anyrun_network_token")
	if gatewayURL == "" || gatewayToken == "" {
		return nil, fmt.Errorf("EnsureSandBox missing gatewayUrl/gatewayToken")
	}
	return &SandboxSession{
		GatewayURL:   gatewayURL,
		GatewayToken: gatewayToken,
		NetworkToken: networkToken,
		Cluster:      firstString(raw, "cluster"),
		TenantID:     firstString(raw, "tenantId", "tenant_id"),
		PodID:        firstString(raw, "podId", "pod_id"),
		ExpiresAt:    time.Now().Add(defaultSandboxTTL),
	}, nil
}

func (c *Client) applyGatewayBackendHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set(HeaderCursorChecksum, CreateCursorChecksum(c.machineID, time.Now()))
	req.Header.Set(HeaderCursorClientType, DefaultClientType)
	req.Header.Set(HeaderCursorClientVersion, GatewayClientVersion)
	req.Header.Set(HeaderSandBoxNamespace, DefaultBoxNamespace)
	req.Header.Set(HeaderGhostMode, "false")
	req.Header.Set(HeaderRequestID, uuid.NewString())
}

func (c *Client) gatewayHeaders(sess *SandboxSession) http.Header {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	h.Set("Authorization", "Bearer "+sess.GatewayToken)
	if sess.NetworkToken != "" {
		h.Set(HeaderAnyrunNetworkToken, sess.NetworkToken)
	}
	return h
}

// GatewayCall POSTs JSON to {gateway}/api/{method}.
func (c *Client) GatewayCall(ctx context.Context, sess *SandboxSession, method string, body any) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := sess.GatewayURL + "/api/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header = c.gatewayHeaders(sess)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway %s: %w", method, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       fmt.Sprintf("gateway %s: %s", method, string(data)),
			Headers:    resp.Header.Clone(),
		}
	}
	if len(data) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return json.RawMessage(data), nil
}

// CheckGatewayHealth GETs {gateway}/health. Returns nil when healthy.
func (c *Client) CheckGatewayHealth(ctx context.Context, sess *SandboxSession) error {
	hctx, cancel := context.WithTimeout(ctx, defaultHealthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(hctx, http.MethodGet, sess.GatewayURL+"/health", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+sess.GatewayToken)
	if sess.NetworkToken != "" {
		req.Header.Set(HeaderAnyrunNetworkToken, sess.NetworkToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gateway health: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
			Headers:    resp.Header.Clone(),
		}
	}
	return nil
}

// EnsureHealthySandbox ensures a sandbox session and that /health succeeds,
// recreating the box on failure.
func (c *Client) EnsureHealthySandbox(ctx context.Context) (*SandboxSession, error) {
	sess, err := c.EnsureSandBox(ctx, false)
	if err != nil {
		return nil, err
	}
	if err := c.CheckGatewayHealth(ctx, sess); err == nil {
		return sess, nil
	}
	// Invalidate and force wake.
	InvalidateSandboxCache(c.token)
	sess, err = c.EnsureSandBox(ctx, true)
	if err != nil {
		return nil, err
	}
	if err := c.CheckGatewayHealth(ctx, sess); err != nil {
		// Health endpoint may 404 on some clusters; still try API.
		if up, ok := err.(*UpstreamError); ok && up.StatusCode == http.StatusNotFound {
			return sess, nil
		}
		// Probe listAgents as secondary health signal.
		if _, listErr := c.GatewayCall(ctx, sess, "listAgents", map[string]any{}); listErr != nil {
			return nil, fmt.Errorf("sandbox unhealthy: health=%v listAgents=%v", err, listErr)
		}
	}
	return sess, nil
}

// EnsureAgent returns a stable agent id for the conversation, creating one if needed.
func (c *Client) EnsureAgent(ctx context.Context, sess *SandboxSession, conversationID string) (string, error) {
	cacheKey := tokenCacheKey(c.token) + ":" + firstNonEmpty(conversationID, "default")
	agentIDMu.Lock()
	if id := agentIDCache[cacheKey]; id != "" {
		agentIDMu.Unlock()
		return id, nil
	}
	agentIDMu.Unlock()

	if conversationID != "" {
		raw, err := c.GatewayCall(ctx, sess, "listAgents", map[string]any{})
		if err == nil {
			agents := unwrapAgents(raw)
			for _, a := range agents {
				if a.ID == conversationID || a.Name == conversationID {
					agentIDMu.Lock()
					agentIDCache[cacheKey] = a.ID
					agentIDMu.Unlock()
					return a.ID, nil
				}
			}
		}
	}

	name := "chat-" + uuid.NewString()[:8]
	if conversationID != "" {
		suffix := conversationID
		if len(suffix) > 8 {
			suffix = suffix[:8]
		}
		name = "chat-" + suffix
	}
	var lastErr error
	for attempt := 0; attempt < defaultMaxEnsureRetries; attempt++ {
		raw, err := c.GatewayCall(ctx, sess, "createAgent", map[string]any{
			"name":        name,
			"description": "",
			"title":       "",
			"avatarShape": "",
			"avatarColor": "",
			"origin":      "user",
		})
		if err == nil {
			id := unwrapAgentID(raw)
			if id == "" {
				lastErr = fmt.Errorf("createAgent returned no id")
				continue
			}
			agentIDMu.Lock()
			agentIDCache[cacheKey] = id
			agentIDMu.Unlock()
			return id, nil
		}
		lastErr = err
		if up, ok := err.(*UpstreamError); ok && up.StatusCode >= 400 && up.StatusCode < 500 && up.StatusCode != 429 {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("createAgent failed")
	}
	return "", lastErr
}

// BindAgentDefaultModel best-effort sets host agentDefaultModel via setHostSettings.
// Gateway may reject this on some clusters; failures are non-fatal.
func (c *Client) BindAgentDefaultModel(ctx context.Context, sess *SandboxSession, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	// Drop local suffix markers used by billing.
	modelID, maxMode, _ := ResolveRequestedModel(model)
	body := map[string]any{
		"agentDefaultModel": map[string]any{
			"modelId": modelID,
			"maxMode": maxMode,
		},
	}
	_, err := c.GatewayCall(ctx, sess, "setHostSettings", body)
	return err
}

// FormatMessagesForGateway flattens OpenAI messages (+ tools) into a single prompt string.
func FormatMessagesForGateway(messages []OpenAIMessage, tools []OpenAITool) string {
	var parts []string
	if len(tools) > 0 {
		var tb strings.Builder
		tb.WriteString("[AVAILABLE_TOOLS]\n")
		for _, t := range tools {
			if t.Type != "function" || t.Function.Name == "" {
				continue
			}
			tb.WriteString("- name: ")
			tb.WriteString(t.Function.Name)
			if t.Function.Description != "" {
				tb.WriteString("\n  description: ")
				tb.WriteString(t.Function.Description)
			}
			if len(t.Function.Parameters) > 0 {
				tb.WriteString("\n  parameters: ")
				tb.Write(t.Function.Parameters)
			}
			tb.WriteByte('\n')
		}
		tb.WriteString("When you need a tool, reply with a JSON block:\n")
		tb.WriteString(`{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"...","arguments":"{...}"}}]}`)
		tb.WriteByte('\n')
		parts = append(parts, tb.String())
	}

	toolCallMap := BuildToolCallMap(messages)
	for _, m := range messages {
		text := openAIContentToText(m.Content)
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "system", "developer":
			if text != "" {
				parts = append(parts, "[SYSTEM]\n"+text)
			}
		case "user":
			if text != "" {
				parts = append(parts, "[USER]\n"+text)
			}
		case "assistant":
			var b strings.Builder
			b.WriteString("[ASSISTANT]\n")
			if text != "" {
				b.WriteString(text)
			}
			if len(m.ToolCalls) > 0 {
				if text != "" {
					b.WriteByte('\n')
				}
				raw, _ := json.Marshal(map[string]any{"tool_calls": m.ToolCalls})
				b.Write(raw)
			}
			parts = append(parts, b.String())
		case "tool":
			name := m.Name
			if name == "" {
				name = toolCallMap[m.ToolCallID]
			}
			if name == "" {
				name = m.ToolCallID
			}
			if name == "" {
				name = "result"
			}
			parts = append(parts, fmt.Sprintf("[TOOL %s]\n%s", name, text))
		default:
			if text != "" {
				parts = append(parts, "["+strings.ToUpper(role)+"]\n"+text)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func openAIContentToText(content json.RawMessage) string {
	if len(content) == 0 || string(content) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	var parts []OpenAIContentPart
	if err := json.Unmarshal(content, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" || p.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return strings.TrimSpace(string(content))
}

// EstimateTokens rough-counts tokens for billing when upstream omits usage.
func EstimateTokens(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	runes := []rune(s)
	ascii := 0
	for _, r := range runes {
		if r <= 0x7f {
			ascii++
		}
	}
	ratio := float64(ascii) / float64(len(runes))
	if ratio >= 0.8 {
		n := (len(runes) + 3) / 4
		if n < 1 {
			n = 1
		}
		return n
	}
	// CJK-heavy: ~1.5 chars/token
	n := int(float64(len(runes))/1.5 + 0.5)
	if n < 1 {
		n = 1
	}
	return n
}

// SendPrompt posts a prompt to an agent.
func (c *Client) SendPrompt(ctx context.Context, sess *SandboxSession, agentID, prompt, clientNonce string) error {
	if clientNonce == "" {
		clientNonce = uuid.NewString()
	}
	raw, err := c.GatewayCall(ctx, sess, "sendPrompt", map[string]any{
		"agentId":     agentID,
		"prompt":      prompt,
		"clientNonce": clientNonce,
	})
	if err != nil {
		return err
	}
	var parsed struct {
		Accepted *bool `json:"accepted"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if parsed.Accepted != nil && !*parsed.Accepted {
		return fmt.Errorf("sendPrompt not accepted: %s", string(raw))
	}
	return nil
}

// GetAgentTranscriptTail fetches the latest transcript entries.
func (c *Client) GetAgentTranscriptTail(ctx context.Context, sess *SandboxSession, agentID string, limit int) ([]TranscriptEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	raw, err := c.GatewayCall(ctx, sess, "getAgentTranscriptTail", map[string]any{
		"id":    agentID,
		"limit": limit,
	})
	if err != nil {
		return nil, err
	}
	return parseTranscriptEntries(raw), nil
}

// OpenGatewayEvents opens GET {gateway}/events as an SSE stream.
func (c *Client) OpenGatewayEvents(ctx context.Context, sess *SandboxSession) (*GatewayEventStream, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sess.GatewayURL+"/events", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+sess.GatewayToken)
	if sess.NetworkToken != "" {
		req.Header.Set(HeaderAnyrunNetworkToken, sess.NetworkToken)
	}
	// Use a client without a global timeout for long-lived SSE.
	httpClient := c.httpClient
	if httpClient.Timeout > 0 {
		clone := *httpClient
		clone.Timeout = 0
		httpClient = &clone
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway events: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &UpstreamError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
			Headers:    resp.Header.Clone(),
		}
	}
	sc := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 2<<20)
	return &GatewayEventStream{body: resp.Body, scanner: sc}, nil
}

// GatewayEventStream reads SSE frames from the sandbox gateway.
type GatewayEventStream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	buf     []string
}

func (s *GatewayEventStream) Close() error {
	if s.body != nil {
		return s.body.Close()
	}
	return nil
}

// Recv returns the next GatewayEvent or io.EOF.
func (s *GatewayEventStream) Recv() (*GatewayEvent, error) {
	for s.scanner.Scan() {
		line := s.scanner.Text()
		if strings.HasPrefix(line, "data:") {
			s.buf = append(s.buf, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		if line == "" && len(s.buf) > 0 {
			payload := strings.Join(s.buf, "\n")
			s.buf = s.buf[:0]
			var ev GatewayEvent
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				continue
			}
			if ev.Channel == "" {
				continue
			}
			return &ev, nil
		}
	}
	if err := s.scanner.Err(); err != nil {
		return nil, err
	}
	// Flush trailing data without blank line.
	if len(s.buf) > 0 {
		payload := strings.Join(s.buf, "\n")
		s.buf = s.buf[:0]
		var ev GatewayEvent
		if err := json.Unmarshal([]byte(payload), &ev); err == nil && ev.Channel != "" {
			return &ev, nil
		}
	}
	return nil, io.EOF
}

// ChatViaGateway runs one chat turn over the sandbox gateway.
// For Stream=true, deltas are sent on the returned channel; the channel is closed when done.
// For Stream=false, the channel yields a single final delta then closes.
func (c *Client) ChatViaGateway(ctx context.Context, req *GatewayChatRequest) (<-chan ChatDelta, *GatewayChatResult, error) {
	if req == nil {
		return nil, nil, fmt.Errorf("nil gateway chat request")
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultGatewayTimeout
	}
	pollEvery := req.PollInterval
	if pollEvery <= 0 {
		pollEvery = defaultPollInterval
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)

	sess, err := c.EnsureHealthySandbox(ctx)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	// Best-effort model bind (host prefs). Ignore failure.
	_ = c.BindAgentDefaultModel(ctx, sess, req.Model)

	agentID, err := c.EnsureAgent(ctx, sess, req.ConversationID)
	if err != nil {
		// Retry once with fresh sandbox.
		InvalidateSandboxCache(c.token)
		sess, err2 := c.EnsureHealthySandbox(ctx)
		if err2 != nil {
			cancel()
			return nil, nil, err
		}
		agentID, err = c.EnsureAgent(ctx, sess, req.ConversationID)
		if err != nil {
			cancel()
			return nil, nil, err
		}
	}

	prompt := FormatMessagesForGateway(req.Messages, req.Tools)
	if strings.TrimSpace(prompt) == "" {
		cancel()
		return nil, nil, fmt.Errorf("no message content to send")
	}

	inputTokens := EstimateTokens(prompt)
	startedAt := time.Now()
	clientNonce := uuid.NewString()

	if err := c.SendPrompt(ctx, sess, agentID, prompt, clientNonce); err != nil {
		// Session may have expired — recreate and retry once.
		if isTransientGatewayErr(err) {
			InvalidateSandboxCache(c.token)
			sess, err2 := c.EnsureHealthySandbox(ctx)
			if err2 == nil {
				agentID, err2 = c.EnsureAgent(ctx, sess, req.ConversationID)
				if err2 == nil {
					err = c.SendPrompt(ctx, sess, agentID, prompt, clientNonce)
				} else {
					err = err2
				}
			}
		}
		if err != nil {
			cancel()
			return nil, nil, err
		}
	}

	out := make(chan ChatDelta, 16)
	result := &GatewayChatResult{
		AgentID: agentID,
		Model:   req.Model,
		Usage:   ChatUsageEstimate{InputTokens: inputTokens},
	}

	go func() {
		defer close(out)
		defer cancel()

		var deltas <-chan ChatDelta
		var streamErr error
		if req.Stream {
			deltas, streamErr = c.streamViaEvents(ctx, sess, agentID, startedAt)
			if streamErr != nil {
				deltas = c.streamViaPoll(ctx, sess, agentID, startedAt, pollEvery)
			}
		} else {
			deltas = c.streamViaPoll(ctx, sess, agentID, startedAt, pollEvery)
		}

		var textBuilder, reasonBuilder strings.Builder
		toolCalls := map[string]*OpenAIToolCall{}
		toolOrder := []string{}
		finish := "stop"
		var firstToken time.Time

		for d := range deltas {
			if firstToken.IsZero() && (d.Text != "" || d.Reasoning != "" || d.ToolName != "") {
				firstToken = time.Now()
				result.FirstTokenAt = firstToken
			}
			if d.Text != "" {
				textBuilder.WriteString(d.Text)
			}
			if d.Reasoning != "" {
				reasonBuilder.WriteString(d.Reasoning)
			}
			if d.ToolName != "" || d.ToolCallID != "" {
				id := d.ToolCallID
				if id == "" {
					id = fmt.Sprintf("call_%d", len(toolOrder))
				}
				tc, ok := toolCalls[id]
				if !ok {
					tc = &OpenAIToolCall{ID: id, Type: "function"}
					toolCalls[id] = tc
					toolOrder = append(toolOrder, id)
				}
				if d.ToolName != "" {
					tc.Function.Name = d.ToolName
				}
				if d.ToolArgs != "" {
					tc.Function.Arguments += d.ToolArgs
				}
				finish = "tool_calls"
			}
			if d.FinishReason != "" {
				finish = d.FinishReason
			}
			out <- d
			if d.Done {
				break
			}
		}

		result.Text = textBuilder.String()
		result.Reasoning = reasonBuilder.String()
		result.FinishReason = finish
		result.Usage.OutputTokens = EstimateTokens(result.Text + result.Reasoning)
		for _, id := range toolOrder {
			result.ToolCalls = append(result.ToolCalls, *toolCalls[id])
		}

		// If nothing was emitted (e.g. empty channel), still publish final.
		if firstToken.IsZero() && result.Text == "" && len(result.ToolCalls) == 0 {
			// Try one last poll for a complete reply.
			entries, err := c.GetAgentTranscriptTail(ctx, sess, agentID, 80)
			if err == nil {
				agg := aggregateTranscriptSince(entries, startedAt)
				result.Text = agg.Text
				result.Reasoning = agg.Reasoning
				result.ToolCalls = agg.ToolCalls
				if len(result.ToolCalls) > 0 {
					result.FinishReason = "tool_calls"
				}
				result.Usage.OutputTokens = EstimateTokens(result.Text + result.Reasoning)
				out <- ChatDelta{
					Text:         result.Text,
					Reasoning:    result.Reasoning,
					FinishReason: result.FinishReason,
					Done:         true,
					Usage:        &result.Usage,
				}
			}
		}
	}()

	return out, result, nil
}

// ChatViaGatewaySync runs a non-streaming gateway chat and returns the full result.
func (c *Client) ChatViaGatewaySync(ctx context.Context, req *GatewayChatRequest) (*GatewayChatResult, error) {
	reqCopy := *req
	reqCopy.Stream = false
	ch, result, err := c.ChatViaGateway(ctx, &reqCopy)
	if err != nil {
		return nil, err
	}
	// Drain channel so the goroutine finishes aggregating.
	for range ch {
	}
	if result == nil {
		return nil, fmt.Errorf("empty gateway chat result")
	}
	if result.Text == "" && len(result.ToolCalls) == 0 {
		return nil, fmt.Errorf("timed out waiting for gateway reply")
	}
	return result, nil
}

func (c *Client) streamViaEvents(ctx context.Context, sess *SandboxSession, agentID string, startedAt time.Time) (<-chan ChatDelta, error) {
	es, err := c.OpenGatewayEvents(ctx, sess)
	if err != nil {
		return nil, err
	}
	out := make(chan ChatDelta, 16)
	go func() {
		defer close(out)
		defer es.Close()

		seenText := ""
		seenTools := map[string]string{} // id -> args so far
		idle := time.NewTimer(defaultGatewayTimeout)
		defer idle.Stop()

		for {
			type recvResult struct {
				ev  *GatewayEvent
				err error
			}
			ch := make(chan recvResult, 1)
			go func() {
				ev, err := es.Recv()
				ch <- recvResult{ev, err}
			}()

			var rr recvResult
			select {
			case <-ctx.Done():
				out <- ChatDelta{Done: true, FinishReason: "stop"}
				return
			case <-idle.C:
				out <- ChatDelta{Done: true, FinishReason: "stop"}
				return
			case rr = <-ch:
			}

			if rr.err != nil {
				out <- ChatDelta{Done: true, FinishReason: "stop"}
				return
			}
			ev := rr.ev
			if ev.Channel != "transcript" {
				continue
			}
			entries := extractEntriesFromTranscriptPayload(ev.Payload, agentID)
			if len(entries) == 0 {
				continue
			}
			idle.Reset(30 * time.Second)

			deltas, complete := diffTranscript(entries, startedAt, &seenText, seenTools)
			for _, d := range deltas {
				out <- d
			}
			if complete {
				out <- ChatDelta{Done: true, FinishReason: finishReasonFromSeen(seenTools)}
				return
			}
		}
	}()
	return out, nil
}

func (c *Client) streamViaPoll(ctx context.Context, sess *SandboxSession, agentID string, startedAt time.Time, every time.Duration) <-chan ChatDelta {
	out := make(chan ChatDelta, 16)
	go func() {
		defer close(out)
		seenText := ""
		seenTools := map[string]string{}
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				out <- ChatDelta{Done: true, FinishReason: finishReasonFromSeen(seenTools)}
				return
			case <-ticker.C:
				entries, err := c.GetAgentTranscriptTail(ctx, sess, agentID, 80)
				if err != nil {
					if ctx.Err() != nil {
						out <- ChatDelta{Done: true, FinishReason: "stop"}
						return
					}
					continue
				}
				deltas, complete := diffTranscript(entries, startedAt, &seenText, seenTools)
				for _, d := range deltas {
					out <- d
				}
				if complete {
					out <- ChatDelta{Done: true, FinishReason: finishReasonFromSeen(seenTools)}
					return
				}
			}
		}
	}()
	return out
}

func finishReasonFromSeen(tools map[string]string) string {
	if len(tools) > 0 {
		return "tool_calls"
	}
	return "stop"
}

type aggregatedReply struct {
	Text      string
	Reasoning string
	ToolCalls []OpenAIToolCall
}

func aggregateTranscriptSince(entries []TranscriptEntry, startedAt time.Time) aggregatedReply {
	startMs := startedAt.UnixMilli() - 1000
	var texts []string
	var tools []OpenAIToolCall
	toolIdx := map[string]int{}
	for _, e := range entries {
		if e.TimestampMs > 0 && e.TimestampMs < startMs {
			continue
		}
		switch e.Kind {
		case "send-message":
			if e.IsStreaming {
				continue
			}
			t := extractSendMessageText(e)
			if t != "" {
				texts = append(texts, t)
			}
			// Some replies embed tool_calls JSON in text.
			if embedded := parseEmbeddedToolCalls(t); len(embedded) > 0 {
				tools = append(tools, embedded...)
			}
		case "message":
			if e.IsStreaming {
				continue
			}
			if strings.EqualFold(e.Role, "assistant") || e.Role == "" {
				t := openAIContentToText(e.Content)
				if t == "" && e.Message != nil {
					t = e.Message.Content
				}
				if t != "" {
					texts = append(texts, t)
				}
			}
		case "tool-call":
			id := e.ToolCallID
			if id == "" {
				id = e.ID
			}
			if id == "" {
				id = fmt.Sprintf("call_%d", len(tools))
			}
			args := rawToString(e.Args)
			if args == "" {
				args = rawToString(e.Arguments)
			}
			if args == "" {
				args = "{}"
			}
			if idx, ok := toolIdx[id]; ok {
				tools[idx].Function.Arguments = args
				if e.Name != "" {
					tools[idx].Function.Name = e.Name
				}
			} else {
				toolIdx[id] = len(tools)
				tools = append(tools, OpenAIToolCall{
					ID:   id,
					Type: "function",
					Function: OpenAIFunctionCallDesc{
						Name:      e.Name,
						Arguments: args,
					},
				})
			}
		}
	}
	return aggregatedReply{
		Text:      strings.Join(texts, ""),
		ToolCalls: tools,
	}
}

func diffTranscript(entries []TranscriptEntry, startedAt time.Time, seenText *string, seenTools map[string]string) ([]ChatDelta, bool) {
	agg := aggregateTranscriptSince(entries, startedAt)
	var deltas []ChatDelta

	// Text delta
	if strings.HasPrefix(agg.Text, *seenText) {
		delta := strings.TrimPrefix(agg.Text, *seenText)
		if delta != "" {
			*seenText = agg.Text
			deltas = append(deltas, ChatDelta{Text: delta})
		}
	} else if agg.Text != "" && agg.Text != *seenText {
		// Non-prefix growth (rare) — emit whole new text as replacement delta.
		delta := agg.Text
		if *seenText != "" && strings.Contains(agg.Text, *seenText) {
			// Can't compute clean delta; emit suffix after last common.
			delta = agg.Text[len(*seenText):]
			if delta == agg.Text {
				delta = agg.Text
			}
		}
		*seenText = agg.Text
		if delta != "" {
			deltas = append(deltas, ChatDelta{Text: delta})
		}
	}

	// Tool deltas
	for i, tc := range agg.ToolCalls {
		prev := seenTools[tc.ID]
		if tc.Function.Arguments != prev || (tc.Function.Name != "" && prev == "" && tc.Function.Arguments == "") {
			argDelta := tc.Function.Arguments
			if strings.HasPrefix(tc.Function.Arguments, prev) {
				argDelta = strings.TrimPrefix(tc.Function.Arguments, prev)
			}
			seenTools[tc.ID] = tc.Function.Arguments
			deltas = append(deltas, ChatDelta{
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				ToolArgs:   argDelta,
				ToolIndex:  i,
			})
		}
	}

	complete := false
	// Complete when we have non-streaming assistant text or finished tool calls,
	// and no entry is still in-flight after start.
	inFlight := false
	startMs := startedAt.UnixMilli() - 1000
	for _, e := range entries {
		if e.TimestampMs > 0 && e.TimestampMs < startMs {
			continue
		}
		if e.Kind == "send-message" || e.Kind == "message" {
			if e.IsStreaming {
				inFlight = true
			}
		}
		if e.Kind == "tool-call" && (e.Status == "pending" || e.Status == "running") {
			inFlight = true
		}
	}
	if !inFlight && (*seenText != "" || len(seenTools) > 0) {
		// Require a short settle: at least one complete send-message after start.
		for _, e := range entries {
			if e.TimestampMs > 0 && e.TimestampMs < startMs {
				continue
			}
			if e.Kind == "send-message" && !e.IsStreaming && extractSendMessageText(e) != "" {
				complete = true
				break
			}
			if e.Kind == "tool-call" && e.Status != "" && e.Status != "pending" && e.Status != "running" {
				complete = true
				break
			}
			if e.Kind == "message" && !e.IsStreaming && strings.EqualFold(e.Role, "assistant") {
				complete = true
				break
			}
		}
	}
	return deltas, complete
}

func extractSendMessageText(e TranscriptEntry) string {
	if e.Message != nil {
		if e.Message.Content != "" {
			return e.Message.Content
		}
	}
	// Fallback: message may be raw on Content
	if len(e.Content) > 0 {
		var msg TranscriptMsg
		if json.Unmarshal(e.Content, &msg) == nil && msg.Content != "" {
			return msg.Content
		}
		return openAIContentToText(e.Content)
	}
	return ""
}

func parseEmbeddedToolCalls(text string) []OpenAIToolCall {
	text = strings.TrimSpace(text)
	if text == "" || !strings.Contains(text, "tool_calls") {
		return nil
	}
	// Try whole text as JSON.
	var wrap struct {
		ToolCalls []OpenAIToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(text), &wrap); err == nil && len(wrap.ToolCalls) > 0 {
		return wrap.ToolCalls
	}
	// Find first {...} block containing tool_calls.
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(text[start:end+1]), &wrap); err == nil && len(wrap.ToolCalls) > 0 {
			return wrap.ToolCalls
		}
	}
	return nil
}

func extractEntriesFromTranscriptPayload(payload json.RawMessage, agentID string) []TranscriptEntry {
	if len(payload) == 0 {
		return nil
	}
	// Common shapes: {entries:[...], agentId}, {agentId, entry}, raw array, snapshot with entries.
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(payload, &asMap); err == nil {
		if aidRaw, ok := asMap["agentId"]; ok {
			var aid string
			_ = json.Unmarshal(aidRaw, &aid)
			if aid != "" && agentID != "" && aid != agentID {
				// Still allow if activeAgentId matches
				if a2, ok := asMap["activeAgentId"]; ok {
					var active string
					_ = json.Unmarshal(a2, &active)
					if active != "" && active != agentID {
						return nil
					}
				} else {
					return nil
				}
			}
		}
		for _, key := range []string{"entries", "transcript", "rows"} {
			if raw, ok := asMap[key]; ok {
				return parseTranscriptEntries(raw)
			}
		}
		if raw, ok := asMap["entry"]; ok {
			var e TranscriptEntry
			if json.Unmarshal(raw, &e) == nil && e.Kind != "" {
				e.Raw = raw
				return []TranscriptEntry{e}
			}
			// entry may be nested under body
			return parseTranscriptEntries(raw)
		}
		// type=snapshot payloads often embed entries at top level
		if _, ok := asMap["kind"]; ok {
			var e TranscriptEntry
			if json.Unmarshal(payload, &e) == nil && e.Kind != "" {
				e.Raw = payload
				return []TranscriptEntry{e}
			}
		}
	}
	return parseTranscriptEntries(payload)
}

func parseTranscriptEntries(raw json.RawMessage) []TranscriptEntry {
	if len(raw) == 0 {
		return nil
	}
	// Array
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		out := make([]TranscriptEntry, 0, len(arr))
		for _, item := range arr {
			if e, ok := decodeTranscriptEntry(item); ok {
				out = append(out, e)
			}
		}
		return out
	}
	// Object with entries
	var wrap struct {
		Entries []json.RawMessage `json:"entries"`
		Page    struct {
			Entries []json.RawMessage `json:"entries"`
		} `json:"page"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil {
		src := wrap.Entries
		if len(src) == 0 {
			src = wrap.Page.Entries
		}
		out := make([]TranscriptEntry, 0, len(src))
		for _, item := range src {
			if e, ok := decodeTranscriptEntry(item); ok {
				out = append(out, e)
			}
		}
		return out
	}
	if e, ok := decodeTranscriptEntry(raw); ok {
		return []TranscriptEntry{e}
	}
	return nil
}

func decodeTranscriptEntry(raw json.RawMessage) (TranscriptEntry, bool) {
	var e TranscriptEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return TranscriptEntry{}, false
	}
	e.Raw = raw
	if e.Kind == "" {
		return TranscriptEntry{}, false
	}
	// Normalize message from nested structures.
	if e.Message == nil && e.Kind == "send-message" {
		var loose struct {
			Message json.RawMessage `json:"message"`
		}
		if json.Unmarshal(raw, &loose) == nil && len(loose.Message) > 0 {
			var msg TranscriptMsg
			if json.Unmarshal(loose.Message, &msg) == nil {
				e.Message = &msg
			}
		}
	}
	return e, true
}

type agentRecord struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func unwrapAgents(raw json.RawMessage) []agentRecord {
	var arr []agentRecord
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var wrap struct {
		Agents []agentRecord `json:"agents"`
	}
	if json.Unmarshal(raw, &wrap) == nil {
		return wrap.Agents
	}
	return nil
}

func unwrapAgentID(raw json.RawMessage) string {
	var wrap struct {
		ID    string `json:"id"`
		Agent *struct {
			ID string `json:"id"`
		} `json:"agent"`
	}
	if json.Unmarshal(raw, &wrap) == nil {
		if wrap.Agent != nil && wrap.Agent.ID != "" {
			return wrap.Agent.ID
		}
		return wrap.ID
	}
	return ""
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if t != "" {
					return t
				}
			}
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func rawToString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

func isTransientGatewayErr(err error) bool {
	if err == nil {
		return false
	}
	if up, ok := err.(*UpstreamError); ok {
		if up.StatusCode == http.StatusUnauthorized ||
			up.StatusCode == http.StatusRequestTimeout ||
			up.StatusCode == http.StatusTooManyRequests ||
			up.StatusCode >= 500 {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "temporarily")
}
