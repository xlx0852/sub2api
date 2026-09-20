package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayService_BuildAnthropicVertexServiceAccountRequestIsRetired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	account := &Account{
		ID:       301,
		Platform: PlatformAnthropic,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"project_id": "vertex-proj",
			"location":   "us-east5",
		},
	}
	body := []byte(`{"model":"claude-sonnet-4-5","stream":false,"max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`)

	svc := &GatewayService{}
	_, _, err := svc.buildUpstreamRequest(
		context.Background(),
		c,
		account,
		body,
		"vertex-token",
		"service_account",
		"claude-sonnet-4-5@20250929",
		false,
		false,
	)
	require.ErrorIs(t, err, ErrRetiredVertexServiceAccount)
}
