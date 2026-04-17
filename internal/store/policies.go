package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Policy struct {
	ID                int64     `json:"id"`
	Label             string    `json:"label,omitempty"`
	Labels            []string  `json:"labels"`
	SubnetOCID        string    `json:"subnetOcid,omitempty"`
	Shape             string    `json:"shape"`
	OCPU              int       `json:"ocpu"`
	MemoryGB          int       `json:"memoryGb"`
	MaxRunners        int       `json:"maxRunners"`
	TTLMinutes        int       `json:"ttlMinutes"`
	Spot              bool      `json:"spot"`
	Enabled           bool      `json:"enabled"`
	WarmEnabled       bool      `json:"warmEnabled"`
	WarmMinIdle       int       `json:"warmMinIdle"`
	WarmTTLMinutes    int       `json:"warmTtlMinutes"`
	WarmRepoAllowlist []string  `json:"warmRepoAllowlist"`
	BudgetEnabled     bool      `json:"budgetEnabled"`
	BudgetCapAmount   float64   `json:"budgetCapAmount"`
	BudgetWindowDays  int       `json:"budgetWindowDays"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (s *Store) ListPolicies(ctx context.Context) ([]Policy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, subnet_ocid, shape, ocpu, memory_gb, max_runners, ttl_minutes, spot, enabled, warm_enabled, warm_min_idle, warm_ttl_minutes, warm_repo_allowlist_json, budget_enabled, budget_cap_amount, budget_window_days, created_at, updated_at FROM policies ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	policies := []Policy{}
	for rows.Next() {
		var policy Policy
		var warmRepoAllowlistJSON string
		var spot, enabled, warmEnabled, budgetEnabled int
		if err := rows.Scan(&policy.ID, &policy.SubnetOCID, &policy.Shape, &policy.OCPU, &policy.MemoryGB, &policy.MaxRunners, &policy.TTLMinutes, &spot, &enabled, &warmEnabled, &policy.WarmMinIdle, &policy.WarmTTLMinutes, &warmRepoAllowlistJSON, &budgetEnabled, &policy.BudgetCapAmount, &policy.BudgetWindowDays, &policy.CreatedAt, &policy.UpdatedAt); err != nil {
			return nil, err
		}
		policy.Spot = spot == 1
		policy.Enabled = enabled == 1
		policy.WarmEnabled = warmEnabled == 1
		policy.BudgetEnabled = budgetEnabled == 1
		policy.Labels, err = s.labelsForPolicy(ctx, policy.ID)
		if err != nil {
			return nil, err
		}
		if err := unmarshalJSONArray(warmRepoAllowlistJSON, &policy.WarmRepoAllowlist); err != nil {
			return nil, err
		}
		policy.WarmRepoAllowlist = normalizeStrings(policy.WarmRepoAllowlist)
		if len(policy.Labels) > 0 {
			policy.Label = strings.Join(policy.Labels, "-")
		}
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func (s *Store) FindPolicyByID(ctx context.Context, id int64) (Policy, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, subnet_ocid, shape, ocpu, memory_gb, max_runners, ttl_minutes, spot, enabled, warm_enabled, warm_min_idle, warm_ttl_minutes, warm_repo_allowlist_json, budget_enabled, budget_cap_amount, budget_window_days, created_at, updated_at FROM policies WHERE id = ?`, id)
	var policy Policy
	var warmRepoAllowlistJSON string
	var spot, enabled, warmEnabled, budgetEnabled int
	if err := row.Scan(&policy.ID, &policy.SubnetOCID, &policy.Shape, &policy.OCPU, &policy.MemoryGB, &policy.MaxRunners, &policy.TTLMinutes, &spot, &enabled, &warmEnabled, &policy.WarmMinIdle, &policy.WarmTTLMinutes, &warmRepoAllowlistJSON, &budgetEnabled, &policy.BudgetCapAmount, &policy.BudgetWindowDays, &policy.CreatedAt, &policy.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Policy{}, ErrNotFound
		}
		return Policy{}, err
	}
	policy.Spot = spot == 1
	policy.Enabled = enabled == 1
	policy.WarmEnabled = warmEnabled == 1
	policy.BudgetEnabled = budgetEnabled == 1
	var err error
	policy.Labels, err = s.labelsForPolicy(ctx, policy.ID)
	if err != nil {
		return Policy{}, err
	}
	if err := unmarshalJSONArray(warmRepoAllowlistJSON, &policy.WarmRepoAllowlist); err != nil {
		return Policy{}, err
	}
	policy.WarmRepoAllowlist = normalizeStrings(policy.WarmRepoAllowlist)
	if len(policy.Labels) > 0 {
		policy.Label = strings.Join(policy.Labels, "-")
	}
	return policy, nil
}

func (s *Store) CreatePolicy(ctx context.Context, policy Policy) (Policy, error) {
	policy, err := normalizePolicyInput(policy)
	if err != nil {
		return Policy{}, err
	}
	now := s.now().UTC()
	warmRepoAllowlistJSON, err := marshalJSONArray(policy.WarmRepoAllowlist)
	if err != nil {
		return Policy{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO policies (subnet_ocid, shape, ocpu, memory_gb, max_runners, ttl_minutes, spot, enabled, warm_enabled, warm_min_idle, warm_ttl_minutes, warm_repo_allowlist_json, budget_enabled, budget_cap_amount, budget_window_days, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(policy.SubnetOCID),
		strings.TrimSpace(policy.Shape),
		policy.OCPU,
		policy.MemoryGB,
		policy.MaxRunners,
		policy.TTLMinutes,
		boolAsInt(policy.Spot),
		boolAsInt(policy.Enabled),
		boolAsInt(policy.WarmEnabled),
		policy.WarmMinIdle,
		policy.WarmTTLMinutes,
		warmRepoAllowlistJSON,
		boolAsInt(policy.BudgetEnabled),
		policy.BudgetCapAmount,
		policy.BudgetWindowDays,
		now,
		now,
	)
	if err != nil {
		return Policy{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Policy{}, err
	}
	if err := s.replacePolicyLabels(ctx, id, policy.Labels); err != nil {
		return Policy{}, err
	}
	return s.FindPolicyByID(ctx, id)
}

func (s *Store) UpdatePolicy(ctx context.Context, id int64, policy Policy) (Policy, error) {
	policy, err := normalizePolicyInput(policy)
	if err != nil {
		return Policy{}, err
	}
	warmRepoAllowlistJSON, err := marshalJSONArray(policy.WarmRepoAllowlist)
	if err != nil {
		return Policy{}, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE policies SET subnet_ocid = ?, shape = ?, ocpu = ?, memory_gb = ?, max_runners = ?, ttl_minutes = ?, spot = ?, enabled = ?, warm_enabled = ?, warm_min_idle = ?, warm_ttl_minutes = ?, warm_repo_allowlist_json = ?, budget_enabled = ?, budget_cap_amount = ?, budget_window_days = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(policy.SubnetOCID),
		strings.TrimSpace(policy.Shape),
		policy.OCPU,
		policy.MemoryGB,
		policy.MaxRunners,
		policy.TTLMinutes,
		boolAsInt(policy.Spot),
		boolAsInt(policy.Enabled),
		boolAsInt(policy.WarmEnabled),
		policy.WarmMinIdle,
		policy.WarmTTLMinutes,
		warmRepoAllowlistJSON,
		boolAsInt(policy.BudgetEnabled),
		policy.BudgetCapAmount,
		policy.BudgetWindowDays,
		s.now().UTC(),
		id,
	)
	if err != nil {
		return Policy{}, err
	}
	if err := s.replacePolicyLabels(ctx, id, policy.Labels); err != nil {
		return Policy{}, err
	}
	return s.FindPolicyByID(ctx, id)
}

func (s *Store) DeletePolicy(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM policy_labels WHERE policy_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM policies WHERE id = ?`, id)
	return err
}

func (s *Store) labelsForPolicy(ctx context.Context, policyID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT label FROM policy_labels WHERE policy_id = ? ORDER BY label ASC`, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	labels := []string{}
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}

func (s *Store) replacePolicyLabels(ctx context.Context, policyID int64, labels []string) error {
	normalized := normalizeLabels(labels)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM policy_labels WHERE policy_id = ?`, policyID); err != nil {
		return err
	}
	for _, label := range normalized {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO policy_labels (policy_id, label) VALUES (?, ?)`, policyID, label); err != nil {
			return err
		}
	}
	return nil
}

func normalizePolicyInput(policy Policy) (Policy, error) {
	policy.SubnetOCID = strings.TrimSpace(policy.SubnetOCID)
	policy.Shape = strings.TrimSpace(policy.Shape)
	policy.WarmRepoAllowlist = normalizeStrings(policy.WarmRepoAllowlist)
	if policy.WarmMinIdle < 0 || policy.WarmMinIdle > 1 {
		return Policy{}, errors.New("warmMinIdle must be 0 or 1 in v1")
	}
	if !policy.WarmEnabled {
		policy.WarmMinIdle = 0
		policy.WarmRepoAllowlist = nil
	}
	if policy.BudgetWindowDays <= 0 {
		policy.BudgetWindowDays = 7
	}
	if policy.BudgetWindowDays != 7 {
		return Policy{}, errors.New("budgetWindowDays must stay fixed at 7 in v1")
	}
	return policy, nil
}
