package scheduling

type StickyDecision string

const (
	StickyNoSession         StickyDecision = "no_session"
	StickyNoBinding         StickyDecision = "no_binding"
	StickyExcluded          StickyDecision = "excluded"
	StickyCandidateMissing  StickyDecision = "candidate_missing"
	StickyCandidateMismatch StickyDecision = "candidate_mismatch"
	StickyClear             StickyDecision = "clear"
	StickyReady             StickyDecision = "ready"
)

type StickyBinding struct {
	AccountID int64
	Decision  StickyDecision
}

func (binding StickyBinding) Ready() bool {
	return binding.Decision == StickyReady
}

// EvaluateStickyBinding classifies only the behavior-equivalent session,
// binding and exclusion checks. It performs no cache or scheduling effects.
func EvaluateStickyBinding(sessionKey string, accountID int64, excludedIDs map[int64]struct{}) StickyBinding {
	switch {
	case sessionKey == "":
		return StickyBinding{AccountID: accountID, Decision: StickyNoSession}
	case accountID <= 0:
		return StickyBinding{AccountID: accountID, Decision: StickyNoBinding}
	case IsExcluded(excludedIDs, accountID):
		return StickyBinding{AccountID: accountID, Decision: StickyExcluded}
	default:
		return StickyBinding{AccountID: accountID, Decision: StickyReady}
	}
}

// EvaluateStickyCandidate classifies lookup/clear outcomes after a binding is
// ready. Strategy-specific gates and clear side effects remain with callers.
func EvaluateStickyCandidate(binding StickyBinding, candidateID int64, shouldClear bool) StickyDecision {
	if !binding.Ready() {
		return binding.Decision
	}
	if candidateID <= 0 {
		return StickyCandidateMissing
	}
	if candidateID != binding.AccountID {
		return StickyCandidateMismatch
	}
	if shouldClear {
		return StickyClear
	}
	return StickyReady
}
