package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ohoci/internal/admission"
	"ohoci/internal/githubapp"
	"ohoci/internal/ocibilling"
	"ohoci/internal/policy"
	"ohoci/internal/store"
)

type jobListItem struct {
	store.Job
	SummaryCode     string               `json:"summaryCode,omitempty"`
	BlockingStage   string               `json:"blockingStage,omitempty"`
	BlockingMessage string               `json:"blockingMessage,omitempty"`
	Diagnostic      *store.JobDiagnostic `json:"diagnostic,omitempty"`
}

type compatibilityCheckRequest struct {
	RepoOwner string   `json:"repoOwner"`
	RepoName  string   `json:"repoName"`
	Labels    []string `json:"labels"`
}

type compatibilityCheckResponse struct {
	NormalizedLabels []string                         `json:"normalizedLabels"`
	BlockingStage    string                           `json:"blockingStage,omitempty"`
	SummaryCode      string                           `json:"summaryCode"`
	MatchedPolicy    *store.Policy                    `json:"matchedPolicy,omitempty"`
	LaunchRequired   bool                             `json:"launchRequired"`
	WarmCandidate    *store.Runner                    `json:"warmCandidate,omitempty"`
	StageStatuses    map[string]store.DiagnosticStage `json:"stageStatuses"`
	PolicyChecks     []admission.PolicyCheck          `json:"policyChecks"`
}

type billingPoliciesResponse struct {
	ocibilling.PolicyBreakdown
	Guardrails ocibilling.GuardrailReport `json:"guardrails"`
}

func listJobItems(ctx context.Context, deps Dependencies) ([]jobListItem, error) {
	jobs, err := deps.Store.ListJobs(ctx, 100)
	if err != nil {
		return nil, err
	}
	jobIDs := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		jobIDs = append(jobIDs, job.ID)
	}
	diagnostics, err := deps.Store.ListJobDiagnosticsByJobIDs(ctx, jobIDs)
	if err != nil {
		return nil, err
	}
	items := make([]jobListItem, 0, len(jobs))
	for _, job := range jobs {
		item := jobListItem{Job: job}
		if diagnostic, ok := diagnostics[job.ID]; ok {
			diagnosticCopy := diagnostic
			item.Diagnostic = &diagnosticCopy
			item.SummaryCode = strings.TrimSpace(diagnostic.SummaryCode)
			item.BlockingStage = strings.TrimSpace(diagnostic.BlockingStage)
			item.BlockingMessage = blockingDiagnosticMessage(diagnostic)
		}
		if item.BlockingMessage == "" {
			item.BlockingMessage = strings.TrimSpace(job.ErrorMessage)
		}
		items = append(items, item)
	}
	return items, nil
}

func blockingDiagnosticMessage(diagnostic store.JobDiagnostic) string {
	if diagnostic.BlockingStage != "" {
		if status, ok := diagnostic.StageStatuses[diagnostic.BlockingStage]; ok {
			return strings.TrimSpace(status.Message)
		}
	}
	for _, stageName := range []string{
		admission.StageSetupReady,
		admission.StageRepoAllowed,
		admission.StagePolicyMatch,
		admission.StageCapacityOK,
		admission.StageBudgetOK,
		admission.StageWarmCandidate,
		admission.StageLaunchRequired,
		admission.StageRunnerRegistration,
		admission.StageRunnerAttachment,
		admission.StageCleanup,
	} {
		status, ok := diagnostic.StageStatuses[stageName]
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(status.State), "blocked") {
			return strings.TrimSpace(status.Message)
		}
	}
	return ""
}

func policyCompatibilityCheckHandler(deps Dependencies) http.Handler {
	return requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		var payload compatibilityCheckRequest
		if err := decodeJSON(r, &payload); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		payload.RepoOwner = strings.TrimSpace(payload.RepoOwner)
		payload.RepoName = strings.TrimSpace(payload.RepoName)
		if payload.RepoOwner == "" || payload.RepoName == "" {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "repoOwner and repoName are required"})
		}
		var installationID int64
		if deps.GitHub != nil {
			_, record, err := deps.GitHub.ResolveClientForRepository(r.Context(), payload.RepoOwner, payload.RepoName)
			switch {
			case err == nil && record != nil:
				installationID = record.InstallationID
			case err != nil && !errors.Is(err, githubapp.ErrNotConfigured):
				return err
			}
		}
		decision, err := resolveAdmissionService(deps).Evaluate(r.Context(), admission.Input{
			InstallationID: installationID,
			RepoOwner:      payload.RepoOwner,
			RepoName:       payload.RepoName,
			Labels:         payload.Labels,
		})
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, compatibilityCheckResponse{
			NormalizedLabels: policy.ManagedLabels(payload.Labels),
			BlockingStage:    decision.BlockingStage,
			SummaryCode:      decision.SummaryCode,
			MatchedPolicy:    decision.MatchedPolicy,
			LaunchRequired:   decision.LaunchRequired,
			WarmCandidate:    decision.WarmCandidate,
			StageStatuses:    decision.StageStatuses,
			PolicyChecks:     decision.PolicyChecks,
		})
	})))
}

func jobDiagnosticsHandler(deps Dependencies) http.Handler {
	return requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		jobID, err := pathIDBetween(r.URL.Path, "/api/v1/jobs/", "/diagnostics")
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid job id"})
		}
		job, err := deps.Store.FindJobByID(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return writeStatus(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			}
			return err
		}
		diagnostic, err := deps.Store.FindJobDiagnosticByJobID(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return writeStatus(w, http.StatusNotFound, map[string]string{"error": "job diagnostics not found"})
			}
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{
			"job":        job,
			"diagnostic": diagnostic,
		})
	})))
}

func githubDriftHandler(deps Dependencies) http.Handler {
	return requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.GitHub == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "GitHub config service is not configured"})
		}
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		status, err := deps.GitHub.CurrentDrift(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, status)
	})))
}

func githubDriftReconcileHandler(deps Dependencies) http.Handler {
	return requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.GitHub == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "GitHub config service is not configured"})
		}
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		status, err := deps.GitHub.ReconcileDrift(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, status)
	})))
}

func billingGuardrailsHandler(deps Dependencies) http.Handler {
	return requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.OCIBilling == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "OCI billing service is not configured"})
		}
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		report, err := deps.OCIBilling.PolicyGuardrails(r.Context(), ocibilling.DefaultBudgetWindowDays)
		if err != nil {
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, report)
	})))
}

func billingPoliciesHandler(deps Dependencies) http.Handler {
	return requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		if deps.OCIBilling == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "OCI billing service is not configured"})
		}
		days, err := billingDaysFromQuery(r)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		windowEnd := time.Now().UTC()
		windowStart := windowEnd.AddDate(0, 0, -days)
		report, err := deps.OCIBilling.PolicyBreakdown(r.Context(), ocibilling.PolicyBreakdownRequest{
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
		})
		if err != nil {
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		guardrails, err := deps.OCIBilling.PolicyGuardrails(r.Context(), ocibilling.DefaultBudgetWindowDays)
		if err != nil {
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, billingPoliciesResponse{
			PolicyBreakdown: report,
			Guardrails:      guardrails,
		})
	})))
}

func pathIDBetween(pathValue, prefix, suffix string) (int64, error) {
	value := strings.TrimPrefix(pathValue, prefix)
	value = strings.TrimSuffix(value, suffix)
	value = strings.Trim(value, "/")
	return strconv.ParseInt(value, 10, 64)
}

func billingDaysFromQuery(r *http.Request) (int, error) {
	days := 7
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 90 {
			return 0, errors.New("days must be between 1 and 90")
		}
		days = parsed
	}
	return days, nil
}
