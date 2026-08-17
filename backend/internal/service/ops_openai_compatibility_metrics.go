package service

import "time"

// OpsOpenAICompatibilityFallbackMetrics is a process-local snapshot. Operators
// must query every API instance before retiring a compatibility fallback.
type OpsOpenAICompatibilityFallbackMetrics struct {
	ObservedAt             time.Time                                  `json:"observed_at"`
	Scope                  string                                     `json:"scope"`
	ReadOldFallbackEnabled bool                                       `json:"read_old_fallback_enabled"`
	DualWriteOldEnabled    bool                                       `json:"dual_write_old_enabled"`
	LegacyTTLMaxSeconds    int                                        `json:"legacy_ttl_max_seconds"`
	Metrics                OpenAICompatibilityFallbackMetricsSnapshot `json:"metrics"`
}

// GetOpenAICompatibilityFallbackMetrics exposes the existing monotonic
// compatibility counters without changing or resetting gateway state.
func (s *OpsService) GetOpenAICompatibilityFallbackMetrics() *OpsOpenAICompatibilityFallbackMetrics {
	readOldFallbackEnabled := true
	dualWriteOldEnabled := true

	switch {
	case s != nil && s.openAIGatewayService != nil:
		readOldFallbackEnabled = s.openAIGatewayService.openAISessionHashReadOldFallbackEnabled()
		dualWriteOldEnabled = s.openAIGatewayService.openAISessionHashDualWriteOldEnabled()
	case s != nil && s.cfg != nil:
		readOldFallbackEnabled = s.cfg.Gateway.OpenAIWS.SessionHashReadOldFallback
		dualWriteOldEnabled = s.cfg.Gateway.OpenAIWS.SessionHashDualWriteOld
	}

	return &OpsOpenAICompatibilityFallbackMetrics{
		ObservedAt:             time.Now().UTC(),
		Scope:                  "process",
		ReadOldFallbackEnabled: readOldFallbackEnabled,
		DualWriteOldEnabled:    dualWriteOldEnabled,
		LegacyTTLMaxSeconds:    int(openAIStickyLegacyTTLMax / time.Second),
		Metrics:                SnapshotOpenAICompatibilityFallbackMetrics(),
	}
}
