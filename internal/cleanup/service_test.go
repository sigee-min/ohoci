package cleanup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"ohoci/internal/githubapp"
	"ohoci/internal/oci"
	"ohoci/internal/session"
	"ohoci/internal/store"
)

func TestCleanupTerminatesExpiredRunner(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	policyRecord, err := db.CreatePolicy(ctx, store.Policy{Labels: []string{"oci", "cpu"}, Shape: "VM.Standard.A1.Flex", OCPU: 2, MemoryGB: 8, MaxRunners: 1, TTLMinutes: 30, Enabled: true})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	jobRecord, err := db.UpsertJob(ctx, store.Job{GitHubJobID: 1, DeliveryID: "delivery-1", InstallationID: 456, RepoOwner: "example", RepoName: "repo", RunID: 1, RunAttempt: 1, Status: "queued", Labels: []string{"oci", "cpu"}})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	controller := &oci.FakeController{Instances: map[string]oci.Instance{}}
	instance, err := controller.LaunchInstance(ctx, oci.LaunchRequest{DisplayName: "runner-1", Shape: "VM.Standard.A1.Flex"})
	if err != nil {
		t.Fatalf("launch instance: %v", err)
	}
	expired := time.Now().Add(-time.Minute)
	runnerRecord, err := db.CreateRunner(ctx, store.Runner{PolicyID: policyRecord.ID, JobID: jobRecord.ID, InstallationID: 456, InstanceOCID: instance.ID, RepoOwner: "example", RepoName: "repo", RunnerName: "runner-1", Status: "launching", Labels: []string{"oci", "cpu"}, ExpiresAt: &expired})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}
	ghAPI := newCleanupGitHubAPIFixture(t, []githubapp.RepositoryRunner{
		{ID: 77, Name: "runner-1", Status: "online", Busy: false},
	})
	defer ghAPI.Close()
	githubService, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults: githubapp.Config{
			APIBaseURL:                      ghAPI.server.URL,
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKey,
			WebhookSecret:                   "secret",
			SelectedRepos:                   []string{"example/repo"},
			AccountLogin:                    "example-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"example/repo"},
		},
		EncryptionKey: "cleanup-secret",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}
	service := New(db, controller, githubService, session.New(db, "secret", time.Hour))
	result, err := service.RunOnce(ctx)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.Terminated != 1 {
		t.Fatalf("expected 1 terminated runner, got %d", result.Terminated)
	}
	updatedRunner, err := db.FindRunnerByID(ctx, runnerRecord.ID)
	if err != nil {
		t.Fatalf("find runner after cleanup: %v", err)
	}
	if updatedRunner.Status != "terminating" {
		t.Fatalf("expected terminating runner, got %q", updatedRunner.Status)
	}
	if updatedRunner.GitHubRunnerID != 77 {
		t.Fatalf("expected synced runner ID 77, got %d", updatedRunner.GitHubRunnerID)
	}
	if len(ghAPI.deletedRunnerIDs) != 1 || ghAPI.deletedRunnerIDs[0] != 77 {
		t.Fatalf("expected GitHub runner 77 to be deleted, got %#v", ghAPI.deletedRunnerIDs)
	}
}

func TestCleanupDeletesRunnerAfterOCITerminalState(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	policyRecord, err := db.CreatePolicy(ctx, store.Policy{Labels: []string{"oci", "cpu"}, Shape: "VM.Standard.A1.Flex", OCPU: 2, MemoryGB: 8, MaxRunners: 1, TTLMinutes: 30, Enabled: true})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	jobRecord, err := db.UpsertJob(ctx, store.Job{GitHubJobID: 2, DeliveryID: "delivery-2", InstallationID: 456, RepoOwner: "example", RepoName: "repo", RunID: 2, RunAttempt: 1, Status: "completed", Labels: []string{"oci", "cpu"}})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	controller := &oci.FakeController{Instances: map[string]oci.Instance{
		"ocid1.instance.oc1..terminal": {
			ID:          "ocid1.instance.oc1..terminal",
			DisplayName: "runner-2",
			State:       "TERMINATED",
		},
	}}
	expired := time.Now().Add(-time.Minute)
	runnerRecord, err := db.CreateRunner(ctx, store.Runner{
		PolicyID:       policyRecord.ID,
		JobID:          jobRecord.ID,
		InstallationID: 456,
		InstanceOCID:   "ocid1.instance.oc1..terminal",
		RepoOwner:      "example",
		RepoName:       "repo",
		RunnerName:     "runner-2",
		Status:         "completed",
		Labels:         []string{"oci", "cpu"},
		ExpiresAt:      &expired,
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}

	ghAPI := newCleanupGitHubAPIFixture(t, []githubapp.RepositoryRunner{
		{ID: 88, Name: "runner-2", Status: "offline", Busy: false},
	})
	defer ghAPI.Close()
	githubService, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults: githubapp.Config{
			APIBaseURL:                      ghAPI.server.URL,
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKey,
			WebhookSecret:                   "secret",
			SelectedRepos:                   []string{"example/repo"},
			AccountLogin:                    "example-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"example/repo"},
		},
		EncryptionKey: "cleanup-secret",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}
	service := New(db, controller, githubService, session.New(db, "secret", time.Hour))

	result, err := service.RunOnce(ctx)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.Terminated != 1 {
		t.Fatalf("expected 1 terminated runner, got %d", result.Terminated)
	}
	updatedRunner, err := db.FindRunnerByID(ctx, runnerRecord.ID)
	if err != nil {
		t.Fatalf("find runner: %v", err)
	}
	if updatedRunner.Status != "terminated" || updatedRunner.TerminatedAt == nil {
		t.Fatalf("expected terminated runner after OCI terminal state, got status=%q terminatedAt=%v", updatedRunner.Status, updatedRunner.TerminatedAt)
	}
	if updatedRunner.GitHubRunnerID != 88 {
		t.Fatalf("expected runner ID 88 to be persisted, got %d", updatedRunner.GitHubRunnerID)
	}
	if len(ghAPI.deletedRunnerIDs) != 1 || ghAPI.deletedRunnerIDs[0] != 88 {
		t.Fatalf("expected GitHub runner 88 to be deleted, got %#v", ghAPI.deletedRunnerIDs)
	}
}

func TestCleanupResolvesGitHubClientByStoredConfigID(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	policyRecord, err := db.CreatePolicy(ctx, store.Policy{Labels: []string{"oci", "cpu"}, Shape: "VM.Standard.A1.Flex", OCPU: 2, MemoryGB: 8, MaxRunners: 1, TTLMinutes: 30, Enabled: true})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}

	alphaAPI := newCleanupGitHubAPIFixtureWithInstallation(t, 456, []githubapp.RepositoryRunner{
		{ID: 77, Name: "runner-alpha", Status: "online", Busy: false},
	})
	defer alphaAPI.Close()

	betaAPI := newCleanupGitHubAPIFixtureWithInstallation(t, 654, []githubapp.RepositoryRunner{
		{ID: 166, Name: "runner-beta", Status: "online", Busy: false},
	})
	defer betaAPI.Close()

	githubService, err := githubapp.NewService(db, githubapp.ServiceOptions{
		EncryptionKey: "cleanup-secret",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}

	if _, err := githubService.Save(ctx, githubapp.Input{
		Name:           "alpha-prod",
		Tags:           []string{"prod", "alpha"},
		APIBaseURL:     alphaAPI.server.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "alpha-secret",
		SelectedRepos:  []string{"example/repo"},
	}); err != nil {
		t.Fatalf("save alpha config: %v", err)
	}

	betaConfig, err := githubService.Save(ctx, githubapp.Input{
		Name:           "beta-stage",
		Tags:           []string{"staging", "beta"},
		APIBaseURL:     betaAPI.server.URL,
		AppID:          124,
		InstallationID: 654,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "beta-secret",
		SelectedRepos:  []string{"example/repo"},
	})
	if err != nil {
		t.Fatalf("save beta config: %v", err)
	}

	jobRecord, err := db.UpsertJob(ctx, store.Job{
		GitHubJobID:      3,
		DeliveryID:       "delivery-3",
		InstallationID:   654,
		GitHubConfigID:   betaConfig.Config.ID,
		GitHubConfigName: betaConfig.Config.Name,
		GitHubConfigTags: betaConfig.Config.Tags,
		RepoOwner:        "example",
		RepoName:         "repo",
		RunID:            3,
		RunAttempt:       1,
		Status:           "queued",
		Labels:           []string{"oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	controller := &oci.FakeController{Instances: map[string]oci.Instance{}}
	instance, err := controller.LaunchInstance(ctx, oci.LaunchRequest{DisplayName: "runner-beta", Shape: "VM.Standard.A1.Flex"})
	if err != nil {
		t.Fatalf("launch instance: %v", err)
	}
	expired := time.Now().Add(-time.Minute)
	runnerRecord, err := db.CreateRunner(ctx, store.Runner{
		PolicyID:         policyRecord.ID,
		JobID:            jobRecord.ID,
		InstallationID:   654,
		GitHubConfigID:   betaConfig.Config.ID,
		GitHubConfigName: betaConfig.Config.Name,
		GitHubConfigTags: betaConfig.Config.Tags,
		InstanceOCID:     instance.ID,
		RepoOwner:        "example",
		RepoName:         "repo",
		RunnerName:       "runner-beta",
		Status:           "launching",
		Labels:           []string{"oci", "cpu"},
		ExpiresAt:        &expired,
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}

	service := New(db, controller, githubService, session.New(db, "secret", time.Hour))
	result, err := service.RunOnce(ctx)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.Terminated != 1 {
		t.Fatalf("expected 1 terminated runner, got %d", result.Terminated)
	}

	updatedRunner, err := db.FindRunnerByID(ctx, runnerRecord.ID)
	if err != nil {
		t.Fatalf("find updated runner: %v", err)
	}
	if updatedRunner.GitHubRunnerID != 166 {
		t.Fatalf("expected beta runner id to be synced, got %d", updatedRunner.GitHubRunnerID)
	}
	if len(alphaAPI.deletedRunnerIDs) != 0 {
		t.Fatalf("expected alpha app to stay unused, got %#v", alphaAPI.deletedRunnerIDs)
	}
	if len(betaAPI.deletedRunnerIDs) != 1 || betaAPI.deletedRunnerIDs[0] != 166 {
		t.Fatalf("expected beta app to delete runner 166, got %#v", betaAPI.deletedRunnerIDs)
	}
}

func TestCleanupResolvesRetiredConfigIDByExactRouteIdentity(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	policyRecord, err := db.CreatePolicy(ctx, store.Policy{Labels: []string{"oci", "cpu"}, Shape: "VM.Standard.A1.Flex", OCPU: 2, MemoryGB: 8, MaxRunners: 1, TTLMinutes: 30, Enabled: true})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}

	replacementAPI := newCleanupGitHubAPIFixtureWithInstallation(t, 456, []githubapp.RepositoryRunner{
		{ID: 188, Name: "runner-alpha-retired", Status: "online", Busy: false},
	})
	defer replacementAPI.Close()

	unrelatedAPI := newCleanupGitHubAPIFixtureWithInstallation(t, 456, []githubapp.RepositoryRunner{
		{ID: 999, Name: "runner-alpha-retired", Status: "online", Busy: false},
	})
	defer unrelatedAPI.Close()

	githubService, err := githubapp.NewService(db, githubapp.ServiceOptions{
		EncryptionKey: "cleanup-secret",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}

	original, err := githubService.Save(ctx, githubapp.Input{
		Name:           "alpha-old",
		Tags:           []string{"alpha", "old"},
		APIBaseURL:     replacementAPI.server.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "alpha-secret-1",
		SelectedRepos:  []string{"example/repo"},
	})
	if err != nil {
		t.Fatalf("save original config: %v", err)
	}

	replacement, err := githubService.Save(ctx, githubapp.Input{
		Name:           "alpha-new",
		Tags:           []string{"alpha", "new"},
		APIBaseURL:     replacementAPI.server.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "alpha-secret-2",
		SelectedRepos:  []string{"example/repo"},
	})
	if err != nil {
		t.Fatalf("save replacement config: %v", err)
	}

	unrelated, err := githubService.Save(ctx, githubapp.Input{
		Name:           "beta-same-installation",
		Tags:           []string{"beta"},
		APIBaseURL:     unrelatedAPI.server.URL,
		AppID:          124,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "beta-secret-1",
		SelectedRepos:  []string{"example/repo"},
	})
	if err != nil {
		t.Fatalf("save unrelated config: %v", err)
	}

	jobRecord, err := db.UpsertJob(ctx, store.Job{
		GitHubJobID:      44,
		DeliveryID:       "delivery-44",
		InstallationID:   456,
		GitHubConfigID:   original.Config.ID,
		GitHubConfigName: original.Config.Name,
		GitHubConfigTags: original.Config.Tags,
		RepoOwner:        "example",
		RepoName:         "repo",
		RunID:            44,
		RunAttempt:       1,
		Status:           "queued",
		Labels:           []string{"oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	controller := &oci.FakeController{Instances: map[string]oci.Instance{}}
	instance, err := controller.LaunchInstance(ctx, oci.LaunchRequest{DisplayName: "runner-alpha-retired", Shape: "VM.Standard.A1.Flex"})
	if err != nil {
		t.Fatalf("launch instance: %v", err)
	}
	expired := time.Now().Add(-time.Minute)
	runnerRecord, err := db.CreateRunner(ctx, store.Runner{
		PolicyID:         policyRecord.ID,
		JobID:            jobRecord.ID,
		InstallationID:   456,
		GitHubConfigID:   original.Config.ID,
		GitHubConfigName: original.Config.Name,
		GitHubConfigTags: original.Config.Tags,
		InstanceOCID:     instance.ID,
		RepoOwner:        "example",
		RepoName:         "repo",
		RunnerName:       "runner-alpha-retired",
		Status:           "launching",
		Labels:           []string{"oci", "cpu"},
		ExpiresAt:        &expired,
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}

	service := New(db, controller, githubService, session.New(db, "secret", time.Hour))
	result, err := service.RunOnce(ctx)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.Terminated != 1 {
		t.Fatalf("expected 1 terminated runner, got %d", result.Terminated)
	}

	updatedRunner, err := db.FindRunnerByID(ctx, runnerRecord.ID)
	if err != nil {
		t.Fatalf("find updated runner: %v", err)
	}
	if updatedRunner.GitHubRunnerID != 188 {
		t.Fatalf("expected retired config id to resolve replacement runner id 188, got %#v", updatedRunner)
	}
	if len(replacementAPI.deletedRunnerIDs) != 1 || replacementAPI.deletedRunnerIDs[0] != 188 {
		t.Fatalf("expected exact replacement app to delete runner 188, got %#v", replacementAPI.deletedRunnerIDs)
	}
	if len(unrelatedAPI.deletedRunnerIDs) != 0 {
		t.Fatalf("expected same-installation unrelated app to remain unused, got %#v", unrelatedAPI.deletedRunnerIDs)
	}
	if replacement.Config.ID == unrelated.Config.ID {
		t.Fatalf("expected distinct replacement and unrelated configs, got %#v %#v", replacement.Config, unrelated.Config)
	}
}

func TestCleanupRefusesRotatedEnvConfigIDInstallationFallback(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	policyRecord, err := db.CreatePolicy(ctx, store.Policy{Labels: []string{"oci", "cpu"}, Shape: "VM.Standard.A1.Flex", OCPU: 2, MemoryGB: 8, MaxRunners: 1, TTLMinutes: 30, Enabled: true})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}

	oldGitHubService, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults: githubapp.Config{
			Name:                            "env-gh-app-old",
			Tags:                            []string{"env", "old"},
			APIBaseURL:                      "https://github-old.example.test/api/v3/",
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKey,
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"example/repo"},
			AccountLogin:                    "example-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"example/repo"},
		},
		EncryptionKey: "cleanup-secret",
	})
	if err != nil {
		t.Fatalf("new old github service: %v", err)
	}
	oldStatus, err := oldGitHubService.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status for old env route: %v", err)
	}
	if oldStatus.EffectiveConfig.ID == 0 {
		t.Fatalf("expected synthetic env config id for old route, got %#v", oldStatus.EffectiveConfig)
	}

	currentAPI := newCleanupGitHubAPIFixtureWithInstallation(t, 456, []githubapp.RepositoryRunner{
		{ID: 277, Name: "runner-env-rotated", Status: "online", Busy: false},
	})
	defer currentAPI.Close()

	currentGitHubService, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults: githubapp.Config{
			Name:                            "env-gh-app-current",
			Tags:                            []string{"current", "env"},
			APIBaseURL:                      currentAPI.server.URL,
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKey,
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"example/repo"},
			AccountLogin:                    "example-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"example/repo"},
		},
		EncryptionKey: "cleanup-secret",
	})
	if err != nil {
		t.Fatalf("new current github service: %v", err)
	}

	jobRecord, err := db.UpsertJob(ctx, store.Job{
		GitHubJobID:      55,
		DeliveryID:       "delivery-55",
		InstallationID:   456,
		GitHubConfigID:   oldStatus.EffectiveConfig.ID,
		GitHubConfigName: oldStatus.EffectiveConfig.Name,
		GitHubConfigTags: oldStatus.EffectiveConfig.Tags,
		RepoOwner:        "example",
		RepoName:         "repo",
		RunID:            55,
		RunAttempt:       1,
		Status:           "queued",
		Labels:           []string{"oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	controller := &oci.FakeController{Instances: map[string]oci.Instance{}}
	instance, err := controller.LaunchInstance(ctx, oci.LaunchRequest{DisplayName: "runner-env-rotated", Shape: "VM.Standard.A1.Flex"})
	if err != nil {
		t.Fatalf("launch instance: %v", err)
	}
	expired := time.Now().Add(-time.Minute)
	runnerRecord, err := db.CreateRunner(ctx, store.Runner{
		PolicyID:         policyRecord.ID,
		JobID:            jobRecord.ID,
		InstallationID:   456,
		GitHubConfigID:   oldStatus.EffectiveConfig.ID,
		GitHubConfigName: oldStatus.EffectiveConfig.Name,
		GitHubConfigTags: oldStatus.EffectiveConfig.Tags,
		InstanceOCID:     instance.ID,
		RepoOwner:        "example",
		RepoName:         "repo",
		RunnerName:       "runner-env-rotated",
		Status:           "launching",
		Labels:           []string{"oci", "cpu"},
		ExpiresAt:        &expired,
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}

	service := New(db, controller, currentGitHubService, session.New(db, "secret", time.Hour))
	result, err := service.RunOnce(ctx)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.Terminated != 1 {
		t.Fatalf("expected 1 terminated runner, got %d", result.Terminated)
	}

	updatedRunner, err := db.FindRunnerByID(ctx, runnerRecord.ID)
	if err != nil {
		t.Fatalf("find updated runner: %v", err)
	}
	if updatedRunner.Status != "terminating" {
		t.Fatalf("expected terminating runner, got %#v", updatedRunner)
	}
	if updatedRunner.GitHubRunnerID != 0 {
		t.Fatalf("expected rotated env config id to refuse github runner sync, got %#v", updatedRunner)
	}
	if len(currentAPI.deletedRunnerIDs) != 0 {
		t.Fatalf("expected cleanup to avoid deleting via current env route, got %#v", currentAPI.deletedRunnerIDs)
	}
}

type cleanupGitHubAPIFixture struct {
	server           *httptest.Server
	runners          map[string]githubapp.RepositoryRunner
	deletedRunnerIDs []int64
}

func newCleanupGitHubAPIFixture(t *testing.T, runners []githubapp.RepositoryRunner) *cleanupGitHubAPIFixture {
	t.Helper()
	return newCleanupGitHubAPIFixtureWithInstallation(t, 456, runners)
}

func newCleanupGitHubAPIFixtureWithInstallation(t *testing.T, installationID int64, runners []githubapp.RepositoryRunner) *cleanupGitHubAPIFixture {
	t.Helper()
	fixture := &cleanupGitHubAPIFixture{
		runners: map[string]githubapp.RepositoryRunner{},
	}
	for _, runner := range runners {
		fixture.runners[runner.Name] = runner
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app":
			writeCleanupJSON(t, w, map[string]any{"id": 123})
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/"+strconv.FormatInt(installationID, 10)+"/access_tokens":
			writeCleanupJSON(t, w, map[string]any{"token": "installation-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations/"+strconv.FormatInt(installationID, 10):
			writeCleanupJSON(t, w, map[string]any{
				"account": map[string]any{
					"login": "example-org",
					"type":  "Organization",
				},
				"repository_selection": "selected",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			writeCleanupJSON(t, w, map[string]any{
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
		case r.Method == http.MethodGet && r.URL.Path == "/repos/example/repo/actions/runners":
			name := r.URL.Query().Get("name")
			items := []githubapp.RepositoryRunner{}
			if runner, ok := fixture.runners[name]; ok {
				items = append(items, runner)
			}
			writeCleanupJSON(t, w, map[string]any{"runners": items})
		case r.Method == http.MethodDelete && len(r.URL.Path) > len("/repos/example/repo/actions/runners/"):
			id, err := strconv.ParseInt(r.URL.Path[len("/repos/example/repo/actions/runners/"):], 10, 64)
			if err != nil {
				t.Fatalf("parse delete runner id: %v", err)
			}
			fixture.deletedRunnerIDs = append(fixture.deletedRunnerIDs, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	return fixture
}

func (f *cleanupGitHubAPIFixture) Close() {
	if f.server != nil {
		f.server.Close()
	}
}

func writeCleanupJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode cleanup payload: %v", err)
	}
}

const testPrivateKey = "-----BEGIN PRIVATE KEY-----\nMIICdgIBADANBgkqhkiG9w0BAQEFAASCAmAwggJcAgEAAoGBAOW0ED5HhOi+am89\n+A8Gs84lcTxj95fyY/m4El01AaOMwB6Ufnx8lIIY7abn71exSaKDzsFNEM+uBkdH\nW8mG+Lna3TGmRS52G46DnulBiREnpRV+NIQwMjZHpQ5WvW9nzePZ4navmdnhyrcE\npYA3vKJKND/p8+8mlD0G8CfD0Ko3AgMBAAECgYA1HvMys+90s7SBjV80emRSpC4P\nvT6hERk1wu/cRknevMohSE4IE/d0LrenBbRAH2vb/YdvBJeCr8gb69C6RlB2mo25\ngMv8A+zggDGyIJEq5JCIGsFWa463bd8P/Y+tZ6ZsCULVuksWl+suvhoJvr3zBeeM\neQMF3rd8hzhYa5iqYQJBAPakEcZAcMAQWcjzBQKmdZoP+zXvExMOrDlFKeqsbeWP\nVHFrpcZ+t/A3SwKKOmX5Ie50rPtCBi+2NfLYYebGnv0CQQDua3Vvomv1zyJmuEi+\nHr+rqHtzjjA8vVUCK8Tb9UEqWLZ3JQNcoGvgHUZrw3Euq1nqvOYYHsZGTLXSIrlu\nwaZDAkBa+tSvq++reZyVGsgbXSn+ZazGDWWc3wm6qn+22FpFluSQXiQtn2rcipj5\n2+GE4iyZGKMCoC1GBlHKPfWHOndFAkAwso44EQrQGFDEfluNSaaIn08n2SENJvbY\nDKyW6M84oQoT5+F55+Jg0lnx5OeXSrSA97hfsNl6vmxc0W7iqncVAkEArERxQtrn\nd/fYemHb9Wv5ibLOZWoPCNy2WACMGyHQ7+3+pB/IxI9ueUrnrRaCAQLkDuhF82sW\nnApG0TpVWHyZUQ==\n-----END PRIVATE KEY-----"
