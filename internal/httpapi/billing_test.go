package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"ohoci/internal/githubapp"
	"ohoci/internal/ocibilling"
	"ohoci/internal/store"
)

func TestPolicyBillingReportReturnsReadModel(t *testing.T) {
	ctx := context.Background()
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{
		githubDefaults:      readyGitHubDefaults,
		billingTagNamespace: "ohoci",
	})
	token := authenticatedToken(t, handler.auth)

	policy, err := db.CreatePolicy(ctx, store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E4.Flex",
		OCPU:       1,
		MemoryGB:   16,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..billing",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..billing",
		ImageOCID:          "ocid1.image.oc1..billing",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}
	launchedAt := time.Now().UTC().Add(-2 * time.Hour)
	expiresAt := time.Now().UTC().Add(2 * time.Hour)
	if _, err := db.CreateRunner(ctx, store.Runner{
		PolicyID:       policy.ID,
		JobID:          1,
		InstallationID: 1,
		InstanceOCID:   "ocid1.instance.oc1..billing",
		RepoOwner:      "example",
		RepoName:       "repo",
		RunnerName:     "runner-billing",
		Status:         "completed",
		Labels:         []string{"self-hosted", "oci", "cpu"},
		LaunchedAt:     &launchedAt,
		ExpiresAt:      &expiresAt,
	}); err != nil {
		t.Fatalf("create runner: %v", err)
	}

	response := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/billing/policies?days=7", nil, cfg.SessionCookieName, token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected billing report success, got %d: %s", response.Code, response.Body.String())
	}

	var report ocibilling.PolicyBreakdown
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode billing report: %v", err)
	}
	if !report.TagAttributionReady {
		t.Fatalf("expected tag attribution ready in report")
	}
	if report.TagNamespace != "ohoci" {
		t.Fatalf("expected billing tag namespace ohoci, got %q", report.TagNamespace)
	}
	if len(report.Items) != 1 {
		t.Fatalf("expected one billing item, got %d", len(report.Items))
	}
	if report.Items[0].PolicyID != policy.ID {
		t.Fatalf("expected policy ID %d, got %d", policy.ID, report.Items[0].PolicyID)
	}
}

func TestPolicyBillingReportRejectsInvalidWindow(t *testing.T) {
	ctx := context.Background()
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{
		githubDefaults: githubapp.Config{
			APIBaseURL:                      "https://api.github.com",
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKey,
			WebhookSecret:                   "webhook-secret",
			SelectedRepos:                   []string{"example/repo"},
			AccountLogin:                    "example-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"example/repo"},
		},
	})
	token := authenticatedToken(t, handler.auth)
	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..billing",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..billing",
		ImageOCID:          "ocid1.image.oc1..billing",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}

	response := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/billing/policies?days=0", nil, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid billing window, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "days must be between 1 and 90") {
		t.Fatalf("expected explicit days validation, got %s", response.Body.String())
	}
}
