package service

import (
	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
	"time"
)

// simpleAccountOrder is the shared adapter for every provider. Unknown load is
// explicitly unavailable, never interpreted as a measured empty account.
func simpleAccountOrder(accounts []*Account, loads map[int64]*AccountLoadInfo, latency scheduling.LatencyPreference) []*Account {
	snapshots := make([]scheduling.CandidateSnapshot, 0, len(accounts))
	byID := make(map[int64]*Account, len(accounts))
	for _, a := range accounts {
		if a == nil {
			continue
		}
		c := scheduling.CandidateSnapshot{AccountID: a.ID, Priority: a.Priority}
		if load := loads[a.ID]; load != nil {
			c.Signals.LoadRate = scheduling.AvailableSignal(float64(load.LoadRate), time.Time{})
		}
		snapshots = append(snapshots, c)
		byID[a.ID] = a
	}
	rng := scheduling.NewSelectionRNG(uint64(time.Now().UnixNano()))
	ranked := scheduling.OrderByPriorityLoadAndLatency(snapshots, &rng, latency)
	result := make([]*Account, 0, len(ranked))
	for _, c := range ranked {
		result = append(result, byID[c.AccountID])
	}
	return result
}

func preferSimpleAccount(accounts []*Account, id int64) []*Account {
	for i, a := range accounts {
		if a.ID == id {
			copy(accounts[1:i+1], accounts[:i])
			accounts[0] = a
			break
		}
	}
	return accounts
}
