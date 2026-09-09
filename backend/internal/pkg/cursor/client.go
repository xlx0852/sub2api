package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	cursorpb "github.com/Wei-Shaw/sub2api/internal/pkg/cursor/proto"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	machineID  string
	token      string
}

func NewClient(token string, machineID string, httpClient *http.Client, baseURL string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	if baseURL == "" {
		baseURL = DefaultBackendURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if machineID == "" {
		machineID = uuid.NewString()
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
		machineID:  machineID,
		token:      token,
	}
}

func (c *Client) SetToken(token string) {
	c.token = token
}

func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set(HeaderCursorChecksum, CreateCursorChecksum(c.machineID, time.Now()))
	req.Header.Set(HeaderCursorClientType, DefaultClientType)
	req.Header.Set(HeaderCursorClientVersion, DefaultClientVersion)
	req.Header.Set(HeaderSandBoxNamespace, DefaultBoxNamespace)
	req.Header.Set(HeaderGhostMode, "false")
	req.Header.Set(HeaderRequestID, uuid.NewString())
}

// UpstreamError represents an HTTP or Connect-RPC error returned by Cursor upstream.
type UpstreamError struct {
	StatusCode int
	Body       string
	Headers    http.Header
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("cursor upstream error (status %d): %s", e.StatusCode, e.Body)
}

// ConnectCodeToHTTPStatus maps a Connect-RPC error code to an HTTP status code.
func ConnectCodeToHTTPStatus(code string) int {
	switch strings.ToLower(code) {
	case "invalid_argument":
		return http.StatusBadRequest
	case "unauthenticated":
		return http.StatusUnauthorized
	case "permission_denied":
		return http.StatusForbidden
	case "not_found":
		return http.StatusNotFound
	case "resource_exhausted":
		return http.StatusTooManyRequests
	case "internal":
		return http.StatusInternalServerError
	case "unavailable":
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// StreamReader reads streamed InferenceStreamResponse messages from Connect stream.
type StreamReader struct {
	body   io.ReadCloser
	cancel context.CancelFunc
}

func (s *StreamReader) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.body != nil {
		return s.body.Close()
	}
	return nil
}

// Recv reads the next message from the stream. Returns io.EOF when done.
func (s *StreamReader) Recv() (*cursorpb.InferenceStreamResponse, error) {
	for {
		flag, payload, err := ReadEnvelope(s.body)
		if err != nil {
			return nil, err
		}
		if (flag & ConnectFlagEndStream) != 0 {
			// Trailer frame (JSON metadata / error)
			if len(payload) > 0 {
				var trailer struct {
					Error *struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if jsonErr := json.Unmarshal(payload, &trailer); jsonErr == nil && trailer.Error != nil {
					return nil, &UpstreamError{
						StatusCode: ConnectCodeToHTTPStatus(trailer.Error.Code),
						Body:       fmt.Sprintf("[%s] %s", trailer.Error.Code, trailer.Error.Message),
					}
				}
			}
			return nil, io.EOF
		}
		if flag == ConnectFlagData {
			resp := &cursorpb.InferenceStreamResponse{}
			if err := proto.Unmarshal(payload, resp); err != nil {
				return nil, fmt.Errorf("unmarshal InferenceStreamResponse: %w", err)
			}
			return resp, nil
		}
	}
}

// StreamInference opens a Connect streaming call to aiserver.v1.InferenceService/Stream.
func (c *Client) StreamInference(ctx context.Context, req *cursorpb.InferenceStreamRequest) (*StreamReader, error) {
	if req.ConversationId == nil || *req.ConversationId == "" {
		cid := uuid.NewString()
		req.ConversationId = &cid
	}
	if req.InvocationId == nil || *req.InvocationId == "" {
		iid := uuid.NewString()
		req.InvocationId = &iid
	}

	ctx, cancel := context.WithCancel(ctx)
	envelope, err := EncodeEnvelope(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("encode request: %w", err)
	}

	url := fmt.Sprintf("%s/aiserver.v1.InferenceService/Stream", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(envelope))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/connect+proto")
	httpReq.Header.Set("Connect-Protocol-Version", "1")
	c.applyHeaders(httpReq)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("execute connect stream: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		cancel()
		return nil, &UpstreamError{
			StatusCode: httpResp.StatusCode,
			Body:       string(body),
			Headers:    httpResp.Header.Clone(),
		}
	}

	return &StreamReader{
		body:   httpResp.Body,
		cancel: cancel,
	}, nil
}

// AvailableModels fetches account-specific IDs using Cursor's unary Connect RPC.
func (c *Client) AvailableModels(ctx context.Context) (*cursorpb.AvailableModelsResponse, error) {
	body, err := proto.Marshal(&cursorpb.AvailableModelsRequest{Scope: 1, IncludeLongContextModels: true})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/aiserver.v1.AiService/AvailableModels", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: string(data), Headers: resp.Header.Clone()}
	}
	result := &cursorpb.AvailableModelsResponse{}
	if err := proto.Unmarshal(data, result); err != nil {
		return nil, err
	}
	return result, nil
}
