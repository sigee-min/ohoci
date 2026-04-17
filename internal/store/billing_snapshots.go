package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type BillingPolicySnapshot struct {
	PolicyID     int64     `json:"policyId"`
	WindowDays   int       `json:"windowDays"`
	Currency     string    `json:"currency"`
	TotalCost    float64   `json:"totalCost"`
	GeneratedAt  time.Time `json:"generatedAt"`
	SourceRegion string    `json:"sourceRegion,omitempty"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

const billingPolicySnapshotColumns = `policy_id, window_days, currency, total_cost, generated_at, source_region, error_message, created_at, updated_at`

func (s *Store) UpsertBillingPolicySnapshot(ctx context.Context, snapshot BillingPolicySnapshot) (BillingPolicySnapshot, error) {
	now := s.now().UTC()
	if _, err := s.FindBillingPolicySnapshot(ctx, snapshot.PolicyID, snapshot.WindowDays); err == nil {
		_, err = s.db.ExecContext(
			ctx,
			`UPDATE billing_policy_snapshots
			 SET currency = ?, total_cost = ?, generated_at = ?, source_region = ?, error_message = ?, updated_at = ?
			 WHERE policy_id = ? AND window_days = ?`,
			snapshot.Currency,
			snapshot.TotalCost,
			snapshot.GeneratedAt.UTC(),
			snapshot.SourceRegion,
			snapshot.ErrorMessage,
			now,
			snapshot.PolicyID,
			snapshot.WindowDays,
		)
		if err != nil {
			return BillingPolicySnapshot{}, err
		}
		return s.FindBillingPolicySnapshot(ctx, snapshot.PolicyID, snapshot.WindowDays)
	} else if !errors.Is(err, ErrNotFound) {
		return BillingPolicySnapshot{}, err
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO billing_policy_snapshots (policy_id, window_days, currency, total_cost, generated_at, source_region, error_message, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.PolicyID,
		snapshot.WindowDays,
		snapshot.Currency,
		snapshot.TotalCost,
		snapshot.GeneratedAt.UTC(),
		snapshot.SourceRegion,
		snapshot.ErrorMessage,
		now,
		now,
	)
	if err != nil {
		return BillingPolicySnapshot{}, err
	}
	return s.FindBillingPolicySnapshot(ctx, snapshot.PolicyID, snapshot.WindowDays)
}

func (s *Store) FindBillingPolicySnapshot(ctx context.Context, policyID int64, windowDays int) (BillingPolicySnapshot, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+billingPolicySnapshotColumns+` FROM billing_policy_snapshots WHERE policy_id = ? AND window_days = ?`, policyID, windowDays)
	return scanBillingPolicySnapshot(row)
}

func (s *Store) ListBillingPolicySnapshots(ctx context.Context, windowDays int) ([]BillingPolicySnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+billingPolicySnapshotColumns+` FROM billing_policy_snapshots WHERE window_days = ? ORDER BY policy_id ASC`, windowDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []BillingPolicySnapshot{}
	for rows.Next() {
		item, err := scanBillingPolicySnapshot(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanBillingPolicySnapshot(scanner interface{ Scan(dest ...any) error }) (BillingPolicySnapshot, error) {
	var item BillingPolicySnapshot
	if err := scanner.Scan(&item.PolicyID, &item.WindowDays, &item.Currency, &item.TotalCost, &item.GeneratedAt, &item.SourceRegion, &item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BillingPolicySnapshot{}, ErrNotFound
		}
		return BillingPolicySnapshot{}, err
	}
	item.GeneratedAt = item.GeneratedAt.UTC()
	return item, nil
}
