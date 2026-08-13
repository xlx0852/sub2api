package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

func normalizeAntigravityModelName(model string) string {
	return domain.NormalizeAntigravityModelName(model)
}

func resolveAntigravityModelKey(requestedModel string) string {
	return domain.NormalizeAntigravityModelName(requestedModel)
}
