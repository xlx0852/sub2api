package scheduling

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestComparePriorityLRU(t *testing.T) {
	older := time.Now().Add(-time.Hour)
	newer := time.Now()

	tests := []struct {
		name  string
		left  *domain.Account
		right *domain.Account
		want  int
	}{
		{name: "lower priority", left: &domain.Account{Priority: 1}, right: &domain.Account{Priority: 2}, want: -1},
		{name: "never used", left: &domain.Account{Priority: 1}, right: &domain.Account{Priority: 1, LastUsedAt: &older}, want: -1},
		{name: "older last used", left: &domain.Account{Priority: 1, LastUsedAt: &older}, right: &domain.Account{Priority: 1, LastUsedAt: &newer}, want: -1},
		{name: "same keys", left: &domain.Account{Priority: 1, LastUsedAt: &older}, right: &domain.Account{Priority: 1, LastUsedAt: &older}, want: 0},
		{name: "nil sorts last", left: nil, right: &domain.Account{}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ComparePriorityLRU(tt.left, tt.right))
		})
	}
}
