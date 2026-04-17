package admission

import (
	"context"
	"errors"
	"strings"
	"time"

	"ohoci/internal/githubapp"
	"ohoci/internal/ocibilling"
	"ohoci/internal/policy"
	"ohoci/internal/setup"
	"ohoci/internal/store"
)

const (
	StageSetupReady         = "setup_ready"
	StageRepoAllowed        = "repo_allowed"
	StagePolicyMatch        = "policy_match"
	StageCapacityOK         = "capacity_ok"
	StageBudgetOK           = "budget_ok"
	StageWarmCandidate      = "warm_candidate"
	StageLaunchRequired     = "launch_required"
	StageRunnerRegistration = "runner_registration"
	StageRunnerAttachment   = "runner_attachment"
	StageCleanup            = "cleanup"
)

type BudgetEvaluator interface {
	EvaluatePolicyBudget(ctx context.Context, policy store.Policy) (ocibilling.PolicyGuardrailStatus, error)
}

type Input struct {
	DeliveryID     string
	InstallationID int64
	RepoOwner      string
	RepoName       string
	Labels         []string
}

type PolicyCheck struct {
	PolicyID         int64    `json:"policyId"`
	PolicyLabel      string   `json:"policyLabel"`
	PolicyLabels     []string `json:"policyLabels"`
	Matched          bool     `json:"matched"`
	Reasons          []string `json:"reasons,omitempty"`
	MissingLabels    []string `json:"missingLabels,omitempty"`
	ExtraLabels      []string `json:"extraLabels,omitempty"`
	CapacityBlocked  bool     `json:"capacityBlocked"`
	ActiveRunners    int      `json:"activeRunners"`
	MaxRunners       int      `json:"maxRunners"`
	BudgetBlocked    bool     `json:"budgetBlocked"`
	BudgetDegraded   bool     `json:"budgetDegraded"`
	BudgetMessage    string   `json:"budgetMessage,omitempty"`
	WarmConfigured   bool     `json:"warmConfigured"`
	WarmRepoEligible bool     `json:"warmRepoEligible"`
}

type Decision struct {
	RequestedLabels []string                         `json:"requestedLabels"`
	BlockingStage   string                           `json:"blockingStage,omitempty"`
	SummaryCode     string                           `json:"summaryCode"`
	MatchedPolicy   *store.Policy                    `json:"matchedPolicy,omitempty"`
	LaunchRequired  bool                             `json:"launchRequired"`
	WarmCandidate   *store.Runner                    `json:"warmCandidate,omitempty"`
	StageStatuses   map[string]store.DiagnosticStage `json:"stageStatuses"`
	PolicyChecks    []PolicyCheck                    `json:"policyChecks"`
}

type Service struct {
	store   *store.Store
	github  *githubapp.Service
	setup   *setup.Service
	billing BudgetEvaluator
	now     func() time.Time
}

func New(storeDB *store.Store, github *githubapp.Service, setupSvc *setup.Service, billing BudgetEvaluator) *Service {
	return &Service{
		store:   storeDB,
		github:  github,
		setup:   setupSvc,
		billing: billing,
		now:     time.Now,
	}
}

func (s *Service) Evaluate(ctx context.Context, input Input) (Decision, error) {
	decision := Decision{
		RequestedLabels: policy.ManagedLabels(input.Labels),
		LaunchRequired:  true,
		SummaryCode:     "admission_ready",
		StageStatuses:   map[string]store.DiagnosticStage{},
	}
	now := s.now().UTC()

	if s.setup != nil {
		setupStatus, err := s.setup.CurrentStatus(ctx)
		if err != nil {
			return decision, err
		}
		if !setupStatus.Ready {
			decision.SummaryCode = "setup_not_ready"
			decision.BlockingStage = StageSetupReady
			decision.StageStatuses[StageSetupReady] = stage("blocked", "setup_not_ready", "setup is not ready", now, map[string]any{"blockers": setupStatus.Blockers})
			return decision, nil
		}
	}
	decision.StageStatuses[StageSetupReady] = stage("passed", "setup_ready", "setup is ready", now, nil)

	client, err := s.resolveGitHubClient(ctx, input.InstallationID)
	if err != nil {
		return decision, err
	}
	if client == nil {
		decision.SummaryCode = "github_not_ready"
		decision.BlockingStage = StageRepoAllowed
		decision.StageStatuses[StageRepoAllowed] = stage("blocked", "github_not_ready", "github routing is not ready", now, nil)
		return decision, nil
	}
	if !client.RepositoryAllowed(input.RepoOwner, input.RepoName) {
		decision.SummaryCode = "repository_not_allowed"
		decision.BlockingStage = StageRepoAllowed
		decision.StageStatuses[StageRepoAllowed] = stage("blocked", "repository_not_allowed", "repository is not allowed", now, map[string]any{"repoOwner": input.RepoOwner, "repoName": input.RepoName})
		return decision, nil
	}
	decision.StageStatuses[StageRepoAllowed] = stage("passed", "repository_allowed", "repository is allowed", now, nil)

	policies, err := s.store.ListPolicies(ctx)
	if err != nil {
		return decision, err
	}
	match := policy.Match(policies, input.Labels)
	explanations := policy.Explain(policies, input.Labels)
	decision.PolicyChecks = make([]PolicyCheck, 0, len(explanations))
	for _, explanation := range explanations {
		var reusableWarmRunner *store.Runner
		if explanation.Policy != nil && explanation.Matched && repoWarmEligible(*explanation.Policy, input.RepoOwner, input.RepoName) {
			warmRunner, err := s.store.FindWarmIdleRunner(ctx, explanation.Policy.ID, input.RepoOwner, input.RepoName)
			switch {
			case err == nil:
				reusableWarmRunner = &warmRunner
			case err != nil && !errorsIsNotFound(err):
				return decision, err
			}
		}
		check := PolicyCheck{
			PolicyID:         explanation.Policy.ID,
			PolicyLabel:      strings.TrimSpace(explanation.Policy.Label),
			PolicyLabels:     append([]string(nil), explanation.PolicyLabels...),
			Matched:          explanation.Matched,
			Reasons:          append([]string(nil), explanation.Reasons...),
			MissingLabels:    append([]string(nil), explanation.MissingLabels...),
			ExtraLabels:      append([]string(nil), explanation.ExtraLabels...),
			MaxRunners:       explanation.Policy.MaxRunners,
			WarmConfigured:   explanation.Policy.WarmEnabled && explanation.Policy.WarmMinIdle > 0,
			WarmRepoEligible: repoWarmEligible(*explanation.Policy, input.RepoOwner, input.RepoName),
		}
		if explanation.Policy != nil && explanation.Matched {
			activeCount, err := s.store.CountActiveRunnersForPolicy(ctx, explanation.Policy.ID)
			if err != nil {
				return decision, err
			}
			check.ActiveRunners = activeCount
			effectiveActiveCount := activeCount
			if reusableWarmRunner != nil && effectiveActiveCount > 0 {
				effectiveActiveCount--
			}
			check.CapacityBlocked = effectiveActiveCount >= explanation.Policy.MaxRunners
			if check.CapacityBlocked {
				check.Reasons = append(check.Reasons, "policy max runners reached")
			}
			if s.billing != nil && explanation.Policy.BudgetEnabled {
				budgetStatus, err := s.billing.EvaluatePolicyBudget(ctx, *explanation.Policy)
				if err != nil {
					return decision, err
				}
				check.BudgetBlocked = budgetStatus.Blocked
				check.BudgetDegraded = budgetStatus.Degraded
				check.BudgetMessage = budgetStatus.Message
				if check.BudgetBlocked {
					check.Reasons = append(check.Reasons, "policy budget cap reached")
				}
				if check.BudgetDegraded && budgetStatus.Message != "" {
					check.Reasons = append(check.Reasons, budgetStatus.Message)
				}
			}
		}
		decision.PolicyChecks = append(decision.PolicyChecks, check)
	}

	if match.Policy == nil {
		decision.SummaryCode = "no_matching_policy"
		decision.BlockingStage = StagePolicyMatch
		decision.StageStatuses[StagePolicyMatch] = stage("blocked", "no_matching_policy", "no enabled policy matches the requested labels", now, map[string]any{"requestedLabels": decision.RequestedLabels})
		return decision, nil
	}
	decision.MatchedPolicy = match.Policy
	decision.StageStatuses[StagePolicyMatch] = stage("passed", "policy_matched", "policy matched", now, map[string]any{"policyId": match.Policy.ID})

	var reusableWarmRunner *store.Runner
	if repoWarmEligible(*match.Policy, input.RepoOwner, input.RepoName) {
		warmRunner, err := s.store.FindWarmIdleRunner(ctx, match.Policy.ID, input.RepoOwner, input.RepoName)
		switch {
		case err == nil:
			reusableWarmRunner = &warmRunner
		case err != nil && !errorsIsNotFound(err):
			return decision, err
		}
	}

	activeCount, err := s.store.CountActiveRunnersForPolicy(ctx, match.Policy.ID)
	if err != nil {
		return decision, err
	}
	effectiveActiveCount := activeCount
	if reusableWarmRunner != nil && effectiveActiveCount > 0 {
		effectiveActiveCount--
	}
	if effectiveActiveCount >= match.Policy.MaxRunners {
		decision.SummaryCode = "policy_capacity_reached"
		decision.BlockingStage = StageCapacityOK
		decision.StageStatuses[StageCapacityOK] = stage("blocked", "policy_capacity_reached", "policy max runners reached", now, map[string]any{"activeRunners": activeCount, "effectiveActiveRunners": effectiveActiveCount, "maxRunners": match.Policy.MaxRunners})
		return decision, nil
	}
	decision.StageStatuses[StageCapacityOK] = stage("passed", "capacity_available", "policy has capacity", now, map[string]any{"activeRunners": activeCount, "effectiveActiveRunners": effectiveActiveCount, "maxRunners": match.Policy.MaxRunners})

	if s.billing != nil && match.Policy.BudgetEnabled {
		budgetStatus, err := s.billing.EvaluatePolicyBudget(ctx, *match.Policy)
		if err != nil {
			return decision, err
		}
		switch {
		case budgetStatus.Blocked:
			decision.SummaryCode = "budget_blocked"
			decision.BlockingStage = StageBudgetOK
			decision.StageStatuses[StageBudgetOK] = stage("blocked", "budget_blocked", firstNonEmpty(budgetStatus.Message, "policy budget cap reached"), now, map[string]any{"currency": budgetStatus.Currency, "totalCost": budgetStatus.TotalCost, "budgetCapAmount": budgetStatus.BudgetCapAmount})
			return decision, nil
		case budgetStatus.Degraded:
			decision.StageStatuses[StageBudgetOK] = stage("degraded", firstNonEmpty(budgetStatus.ReasonCode, "budget_snapshot_degraded"), firstNonEmpty(budgetStatus.Message, "billing snapshot is degraded"), now, nil)
		default:
			decision.StageStatuses[StageBudgetOK] = stage("passed", "budget_available", "budget guardrail allows launch", now, map[string]any{"currency": budgetStatus.Currency, "totalCost": budgetStatus.TotalCost})
		}
	} else {
		decision.StageStatuses[StageBudgetOK] = stage("skipped", "budget_disabled", "budget guardrail is disabled", now, nil)
	}

	if match.Policy.WarmEnabled && match.Policy.WarmMinIdle > 0 && repoWarmEligible(*match.Policy, input.RepoOwner, input.RepoName) {
		switch {
		case reusableWarmRunner != nil:
			decision.WarmCandidate = reusableWarmRunner
			decision.LaunchRequired = false
			decision.SummaryCode = "warm_candidate_found"
			decision.StageStatuses[StageWarmCandidate] = stage("passed", "warm_candidate_found", "warm idle runner is available", now, map[string]any{"runnerId": reusableWarmRunner.ID})
			decision.StageStatuses[StageLaunchRequired] = stage("passed", "launch_not_required", "launch is not required because a warm runner is available", now, nil)
			return decision, nil
		default:
			decision.StageStatuses[StageWarmCandidate] = stage("passed", "warm_not_ready", "warm pool is configured but no idle runner is available", now, nil)
		}
	} else {
		decision.StageStatuses[StageWarmCandidate] = stage("skipped", "warm_not_configured", "warm pool is not configured for this repository", now, nil)
	}
	decision.StageStatuses[StageLaunchRequired] = stage("passed", "launch_required", "launch is required", now, nil)
	return decision, nil
}

func stage(state, code, message string, now time.Time, details map[string]any) store.DiagnosticStage {
	return store.DiagnosticStage{
		State:     state,
		Code:      code,
		Message:   message,
		Details:   details,
		UpdatedAt: now,
	}
}

func repoWarmEligible(policy store.Policy, repoOwner, repoName string) bool {
	if !policy.WarmEnabled || policy.WarmMinIdle <= 0 {
		return false
	}
	fullName := strings.ToLower(strings.TrimSpace(repoOwner) + "/" + strings.TrimSpace(repoName))
	for _, item := range policy.WarmRepoAllowlist {
		if strings.EqualFold(strings.TrimSpace(item), fullName) {
			return true
		}
	}
	return false
}

func (s *Service) resolveGitHubClient(ctx context.Context, installationID int64) (*githubapp.Client, error) {
	if s.github == nil {
		return nil, nil
	}
	if installationID > 0 {
		client, err := s.github.ResolveClientByInstallationID(ctx, installationID)
		if err == nil {
			return client, nil
		}
	}
	client, err := s.github.ResolveClient(ctx)
	if err == nil {
		return client, nil
	}
	return nil, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func errorsIsNotFound(err error) bool {
	return errors.Is(err, store.ErrNotFound)
}
