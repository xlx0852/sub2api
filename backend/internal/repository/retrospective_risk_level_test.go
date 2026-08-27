package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestRetrospectiveRiskLevelRequiresRepeatedFinding(t *testing.T) {
	require.Equal(t, domain.RiskLevelWatch, retrospectiveRiskLevel(0, 0.99))
	require.Equal(t, domain.RiskLevelHigh, retrospectiveRiskLevel(1, 0.85))
	require.Equal(t, domain.RiskLevelCritical, retrospectiveRiskLevel(2, 0.95))
}
