package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCreateAccountRequestRejectsRetiredBedrockType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", bytes.NewBufferString(`{
		"name":"retired-bedrock",
		"platform":"anthropic",
		"type":"bedrock",
		"credentials":{"api_key":"not-written"}
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	var request CreateAccountRequest
	require.Error(t, ctx.ShouldBindJSON(&request))
}
