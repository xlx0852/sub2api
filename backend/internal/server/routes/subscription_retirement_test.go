package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionRoutesAreNotRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{}}
	allow := func(c *gin.Context) { c.Next() }

	RegisterUserRoutes(v1, handlers, allow, nil)
	RegisterAdminRoutes(v1, handlers, allow, nil)
	RegisterPaymentRoutes(v1, &handler.PaymentHandler{}, &handler.PaymentWebhookHandler{}, &adminhandler.PaymentHandler{}, allow, allow, nil)

	registered := make(map[string]struct{})
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	removed := []string{
		http.MethodGet + " /api/v1/subscriptions",
		http.MethodGet + " /api/v1/subscriptions/active",
		http.MethodGet + " /api/v1/admin/subscriptions",
		http.MethodPost + " /api/v1/admin/subscriptions/assign",
		http.MethodGet + " /api/v1/admin/groups/:id/subscriptions",
		http.MethodGet + " /api/v1/admin/users/:id/subscriptions",
		http.MethodGet + " /api/v1/payment/plans",
		http.MethodGet + " /api/v1/admin/payment/plans",
	}
	for _, route := range removed {
		_, ok := registered[route]
		require.False(t, ok, "%s must stay retired", route)
	}

	_, profitCyclesOK := registered[http.MethodGet+" /api/v1/admin/profit/configs/:account_id/cycles"]
	require.True(t, profitCyclesOK, "account procurement subscription cycles must remain available")
}
