package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type performanceRepoStub struct {
	opsRepoMock
	captured *OpsPerformanceFilter
}

func (s *performanceRepoStub) GetPerformanceWindow(_ context.Context, filter *OpsPerformanceFilter) (*OpsPerformanceWindow, error) {
	s.captured = filter
	return &OpsPerformanceWindow{}, nil
}

func TestOpsServiceGetPerformanceWindowDefaultsPointLimit(t *testing.T) {
	repo := &performanceRepoStub{}
	svc := &OpsService{opsRepo: repo}
	_, err := svc.GetPerformanceWindow(context.Background(), &OpsPerformanceFilter{StartTime: time.Now().Add(-time.Hour), EndTime: time.Now()})
	require.NoError(t, err)
	require.Equal(t, 500, repo.captured.PointLimit)
}

func TestOpsServiceGetPerformanceWindowRejectsOversizedPointLimit(t *testing.T) {
	svc := &OpsService{opsRepo: &performanceRepoStub{}}
	_, err := svc.GetPerformanceWindow(context.Background(), &OpsPerformanceFilter{
		StartTime: time.Now().Add(-time.Hour), EndTime: time.Now(), PointLimit: 2001,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "OPS_POINT_LIMIT_INVALID")
}
