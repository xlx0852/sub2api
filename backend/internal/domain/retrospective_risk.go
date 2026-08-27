package domain

import "time"

const (
	RiskLevelNormal   = "normal"
	RiskLevelWatch    = "watch"
	RiskLevelHigh     = "high"
	RiskLevelCritical = "critical"
)

type RetrospectiveEvidence struct {
	ID              string     `json:"id"`
	UserID          int64      `json:"user_id"`
	UserEmail       string     `json:"user_email,omitempty"`
	APIKeyID        *int64     `json:"api_key_id,omitempty"`
	RequestID       string     `json:"request_id"`
	Endpoint        string     `json:"endpoint"`
	Provider        string     `json:"provider"`
	Model           string     `json:"model"`
	ContentHash     string     `json:"content_hash"`
	CaptureDay      time.Time  `json:"capture_day"`
	CaptureReason   string     `json:"capture_reason"`
	ObjectKey       string     `json:"object_key"`
	KeyVersion      string     `json:"key_version"`
	Status          string     `json:"status"`
	PlaintextBytes  int64      `json:"plaintext_bytes"`
	StoredBytes     int64      `json:"stored_bytes"`
	Truncated       bool       `json:"truncated"`
	RedactedExcerpt string     `json:"redacted_excerpt"`
	ReviewAttempts  int        `json:"review_attempts"`
	LastError       string     `json:"last_error"`
	CapturedAt      time.Time  `json:"captured_at"`
	StoredAt        *time.Time `json:"stored_at,omitempty"`
	ClaimedAt       *time.Time `json:"claimed_at,omitempty"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
	ExpiresAt       time.Time  `json:"expires_at"`
}

type RetrospectiveReviewRun struct {
	ID            int64      `json:"id"`
	RunKey        string     `json:"run_key"`
	ScheduledDay  time.Time  `json:"scheduled_day"`
	Status        string     `json:"status"`
	Manual        bool       `json:"manual"`
	PolicyVersion string     `json:"policy_version"`
	ClaimedCount  int64      `json:"claimed_count"`
	ReviewedCount int64      `json:"reviewed_count"`
	FlaggedCount  int64      `json:"flagged_count"`
	FailedCount   int64      `json:"failed_count"`
	InputTokens   int64      `json:"input_tokens"`
	CostUnits     float64    `json:"cost_units"`
	Error         string     `json:"error"`
	StartedAt     time.Time  `json:"started_at"`
	HeartbeatAt   time.Time  `json:"heartbeat_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

type RetrospectiveReviewResult struct {
	Flagged         bool
	HighestCategory string
	HighestScore    float64
	ReasonCodes     []string
	RedactedSummary string
	InputTokens     int64
	CostUnits       float64
}

type UserRiskProfile struct {
	UserID       int64      `json:"user_id"`
	Email        string     `json:"email"`
	Username     string     `json:"username"`
	Level        string     `json:"level"`
	Score        float64    `json:"score"`
	Confidence   float64    `json:"confidence"`
	ReasonCodes  []string   `json:"reason_codes"`
	Provenance   string     `json:"provenance"`
	ReviewStatus string     `json:"review_status"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	LastEventAt  *time.Time `json:"last_event_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type RiskEvent struct {
	ID            int64     `json:"id"`
	EventKey      string    `json:"event_key"`
	UserID        int64     `json:"user_id"`
	EvidenceID    string    `json:"evidence_id,omitempty"`
	ReviewItemID  *int64    `json:"review_item_id,omitempty"`
	EventType     string    `json:"event_type"`
	PreviousLevel string    `json:"previous_level"`
	NewLevel      string    `json:"new_level"`
	Confidence    float64   `json:"confidence"`
	ReasonCodes   []string  `json:"reason_codes"`
	Source        string    `json:"source"`
	ActorAdminID  *int64    `json:"actor_admin_id,omitempty"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
}

type RetrospectiveEvidenceCase struct {
	RetrospectiveEvidence
	ReviewStatus    string   `json:"review_status,omitempty"`
	Flagged         bool     `json:"flagged"`
	HighestCategory string   `json:"highest_category,omitempty"`
	HighestScore    float64  `json:"highest_score"`
	ReasonCodes     []string `json:"reason_codes"`
	RedactedSummary string   `json:"redacted_summary,omitempty"`
}

type EvidenceAccessGrant struct {
	ID         int64      `json:"id"`
	EvidenceID string     `json:"evidence_id"`
	AdminID    int64      `json:"admin_id"`
	Reason     string     `json:"reason"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
}

type EvidenceAccessAuthorization struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type RawEvidenceView struct {
	EvidenceID string    `json:"evidence_id"`
	Content    string    `json:"content"`
	Protocol   string    `json:"protocol"`
	Model      string    `json:"model"`
	CapturedAt time.Time `json:"captured_at"`
}

type EvidenceObjectPage struct {
	Keys       []string
	NextCursor string
}

type RetrospectiveRiskStatus struct {
	Configured         bool                    `json:"configured"`
	ConnectivityOK     bool                    `json:"connectivity_ok"`
	ConnectivityError  string                  `json:"connectivity_error,omitempty"`
	CaptureEnabled     bool                    `json:"capture_enabled"`
	ReviewEnabled      bool                    `json:"review_enabled"`
	QueueSize          int                     `json:"queue_size"`
	QueueLength        int                     `json:"queue_length"`
	Enqueued           int64                   `json:"enqueued"`
	Dropped            int64                   `json:"dropped"`
	Stored             int64                   `json:"stored"`
	StorageErrors      int64                   `json:"storage_errors"`
	StoredBytes        int64                   `json:"stored_bytes"`
	EvidenceItems      int64                   `json:"evidence_items"`
	ActiveRiskProfiles int64                   `json:"active_risk_profiles"`
	LatestReviewRun    *RetrospectiveReviewRun `json:"latest_review_run,omitempty"`
}
