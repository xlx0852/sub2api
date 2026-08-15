package scheduling

import "time"

// Signal 是调度评分输入的单个可选观测值。
// Available=false 与 Value=0 严格区分，避免把未知样本当成有利零值。
type Signal struct {
	Value      float64
	Available  bool
	ObservedAt time.Time
}

func AvailableSignal(value float64, observedAt time.Time) Signal {
	return Signal{Value: value, Available: true, ObservedAt: observedAt}
}

func UnavailableSignal() Signal {
	return Signal{}
}

func (s Signal) Fresh(now time.Time, maxAge time.Duration) bool {
	if !s.Available {
		return false
	}
	if maxAge <= 0 || s.ObservedAt.IsZero() {
		return true
	}
	age := now.Sub(s.ObservedAt)
	return age >= 0 && age <= maxAge
}

type CandidateSignals struct {
	LoadRate       Signal
	QueueDepth     Signal
	ErrorRate      Signal
	TTFTMillis     Signal
	QuotaHeadroom  Signal
	WindowHeadroom Signal
	ResetSeconds   Signal
	CompactError   Signal
	CompactMillis  Signal
}

type AffinitySignals struct {
	PreviousResponse bool
	SessionSticky    bool
	CredentialClass  string
	Generation       uint64
}

// CandidateSnapshot 是进入共享 scorer 的不可变值快照。
// 构造后调用方不得复用或修改 Capabilities 的底层切片。
type CandidateSnapshot struct {
	AccountID       int64
	Platform        string
	Priority        int
	CredentialClass string
	Capabilities    []string
	Signals         CandidateSignals
	Affinity        AffinitySignals
}

func NewCandidateSnapshot(snapshot CandidateSnapshot) CandidateSnapshot {
	snapshot.Capabilities = append([]string(nil), snapshot.Capabilities...)
	return snapshot
}

type ScoreWeights struct {
	Priority         float64
	Load             float64
	Queue            float64
	ErrorRate        float64
	TTFT             float64
	Reset            float64
	QuotaHeadroom    float64
	CompactError     float64
	CompactLatency   float64
	PreviousResponse float64
	SessionSticky    float64
}

type ScoreBreakdown struct {
	Priority         float64
	Load             float64
	Queue            float64
	ErrorRate        float64
	TTFT             float64
	Reset            float64
	QuotaHeadroom    float64
	CompactError     float64
	CompactLatency   float64
	PreviousResponse float64
	SessionSticky    float64
	Total            float64
}

func (b ScoreBreakdown) ComponentTotal() float64 {
	return b.Priority + b.Load + b.Queue + b.ErrorRate + b.TTFT + b.Reset +
		b.QuotaHeadroom + b.CompactError + b.CompactLatency +
		b.PreviousResponse + b.SessionSticky
}

type AffinityKind string

const (
	AffinityNone             AffinityKind = "none"
	AffinityPreviousResponse AffinityKind = "previous_response"
	AffinitySession          AffinityKind = "session"
)

type AffinityDecision struct {
	Kind            AffinityKind
	AccountID       int64
	Valid           bool
	Weighted        bool
	Escaped         bool
	Reason          string
	CredentialClass string
	Generation      uint64
}

func NoAffinityDecision() AffinityDecision {
	return AffinityDecision{Kind: AffinityNone}
}
