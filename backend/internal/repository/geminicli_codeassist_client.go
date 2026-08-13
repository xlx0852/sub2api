package repository

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/ports"
)

type geminiCliCodeAssistClient struct{}

func NewGeminiCliCodeAssistClient() ports.GeminiCliCodeAssistClient {
	return &geminiCliCodeAssistClient{}
}

func (c *geminiCliCodeAssistClient) LoadCodeAssist(ctx context.Context, accessToken, proxyURL string, req any) (any, error) {
	return nil, fmt.Errorf("gemini code assist retired")
}

func (c *geminiCliCodeAssistClient) OnboardUser(ctx context.Context, accessToken, proxyURL string, req any) (any, error) {
	return nil, fmt.Errorf("gemini code assist retired")
}
