package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *profitRepository) enrichSubscriptionCycles(ctx context.Context, cycles []*service.AccountSubscriptionCycle) ([]*service.AccountSubscriptionCycle, error) {
	if len(cycles) == 0 {
		return cycles, nil
	}
	cycleIDs := make([]int64, 0, len(cycles))
	cyclesByID := make(map[int64]*service.AccountSubscriptionCycle, len(cycles))
	for _, cycle := range cycles {
		cycleIDs = append(cycleIDs, cycle.ID)
		cyclesByID[cycle.ID] = cycle
	}

	terminationRows, err := r.db.QueryContext(ctx, `
		SELECT id, cycle_id, account_id, effective_at, reason, notes,
		       reversed_at, reversal_reason, created_at, updated_at
		FROM account_subscription_terminations
		WHERE cycle_id = ANY($1) AND reversed_at IS NULL
		ORDER BY effective_at DESC, id DESC
	`, pq.Array(cycleIDs))
	if err != nil {
		return nil, err
	}
	for terminationRows.Next() {
		termination, scanErr := scanSubscriptionTermination(terminationRows)
		if scanErr != nil {
			_ = terminationRows.Close()
			return nil, scanErr
		}
		if cycle := cyclesByID[termination.CycleID]; cycle != nil && cycle.Termination == nil {
			cycle.Termination = termination
		}
	}
	if err := terminationRows.Err(); err != nil {
		_ = terminationRows.Close()
		return nil, err
	}
	_ = terminationRows.Close()

	refundRows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.termination_id, t.cycle_id, t.account_id, r.amount, r.currency,
		       r.received_at, r.notes, r.voided_at, r.void_reason, r.created_at, r.updated_at
		FROM account_subscription_refunds r
		JOIN account_subscription_terminations t ON t.id = r.termination_id
		WHERE t.cycle_id = ANY($1)
		ORDER BY r.received_at ASC, r.id ASC
	`, pq.Array(cycleIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = refundRows.Close() }()
	for refundRows.Next() {
		refund, scanErr := scanSubscriptionRefund(refundRows)
		if scanErr != nil {
			return nil, scanErr
		}
		if cycle := cyclesByID[refund.CycleID]; cycle != nil {
			cycle.Refunds = append(cycle.Refunds, refund)
		}
	}
	if err := refundRows.Err(); err != nil {
		return nil, err
	}
	return cycles, nil
}

func (r *profitRepository) CreateSubscriptionTermination(ctx context.Context, termination *service.AccountSubscriptionTermination, initialRefund *service.AccountSubscriptionRefund) (*service.SubscriptionTerminationWriteResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		accountID  int64
		startsAt   time.Time
		periodFee  float64
		periodDays int
		currency   string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT account_id, starts_at, period_fee, period_days, currency
		FROM account_subscription_cycles
		WHERE id = $1
		FOR UPDATE
	`, termination.CycleID).Scan(&accountID, &startsAt, &periodFee, &periodDays, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionCycleNotFound
	}
	if err != nil {
		return nil, err
	}
	cycleEnd := startsAt.AddDate(0, 0, periodDays)
	if termination.EffectiveAt.Before(startsAt) || termination.EffectiveAt.After(cycleEnd) {
		return nil, fmt.Errorf("termination effective_at must be inside the subscription cycle")
	}
	var hasActive bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM account_subscription_terminations
			WHERE cycle_id = $1 AND reversed_at IS NULL
		)
	`, termination.CycleID).Scan(&hasActive); err != nil {
		return nil, err
	}
	if hasActive {
		return nil, service.ErrSubscriptionCycleAlreadyTerminated
	}
	termination.AccountID = accountID
	if termination.Reason == "" {
		termination.Reason = "upstream_banned"
	}
	row := tx.QueryRowContext(ctx, `
		INSERT INTO account_subscription_terminations (
			cycle_id, account_id, effective_at, reason, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING id, cycle_id, account_id, effective_at, reason, notes,
		          reversed_at, reversal_reason, created_at, updated_at
	`, termination.CycleID, accountID, termination.EffectiveAt, termination.Reason, termination.Notes)
	createdTermination, err := scanSubscriptionTermination(row)
	if err != nil {
		return nil, err
	}

	var createdRefund *service.AccountSubscriptionRefund
	if initialRefund != nil && initialRefund.Amount > 0 {
		var existingRefundTotal float64
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(r.amount), 0)
			FROM account_subscription_refunds r
			JOIN account_subscription_terminations t ON t.id = r.termination_id
			WHERE t.cycle_id = $1 AND r.voided_at IS NULL
		`, termination.CycleID).Scan(&existingRefundTotal); err != nil {
			return nil, err
		}
		if existingRefundTotal+initialRefund.Amount > periodFee+0.00000001 {
			return nil, service.ErrSubscriptionRefundExceedsFee
		}
		if initialRefund.ReceivedAt.Before(termination.EffectiveAt) {
			return nil, fmt.Errorf("refund received_at cannot be before termination effective_at")
		}
		if initialRefund.Currency == "" {
			initialRefund.Currency = currency
		}
		refundRow := tx.QueryRowContext(ctx, `
			INSERT INTO account_subscription_refunds (
				termination_id, amount, currency, received_at, notes, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			RETURNING id, termination_id, $6::bigint, $7::bigint, amount, currency,
			          received_at, notes, voided_at, void_reason, created_at, updated_at
		`, createdTermination.ID, initialRefund.Amount, initialRefund.Currency, initialRefund.ReceivedAt, initialRefund.Notes, termination.CycleID, accountID)
		createdRefund, err = scanSubscriptionRefund(refundRow)
		if err != nil {
			return nil, err
		}
	}

	rows, err := tx.QueryContext(ctx, `
		UPDATE accounts
		SET status = $2, schedulable = FALSE, updated_at = NOW()
		WHERE deleted_at IS NULL AND (id = $1 OR parent_account_id = $1)
		RETURNING id
	`, accountID, service.StatusDisabled)
	if err != nil {
		return nil, err
	}
	disabledIDs := make([]int64, 0, 2)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		disabledIDs = append(disabledIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	if len(disabledIDs) == 0 {
		return nil, service.ErrAccountNotFound
	}
	for _, id := range disabledIDs {
		id := id
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &service.SubscriptionTerminationWriteResult{
		Termination:        createdTermination,
		InitialRefund:      createdRefund,
		DisabledAccountIDs: disabledIDs,
	}, nil
}

func (r *profitRepository) CreateSubscriptionRefund(ctx context.Context, refund *service.AccountSubscriptionRefund) (*service.AccountSubscriptionRefund, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		cycleID     int64
		accountID   int64
		periodFee   float64
		currency    string
		effectiveAt time.Time
		reversedAt  *time.Time
	)
	err = tx.QueryRowContext(ctx, `
		SELECT t.cycle_id, t.account_id, c.period_fee, c.currency, t.effective_at, t.reversed_at
		FROM account_subscription_terminations t
		JOIN account_subscription_cycles c ON c.id = t.cycle_id
		WHERE t.id = $1
		FOR UPDATE OF t, c
	`, refund.TerminationID).Scan(&cycleID, &accountID, &periodFee, &currency, &effectiveAt, &reversedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionTerminationNotFound
	}
	if err != nil {
		return nil, err
	}
	if reversedAt != nil {
		return nil, service.ErrSubscriptionTerminationReversed
	}
	if refund.Amount <= 0 {
		return nil, fmt.Errorf("refund amount must be positive")
	}
	if refund.ReceivedAt.Before(effectiveAt) {
		return nil, fmt.Errorf("refund received_at cannot be before termination effective_at")
	}
	var refundTotal float64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(r.amount), 0)
		FROM account_subscription_refunds r
		JOIN account_subscription_terminations t ON t.id = r.termination_id
		WHERE t.cycle_id = $1 AND r.voided_at IS NULL
	`, cycleID).Scan(&refundTotal); err != nil {
		return nil, err
	}
	if refundTotal+refund.Amount > periodFee+0.00000001 {
		return nil, service.ErrSubscriptionRefundExceedsFee
	}
	if refund.Currency == "" {
		refund.Currency = currency
	}
	row := tx.QueryRowContext(ctx, `
		INSERT INTO account_subscription_refunds (
			termination_id, amount, currency, received_at, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING id, termination_id, $6::bigint, $7::bigint, amount, currency,
		          received_at, notes, voided_at, void_reason, created_at, updated_at
	`, refund.TerminationID, refund.Amount, refund.Currency, refund.ReceivedAt, refund.Notes, cycleID, accountID)
	created, err := scanSubscriptionRefund(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (r *profitRepository) VoidSubscriptionRefund(ctx context.Context, id int64, reason string, voidedAt time.Time) (*service.AccountSubscriptionRefund, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `
		SELECT r.id, r.termination_id, t.cycle_id, t.account_id, r.amount, r.currency,
		       r.received_at, r.notes, r.voided_at, r.void_reason, r.created_at, r.updated_at
		FROM account_subscription_refunds r
		JOIN account_subscription_terminations t ON t.id = r.termination_id
		WHERE r.id = $1
		FOR UPDATE OF r
	`, id)
	refund, err := scanSubscriptionRefund(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionRefundNotFound
	}
	if err != nil {
		return nil, err
	}
	if refund.VoidedAt != nil {
		return nil, service.ErrSubscriptionRefundVoided
	}
	row = tx.QueryRowContext(ctx, `
		UPDATE account_subscription_refunds r
		SET voided_at = $2, void_reason = $3, updated_at = NOW()
		FROM account_subscription_terminations t
		WHERE r.id = $1 AND t.id = r.termination_id
		RETURNING r.id, r.termination_id, t.cycle_id, t.account_id, r.amount, r.currency,
		          r.received_at, r.notes, r.voided_at, r.void_reason, r.created_at, r.updated_at
	`, id, voidedAt, reason)
	updated, err := scanSubscriptionRefund(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *profitRepository) ReverseSubscriptionTermination(ctx context.Context, id int64, reason string, reversedAt time.Time) (*service.AccountSubscriptionTermination, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `
		SELECT id, cycle_id, account_id, effective_at, reason, notes,
		       reversed_at, reversal_reason, created_at, updated_at
		FROM account_subscription_terminations
		WHERE id = $1
		FOR UPDATE
	`, id)
	termination, err := scanSubscriptionTermination(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionTerminationNotFound
	}
	if err != nil {
		return nil, err
	}
	if termination.ReversedAt != nil {
		return nil, service.ErrSubscriptionTerminationReversed
	}
	row = tx.QueryRowContext(ctx, `
		UPDATE account_subscription_terminations
		SET reversed_at = $2, reversal_reason = $3, updated_at = NOW()
		WHERE id = $1
		RETURNING id, cycle_id, account_id, effective_at, reason, notes,
		          reversed_at, reversal_reason, created_at, updated_at
	`, id, reversedAt, reason)
	updated, err := scanSubscriptionTermination(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}

func (r *profitRepository) GetSubscriptionCycleRevenueBatch(ctx context.Context, ranges []service.SubscriptionCycleUsageRange) (map[int64]float64, error) {
	result := make(map[int64]float64, len(ranges))
	if len(ranges) == 0 {
		return result, nil
	}
	type rangePayload struct {
		CycleID   int64     `json:"cycle_id"`
		AccountID int64     `json:"account_id"`
		Start     time.Time `json:"start_at"`
		End       time.Time `json:"end_at"`
	}
	payload := make([]rangePayload, 0, len(ranges))
	for _, item := range ranges {
		payload = append(payload, rangePayload{CycleID: item.CycleID, AccountID: item.AccountID, Start: item.Start, End: item.End})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT x.cycle_id, COALESCE(SUM(u.actual_cost), 0)
		FROM jsonb_to_recordset($1::jsonb) AS x(
			cycle_id bigint,
			account_id bigint,
			start_at timestamptz,
			end_at timestamptz
		)
		LEFT JOIN usage_logs u
		  ON u.account_id = x.account_id
		 AND u.created_at >= x.start_at
		 AND u.created_at < x.end_at
		GROUP BY x.cycle_id
	`, encoded)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cycleID int64
		var revenue float64
		if err := rows.Scan(&cycleID, &revenue); err != nil {
			return nil, err
		}
		result[cycleID] = revenue
	}
	return result, rows.Err()
}

func scanSubscriptionTermination(row costConfigScanner) (*service.AccountSubscriptionTermination, error) {
	termination := &service.AccountSubscriptionTermination{}
	err := row.Scan(
		&termination.ID,
		&termination.CycleID,
		&termination.AccountID,
		&termination.EffectiveAt,
		&termination.Reason,
		&termination.Notes,
		&termination.ReversedAt,
		&termination.ReversalReason,
		&termination.CreatedAt,
		&termination.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return termination, nil
}

func scanSubscriptionRefund(row costConfigScanner) (*service.AccountSubscriptionRefund, error) {
	refund := &service.AccountSubscriptionRefund{}
	err := row.Scan(
		&refund.ID,
		&refund.TerminationID,
		&refund.CycleID,
		&refund.AccountID,
		&refund.Amount,
		&refund.Currency,
		&refund.ReceivedAt,
		&refund.Notes,
		&refund.VoidedAt,
		&refund.VoidReason,
		&refund.CreatedAt,
		&refund.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return refund, nil
}
