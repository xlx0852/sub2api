package scheduling

import (
	"math"
	"sync"
	"time"
)

type RuntimeFeedback struct {
	AccountID          int64
	ObservedAt         time.Time
	LoadRate           *float64
	QueueDepth         *float64
	ErrorSample        *float64
	TTFTMillis         *float64
	QuotaHeadroom      *float64
	WindowHeadroom     *float64
	CompactErrorSample *float64
	CompactMillis      *float64
}

type RuntimeStatsSnapshot struct {
	AccountID      int64
	LoadRate       Signal
	QueueDepth     Signal
	ErrorRate      Signal
	TTFTMillis     Signal
	QuotaHeadroom  Signal
	WindowHeadroom Signal
	CompactError   Signal
	CompactMillis  Signal
}

type runtimeStatEntry struct {
	loadRate          float64
	hasLoadRate       bool
	queueDepth        float64
	hasQueueDepth     bool
	errorRate         float64
	hasErrorRate      bool
	ttftMillis        float64
	hasTTFT           bool
	quotaHeadroom     float64
	hasQuotaHeadroom  bool
	windowHeadroom    float64
	hasWindowHeadroom bool
	compactError      float64
	hasCompactError   bool
	compactMillis     float64
	hasCompactMillis  bool
	updatedAt         time.Time
}

type RuntimeStatsStore struct {
	mu       sync.Mutex
	entries  map[int64]runtimeStatEntry
	ttl      time.Duration
	capacity int
	alpha    float64
	now      func() time.Time
}

func NewRuntimeStatsStore(ttl time.Duration, capacity int, alpha float64, now func() time.Time) *RuntimeStatsStore {
	if capacity <= 0 {
		capacity = 1
	}
	if alpha <= 0 || alpha > 1 || math.IsNaN(alpha) {
		alpha = 0.2
	}
	if now == nil {
		now = time.Now
	}
	return &RuntimeStatsStore{
		entries:  make(map[int64]runtimeStatEntry),
		ttl:      ttl,
		capacity: capacity,
		alpha:    alpha,
		now:      now,
	}
}

func (s *RuntimeStatsStore) Report(feedback RuntimeFeedback) bool {
	if s == nil || feedback.AccountID <= 0 {
		return false
	}
	observedAt := feedback.ObservedAt
	if observedAt.IsZero() {
		observedAt = s.now()
	}
	if !validNonNegativeFeedbackValue(feedback.LoadRate) || !validNonNegativeFeedbackValue(feedback.QueueDepth) ||
		!validFeedbackValue(feedback.ErrorSample) || !validPositiveFeedbackValue(feedback.TTFTMillis) ||
		!validFeedbackValue(feedback.QuotaHeadroom) || !validFeedbackValue(feedback.WindowHeadroom) ||
		!validFeedbackValue(feedback.CompactErrorSample) || !validPositiveFeedbackValue(feedback.CompactMillis) {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(observedAt)
	entry, exists := s.entries[feedback.AccountID]
	if !exists && len(s.entries) >= s.capacity {
		s.evictOldestLocked()
	}
	if feedback.LoadRate != nil {
		entry.loadRate, entry.hasLoadRate = updateRuntimeEWMA(entry.loadRate, entry.hasLoadRate, *feedback.LoadRate, s.alpha), true
	}
	if feedback.QueueDepth != nil {
		entry.queueDepth, entry.hasQueueDepth = updateRuntimeEWMA(entry.queueDepth, entry.hasQueueDepth, *feedback.QueueDepth, s.alpha), true
	}
	if feedback.ErrorSample != nil {
		entry.errorRate, entry.hasErrorRate = updateRuntimeEWMA(entry.errorRate, entry.hasErrorRate, clampUnit(*feedback.ErrorSample), s.alpha), true
	}
	if feedback.TTFTMillis != nil {
		entry.ttftMillis, entry.hasTTFT = updateRuntimeEWMA(entry.ttftMillis, entry.hasTTFT, *feedback.TTFTMillis, s.alpha), true
	}
	if feedback.QuotaHeadroom != nil {
		entry.quotaHeadroom, entry.hasQuotaHeadroom = updateRuntimeEWMA(entry.quotaHeadroom, entry.hasQuotaHeadroom, clampUnit(*feedback.QuotaHeadroom), s.alpha), true
	}
	if feedback.WindowHeadroom != nil {
		entry.windowHeadroom, entry.hasWindowHeadroom = updateRuntimeEWMA(entry.windowHeadroom, entry.hasWindowHeadroom, clampUnit(*feedback.WindowHeadroom), s.alpha), true
	}
	if feedback.CompactErrorSample != nil {
		entry.compactError, entry.hasCompactError = updateRuntimeEWMA(entry.compactError, entry.hasCompactError, clampUnit(*feedback.CompactErrorSample), s.alpha), true
	}
	if feedback.CompactMillis != nil {
		entry.compactMillis, entry.hasCompactMillis = updateRuntimeEWMA(entry.compactMillis, entry.hasCompactMillis, *feedback.CompactMillis, s.alpha), true
	}
	entry.updatedAt = observedAt
	s.entries[feedback.AccountID] = entry
	return true
}

func (s *RuntimeStatsStore) Snapshot(accountID int64, now time.Time) RuntimeStatsSnapshot {
	result := RuntimeStatsSnapshot{AccountID: accountID}
	if s == nil || accountID <= 0 {
		return result
	}
	if now.IsZero() {
		now = s.now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[accountID]
	if !ok {
		return result
	}
	if s.expired(entry, now) {
		delete(s.entries, accountID)
		return result
	}
	if entry.hasLoadRate {
		result.LoadRate = AvailableSignal(entry.loadRate, entry.updatedAt)
	}
	if entry.hasQueueDepth {
		result.QueueDepth = AvailableSignal(entry.queueDepth, entry.updatedAt)
	}
	if entry.hasErrorRate {
		result.ErrorRate = AvailableSignal(entry.errorRate, entry.updatedAt)
	}
	if entry.hasTTFT {
		result.TTFTMillis = AvailableSignal(entry.ttftMillis, entry.updatedAt)
	}
	if entry.hasQuotaHeadroom {
		result.QuotaHeadroom = AvailableSignal(entry.quotaHeadroom, entry.updatedAt)
	}
	if entry.hasWindowHeadroom {
		result.WindowHeadroom = AvailableSignal(entry.windowHeadroom, entry.updatedAt)
	}
	if entry.hasCompactError {
		result.CompactError = AvailableSignal(entry.compactError, entry.updatedAt)
	}
	if entry.hasCompactMillis {
		result.CompactMillis = AvailableSignal(entry.compactMillis, entry.updatedAt)
	}
	return result
}

func (s *RuntimeStatsStore) Cleanup(now time.Time) int {
	if s == nil {
		return 0
	}
	if now.IsZero() {
		now = s.now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removeExpiredLocked(now)
}

func (s *RuntimeStatsStore) Remove(accountID int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.entries, accountID)
	s.mu.Unlock()
}

func (s *RuntimeStatsStore) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func (s *RuntimeStatsStore) expired(entry runtimeStatEntry, now time.Time) bool {
	return s.ttl > 0 && !entry.updatedAt.IsZero() && now.Sub(entry.updatedAt) > s.ttl
}

func (s *RuntimeStatsStore) removeExpiredLocked(now time.Time) int {
	removed := 0
	for accountID, entry := range s.entries {
		if s.expired(entry, now) {
			delete(s.entries, accountID)
			removed++
		}
	}
	return removed
}

func (s *RuntimeStatsStore) evictOldestLocked() {
	var oldestID int64
	var oldestAt time.Time
	for accountID, entry := range s.entries {
		if oldestID == 0 || entry.updatedAt.Before(oldestAt) || (entry.updatedAt.Equal(oldestAt) && accountID < oldestID) {
			oldestID, oldestAt = accountID, entry.updatedAt
		}
	}
	if oldestID > 0 {
		delete(s.entries, oldestID)
	}
}

func updateRuntimeEWMA(previous float64, hasPrevious bool, sample, alpha float64) float64 {
	if !hasPrevious {
		return sample
	}
	return alpha*sample + (1-alpha)*previous
}

func validFeedbackValue(value *float64) bool {
	return value == nil || (!math.IsNaN(*value) && !math.IsInf(*value, 0))
}

func validPositiveFeedbackValue(value *float64) bool {
	return validFeedbackValue(value) && (value == nil || *value > 0)
}

func validNonNegativeFeedbackValue(value *float64) bool {
	return validFeedbackValue(value) && (value == nil || *value >= 0)
}
