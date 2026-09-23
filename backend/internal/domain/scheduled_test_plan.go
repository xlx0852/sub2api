// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"
)

// ScheduledTestPlan represents a scheduled test plan domain model.
type ScheduledTestPlan struct {
	ID             int64  `json:"id"`
	AccountID      int64  `json:"account_id"`
	ModelID        string `json:"model_id"`
	CronExpression string `json:"cron_expression"`
	Enabled        bool   `json:"enabled"`
	MaxResults     int    `json:"max_results"`
	AutoRecover    bool   `json:"auto_recover"`
	// AutoPurity runs an active model-purity attribution after the connectivity chain.
	AutoPurity bool `json:"auto_purity"`
	// PurityIntervalMinutes throttles AutoPurity (0 = every run). Defaults to daily so a
	// frequent diagnostics cron cannot repeatedly spend upstream tokens.
	PurityIntervalMinutes int `json:"purity_interval_minutes"`
	// PurityAnswers is how many challenge answers the probe requests (1..6).
	PurityAnswers int        `json:"purity_answers"`
	LastRunAt     *time.Time `json:"last_run_at"`
	NextRunAt     *time.Time `json:"next_run_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ScheduledTestResult represents a single test execution result.
type ScheduledTestResult struct {
	ID           int64     `json:"id"`
	PlanID       int64     `json:"plan_id"`
	Status       string    `json:"status"`
	ResponseText string    `json:"response_text"`
	ErrorMessage string    `json:"error_message"`
	LatencyMs    int64     `json:"latency_ms"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// ScheduledDiagnosticsStatus summarizes whether an account has diagnostics enabled.
type ScheduledDiagnosticsStatus struct {
	AccountID    int64      `json:"account_id"`
	Enabled      bool       `json:"enabled"`
	PlanCount    int        `json:"plan_count"`
	EnabledCount int        `json:"enabled_count"`
	NextRunAt    *time.Time `json:"next_run_at,omitempty"`
}
