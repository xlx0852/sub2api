package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestUsageLogInsertModesExpandOneAuthoritativeColumnList(t *testing.T) {
	log := &domain.UsageLog{
		UserID: 1, APIKeyID: 2, AccountID: 3, RequestID: "req-columns", Model: "gpt-5",
		CreatedAt: time.Now().UTC(),
	}
	prepared := prepareUsageLogInsert(log)
	key := usageLogBatchKey(log.RequestID, log.APIKeyID)
	batchQuery, _ := buildUsageLogBatchInsertQuery([]string{key}, map[string]usageLogInsertPrepared{key: prepared})
	bestEffortQuery, _ := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})

	for name, query := range map[string]string{"batch": batchQuery, "best_effort": bestEffortQuery} {
		t.Run(name, func(t *testing.T) {
			require.NotContains(t, query, usageLogInsertColumnsMarker)
			require.Contains(t, query, "first_output_stages")
			require.GreaterOrEqual(t, strings.Count(query, "created_at"), 2)
		})
	}
}
