package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"ohoci/internal/store"
)

func TestPolicyCompatibilityCheckReturnsBudgetBlocked(t *testing.T) {
	ctx := context.Background()
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{githubDefaults: readyGitHubDefaults})
	token := authenticatedToken(t, handler.auth)

	markRuntimeReady(t, db)

	policyRecord, err := db.CreatePolicy(ctx, store.Policy{
		Labels:           []string{"oci", "cpu"},
		Shape:            "VM.Standard.E4.Flex",
		OCPU:             2,
		MemoryGB:         8,
		MaxRunners:       1,
		TTLMinutes:       30,
		Enabled:          true,
		BudgetEnabled:    true,
		BudgetCapAmount:  10,
		BudgetWindowDays: 7,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if _, err := db.UpsertBillingPolicySnapshot(ctx, store.BillingPolicySnapshot{
		PolicyID:    policyRecord.ID,
		WindowDays:  7,
		Currency:    "USD",
		TotalCost:   12,
		GeneratedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert billing snapshot: %v", err)
	}

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/policies/compatibility-check", map[string]any{
		"repoOwner": "example",
		"repoName":  "repo",
		"labels":    []string{"self-hosted", "oci", "cpu"},
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected compatibility check success, got %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		NormalizedLabels []string `json:"normalizedLabels"`
		BlockingStage    string   `json:"blockingStage"`
		SummaryCode      string   `json:"summaryCode"`
		PolicyChecks     []struct {
			PolicyID      int64  `json:"policyId"`
			BudgetBlocked bool   `json:"budgetBlocked"`
			BudgetMessage string `json:"budgetMessage"`
		} `json:"policyChecks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.BlockingStage != "budget_ok" || payload.SummaryCode != "budget_blocked" {
		t.Fatalf("expected budget block response, got %#v", payload)
	}
	if len(payload.NormalizedLabels) != 2 || payload.NormalizedLabels[0] != "cpu" || payload.NormalizedLabels[1] != "oci" {
		t.Fatalf("expected normalized labels, got %#v", payload.NormalizedLabels)
	}
	if len(payload.PolicyChecks) != 1 || payload.PolicyChecks[0].PolicyID != policyRecord.ID || !payload.PolicyChecks[0].BudgetBlocked {
		t.Fatalf("expected blocked policy check, got %#v", payload.PolicyChecks)
	}
}

func TestJobDiagnosticsEndpointReturnsStructuredPayload(t *testing.T) {
	ctx := context.Background()
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{githubDefaults: readyGitHubDefaults})
	token := authenticatedToken(t, handler.auth)

	markRuntimeReady(t, db)

	jobRecord, err := db.UpsertJob(ctx, store.Job{
		GitHubJobID:    401,
		DeliveryID:     "delivery-401",
		InstallationID: readyGitHubDefaults.InstallationID,
		RepoOwner:      "example",
		RepoName:       "repo",
		RunID:          401,
		RunAttempt:     1,
		Status:         "queued",
		Labels:         []string{"self-hosted", "oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("upsert job: %v", err)
	}
	if _, err := db.UpsertJobDiagnostic(ctx, store.JobDiagnostic{
		JobID:         jobRecord.ID,
		DeliveryID:    jobRecord.DeliveryID,
		SummaryCode:   "budget_blocked",
		BlockingStage: "budget_ok",
		StageStatuses: map[string]store.DiagnosticStage{
			"policy_match": {
				State:     "passed",
				Code:      "policy_matched",
				Message:   "policy matched",
				UpdatedAt: time.Now().UTC(),
			},
			"budget_ok": {
				State:     "blocked",
				Code:      "budget_blocked",
				Message:   "policy budget cap reached",
				UpdatedAt: time.Now().UTC(),
			},
		},
	}); err != nil {
		t.Fatalf("upsert diagnostics: %v", err)
	}

	response := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/jobs/"+jsonInt64(jobRecord.ID)+"/diagnostics", nil, cfg.SessionCookieName, token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected diagnostics success, got %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Job struct {
			ID int64 `json:"id"`
		} `json:"job"`
		Diagnostic struct {
			JobID         int64                            `json:"jobId"`
			SummaryCode   string                           `json:"summaryCode"`
			BlockingStage string                           `json:"blockingStage"`
			StageStatuses map[string]store.DiagnosticStage `json:"stageStatuses"`
		} `json:"diagnostic"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Job.ID != jobRecord.ID || payload.Diagnostic.JobID != jobRecord.ID {
		t.Fatalf("expected job and diagnostic payload for %d, got %#v", jobRecord.ID, payload)
	}
	if payload.Diagnostic.SummaryCode != "budget_blocked" || payload.Diagnostic.BlockingStage != "budget_ok" {
		t.Fatalf("expected structured diagnostic summary, got %#v", payload.Diagnostic)
	}
}

func TestGitHubDriftEndpointsReturnStatusAndReconcile(t *testing.T) {
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{githubDefaults: readyGitHubDefaults})
	token := authenticatedToken(t, handler.auth)

	markRuntimeReady(t, db)

	getResponse := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/github/drift", nil, cfg.SessionCookieName, token)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("expected drift status success, got %d: %s", getResponse.Code, getResponse.Body.String())
	}
	postResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/drift/reconcile", nil, cfg.SessionCookieName, token)
	if postResponse.Code != http.StatusOK {
		t.Fatalf("expected drift reconcile success, got %d: %s", postResponse.Code, postResponse.Body.String())
	}

	var payload struct {
		Severity string `json:"severity"`
	}
	if err := json.Unmarshal(postResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Severity == "" {
		t.Fatalf("expected reconcile payload to include severity, got %s", postResponse.Body.String())
	}
}

func TestBillingGuardrailsEndpointReturnsSnapshotStatus(t *testing.T) {
	ctx := context.Background()
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{githubDefaults: readyGitHubDefaults})
	token := authenticatedToken(t, handler.auth)

	markRuntimeReady(t, db)

	policyRecord, err := db.CreatePolicy(ctx, store.Policy{
		Labels:           []string{"oci", "cpu"},
		Shape:            "VM.Standard.E4.Flex",
		OCPU:             2,
		MemoryGB:         8,
		MaxRunners:       1,
		TTLMinutes:       30,
		Enabled:          true,
		BudgetEnabled:    true,
		BudgetCapAmount:  10,
		BudgetWindowDays: 7,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if _, err := db.UpsertBillingPolicySnapshot(ctx, store.BillingPolicySnapshot{
		PolicyID:    policyRecord.ID,
		WindowDays:  7,
		Currency:    "USD",
		TotalCost:   4.5,
		GeneratedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert billing snapshot: %v", err)
	}

	response := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/billing/guardrails", nil, cfg.SessionCookieName, token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected guardrails success, got %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Items []struct {
			PolicyID  int64   `json:"policyId"`
			Currency  string  `json:"currency"`
			TotalCost float64 `json:"totalCost"`
			Blocked   bool    `json:"blocked"`
			Degraded  bool    `json:"degraded"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].PolicyID != policyRecord.ID {
		t.Fatalf("expected one guardrail item, got %#v", payload.Items)
	}
	if payload.Items[0].Currency != "USD" || payload.Items[0].TotalCost != 4.5 || payload.Items[0].Blocked || payload.Items[0].Degraded {
		t.Fatalf("expected healthy guardrail snapshot, got %#v", payload.Items[0])
	}
}

func TestCreatePolicyRejectsWarmMinIdleAboveOne(t *testing.T) {
	ctx := context.Background()
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{githubDefaults: readyGitHubDefaults})
	token := authenticatedToken(t, handler.auth)

	markRuntimeReady(t, db)

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/policies", store.Policy{
		Labels:            []string{"oci", "cpu"},
		Shape:             "VM.Standard.E4.Flex",
		OCPU:              2,
		MemoryGB:          8,
		MaxRunners:        1,
		TTLMinutes:        30,
		Enabled:           true,
		WarmEnabled:       true,
		WarmMinIdle:       2,
		WarmTTLMinutes:    30,
		WarmRepoAllowlist: []string{"example/repo"},
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected warmMinIdle contract rejection, got %d: %s", response.Code, response.Body.String())
	}
	_ = ctx
}

func TestCreatePolicyRejectsNonSevenBudgetWindow(t *testing.T) {
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{githubDefaults: readyGitHubDefaults})
	token := authenticatedToken(t, handler.auth)

	markRuntimeReady(t, db)

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/policies", store.Policy{
		Labels:           []string{"oci", "cpu"},
		Shape:            "VM.Standard.E4.Flex",
		OCPU:             2,
		MemoryGB:         8,
		MaxRunners:       1,
		TTLMinutes:       30,
		Enabled:          true,
		BudgetEnabled:    true,
		BudgetCapAmount:  10,
		BudgetWindowDays: 14,
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected budgetWindowDays contract rejection, got %d: %s", response.Code, response.Body.String())
	}
}

func markRuntimeReady(t *testing.T, db *store.Store) {
	t.Helper()
	if _, err := db.SaveOCIRuntimeSettings(context.Background(), store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..ad1",
		ImageOCID:          "ocid1.image.oc1..ubuntu",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}
}

func jsonInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}
