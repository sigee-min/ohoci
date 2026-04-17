package ocibilling

import (
	"context"
	"time"

	"ohoci/internal/store"
)

const (
	DefaultBudgetWindowDays  = 7
	defaultSnapshotFreshness = 30 * time.Minute
)

type PolicyGuardrailStatus struct {
	PolicyID            int64      `json:"policyId"`
	PolicyLabel         string     `json:"policyLabel"`
	BudgetEnabled       bool       `json:"budgetEnabled"`
	BudgetCapAmount     float64    `json:"budgetCapAmount"`
	BudgetWindowDays    int        `json:"budgetWindowDays"`
	Currency            string     `json:"currency,omitempty"`
	TotalCost           float64    `json:"totalCost"`
	Blocked             bool       `json:"blocked"`
	Degraded            bool       `json:"degraded"`
	ReasonCode          string     `json:"reasonCode,omitempty"`
	Message             string     `json:"message,omitempty"`
	SnapshotGeneratedAt *time.Time `json:"snapshotGeneratedAt,omitempty"`
}

type GuardrailReport struct {
	GeneratedAt time.Time               `json:"generatedAt"`
	WindowDays  int                     `json:"windowDays"`
	Items       []PolicyGuardrailStatus `json:"items"`
}

func (s *Service) RefreshPolicySnapshots(ctx context.Context, windowDays int) (GuardrailReport, error) {
	if windowDays <= 0 {
		windowDays = DefaultBudgetWindowDays
	}
	windowEnd := time.Now().UTC()
	windowStart := windowEnd.AddDate(0, 0, -windowDays)
	report, err := s.PolicyBreakdown(ctx, PolicyBreakdownRequest{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
	if err != nil {
		return GuardrailReport{}, err
	}

	policies, err := s.store.ListPolicies(ctx)
	if err != nil {
		return GuardrailReport{}, err
	}
	costByPolicy := map[int64]PolicyCostItem{}
	for _, item := range report.Items {
		costByPolicy[item.PolicyID] = item
	}
	for _, policy := range policies {
		item := costByPolicy[policy.ID]
		if _, err := s.store.UpsertBillingPolicySnapshot(ctx, store.BillingPolicySnapshot{
			PolicyID:     policy.ID,
			WindowDays:   windowDays,
			Currency:     firstNonEmpty(item.Currency, report.Currency),
			TotalCost:    item.TotalCost,
			GeneratedAt:  report.GeneratedAt,
			SourceRegion: report.SourceRegion,
			ErrorMessage: "",
		}); err != nil {
			return GuardrailReport{}, err
		}
	}
	return s.PolicyGuardrails(ctx, windowDays)
}

func (s *Service) PolicyGuardrails(ctx context.Context, windowDays int) (GuardrailReport, error) {
	if windowDays <= 0 {
		windowDays = DefaultBudgetWindowDays
	}
	policies, err := s.store.ListPolicies(ctx)
	if err != nil {
		return GuardrailReport{}, err
	}
	snapshots, err := s.store.ListBillingPolicySnapshots(ctx, windowDays)
	if err != nil {
		return GuardrailReport{}, err
	}
	snapshotByPolicyID := map[int64]store.BillingPolicySnapshot{}
	for _, snapshot := range snapshots {
		snapshotByPolicyID[snapshot.PolicyID] = snapshot
	}
	items := make([]PolicyGuardrailStatus, 0, len(policies))
	now := time.Now().UTC()
	for _, policy := range policies {
		item := PolicyGuardrailStatus{
			PolicyID:         policy.ID,
			PolicyLabel:      policyLabel(policy),
			BudgetEnabled:    policy.BudgetEnabled,
			BudgetCapAmount:  policy.BudgetCapAmount,
			BudgetWindowDays: budgetWindowDays(policy),
		}
		snapshot, ok := snapshotByPolicyID[policy.ID]
		if !ok {
			if policy.BudgetEnabled {
				item.Degraded = true
				item.ReasonCode = "budget_snapshot_missing"
				item.Message = "billing snapshot is not ready"
			}
			items = append(items, item)
			continue
		}
		item.Currency = snapshot.Currency
		item.TotalCost = snapshot.TotalCost
		generatedAt := snapshot.GeneratedAt.UTC()
		item.SnapshotGeneratedAt = &generatedAt
		if snapshot.ErrorMessage != "" {
			item.Degraded = true
			item.ReasonCode = "budget_snapshot_error"
			item.Message = snapshot.ErrorMessage
		} else if now.Sub(generatedAt) > defaultSnapshotFreshness {
			item.Degraded = true
			item.ReasonCode = "budget_snapshot_stale"
			item.Message = "billing snapshot is stale"
		}
		if policy.BudgetEnabled && !item.Degraded && policy.BudgetCapAmount > 0 && snapshot.TotalCost >= policy.BudgetCapAmount {
			item.Blocked = true
			item.ReasonCode = "budget_cap_exceeded"
			item.Message = "policy budget cap reached"
		}
		items = append(items, item)
	}
	return GuardrailReport{
		GeneratedAt: now,
		WindowDays:  windowDays,
		Items:       items,
	}, nil
}

func (s *Service) EvaluatePolicyBudget(ctx context.Context, policy store.Policy) (PolicyGuardrailStatus, error) {
	report, err := s.PolicyGuardrails(ctx, budgetWindowDays(policy))
	if err != nil {
		return PolicyGuardrailStatus{}, err
	}
	for _, item := range report.Items {
		if item.PolicyID == policy.ID {
			return item, nil
		}
	}
	return PolicyGuardrailStatus{
		PolicyID:         policy.ID,
		PolicyLabel:      policyLabel(policy),
		BudgetEnabled:    policy.BudgetEnabled,
		BudgetCapAmount:  policy.BudgetCapAmount,
		BudgetWindowDays: budgetWindowDays(policy),
		Degraded:         policy.BudgetEnabled,
		ReasonCode:       "budget_snapshot_missing",
		Message:          "billing snapshot is not ready",
	}, nil
}

func budgetWindowDays(policy store.Policy) int {
	if policy.BudgetWindowDays > 0 {
		return policy.BudgetWindowDays
	}
	return DefaultBudgetWindowDays
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
