package admin

import "github.com/Wei-Shaw/sub2api/internal/service"

func (h *AccountHandler) SetIdempotencyCoordinator(c *service.IdempotencyCoordinator) {
	if h != nil {
		h.idempotency = c
	}
}

func (h *UserHandler) SetIdempotencyCoordinator(c *service.IdempotencyCoordinator) {
	if h != nil {
		h.idempotency = c
	}
}

func (h *ProxyHandler) SetIdempotencyCoordinator(c *service.IdempotencyCoordinator) {
	if h != nil {
		h.idempotency = c
	}
}

func (h *RedeemHandler) SetIdempotencyCoordinator(c *service.IdempotencyCoordinator) {
	if h != nil {
		h.idempotency = c
	}
}

func (h *UsageHandler) SetIdempotencyCoordinator(c *service.IdempotencyCoordinator) {
	if h != nil {
		h.idempotency = c
	}
}

func (h *SubscriptionHandler) SetIdempotencyCoordinator(c *service.IdempotencyCoordinator) {
	if h != nil {
		h.idempotency = c
	}
}
