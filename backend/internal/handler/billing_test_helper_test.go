package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type billingEligibleUserRepository struct {
	service.UserRepository
}

func (billingEligibleUserRepository) GetByID(_ context.Context, id int64) (*service.User, error) {
	return &service.User{ID: id, Status: service.StatusActive, Balance: 1_000_000}, nil
}

func (billingEligibleUserRepository) DeductBalance(context.Context, int64, float64) error {
	return nil
}

func newBillingEligibleCache(t testing.TB, cfg *config.Config) *service.BillingCacheService {
	t.Helper()
	svc := service.NewBillingCacheService(nil, billingEligibleUserRepository{}, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)
	return svc
}
