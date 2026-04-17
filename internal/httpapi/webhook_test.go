package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"ohoci/internal/auth"
	"ohoci/internal/cleanup"
	"ohoci/internal/config"
	"ohoci/internal/githubapp"
	"ohoci/internal/oci"
	"ohoci/internal/ociruntime"
	"ohoci/internal/session"
	"ohoci/internal/store"
)

func TestWebhookRejectsInvalidSignature(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sessions := session.New(db, "secret", time.Hour)
	authService := auth.New(db, sessions)
	gh, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults: githubapp.Config{
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
		EncryptionKey: "httpapi-secret",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}
	stageWebhookAppConfig(t, db, "staged-webhook-secret")
	handler := New(Dependencies{
		Config: config.Config{
			SessionCookieName:     "ohoci_session",
			SessionSecret:         "secret",
			PublicBaseURL:         "http://localhost:8080",
			RunnerDownloadBaseURL: "https://github.com/actions/runner/releases/download",
			RunnerVersion:         "2.325.0",
			RunnerUser:            "runner",
			RunnerWorkDirectory:   "/home/runner/actions-runner",
		},
		Store:    db,
		Auth:     authService,
		Sessions: sessions,
		GitHub:   gh,
		OCI:      &oci.FakeController{Instances: map[string]oci.Instance{}},
		Cleanup:  cleanup.New(db, &oci.FakeController{Instances: map[string]oci.Instance{}}, gh, sessions),
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", bytes.NewBufferString(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":1},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`))
	request.Header.Set("X-GitHub-Event", "workflow_job")
	request.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	request.Header.Set("X-GitHub-Delivery", "delivery-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestWebhookQueuedProcessesNormallyWhenStagedAppExists(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.CreatePolicy(ctx, store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E4.Flex",
		OCPU:       2,
		MemoryGB:   8,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, controller := newWebhookTestHandler(t, db, ghAPI.server.URL)
	stageWebhookAppConfig(t, db, "staged-webhook-secret")

	sendWebhook(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-active-queued",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          11,
				"run_id":      11,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	})

	if len(controller.LaunchRequests) != 1 {
		t.Fatalf("expected one launch request, got %d", len(controller.LaunchRequests))
	}
	if !strings.Contains(controller.LaunchRequests[0].UserData, "actions-runner-linux-x64-2.325.0.tar.gz") {
		t.Fatalf("expected x64 runner download URL, got %q", controller.LaunchRequests[0].UserData)
	}
	jobRecord, err := db.FindJobByGitHubJobID(ctx, 11)
	if err != nil {
		t.Fatalf("find job after active webhook: %v", err)
	}
	if jobRecord.Status != "provisioning" {
		t.Fatalf("expected active webhook to process normally, got job status %q", jobRecord.Status)
	}
}

func TestWebhookQueuedFromEnvFallbackPersistsGitHubTraceSnapshot(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.CreatePolicy(ctx, store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E4.Flex",
		OCPU:       2,
		MemoryGB:   8,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, controller := newWebhookTestHandler(t, db, ghAPI.server.URL)

	sendWebhook(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-env-queued",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          12,
				"run_id":      12,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	})

	jobRecord, err := db.FindJobByGitHubJobID(ctx, 12)
	if err != nil {
		t.Fatalf("find env job: %v", err)
	}
	if jobRecord.GitHubConfigID == 0 {
		t.Fatalf("expected synthetic env config id on job, got %#v", jobRecord)
	}
	if jobRecord.GitHubConfigName != "env-gh-app" || !reflect.DeepEqual(jobRecord.GitHubConfigTags, []string{"env", "fallback"}) {
		t.Fatalf("expected env trace snapshot on job, got %#v", jobRecord)
	}

	runnerRecord, err := db.FindLatestRunnerByJobID(ctx, jobRecord.ID)
	if err != nil {
		t.Fatalf("find env runner: %v", err)
	}
	if runnerRecord.GitHubConfigID != jobRecord.GitHubConfigID || runnerRecord.GitHubConfigName != "env-gh-app" || !reflect.DeepEqual(runnerRecord.GitHubConfigTags, []string{"env", "fallback"}) {
		t.Fatalf("expected env trace snapshot on runner, got %#v", runnerRecord)
	}

	if len(controller.LaunchRequests) != 1 {
		t.Fatalf("expected one launch request, got %d", len(controller.LaunchRequests))
	}
	launchRequest := controller.LaunchRequests[0]
	if launchRequest.FreeformTags[oci.AuditFreeformTagKeyGitHubConfigID] != strconv.FormatInt(jobRecord.GitHubConfigID, 10) {
		t.Fatalf("expected synthetic env config id tag, got %#v", launchRequest.FreeformTags)
	}
	if launchRequest.FreeformTags[oci.AuditFreeformTagKeyGitHubAppName] != "env-gh-app" {
		t.Fatalf("expected env github app name tag, got %#v", launchRequest.FreeformTags)
	}
	if launchRequest.FreeformTags[oci.AuditFreeformTagKeyGitHubAppTags] != "env,fallback" {
		t.Fatalf("expected env github app tags tag, got %#v", launchRequest.FreeformTags)
	}
}

func TestWebhookJobTraceRemainsFromEnvQueueAfterLaterCMSUpdate(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.CreatePolicy(ctx, store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E4.Flex",
		OCPU:       2,
		MemoryGB:   8,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	ghAPI := newGitHubAPIFixture(t, []githubapp.RepositoryRunner{
		{ID: 99, Name: "ohoci-example-repo-12", Status: "online"},
	})
	defer ghAPI.Close()
	handler, _ := newWebhookTestHandler(t, db, ghAPI.server.URL)

	sendWebhook(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-env-queued",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          12,
				"run_id":      12,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	})

	envJob, err := db.FindJobByGitHubJobID(ctx, 12)
	if err != nil {
		t.Fatalf("find env job after queue: %v", err)
	}
	if envJob.GitHubConfigID == 0 || envJob.GitHubConfigName != "env-gh-app" || !reflect.DeepEqual(envJob.GitHubConfigTags, []string{"env", "fallback"}) {
		t.Fatalf("expected env trace snapshot after queue, got %#v", envJob)
	}

	active := saveActiveWebhookAppConfig(t, db, githubapp.Input{
		Name:           "cms-active",
		Tags:           []string{"cms", "active"},
		APIBaseURL:     ghAPI.server.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "cms-active-secret",
		SelectedRepos:  []string{"example/repo"},
	})

	response := sendWebhookWithSecret(t, handler, webhookFixture{
		Action:     "in_progress",
		DeliveryID: "delivery-cms-progress",
		Body: map[string]any{
			"action": "in_progress",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          12,
				"run_id":      12,
				"run_attempt": 1,
				"status":      "in_progress",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	}, "cms-active-secret")
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected in_progress webhook acceptance via CMS config, got %d: %s", response.Code, response.Body.String())
	}

	jobRecord, err := db.FindJobByGitHubJobID(ctx, 12)
	if err != nil {
		t.Fatalf("find job after cms progress: %v", err)
	}
	if jobRecord.Status != "in_progress" {
		t.Fatalf("expected in_progress job, got %#v", jobRecord)
	}
	if jobRecord.GitHubConfigID != envJob.GitHubConfigID || jobRecord.GitHubConfigID == active.Config.ID {
		t.Fatalf("expected original env config id to stay frozen, got %#v", jobRecord)
	}
	if jobRecord.GitHubConfigName != envJob.GitHubConfigName || !reflect.DeepEqual(jobRecord.GitHubConfigTags, envJob.GitHubConfigTags) {
		t.Fatalf("expected original env trace metadata to stay frozen, got %#v", jobRecord)
	}

	runnerRecord, err := db.FindLatestRunnerByJobID(ctx, jobRecord.ID)
	if err != nil {
		t.Fatalf("find runner after cms progress: %v", err)
	}
	if runnerRecord.Status != "in_progress" || runnerRecord.GitHubRunnerID != 99 {
		t.Fatalf("expected runner sync to complete under cms update, got %#v", runnerRecord)
	}
	if runnerRecord.GitHubConfigID != envJob.GitHubConfigID || runnerRecord.GitHubConfigName != envJob.GitHubConfigName || !reflect.DeepEqual(runnerRecord.GitHubConfigTags, envJob.GitHubConfigTags) {
		t.Fatalf("expected runner trace snapshot to remain original env trace, got %#v", runnerRecord)
	}
}

func TestWebhookQueuedMatchesExactActiveConfigAndPersistsAuditTrace(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.CreatePolicy(ctx, store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E4.Flex",
		OCPU:       2,
		MemoryGB:   8,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	handler, controller := newWebhookTestHandler(t, db, "https://api.github.invalid")

	alphaServer := newWebhookConfigServer(t, 123, 456, "alpha-org", "Organization", []string{"example/repo"})
	defer alphaServer.Close()
	alpha := saveActiveWebhookAppConfig(t, db, githubapp.Input{
		Name:           "alpha-prod",
		Tags:           []string{"prod", "alpha"},
		APIBaseURL:     alphaServer.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "alpha-secret",
		SelectedRepos:  []string{"example/repo"},
	})

	betaServer := newWebhookConfigServer(t, 124, 654, "beta-org", "Organization", []string{"example/repo"})
	defer betaServer.Close()
	beta := saveActiveWebhookAppConfig(t, db, githubapp.Input{
		Name:           "beta-stage",
		Tags:           []string{"staging", "beta"},
		APIBaseURL:     betaServer.URL,
		AppID:          124,
		InstallationID: 654,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "beta-secret",
		SelectedRepos:  []string{"example/repo"},
	})

	sendWebhookWithSecret(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-beta-queued",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 654},
			"workflow_job": map[string]any{
				"id":          44,
				"run_id":      44,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	}, "beta-secret")

	if len(controller.LaunchRequests) != 1 {
		t.Fatalf("expected one launch request, got %d", len(controller.LaunchRequests))
	}
	launchRequest := controller.LaunchRequests[0]
	if launchRequest.FreeformTags[oci.AuditFreeformTagKeyGitHubConfigID] != strconv.FormatInt(beta.Config.ID, 10) {
		t.Fatalf("expected beta config id tag, got %#v", launchRequest.FreeformTags)
	}
	if launchRequest.FreeformTags[oci.AuditFreeformTagKeyGitHubAppName] != "beta-stage" {
		t.Fatalf("expected beta config name tag, got %#v", launchRequest.FreeformTags)
	}
	if launchRequest.FreeformTags[oci.AuditFreeformTagKeyGitHubAppTags] != "beta,staging" {
		t.Fatalf("expected beta config tags on freeform tags, got %#v", launchRequest.FreeformTags)
	}
	if launchRequest.DefinedTags[oci.AuditDefinedTagKeyGitHubConfigID] != strconv.FormatInt(beta.Config.ID, 10) {
		t.Fatalf("expected beta config id defined tag, got %#v", launchRequest.DefinedTags)
	}
	if launchRequest.DefinedTags[oci.AuditDefinedTagKeyGitHubAppName] != "beta-stage" {
		t.Fatalf("expected beta config name defined tag, got %#v", launchRequest.DefinedTags)
	}
	if launchRequest.DefinedTags[oci.AuditDefinedTagKeyGitHubAppTags] != "beta,staging" {
		t.Fatalf("expected beta config tags defined tag, got %#v", launchRequest.DefinedTags)
	}

	jobRecord, err := db.FindJobByGitHubJobID(ctx, 44)
	if err != nil {
		t.Fatalf("find job: %v", err)
	}
	if jobRecord.GitHubConfigID != beta.Config.ID || jobRecord.GitHubConfigName != "beta-stage" || !reflect.DeepEqual(jobRecord.GitHubConfigTags, []string{"beta", "staging"}) {
		t.Fatalf("expected beta trace snapshot on job, got %#v", jobRecord)
	}
	if jobRecord.InstallationID != 654 {
		t.Fatalf("expected beta installation id on job, got %d", jobRecord.InstallationID)
	}

	runnerRecord, err := db.FindLatestRunnerByJobID(ctx, jobRecord.ID)
	if err != nil {
		t.Fatalf("find runner: %v", err)
	}
	if runnerRecord.GitHubConfigID != beta.Config.ID || runnerRecord.GitHubConfigName != "beta-stage" || !reflect.DeepEqual(runnerRecord.GitHubConfigTags, []string{"beta", "staging"}) {
		t.Fatalf("expected beta trace snapshot on runner, got %#v", runnerRecord)
	}
	if runnerRecord.InstallationID != 654 {
		t.Fatalf("expected beta installation id on runner, got %d", runnerRecord.InstallationID)
	}

	if alpha.Config.ID == beta.Config.ID {
		t.Fatalf("expected distinct active config ids, got alpha=%d beta=%d", alpha.Config.ID, beta.Config.ID)
	}
}

func TestWebhookRejectsSecretMatchWhenInstallationMismatches(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.CreatePolicy(ctx, store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E4.Flex",
		OCPU:       2,
		MemoryGB:   8,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	handler, controller := newWebhookTestHandler(t, db, "https://api.github.invalid")

	alphaServer := newWebhookConfigServer(t, 123, 456, "alpha-org", "Organization", []string{"example/repo"})
	defer alphaServer.Close()
	_ = saveActiveWebhookAppConfig(t, db, githubapp.Input{
		Name:           "alpha-prod",
		Tags:           []string{"prod", "alpha"},
		APIBaseURL:     alphaServer.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "alpha-secret",
		SelectedRepos:  []string{"example/repo"},
	})

	betaServer := newWebhookConfigServer(t, 124, 654, "beta-org", "Organization", []string{"example/repo"})
	defer betaServer.Close()
	_ = saveActiveWebhookAppConfig(t, db, githubapp.Input{
		Name:           "beta-stage",
		Tags:           []string{"staging", "beta"},
		APIBaseURL:     betaServer.URL,
		AppID:          124,
		InstallationID: 654,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "beta-secret",
		SelectedRepos:  []string{"example/repo"},
	})

	response := sendWebhookWithSecret(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-installation-mismatch",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          45,
				"run_id":      45,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	}, "beta-secret")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected installation mismatch to return 401, got %d: %s", response.Code, response.Body.String())
	}
	if len(controller.LaunchRequests) != 0 {
		t.Fatalf("expected no launch on installation mismatch, got %#v", controller.LaunchRequests)
	}
	if _, err := db.FindJobByGitHubJobID(ctx, 45); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected no job record on installation mismatch, got %v", err)
	}
}

func TestWebhookQueuedFromStagedAppIsIgnored(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, controller := newWebhookTestHandler(t, db, ghAPI.server.URL)
	stageWebhookAppConfig(t, db, "staged-webhook-secret")

	response := sendWebhookWithSecret(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-staged-queued",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          22,
				"run_id":      22,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	}, "staged-webhook-secret")
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Ignored   bool   `json:"ignored"`
		Source    string `json:"source"`
		EventType string `json:"eventType"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Ignored || payload.Source != "staged" || payload.EventType != "workflow_job" {
		t.Fatalf("expected staged workflow_job to be ignored with source marker, got %#v", payload)
	}
	if len(controller.LaunchRequests) != 0 {
		t.Fatalf("expected no launches for staged webhook, got %d", len(controller.LaunchRequests))
	}
	if _, err := db.FindJobByGitHubJobID(ctx, 22); err == nil {
		t.Fatalf("expected staged webhook not to create a job")
	}
	events, err := db.ListEvents(ctx, 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected staged webhook not to create events, got %#v", events)
	}
}

func TestWebhookNonWorkflowJobWithStagedAppSignatureIsIgnored(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, controller := newWebhookTestHandler(t, db, ghAPI.server.URL)
	stageWebhookAppConfig(t, db, "staged-webhook-secret")

	response := sendWebhookEvent(t, handler, "push", webhookFixture{
		DeliveryID: "delivery-staged-push",
		Body: map[string]any{
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
		},
	}, "staged-webhook-secret")
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Ignored   bool   `json:"ignored"`
		Source    string `json:"source"`
		EventType string `json:"eventType"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Ignored || payload.Source != "staged" || payload.EventType != "push" {
		t.Fatalf("expected staged push to be ignored with source marker, got %#v", payload)
	}
	if len(controller.LaunchRequests) != 0 {
		t.Fatalf("expected no launches for ignored non-workflow event, got %d", len(controller.LaunchRequests))
	}
	if _, err := db.FindJobByGitHubJobID(ctx, 1); err == nil {
		t.Fatalf("expected ignored non-workflow webhook not to create a job")
	}
}

func TestWebhookInstallationEventUpdatesStagedAppStatus(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, _ := newWebhookTestHandler(t, db, ghAPI.server.URL)
	stageWebhookAppConfig(t, db, "staged-webhook-secret")

	response := sendWebhookEvent(t, handler, "installation", webhookFixture{
		DeliveryID: "delivery-installation-suspended",
		Body: map[string]any{
			"action": "suspend",
			"installation": map[string]any{
				"id": 456,
				"account": map[string]any{
					"login": "app-org",
					"type":  "Organization",
				},
				"repository_selection": "selected",
			},
		},
	}, "staged-webhook-secret")
	var payload struct {
		Processed bool   `json:"processed"`
		Ignored   bool   `json:"ignored"`
		EventType string `json:"eventType"`
	}
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Processed || payload.Ignored {
		t.Fatalf("expected installation event to be processed, got %#v", payload)
	}

	service, err := githubapp.NewService(db, githubapp.ServiceOptions{
		EncryptionKey: "httpapi-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}
	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current github status: %v", err)
	}
	if status.StagedConfig == nil || status.StagedConfig.InstallationState != "suspended" || status.StagedConfig.InstallationReady {
		t.Fatalf("expected staged app installation to be marked suspended, got %#v", status.StagedConfig)
	}
}

func TestWebhookInstallationEventUpdatesEnvFallbackStatusAfterProcessedResponse(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, controller := newWebhookTestHandler(t, db, ghAPI.server.URL)

	response := sendWebhookEvent(t, handler, "installation", webhookFixture{
		DeliveryID: "delivery-env-installation-suspend",
		Body: map[string]any{
			"action": "suspend",
			"installation": map[string]any{
				"id": 456,
				"account": map[string]any{
					"login": "example-org",
					"type":  "Organization",
				},
				"repository_selection": "selected",
			},
		},
	}, "webhook-secret")
	var payload struct {
		Processed bool   `json:"processed"`
		Ignored   bool   `json:"ignored"`
		EventType string `json:"eventType"`
	}
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Processed || payload.Ignored {
		t.Fatalf("expected env installation event to be processed, got %#v", payload)
	}

	service, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults: githubapp.Config{
			Name:                            "env-gh-app",
			Tags:                            []string{"env", "fallback"},
			APIBaseURL:                      ghAPI.server.URL,
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
		EncryptionKey: "httpapi-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}
	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current github status: %v", err)
	}
	if status.Source != "env" || status.Ready || status.EffectiveConfig.InstallationState != "suspended" || status.EffectiveConfig.InstallationReady {
		t.Fatalf("expected processed env installation event to make env fallback non-ready, got %#v", status)
	}

	workflowResponse := sendWebhookWithSecret(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-env-workflow-after-suspend",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          404,
				"run_id":      404,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	}, "webhook-secret")
	if workflowResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected env workflow webhook to stop routing after suspend, got %d: %s", workflowResponse.Code, workflowResponse.Body.String())
	}
	if len(controller.LaunchRequests) != 0 {
		t.Fatalf("expected no launches after env installation suspend, got %#v", controller.LaunchRequests)
	}
}

func TestWebhookInstallationEventReactivatesSuspendedStoredActiveConfig(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, controller := newWebhookTestHandler(t, db, ghAPI.server.URL)
	active := saveActiveWebhookAppConfig(t, db, githubapp.Input{
		Name:           "cms-active",
		Tags:           []string{"cms", "active"},
		APIBaseURL:     ghAPI.server.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "cms-active-secret",
		SelectedRepos:  []string{"example/repo"},
	})

	service, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults: githubapp.Config{
			Name:                            "env-gh-app",
			Tags:                            []string{"env", "fallback"},
			APIBaseURL:                      ghAPI.server.URL,
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
		EncryptionKey: "httpapi-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}

	activeRecord, err := db.FindActiveGitHubConfigByInstallationID(ctx, 456)
	if err != nil {
		t.Fatalf("find active config: %v", err)
	}
	if err := service.RecordInstallationStatus(ctx, &activeRecord, "suspended", "example-org", "Organization", "selected", []string{"example/repo"}, ""); err != nil {
		t.Fatalf("mark active config suspended: %v", err)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status after suspend: %v", err)
	}
	if status.Source != "env" || len(status.ActiveConfigs) != 0 || status.ActiveConfig != nil {
		t.Fatalf("expected suspended stored active config to be hidden from routable status, got %#v", status)
	}

	workflowResponse := sendWebhookWithSecret(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-suspended-active-workflow",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          46,
				"run_id":      46,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	}, "cms-active-secret")
	if workflowResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected workflow_job to reject suspended active config, got %d: %s", workflowResponse.Code, workflowResponse.Body.String())
	}
	if len(controller.LaunchRequests) != 0 {
		t.Fatalf("expected no launch while active config is suspended, got %#v", controller.LaunchRequests)
	}

	installationResponse := sendWebhookEvent(t, handler, "installation", webhookFixture{
		DeliveryID: "delivery-suspended-active-installation",
		Body: map[string]any{
			"action": "created",
			"installation": map[string]any{
				"id": 456,
				"account": map[string]any{
					"login": "example-org",
					"type":  "Organization",
				},
				"repository_selection": "selected",
			},
		},
	}, "cms-active-secret")
	if installationResponse.Code != http.StatusAccepted {
		t.Fatalf("expected installation event acceptance, got %d: %s", installationResponse.Code, installationResponse.Body.String())
	}
	var payload struct {
		Processed bool `json:"processed"`
	}
	if err := json.Unmarshal(installationResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode installation response: %v", err)
	}
	if !payload.Processed {
		t.Fatalf("expected installation event to reactivate stored active config, got %#v", payload)
	}

	status, err = service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status after installation reactivation: %v", err)
	}
	if status.Source != "cms" || status.ActiveConfig == nil || status.ActiveConfig.ID != active.Config.ID || status.ActiveConfig.InstallationState != "active" || !status.ActiveConfig.InstallationReady {
		t.Fatalf("expected installation event to restore routable active config, got %#v", status)
	}
}

func TestWebhookInstallationRepositoriesEventFallsBackToEnvWhenStoredActiveIsSuspended(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, _ := newWebhookTestHandler(t, db, ghAPI.server.URL)
	_ = saveActiveWebhookAppConfig(t, db, githubapp.Input{
		Name:           "cms-active",
		Tags:           []string{"cms", "active"},
		APIBaseURL:     ghAPI.server.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "cms-active-secret",
		SelectedRepos:  []string{"example/repo"},
	})

	service, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults: githubapp.Config{
			Name:                            "env-gh-app",
			Tags:                            []string{"env", "fallback"},
			APIBaseURL:                      ghAPI.server.URL,
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
		EncryptionKey: "httpapi-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}

	activeRecord, err := db.FindActiveGitHubConfigByInstallationID(ctx, 456)
	if err != nil {
		t.Fatalf("find active config: %v", err)
	}
	if err := service.RecordInstallationStatus(ctx, &activeRecord, "suspended", "example-org", "Organization", "selected", []string{"example/repo"}, ""); err != nil {
		t.Fatalf("mark active config suspended: %v", err)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status after suspend: %v", err)
	}
	if status.Source != "env" || len(status.ActiveConfigs) != 0 || status.ActiveConfig != nil {
		t.Fatalf("expected current status to fall back to env while stored active config is suspended, got %#v", status)
	}

	response := sendWebhookEvent(t, handler, "installation_repositories", webhookFixture{
		DeliveryID: "delivery-env-installation-repositories",
		Body: map[string]any{
			"action": "added",
			"installation": map[string]any{
				"id": 456,
				"account": map[string]any{
					"login": "example-org",
					"type":  "Organization",
				},
				"repository_selection": "selected",
			},
			"repositories_added": []map[string]any{
				{
					"full_name": "example/repo",
					"name":      "repo",
					"private":   true,
					"owner":     map[string]any{"login": "example"},
				},
			},
			"repositories_removed": []any{},
		},
	}, "webhook-secret")
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected env installation_repositories event to be accepted, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Processed bool   `json:"processed"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Processed || payload.Error != "" {
		t.Fatalf("expected env installation_repositories event to process via env fallback, got %#v", payload)
	}
}

func TestWebhookInstallationRepositoriesEventIsProcessed(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, _ := newWebhookTestHandler(t, db, ghAPI.server.URL)
	stageWebhookAppConfig(t, db, "staged-webhook-secret")

	response := sendWebhookEvent(t, handler, "installation_repositories", webhookFixture{
		DeliveryID: "delivery-installation-repositories",
		Body: map[string]any{
			"action": "added",
			"installation": map[string]any{
				"id": 456,
				"account": map[string]any{
					"login": "app-org",
					"type":  "Organization",
				},
				"repository_selection": "selected",
			},
		},
	}, "staged-webhook-secret")
	var payload struct {
		Processed bool `json:"processed"`
		Ignored   bool `json:"ignored"`
	}
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Processed || payload.Ignored {
		t.Fatalf("expected installation_repositories event to be processed, got %#v", payload)
	}
	events, err := db.ListEvents(ctx, 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) == 0 || events[0].EventType != "installation_repositories" || events[0].ProcessedAt == nil {
		t.Fatalf("expected installation_repositories delivery to be recorded as processed, got %#v", events)
	}
}

func TestWebhookRejectsInvalidSignatureWhenActiveAndStagedCoexist(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, _ := newWebhookTestHandler(t, db, ghAPI.server.URL)
	stageWebhookAppConfig(t, db, "staged-webhook-secret")

	response := sendWebhookWithSecret(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-invalid",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          33,
				"run_id":      33,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	}, "wrong-webhook-secret")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", response.Code, response.Body.String())
	}
}

func TestWorkflowJobInProgressSyncsRunnerRegistrationByName(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.CreatePolicy(ctx, store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E4.Flex",
		OCPU:       2,
		MemoryGB:   8,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	ghAPI := newGitHubAPIFixture(t, []githubapp.RepositoryRunner{
		{ID: 99, Name: "ohoci-example-repo-1", Status: "online", Busy: true},
	})
	defer ghAPI.Close()

	handler, _ := newWebhookTestHandler(t, db, ghAPI.server.URL)

	sendWebhook(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-queued",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          1,
				"run_id":      1,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	})

	jobRecord, err := db.FindJobByGitHubJobID(ctx, 1)
	if err != nil {
		t.Fatalf("find job after queue: %v", err)
	}
	if jobRecord.Status != "provisioning" {
		t.Fatalf("expected provisioning job after queue, got %q", jobRecord.Status)
	}
	runnerRecord, err := db.FindLatestRunnerByJobID(ctx, jobRecord.ID)
	if err != nil {
		t.Fatalf("find runner after queue: %v", err)
	}
	if runnerRecord.GitHubRunnerID != 0 {
		t.Fatalf("expected unsynced runner ID after queue, got %d", runnerRecord.GitHubRunnerID)
	}

	sendWebhook(t, handler, webhookFixture{
		Action:     "in_progress",
		DeliveryID: "delivery-progress",
		Body: map[string]any{
			"action": "in_progress",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          1,
				"run_id":      1,
				"run_attempt": 1,
				"status":      "in_progress",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	})

	jobRecord, err = db.FindJobByGitHubJobID(ctx, 1)
	if err != nil {
		t.Fatalf("find job after progress: %v", err)
	}
	if jobRecord.Status != "in_progress" {
		t.Fatalf("expected in_progress job, got %q", jobRecord.Status)
	}
	runnerRecord, err = db.FindLatestRunnerByJobID(ctx, jobRecord.ID)
	if err != nil {
		t.Fatalf("find runner after progress: %v", err)
	}
	if runnerRecord.Status != "in_progress" {
		t.Fatalf("expected in_progress runner, got %q", runnerRecord.Status)
	}
	if runnerRecord.GitHubRunnerID != 99 {
		t.Fatalf("expected synced GitHub runner ID 99, got %d", runnerRecord.GitHubRunnerID)
	}
	if len(ghAPI.runnerLookupNames) == 0 || ghAPI.runnerLookupNames[0] != "ohoci-example-repo-1" {
		t.Fatalf("expected runner lookup by launch name, got %#v", ghAPI.runnerLookupNames)
	}
}

func TestWorkflowJobQueuedUsesCMSRuntimeSubnetOverride(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	policy, err := db.CreatePolicy(ctx, store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E4.Flex",
		OCPU:       2,
		MemoryGB:   8,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		SubnetOCID: "ocid1.subnet.oc1..cms-runtime",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()
	handler, controller := newWebhookTestHandler(t, db, ghAPI.server.URL)
	stageWebhookAppConfig(t, db, "staged-webhook-secret")

	sendWebhook(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-runtime-queued",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          201,
				"run_id":      201,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	})

	if len(controller.LaunchRequests) != 1 {
		t.Fatalf("expected one launch request, got %d", len(controller.LaunchRequests))
	}
	launchRequest := controller.LaunchRequests[0]
	if launchRequest.SubnetID != "ocid1.subnet.oc1..cms-runtime" {
		t.Fatalf("expected CMS runtime subnet override, got %q", launchRequest.SubnetID)
	}
	if launchRequest.FreeformTags[oci.BillingFreeformTagKeyPolicyID] != strconv.FormatInt(policy.ID, 10) {
		t.Fatalf("expected freeform policy tag %d, got %#v", policy.ID, launchRequest.FreeformTags)
	}
	if launchRequest.FreeformTags[oci.BillingFreeformTagKeyRepoOwner] != "example" || launchRequest.FreeformTags[oci.BillingFreeformTagKeyRepoName] != "repo" {
		t.Fatalf("expected repo ownership freeform tags, got %#v", launchRequest.FreeformTags)
	}
	if launchRequest.DefinedTags[oci.BillingDefinedTagKeyPolicyID] != strconv.FormatInt(policy.ID, 10) {
		t.Fatalf("expected defined policy tag %d, got %#v", policy.ID, launchRequest.DefinedTags)
	}
	if launchRequest.DefinedTags[oci.BillingDefinedTagKeyWorkflowJobID] != "201" || launchRequest.DefinedTags[oci.BillingDefinedTagKeyWorkflowRunID] != "201" {
		t.Fatalf("expected workflow defined tags, got %#v", launchRequest.DefinedTags)
	}
}

func TestWorkflowJobCompletedMapsTerminalConclusions(t *testing.T) {
	cases := []struct {
		name       string
		conclusion string
		expected   string
	}{
		{name: "failure", conclusion: "failure", expected: "failed"},
		{name: "cancelled", conclusion: "cancelled", expected: "cancelled"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			if _, err := db.CreatePolicy(ctx, store.Policy{
				Labels:     []string{"oci", "cpu"},
				Shape:      "VM.Standard.E4.Flex",
				OCPU:       2,
				MemoryGB:   8,
				MaxRunners: 1,
				TTLMinutes: 30,
				Enabled:    true,
			}); err != nil {
				t.Fatalf("create policy: %v", err)
			}

			ghAPI := newGitHubAPIFixture(t, nil)
			defer ghAPI.Close()
			handler, controller := newWebhookTestHandler(t, db, ghAPI.server.URL)

			sendWebhook(t, handler, webhookFixture{
				Action:     "queued",
				DeliveryID: "delivery-queued",
				Body: map[string]any{
					"action": "queued",
					"repository": map[string]any{
						"name":  "repo",
						"owner": map[string]any{"login": "example"},
					},
					"installation": map[string]any{"id": 456},
					"workflow_job": map[string]any{
						"id":          1,
						"run_id":      1,
						"run_attempt": 1,
						"status":      "queued",
						"labels":      []string{"self-hosted", "oci", "cpu"},
					},
				},
			})

			sendWebhook(t, handler, webhookFixture{
				Action:     "completed",
				DeliveryID: "delivery-completed-" + tc.name,
				Body: map[string]any{
					"action": "completed",
					"repository": map[string]any{
						"name":  "repo",
						"owner": map[string]any{"login": "example"},
					},
					"installation": map[string]any{"id": 456},
					"workflow_job": map[string]any{
						"id":          1,
						"run_id":      1,
						"run_attempt": 1,
						"status":      "completed",
						"conclusion":  tc.conclusion,
						"runner_id":   77,
						"labels":      []string{"self-hosted", "oci", "cpu"},
					},
				},
			})

			jobRecord, err := db.FindJobByGitHubJobID(ctx, 1)
			if err != nil {
				t.Fatalf("find job after completion: %v", err)
			}
			if jobRecord.Status != tc.expected {
				t.Fatalf("expected job status %q, got %q", tc.expected, jobRecord.Status)
			}

			runnerRecord, err := db.FindLatestRunnerByJobID(ctx, jobRecord.ID)
			if err != nil {
				t.Fatalf("find runner after completion: %v", err)
			}
			if runnerRecord.Status != tc.expected {
				t.Fatalf("expected runner status %q, got %q", tc.expected, runnerRecord.Status)
			}
			if runnerRecord.GitHubRunnerID != 77 {
				t.Fatalf("expected runner ID 77, got %d", runnerRecord.GitHubRunnerID)
			}
			instance, err := controller.GetInstance(ctx, runnerRecord.InstanceOCID)
			if err != nil {
				t.Fatalf("get instance after completion: %v", err)
			}
			if instance.State != "TERMINATED" {
				t.Fatalf("expected terminated instance, got %q", instance.State)
			}
			if len(ghAPI.deletedRunnerIDs) != 1 || ghAPI.deletedRunnerIDs[0] != 77 {
				t.Fatalf("expected GitHub runner delete for 77, got %#v", ghAPI.deletedRunnerIDs)
			}
		})
	}
}

func TestWebhookQueuedFailsWhenShapeArchitectureCannotBeDerived(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.CreatePolicy(ctx, store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.Custom.Flex",
		OCPU:       2,
		MemoryGB:   16,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	ghAPI := newGitHubAPIFixture(t, nil)
	defer ghAPI.Close()

	controller := newStaticCatalogController(oci.CatalogResponse{
		Shapes: []oci.CatalogShape{
			{
				Shape:                "VM.Standard.Custom.Flex",
				ProcessorDescription: "Mystery Accelerator",
				IsFlexible:           true,
				OCPUMin:              1,
				OCPUMax:              4,
				MemoryMinGB:          8,
				MemoryMaxGB:          64,
			},
		},
	})
	handler := newWebhookTestHandlerWithController(t, db, ghAPI.server.URL, controller)

	sendWebhook(t, handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-unknown-arch",
		Body: map[string]any{
			"action": "queued",
			"repository": map[string]any{
				"name":  "repo",
				"owner": map[string]any{"login": "example"},
			},
			"installation": map[string]any{"id": 456},
			"workflow_job": map[string]any{
				"id":          301,
				"run_id":      301,
				"run_attempt": 1,
				"status":      "queued",
				"labels":      []string{"self-hosted", "oci", "cpu"},
			},
		},
	})

	if len(controller.LaunchRequests) != 0 {
		t.Fatalf("expected launch to fail before instance creation, got %d launch requests", len(controller.LaunchRequests))
	}
	jobRecord, err := db.FindJobByGitHubJobID(ctx, 301)
	if err != nil {
		t.Fatalf("find job after failed launch: %v", err)
	}
	if jobRecord.Status != "failed" {
		t.Fatalf("expected failed job status, got %q", jobRecord.Status)
	}
	if !strings.Contains(jobRecord.ErrorMessage, "runner architecture could not be determined") {
		t.Fatalf("expected runner architecture error, got %q", jobRecord.ErrorMessage)
	}
}

func TestValidateSignatureHelper(t *testing.T) {
	client, err := githubapp.New(githubapp.Config{
		APIBaseURL:     "https://api.github.com",
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "webhook-secret",
		SelectedRepos:  []string{"example/repo"},
	})
	if err != nil {
		t.Fatalf("new github client: %v", err)
	}
	body := []byte(`{"hello":"world"}`)
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !client.ValidateWebhookSignature(body, signature) {
		t.Fatalf("expected signature to validate")
	}
}

type webhookFixture struct {
	Action     string
	DeliveryID string
	Body       map[string]any
}

type gitHubAPIFixture struct {
	server            *httptest.Server
	URL               string
	runners           map[string]githubapp.RepositoryRunner
	runnerLookupNames []string
	deletedRunnerIDs  []int64
}

func newWebhookTestHandler(t *testing.T, db *store.Store, apiBaseURL string) (http.Handler, *oci.FakeController) {
	t.Helper()
	controller := &oci.FakeController{Instances: map[string]oci.Instance{}}
	return newWebhookTestHandlerWithController(t, db, apiBaseURL, controller), controller
}

func newWebhookTestHandlerWithController(t *testing.T, db *store.Store, apiBaseURL string, controller oci.Controller) http.Handler {
	t.Helper()
	sessions := session.New(db, "secret", time.Hour)
	authService := auth.New(db, sessions)
	gh, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults: githubapp.Config{
			Name:                            "env-gh-app",
			Tags:                            []string{"env", "fallback"},
			APIBaseURL:                      apiBaseURL,
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
		EncryptionKey: "httpapi-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}
	ociRuntime := ociruntime.New(db, ociruntime.Defaults{
		CompartmentID:      "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetID:           "ocid1.subnet.oc1..regional",
		ImageID:            "ocid1.image.oc1..ubuntu",
	})
	ociRuntime.SetCatalogController(controller)
	return New(Dependencies{
		Config: config.Config{
			SessionCookieName:      "ohoci_session",
			SessionSecret:          "secret",
			PublicBaseURL:          "http://localhost:8080",
			OCIAuthMode:            "fake",
			OCIBillingTagNamespace: "ohoci",
			RunnerDownloadBaseURL:  "https://github.com/actions/runner/releases/download",
			RunnerVersion:          "2.325.0",
			RunnerUser:             "runner",
			RunnerWorkDirectory:    "/home/runner/actions-runner",
		},
		Store:      db,
		Auth:       authService,
		Sessions:   sessions,
		GitHub:     gh,
		OCI:        controller,
		OCIRuntime: ociRuntime,
		Cleanup:    cleanup.New(db, controller, gh, sessions),
	})
}

func newGitHubAPIFixture(t *testing.T, runners []githubapp.RepositoryRunner) *gitHubAPIFixture {
	t.Helper()
	fixture := &gitHubAPIFixture{
		runners: map[string]githubapp.RepositoryRunner{},
	}
	for _, runner := range runners {
		fixture.runners[runner.Name] = runner
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app":
			writeJSONBody(t, w, map[string]any{"id": 123})
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/456/access_tokens":
			writeJSONBody(t, w, map[string]any{"token": "installation-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations/456":
			writeJSONBody(t, w, map[string]any{
				"account": map[string]any{
					"login": "example-org",
					"type":  "Organization",
				},
				"repository_selection": "selected",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			writeJSONBody(t, w, map[string]any{
				"repositories": []map[string]any{
					{
						"full_name": "example/repo",
						"name":      "repo",
						"private":   true,
						"owner":     map[string]any{"login": "example"},
						"permissions": map[string]any{
							"admin": true,
						},
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/example/repo/actions/runners/registration-token":
			writeJSONBody(t, w, map[string]any{
				"token":      "runner-registration-token",
				"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/example/repo/actions/runners":
			name := r.URL.Query().Get("name")
			if name != "" {
				fixture.runnerLookupNames = append(fixture.runnerLookupNames, name)
			}
			items := []githubapp.RepositoryRunner{}
			if name != "" {
				if runner, ok := fixture.runners[name]; ok {
					items = append(items, runner)
				}
			} else {
				for _, runner := range fixture.runners {
					items = append(items, runner)
				}
			}
			writeJSONBody(t, w, map[string]any{"runners": items})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/repos/example/repo/actions/runners/"):
			idText := strings.TrimPrefix(r.URL.Path, "/repos/example/repo/actions/runners/")
			id, err := strconv.ParseInt(idText, 10, 64)
			if err != nil {
				t.Fatalf("parse delete id: %v", err)
			}
			fixture.deletedRunnerIDs = append(fixture.deletedRunnerIDs, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	fixture.URL = fixture.server.URL
	return fixture
}

func (f *gitHubAPIFixture) Close() {
	if f.server != nil {
		f.server.Close()
	}
}

func sendWebhook(t *testing.T, handler http.Handler, fixture webhookFixture) {
	t.Helper()
	body, err := json.Marshal(fixture.Body)
	if err != nil {
		t.Fatalf("marshal webhook body: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", bytes.NewReader(body))
	request.Header.Set("X-GitHub-Event", "workflow_job")
	request.Header.Set("X-GitHub-Delivery", fixture.DeliveryID)
	request.Header.Set("X-Hub-Signature-256", signWebhookBody(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for %s, got %d: %s", fixture.Action, response.Code, response.Body.String())
	}
}

func sendWebhookWithSecret(t *testing.T, handler http.Handler, fixture webhookFixture, secret string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(fixture.Body)
	if err != nil {
		t.Fatalf("marshal webhook body: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", bytes.NewReader(body))
	request.Header.Set("X-GitHub-Event", "workflow_job")
	request.Header.Set("X-GitHub-Delivery", fixture.DeliveryID)
	request.Header.Set("X-Hub-Signature-256", signWebhookBodyWithSecret(body, secret))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func sendWebhookEvent(t *testing.T, handler http.Handler, eventType string, fixture webhookFixture, secret string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(fixture.Body)
	if err != nil {
		t.Fatalf("marshal webhook body: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", bytes.NewReader(body))
	request.Header.Set("X-GitHub-Event", eventType)
	request.Header.Set("X-GitHub-Delivery", fixture.DeliveryID)
	request.Header.Set("X-Hub-Signature-256", signWebhookBodyWithSecret(body, secret))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func signWebhookBody(body []byte) string {
	return signWebhookBodyWithSecret(body, "webhook-secret")
}

func signWebhookBodyWithSecret(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func writeJSONBody(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode payload: %v", err)
	}
}

func stageWebhookAppConfig(t *testing.T, db *store.Store, webhookSecret string) {
	t.Helper()
	appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app":
			writeJSONBody(t, w, map[string]any{"id": 123})
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/456/access_tokens":
			writeJSONBody(t, w, map[string]any{"token": "installation-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations/456":
			writeJSONBody(t, w, map[string]any{
				"account": map[string]any{
					"login": "app-org",
					"type":  "Organization",
				},
				"repository_selection": "selected",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			writeJSONBody(t, w, map[string]any{
				"repositories": []map[string]any{
					{
						"full_name":   "app-org/repo-a",
						"name":        "repo-a",
						"private":     true,
						"owner":       map[string]any{"login": "app-org"},
						"permissions": map[string]any{"admin": true},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(appServer.Close)

	service, err := githubapp.NewService(db, githubapp.ServiceOptions{
		EncryptionKey: "httpapi-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}
	if _, err := service.SaveStagedApp(context.Background(), githubapp.Input{
		APIBaseURL:     appServer.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  webhookSecret,
		SelectedRepos:  []string{"app-org/repo-a"},
	}); err != nil {
		t.Fatalf("save staged app config: %v", err)
	}
}

func saveActiveWebhookAppConfig(t *testing.T, db *store.Store, input githubapp.Input) githubapp.TestResult {
	t.Helper()
	service, err := githubapp.NewService(db, githubapp.ServiceOptions{
		EncryptionKey: "httpapi-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}
	result, err := service.Save(context.Background(), input)
	if err != nil {
		t.Fatalf("save active app config: %v", err)
	}
	return result
}

func newWebhookConfigServer(t *testing.T, appID, installationID int64, accountLogin, accountType string, repositories []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app":
			writeJSONBody(t, w, map[string]any{"id": appID})
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/"+strconv.FormatInt(installationID, 10)+"/access_tokens":
			writeJSONBody(t, w, map[string]any{"token": "installation-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations/"+strconv.FormatInt(installationID, 10):
			writeJSONBody(t, w, map[string]any{
				"account": map[string]any{
					"login": accountLogin,
					"type":  accountType,
				},
				"repository_selection": "selected",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			items := make([]map[string]any, 0, len(repositories))
			for _, repository := range repositories {
				parts := strings.SplitN(repository, "/", 2)
				if len(parts) != 2 {
					t.Fatalf("invalid repository fixture %q", repository)
				}
				items = append(items, map[string]any{
					"full_name": repository,
					"name":      parts[1],
					"private":   true,
					"owner":     map[string]any{"login": parts[0]},
					"permissions": map[string]any{
						"admin": true,
					},
				})
			}
			writeJSONBody(t, w, map[string]any{"repositories": items})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/example/repo/actions/runners/registration-token":
			writeJSONBody(t, w, map[string]any{
				"token":      "runner-registration-token",
				"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

const testPrivateKey = "-----BEGIN PRIVATE KEY-----\nMIICdgIBADANBgkqhkiG9w0BAQEFAASCAmAwggJcAgEAAoGBAOW0ED5HhOi+am89\n+A8Gs84lcTxj95fyY/m4El01AaOMwB6Ufnx8lIIY7abn71exSaKDzsFNEM+uBkdH\nW8mG+Lna3TGmRS52G46DnulBiREnpRV+NIQwMjZHpQ5WvW9nzePZ4navmdnhyrcE\npYA3vKJKND/p8+8mlD0G8CfD0Ko3AgMBAAECgYA1HvMys+90s7SBjV80emRSpC4P\nvT6hERk1wu/cRknevMohSE4IE/d0LrenBbRAH2vb/YdvBJeCr8gb69C6RlB2mo25\ngMv8A+zggDGyIJEq5JCIGsFWa463bd8P/Y+tZ6ZsCULVuksWl+suvhoJvr3zBeeM\neQMF3rd8hzhYa5iqYQJBAPakEcZAcMAQWcjzBQKmdZoP+zXvExMOrDlFKeqsbeWP\nVHFrpcZ+t/A3SwKKOmX5Ie50rPtCBi+2NfLYYebGnv0CQQDua3Vvomv1zyJmuEi+\nHr+rqHtzjjA8vVUCK8Tb9UEqWLZ3JQNcoGvgHUZrw3Euq1nqvOYYHsZGTLXSIrlu\nwaZDAkBa+tSvq++reZyVGsgbXSn+ZazGDWWc3wm6qn+22FpFluSQXiQtn2rcipj5\n2+GE4iyZGKMCoC1GBlHKPfWHOndFAkAwso44EQrQGFDEfluNSaaIn08n2SENJvbY\nDKyW6M84oQoT5+F55+Jg0lnx5OeXSrSA97hfsNl6vmxc0W7iqncVAkEArERxQtrn\nd/fYemHb9Wv5ibLOZWoPCNy2WACMGyHQ7+3+pB/IxI9ueUrnrRaCAQLkDuhF82sW\nnApG0TpVWHyZUQ==\n-----END PRIVATE KEY-----"
