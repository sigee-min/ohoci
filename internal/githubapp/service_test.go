package githubapp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"ohoci/internal/store"
)

func TestServiceCurrentStatusUsesEnvFallback(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		Defaults: Config{
			Name:                            "env-gh-app",
			Tags:                            []string{"prod", "env"},
			APIBaseURL:                      "https://api.github.com",
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKeyPEM(t),
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"env-org/repo"},
			AccountLogin:                    "env-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"env-org/repo"},
		},
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status: %v", err)
	}
	if status.Source != "env" || !status.Ready {
		t.Fatalf("expected ready env status, got %#v", status)
	}
	if !status.HasAppCredentials || !status.HasWebhookSecret {
		t.Fatalf("expected env app credentials and webhook secret to be present, got %#v", status)
	}
	if len(status.EffectiveConfig.SelectedRepos) != 1 || status.EffectiveConfig.SelectedRepos[0] != "env-org/repo" {
		t.Fatalf("unexpected effective repos: %#v", status.EffectiveConfig.SelectedRepos)
	}
	if status.EffectiveConfig.Name != "env-gh-app" || !reflect.DeepEqual(status.EffectiveConfig.Tags, []string{"env", "prod"}) {
		t.Fatalf("expected env trace metadata on effective config, got %#v", status.EffectiveConfig)
	}
}

func TestServiceResolveWebhookSourceEnvFallbackReturnsSyntheticTraceConfig(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		Defaults: Config{
			Name:                            "env-gh-app",
			Tags:                            []string{"prod", "env"},
			APIBaseURL:                      "https://api.github.com",
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKeyPEM(t),
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"env-org/repo"},
			AccountLogin:                    "env-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"env-org/repo"},
		},
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	webhookBody := []byte(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":456},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`)
	resolution, err := service.ResolveWebhookSource(ctx, "workflow_job", webhookBody, signFixture(webhookBody, "env-secret"))
	if err != nil {
		t.Fatalf("resolve env webhook source: %v", err)
	}
	if resolution.Source != WebhookSourceActive || resolution.Client == nil || resolution.Config == nil {
		t.Fatalf("expected env webhook resolution with synthetic config, got %#v", resolution)
	}
	if resolution.Config.ID != syntheticEnvConfigID(normalizeConfig(service.defaults)) {
		t.Fatalf("expected synthetic env config id, got %#v", resolution.Config)
	}
	if resolution.Config.Name != "env-gh-app" || !reflect.DeepEqual(resolution.Config.Tags, []string{"env", "prod"}) {
		t.Fatalf("expected env trace metadata on synthetic config, got %#v", resolution.Config)
	}

	client, err := service.ResolveClientByConfigID(ctx, resolution.Config.ID)
	if err != nil {
		t.Fatalf("resolve env client by synthetic config id: %v", err)
	}
	if client.InstallationID() != 456 {
		t.Fatalf("expected env installation id 456, got %d", client.InstallationID())
	}
}

func TestSyntheticEnvConfigIDUsesNormalizedRouteIdentity(t *testing.T) {
	base := normalizeConfig(Config{
		APIBaseURL:     "https://github.example.test/api/v3/",
		AppID:          123,
		InstallationID: 456,
	})
	sameRoute := normalizeConfig(Config{
		APIBaseURL:     " https://github.example.test/api/v3 ",
		AppID:          123,
		InstallationID: 456,
	})
	rotatedRoute := normalizeConfig(Config{
		APIBaseURL:     "https://github-alt.example.test/api/v3",
		AppID:          123,
		InstallationID: 456,
	})

	baseID := syntheticEnvConfigID(base)
	if baseID == 0 {
		t.Fatalf("expected synthetic env config id for complete route, got %d", baseID)
	}
	if sameRouteID := syntheticEnvConfigID(sameRoute); sameRouteID != baseID {
		t.Fatalf("expected normalized route identity to stay stable, got %d and %d", baseID, sameRouteID)
	}
	if rotatedRouteID := syntheticEnvConfigID(rotatedRoute); rotatedRouteID == baseID {
		t.Fatalf("expected rotated route identity to change synthetic id, got %d", rotatedRouteID)
	}
}

func TestServiceResolveRunnerClientRefusesRotatedEnvSyntheticConfigID(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	oldService, err := NewService(db, ServiceOptions{
		Defaults: Config{
			Name:                            "env-gh-app-old",
			Tags:                            []string{"env", "old"},
			APIBaseURL:                      "https://github-old.example.test/api/v3/",
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKeyPEM(t),
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"env-org/repo"},
			AccountLogin:                    "env-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"env-org/repo"},
		},
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new old service: %v", err)
	}
	oldStatus, err := oldService.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status for old env route: %v", err)
	}

	currentService, err := NewService(db, ServiceOptions{
		Defaults: Config{
			Name:                            "env-gh-app-current",
			Tags:                            []string{"current", "env"},
			APIBaseURL:                      "https://github-current.example.test/api/v3",
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKeyPEM(t),
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"env-org/repo"},
			AccountLogin:                    "env-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"env-org/repo"},
		},
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new current service: %v", err)
	}
	currentStatus, err := currentService.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status for current env route: %v", err)
	}
	if oldStatus.EffectiveConfig.ID == 0 || currentStatus.EffectiveConfig.ID == 0 {
		t.Fatalf("expected synthetic env ids on both routes, got old=%d current=%d", oldStatus.EffectiveConfig.ID, currentStatus.EffectiveConfig.ID)
	}
	if oldStatus.EffectiveConfig.ID == currentStatus.EffectiveConfig.ID {
		t.Fatalf("expected rotated env route to produce a new synthetic config id, got %d", currentStatus.EffectiveConfig.ID)
	}

	client, err := currentService.ResolveClientByConfigID(ctx, currentStatus.EffectiveConfig.ID)
	if err != nil {
		t.Fatalf("resolve current env client by config id: %v", err)
	}
	if client.InstallationID() != 456 {
		t.Fatalf("expected current env installation id 456, got %d", client.InstallationID())
	}
	runnerClient, err := currentService.ResolveRunnerClient(ctx, currentStatus.EffectiveConfig.ID, 456)
	if err != nil {
		t.Fatalf("resolve current env runner client: %v", err)
	}
	if runnerClient.InstallationID() != 456 {
		t.Fatalf("expected current env runner installation id 456, got %d", runnerClient.InstallationID())
	}

	if _, err := currentService.ResolveClientByConfigID(ctx, oldStatus.EffectiveConfig.ID); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected rotated env config id lookup to refuse safely, got %v", err)
	}
	if _, err := currentService.ResolveRunnerClient(ctx, oldStatus.EffectiveConfig.ID, 456); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected rotated env runner lookup to refuse instead of installation fallback, got %v", err)
	}
}

func TestServiceEnvLifecycleStatusPersistsForCurrentStatusAndRouting(t *testing.T) {
	testCases := []struct {
		name          string
		state         string
		expectedError string
	}{
		{name: "suspended", state: "suspended", expectedError: "github app installation is suspended"},
		{name: "deleted", state: "deleted", expectedError: "github app installation was removed"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			service, err := NewService(db, ServiceOptions{
				Defaults: Config{
					Name:                            "env-gh-app",
					Tags:                            []string{"env", "fallback"},
					APIBaseURL:                      "https://api.github.com",
					AppID:                           123,
					InstallationID:                  456,
					PrivateKeyPEM:                   testPrivateKeyPEM(t),
					WebhookSecret:                   "env-secret",
					SelectedRepos:                   []string{"env-org/repo"},
					AccountLogin:                    "env-org",
					AccountType:                     "Organization",
					InstallationState:               "active",
					InstallationRepositorySelection: "selected",
					InstallationRepositories:        []string{"env-org/repo"},
				},
				EncryptionKey: "top-secret",
				PublicBaseURL: "http://localhost:8080",
			})
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			envRecord := service.syntheticEnvConfigRecord(normalizeConfig(service.defaults))
			if err := service.RecordInstallationStatus(ctx, &envRecord, testCase.state, "env-org", "Organization", "selected", nil, ""); err != nil {
				t.Fatalf("record env installation status %q: %v", testCase.state, err)
			}

			status, err := service.CurrentStatus(ctx)
			if err != nil {
				t.Fatalf("current status after env lifecycle update: %v", err)
			}
			if status.Source != "env" || status.Ready || status.Error != testCase.expectedError {
				t.Fatalf("expected env status to persist %q lifecycle change, got %#v", testCase.state, status)
			}
			if status.EffectiveConfig.InstallationState != testCase.state || status.EffectiveConfig.InstallationReady {
				t.Fatalf("expected env effective config to reflect %q lifecycle state, got %#v", testCase.state, status.EffectiveConfig)
			}

			workflowBody := []byte(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":456},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`)
			workflowResolution, err := service.ResolveWebhookSource(ctx, "workflow_job", workflowBody, signFixture(workflowBody, "env-secret"))
			if err != nil {
				t.Fatalf("resolve workflow webhook after env lifecycle update: %v", err)
			}
			if workflowResolution.Source != WebhookSourceUnknown || workflowResolution.Client != nil || workflowResolution.Config != nil {
				t.Fatalf("expected workflow routing to stop for env state %q, got %#v", testCase.state, workflowResolution)
			}

			installationBody := []byte(`{"action":"created","installation":{"id":456,"account":{"login":"env-org","type":"Organization"},"repository_selection":"selected"}}`)
			installationResolution, err := service.ResolveWebhookSource(ctx, "installation", installationBody, signFixture(installationBody, "env-secret"))
			if err != nil {
				t.Fatalf("resolve installation webhook after env lifecycle update: %v", err)
			}
			if installationResolution.Source != WebhookSourceActive || installationResolution.Config == nil || installationResolution.Config.ID != envRecord.ID {
				t.Fatalf("expected env installation lifecycle routing to remain available for %q state, got %#v", testCase.state, installationResolution)
			}
		})
	}
}

func TestServiceEnvRepositoryScopeStatusPersistsAcrossCurrentStatus(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		Defaults: Config{
			Name:                            "env-gh-app",
			Tags:                            []string{"env", "fallback"},
			APIBaseURL:                      "https://api.github.com",
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKeyPEM(t),
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"env-org/repo-a"},
			AccountLogin:                    "env-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"env-org/repo-a"},
		},
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	envRecord := service.syntheticEnvConfigRecord(normalizeConfig(service.defaults))
	if err := service.RecordInstallationStatus(ctx, &envRecord, "active", "env-org", "Organization", "selected", []string{"env-org/repo-b"}, ""); err != nil {
		t.Fatalf("record env repository scope change: %v", err)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status after env repository scope change: %v", err)
	}
	if status.Source != "env" || status.Ready {
		t.Fatalf("expected env status to become not ready after repository scope change, got %#v", status)
	}
	if status.Error != "selected repositories are not installed: env-org/repo-a" {
		t.Fatalf("unexpected env repository scope error: %#v", status)
	}
	if !reflect.DeepEqual(status.EffectiveConfig.InstallationRepositories, []string{"env-org/repo-b"}) {
		t.Fatalf("expected env effective repositories to reflect persisted route state, got %#v", status.EffectiveConfig)
	}
}

func TestServiceSaveActiveAppRejectsEnvWebhookSecretReuse(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		Defaults: Config{
			Name:                            "env-gh-app",
			Tags:                            []string{"prod", "env"},
			APIBaseURL:                      "https://api.github.com",
			AppID:                           123,
			InstallationID:                  456,
			PrivateKeyPEM:                   testPrivateKeyPEM(t),
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"env-org/repo"},
			AccountLogin:                    "env-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"env-org/repo"},
		},
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	server := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               111,
		installationID:      222,
		accountLogin:        "cms-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "cms-org/repo-a", Owner: "cms-org", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer server.Close()

	_, err = service.Save(ctx, Input{
		APIBaseURL:     server.URL,
		AppID:          111,
		InstallationID: 222,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "env-secret",
		SelectedRepos:  []string{"cms-org/repo-a"},
	})
	if err == nil {
		t.Fatalf("expected active app save to reject env webhook secret reuse")
	}
	if !strings.Contains(err.Error(), "staged or live webhook secrets") {
		t.Fatalf("expected live webhook secret conflict, got %v", err)
	}
}

func TestServiceStoredActiveConfigStateAffectsWorkflowRoutingButNotInstallationLifecycle(t *testing.T) {
	testCases := []struct {
		name  string
		state string
	}{
		{name: "suspended", state: "suspended"},
		{name: "deleted", state: "deleted"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			service, err := NewService(db, ServiceOptions{
				Defaults: Config{
					Name:                            "env-gh-app",
					Tags:                            []string{"env", "fallback"},
					APIBaseURL:                      "https://api.github.com",
					AppID:                           900,
					InstallationID:                  901,
					PrivateKeyPEM:                   testPrivateKeyPEM(t),
					WebhookSecret:                   "env-secret",
					SelectedRepos:                   []string{"env-org/repo"},
					AccountLogin:                    "env-org",
					AccountType:                     "Organization",
					InstallationState:               "active",
					InstallationRepositorySelection: "selected",
					InstallationRepositories:        []string{"env-org/repo"},
				},
				EncryptionKey: "top-secret",
				PublicBaseURL: "http://localhost:8080",
			})
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			server := newGitHubAppServer(t, gitHubAppFixtureOptions{
				appID:               111,
				installationID:      456,
				accountLogin:        "cms-org",
				accountType:         "Organization",
				repositorySelection: "selected",
				repositories: []Repository{
					{FullName: "cms-org/repo-a", Owner: "cms-org", Name: "repo-a", Private: true, Admin: true},
				},
			})
			defer server.Close()

			if _, err := service.Save(ctx, Input{
				Name:           "cms-active",
				Tags:           []string{"cms", "active"},
				APIBaseURL:     server.URL,
				AppID:          111,
				InstallationID: 456,
				PrivateKeyPEM:  testPrivateKeyPEM(t),
				WebhookSecret:  "cms-secret",
				SelectedRepos:  []string{"cms-org/repo-a"},
			}); err != nil {
				t.Fatalf("save active app config: %v", err)
			}

			record, err := service.store.FindActiveGitHubConfig(ctx)
			if err != nil {
				t.Fatalf("find active config: %v", err)
			}
			if err := service.RecordInstallationStatus(ctx, &record, testCase.state, "cms-org", "Organization", "selected", []string{"cms-org/repo-a"}, ""); err != nil {
				t.Fatalf("mark active config %s: %v", testCase.state, err)
			}

			status, err := service.CurrentStatus(ctx)
			if err != nil {
				t.Fatalf("current status: %v", err)
			}
			if status.Source != "env" {
				t.Fatalf("expected env fallback after %s active config becomes non-routable, got %#v", testCase.state, status)
			}
			if len(status.ActiveConfigs) != 0 || status.ActiveConfig != nil {
				t.Fatalf("expected non-routable stored active config to be hidden from current status, got %#v", status)
			}
			if !reflect.DeepEqual(status.SelectedRepos, []string{"env-org/repo"}) {
				t.Fatalf("expected env repo scope after %s active config becomes non-routable, got %#v", testCase.state, status.SelectedRepos)
			}

			workflowBody := []byte(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":456},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`)
			workflowResolution, err := service.ResolveWebhookSource(ctx, "workflow_job", workflowBody, signFixture(workflowBody, "cms-secret"))
			if err != nil {
				t.Fatalf("resolve workflow_job webhook source: %v", err)
			}
			if workflowResolution.Source != WebhookSourceUnknown || workflowResolution.Client != nil || workflowResolution.Config != nil {
				t.Fatalf("expected non-routable active config to be excluded from workflow routing, got %#v", workflowResolution)
			}

			installationBody := []byte(`{"action":"created","installation":{"id":456,"account":{"login":"cms-org","type":"Organization"},"repository_selection":"selected"}}`)
			installationResolution, err := service.ResolveWebhookSource(ctx, "installation", installationBody, signFixture(installationBody, "cms-secret"))
			if err != nil {
				t.Fatalf("resolve installation webhook source: %v", err)
			}
			if installationResolution.Source != WebhookSourceActive || installationResolution.Config == nil || installationResolution.Config.ID != record.ID {
				t.Fatalf("expected stored active config to remain resolvable for installation lifecycle events, got %#v", installationResolution)
			}

			envInstallationBody := []byte(`{"action":"created","installation":{"id":901,"account":{"login":"env-org","type":"Organization"},"repository_selection":"selected"}}`)
			envResolution, err := service.ResolveWebhookSource(ctx, "installation", envInstallationBody, signFixture(envInstallationBody, "env-secret"))
			if err != nil {
				t.Fatalf("resolve env installation webhook source: %v", err)
			}
			if envResolution.Source != WebhookSourceActive || envResolution.Config == nil || envResolution.Config.ID != syntheticEnvConfigID(normalizeConfig(service.defaults)) {
				t.Fatalf("expected env fallback to remain eligible for installation lifecycle events when stored active config does not match, got %#v", envResolution)
			}
		})
	}
}

func TestServiceSaveResolveAndClear(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		Defaults: Config{
			APIBaseURL:                      "https://api.github.com",
			AppID:                           111,
			InstallationID:                  222,
			PrivateKeyPEM:                   testPrivateKeyPEM(t),
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"env-org/repo"},
			AccountLogin:                    "env-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"env-org/repo"},
		},
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	server := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               123,
		installationID:      456,
		accountLogin:        "cms-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "cms-org/repo-a", Owner: "cms-org", Name: "repo-a", Private: true, Admin: true},
			{FullName: "cms-org/repo-b", Owner: "cms-org", Name: "repo-b", Private: true, Admin: true},
		},
	})
	defer server.Close()

	result, err := service.Save(ctx, Input{
		APIBaseURL:     server.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "cms-secret",
		SelectedRepos:  []string{"cms-org/repo-b", "cms-org/repo-a"},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(result.Config.SelectedRepos) != 2 || result.Config.AuthMode != store.GitHubAuthModeApp {
		t.Fatalf("unexpected save result: %#v", result)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status after save: %v", err)
	}
	if status.Source != "cms" || !status.Ready {
		t.Fatalf("expected ready cms status, got %#v", status)
	}
	if status.ActiveConfig == nil || status.ActiveConfig.AuthMode != store.GitHubAuthModeApp {
		t.Fatalf("expected active app status, got %#v", status.ActiveConfig)
	}
	if status.StagedConfig != nil {
		t.Fatalf("expected no staged config in active save flow, got %#v", status.StagedConfig)
	}

	client, err := service.ResolveClient(ctx)
	if err != nil {
		t.Fatalf("resolve client: %v", err)
	}
	record, err := db.FindActiveGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("find active config: %v", err)
	}
	cfg, _, err := service.configFromRecord(record)
	if err != nil {
		t.Fatalf("decode active config: %v", err)
	}
	if cfg.AuthMode != store.GitHubAuthModeApp {
		t.Fatalf("expected active config to decode as app, got %#v", cfg)
	}
	body := []byte(`{"ok":true}`)
	if !client.ValidateWebhookSignature(body, signFixture(body, cfg.WebhookSecret)) {
		t.Fatalf("expected resolved client to use CMS webhook secret")
	}

	if err := service.Clear(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	status, err = service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status after clear: %v", err)
	}
	if status.Source != "env" || !status.Ready {
		t.Fatalf("expected env fallback after clear, got %#v", status)
	}
}

func TestServiceSaveStagedAppResolvesDistinctWebhookSecret(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	activeServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               111,
		installationID:      222,
		accountLogin:        "active-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "active-org/repo-a", Owner: "active-org", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer activeServer.Close()

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if _, err := service.Save(ctx, Input{
		APIBaseURL:     activeServer.URL,
		AppID:          111,
		InstallationID: 222,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "active-webhook-secret",
		SelectedRepos:  []string{"active-org/repo-a"},
	}); err != nil {
		t.Fatalf("save active app config: %v", err)
	}

	stagedServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               123,
		installationID:      456,
		accountLogin:        "staged-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "staged-org/repo-a", Owner: "staged-org", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer stagedServer.Close()

	if _, err := service.SaveStagedApp(ctx, Input{
		APIBaseURL:     stagedServer.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "active-webhook-secret",
		SelectedRepos:  []string{"staged-org/repo-a"},
	}); err == nil {
		t.Fatalf("expected staged app save to reject active webhook secret reuse")
	}

	staged, err := service.SaveStagedApp(ctx, Input{
		APIBaseURL:     stagedServer.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		SelectedRepos:  []string{"staged-org/repo-a"},
	})
	if err != nil {
		t.Fatalf("save staged app config: %v", err)
	}
	if staged.Config.AuthMode != store.GitHubAuthModeApp || !staged.Config.IsStaged {
		t.Fatalf("expected staged app result, got %#v", staged.Config)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status: %v", err)
	}
	if status.ActiveConfig == nil || status.ActiveConfig.AuthMode != store.GitHubAuthModeApp || !status.ActiveConfig.IsActive {
		t.Fatalf("expected active app config to remain active, got %#v", status.ActiveConfig)
	}
	if status.StagedConfig == nil || status.StagedConfig.AuthMode != store.GitHubAuthModeApp || !status.StagedConfig.IsStaged {
		t.Fatalf("expected staged app config in status, got %#v", status.StagedConfig)
	}
	if !status.Ready || !status.StagedReady {
		t.Fatalf("expected both active and staged app configs to be ready, got %#v", status)
	}

	activeRecord, err := service.store.FindActiveGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("find active config: %v", err)
	}
	activeCfg, _, err := service.configFromRecord(activeRecord)
	if err != nil {
		t.Fatalf("decode active config: %v", err)
	}
	stagedRecord, err := service.store.FindStagedGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("find staged config: %v", err)
	}
	stagedCfg, _, err := service.configFromRecord(stagedRecord)
	if err != nil {
		t.Fatalf("decode staged config: %v", err)
	}
	if stagedCfg.WebhookSecret == activeCfg.WebhookSecret {
		t.Fatalf("expected staged webhook secret to differ from active secret")
	}

	stagedClient, err := service.ResolveStagedClient(ctx)
	if err != nil {
		t.Fatalf("resolve staged client: %v", err)
	}
	if err := stagedClient.TestConnection(ctx); err != nil {
		t.Fatalf("test staged client: %v", err)
	}

	activeWebhookBody := []byte(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":222},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`)
	stagedWebhookBody := []byte(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":456},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`)

	activeResolution, err := service.ResolveWebhookSource(ctx, "workflow_job", activeWebhookBody, signFixture(activeWebhookBody, activeCfg.WebhookSecret))
	if err != nil {
		t.Fatalf("resolve active webhook source: %v", err)
	}
	if activeResolution.Source != WebhookSourceActive || activeResolution.Client == nil {
		t.Fatalf("expected active webhook source with client, got %#v", activeResolution)
	}

	stagedResolution, err := service.ResolveWebhookSource(ctx, "workflow_job", stagedWebhookBody, signFixture(stagedWebhookBody, stagedCfg.WebhookSecret))
	if err != nil {
		t.Fatalf("resolve staged webhook source: %v", err)
	}
	if stagedResolution.Source != WebhookSourceStaged {
		t.Fatalf("expected staged webhook source, got %#v", stagedResolution)
	}
	if stagedResolution.Client == nil || stagedResolution.Config == nil || stagedResolution.Config.ID != stagedRecord.ID {
		t.Fatalf("expected staged webhook source to return the exact staged config, got %#v", stagedResolution)
	}
}

func TestServiceSaveStagedAppRejectsEnvWebhookSecretAndLegacyCollisionResolvesToStagedAfterClear(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		Defaults: Config{
			Name:                            "env-gh-app",
			Tags:                            []string{"prod", "env"},
			APIBaseURL:                      "https://api.github.com",
			AppID:                           900,
			InstallationID:                  901,
			PrivateKeyPEM:                   testPrivateKeyPEM(t),
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"env-org/repo"},
			AccountLogin:                    "env-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"env-org/repo"},
		},
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	activeServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               111,
		installationID:      222,
		accountLogin:        "active-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "active-org/repo-a", Owner: "active-org", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer activeServer.Close()

	if _, err := service.Save(ctx, Input{
		APIBaseURL:     activeServer.URL,
		AppID:          111,
		InstallationID: 222,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "active-webhook-secret",
		SelectedRepos:  []string{"active-org/repo-a"},
	}); err != nil {
		t.Fatalf("save active app config: %v", err)
	}

	stagedServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               123,
		installationID:      456,
		accountLogin:        "staged-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "staged-org/repo-a", Owner: "staged-org", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer stagedServer.Close()

	_, err = service.SaveStagedApp(ctx, Input{
		APIBaseURL:     stagedServer.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "env-secret",
		SelectedRepos:  []string{"staged-org/repo-a"},
	})
	if err == nil {
		t.Fatalf("expected staged app save to reject env webhook secret reuse")
	}
	if !strings.Contains(err.Error(), "live webhook secrets") {
		t.Fatalf("expected live webhook secret conflict, got %v", err)
	}

	privateKeyCiphertext, err := service.encrypt(testPrivateKeyPEM(t))
	if err != nil {
		t.Fatalf("encrypt staged private key: %v", err)
	}
	webhookSecretCiphertext, err := service.encrypt("env-secret")
	if err != nil {
		t.Fatalf("encrypt staged webhook secret: %v", err)
	}
	stagedRecord, err := service.store.SaveStagedGitHubConfig(ctx, store.GitHubConfig{
		Name:                            "legacy-staged",
		Tags:                            []string{"legacy", "collision"},
		APIBaseURL:                      stagedServer.URL,
		AuthMode:                        store.GitHubAuthModeApp,
		AppID:                           123,
		InstallationID:                  456,
		PrivateKeyCiphertext:            privateKeyCiphertext,
		WebhookSecretCiphertext:         webhookSecretCiphertext,
		SelectedRepos:                   []string{"staged-org/repo-a"},
		AccountLogin:                    "staged-org",
		AccountType:                     "Organization",
		InstallationState:               "active",
		InstallationRepositorySelection: "selected",
		InstallationRepositories:        []string{"staged-org/repo-a"},
	})
	if err != nil {
		t.Fatalf("seed legacy staged collision config: %v", err)
	}

	if err := service.Clear(ctx); err != nil {
		t.Fatalf("clear active configs: %v", err)
	}

	stagedWebhookBody := []byte(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":456},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`)
	resolution, err := service.ResolveWebhookSource(ctx, "workflow_job", stagedWebhookBody, signFixture(stagedWebhookBody, "env-secret"))
	if err != nil {
		t.Fatalf("resolve webhook source on env/staged collision: %v", err)
	}
	if resolution.Source != WebhookSourceStaged {
		t.Fatalf("expected staged webhook source to win env collision after clear, got %#v", resolution)
	}
	if resolution.Client == nil || resolution.Config == nil {
		t.Fatalf("expected staged webhook resolution with client and config, got %#v", resolution)
	}
	if resolution.Config.ID != stagedRecord.ID || resolution.Config.Name != stagedRecord.Name || !reflect.DeepEqual(resolution.Config.Tags, stagedRecord.Tags) {
		t.Fatalf("expected exact staged config to be returned after clear, got %#v", resolution.Config)
	}

	envWebhookBody := []byte(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":901},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`)
	envResolution, err := service.ResolveWebhookSource(ctx, "workflow_job", envWebhookBody, signFixture(envWebhookBody, "env-secret"))
	if err != nil {
		t.Fatalf("resolve env webhook source on env/staged collision: %v", err)
	}
	if envResolution.Source != WebhookSourceActive || envResolution.Config == nil || envResolution.Config.ID != syntheticEnvConfigID(normalizeConfig(service.defaults)) {
		t.Fatalf("expected exact env config to be returned after clear, got %#v", envResolution)
	}
}

func TestServiceSupportsMultipleActiveAppConfigsAndRuntimeResolution(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	alphaServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               211,
		installationID:      311,
		accountLogin:        "alpha-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "alpha-org/repo-a", Owner: "alpha-org", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer alphaServer.Close()

	alpha, err := service.Save(ctx, Input{
		Name:           "alpha-prod",
		Tags:           []string{"prod", "alpha"},
		APIBaseURL:     alphaServer.URL,
		AppID:          211,
		InstallationID: 311,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "alpha-secret",
		SelectedRepos:  []string{"alpha-org/repo-a"},
	})
	if err != nil {
		t.Fatalf("save alpha active config: %v", err)
	}

	betaServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               212,
		installationID:      312,
		accountLogin:        "beta-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "beta-org/repo-b", Owner: "beta-org", Name: "repo-b", Private: true, Admin: true},
		},
	})
	defer betaServer.Close()

	beta, err := service.Save(ctx, Input{
		Name:           "beta-stage",
		Tags:           []string{"staging", "beta"},
		APIBaseURL:     betaServer.URL,
		AppID:          212,
		InstallationID: 312,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "beta-secret",
		SelectedRepos:  []string{"beta-org/repo-b"},
	})
	if err != nil {
		t.Fatalf("save beta active config: %v", err)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status: %v", err)
	}
	if !status.Ready || status.ActiveConfig == nil {
		t.Fatalf("expected ready status with compatibility active config, got %#v", status)
	}
	if len(status.ActiveConfigs) != 2 {
		t.Fatalf("expected two active configs, got %#v", status.ActiveConfigs)
	}
	if status.ActiveConfig.Name != "beta-stage" || status.ActiveConfig.ID != beta.Config.ID {
		t.Fatalf("expected latest ready config as compatibility view, got %#v", status.ActiveConfig)
	}

	alphaWebhookBody := []byte(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":311},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`)
	betaWebhookBody := []byte(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":312},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`)
	mismatchedWebhookBody := []byte(`{"action":"queued","repository":{"name":"repo","owner":{"login":"example"}},"installation":{"id":312},"workflow_job":{"id":1,"run_id":1,"run_attempt":1,"status":"queued","labels":["self-hosted","oci","cpu"]}}`)

	alphaResolution, err := service.ResolveWebhookSource(ctx, "workflow_job", alphaWebhookBody, signFixture(alphaWebhookBody, "alpha-secret"))
	if err != nil {
		t.Fatalf("resolve alpha webhook: %v", err)
	}
	if alphaResolution.Source != WebhookSourceActive || alphaResolution.Client == nil || alphaResolution.Config == nil {
		t.Fatalf("expected active alpha resolution, got %#v", alphaResolution)
	}
	if alphaResolution.Config.ID != alpha.Config.ID || alphaResolution.Config.Name != "alpha-prod" || !reflect.DeepEqual(alphaResolution.Config.Tags, []string{"alpha", "prod"}) {
		t.Fatalf("expected exact alpha config match, got %#v", alphaResolution.Config)
	}

	betaResolution, err := service.ResolveWebhookSource(ctx, "workflow_job", betaWebhookBody, signFixture(betaWebhookBody, "beta-secret"))
	if err != nil {
		t.Fatalf("resolve beta webhook: %v", err)
	}
	if betaResolution.Source != WebhookSourceActive || betaResolution.Config == nil || betaResolution.Config.ID != beta.Config.ID {
		t.Fatalf("expected exact beta config match, got %#v", betaResolution)
	}

	mismatchResolution, err := service.ResolveWebhookSource(ctx, "workflow_job", mismatchedWebhookBody, signFixture(mismatchedWebhookBody, "alpha-secret"))
	if err != nil {
		t.Fatalf("resolve mismatched alpha webhook: %v", err)
	}
	if mismatchResolution.Source != WebhookSourceUnknown || mismatchResolution.Client != nil || mismatchResolution.Config != nil {
		t.Fatalf("expected installation mismatch to be rejected, got %#v", mismatchResolution)
	}

	alphaClient, err := service.ResolveClientByInstallationID(ctx, 311)
	if err != nil {
		t.Fatalf("resolve alpha by installation: %v", err)
	}
	if alphaClient.InstallationID() != 311 {
		t.Fatalf("expected alpha installation client, got %d", alphaClient.InstallationID())
	}

	betaClient, err := service.ResolveClientByConfigID(ctx, beta.Config.ID)
	if err != nil {
		t.Fatalf("resolve beta by config id: %v", err)
	}
	if betaClient.InstallationID() != 312 {
		t.Fatalf("expected beta installation client, got %d", betaClient.InstallationID())
	}
}

func TestServiceSaveActiveAppRetiresSupersededRouteOnly(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	primaryServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               301,
		installationID:      401,
		accountLogin:        "alpha-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "alpha-org/repo-a", Owner: "alpha-org", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer primaryServer.Close()

	original, err := service.Save(ctx, Input{
		Name:           "alpha-old",
		Tags:           []string{"alpha", "old"},
		APIBaseURL:     primaryServer.URL,
		AppID:          301,
		InstallationID: 401,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "alpha-secret-1",
		SelectedRepos:  []string{"alpha-org/repo-a"},
	})
	if err != nil {
		t.Fatalf("save original active config: %v", err)
	}

	otherAppServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               302,
		installationID:      401,
		accountLogin:        "beta-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "beta-org/repo-b", Owner: "beta-org", Name: "repo-b", Private: true, Admin: true},
		},
	})
	defer otherAppServer.Close()

	unrelated, err := service.Save(ctx, Input{
		Name:           "beta-other-app",
		Tags:           []string{"beta"},
		APIBaseURL:     otherAppServer.URL,
		AppID:          302,
		InstallationID: 401,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "beta-secret-1",
		SelectedRepos:  []string{"beta-org/repo-b"},
	})
	if err != nil {
		t.Fatalf("save unrelated active config: %v", err)
	}

	replacement, err := service.Save(ctx, Input{
		Name:           "alpha-new",
		Tags:           []string{"alpha", "new"},
		APIBaseURL:     primaryServer.URL,
		AppID:          301,
		InstallationID: 401,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "alpha-secret-2",
		SelectedRepos:  []string{"alpha-org/repo-a"},
	})
	if err != nil {
		t.Fatalf("save replacement active config: %v", err)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status after replacement save: %v", err)
	}
	if len(status.ActiveConfigs) != 2 {
		t.Fatalf("expected replacement and unrelated routes to remain active, got %#v", status.ActiveConfigs)
	}
	if !reflect.DeepEqual([]int64{status.ActiveConfigs[0].ID, status.ActiveConfigs[1].ID}, []int64{replacement.Config.ID, unrelated.Config.ID}) {
		t.Fatalf("expected status to show replacement first and unrelated second, got %#v", status.ActiveConfigs)
	}
	if status.ActiveConfig == nil || status.ActiveConfig.ID != replacement.Config.ID {
		t.Fatalf("expected compatibility active config to point at replacement route, got %#v", status.ActiveConfig)
	}

	loadedOriginal, err := service.store.FindGitHubConfigByID(ctx, original.Config.ID)
	if err != nil {
		t.Fatalf("reload original config: %v", err)
	}
	if loadedOriginal.IsActive {
		t.Fatalf("expected original config to be retired, got %#v", loadedOriginal)
	}

	loadedUnrelated, err := service.store.FindGitHubConfigByID(ctx, unrelated.Config.ID)
	if err != nil {
		t.Fatalf("reload unrelated config: %v", err)
	}
	if !loadedUnrelated.IsActive {
		t.Fatalf("expected unrelated config to remain active, got %#v", loadedUnrelated)
	}
}

func TestServicePromoteStagedAppMakesStagedConfigActive(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	activeServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               111,
		installationID:      222,
		accountLogin:        "active-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "active-org/repo-a", Owner: "active-org", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer activeServer.Close()
	if _, err := service.Save(ctx, Input{
		APIBaseURL:     activeServer.URL,
		AppID:          111,
		InstallationID: 222,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "active-webhook-secret",
		SelectedRepos:  []string{"active-org/repo-a"},
	}); err != nil {
		t.Fatalf("save active app config: %v", err)
	}

	stagedServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               123,
		installationID:      456,
		accountLogin:        "staged-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "staged-org/repo-a", Owner: "staged-org", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer stagedServer.Close()
	if _, err := service.SaveStagedApp(ctx, Input{
		APIBaseURL:     stagedServer.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "staged-webhook-secret",
		SelectedRepos:  []string{"staged-org/repo-a"},
	}); err != nil {
		t.Fatalf("save staged app config: %v", err)
	}

	if err := service.PromoteStagedApp(ctx); err != nil {
		t.Fatalf("promote staged app: %v", err)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status after promote: %v", err)
	}
	if status.Source != "cms" || !status.Ready {
		t.Fatalf("expected cms status after promote, got %#v", status)
	}
	if status.ActiveConfig == nil || status.ActiveConfig.AuthMode != store.GitHubAuthModeApp || !status.ActiveConfig.IsActive {
		t.Fatalf("expected app config to become active, got %#v", status.ActiveConfig)
	}
	if status.ActiveConfig.AppID != 123 || status.ActiveConfig.InstallationID != 456 {
		t.Fatalf("expected staged app to become active, got %#v", status.ActiveConfig)
	}
	if status.StagedConfig != nil {
		t.Fatalf("expected staged config to be cleared after promote, got %#v", status.StagedConfig)
	}
}

func TestServicePromoteStagedAppRetiresSupersededRouteOnly(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	primaryServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               501,
		installationID:      601,
		accountLogin:        "alpha-org",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "alpha-org/repo-a", Owner: "alpha-org", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer primaryServer.Close()

	original, err := service.Save(ctx, Input{
		Name:           "alpha-old",
		Tags:           []string{"alpha", "old"},
		APIBaseURL:     primaryServer.URL,
		AppID:          501,
		InstallationID: 601,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "alpha-secret-1",
		SelectedRepos:  []string{"alpha-org/repo-a"},
	})
	if err != nil {
		t.Fatalf("save original active config: %v", err)
	}

	otherBaseServer := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:               501,
		installationID:      601,
		accountLogin:        "alpha-enterprise",
		accountType:         "Organization",
		repositorySelection: "selected",
		repositories: []Repository{
			{FullName: "alpha-enterprise/repo-a", Owner: "alpha-enterprise", Name: "repo-a", Private: true, Admin: true},
		},
	})
	defer otherBaseServer.Close()

	unrelated, err := service.Save(ctx, Input{
		Name:           "alpha-other-base",
		Tags:           []string{"enterprise"},
		APIBaseURL:     otherBaseServer.URL,
		AppID:          501,
		InstallationID: 601,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "alpha-secret-enterprise",
		SelectedRepos:  []string{"alpha-enterprise/repo-a"},
	})
	if err != nil {
		t.Fatalf("save unrelated active config: %v", err)
	}

	staged, err := service.SaveStagedApp(ctx, Input{
		Name:           "alpha-rotated",
		Tags:           []string{"alpha", "new"},
		APIBaseURL:     primaryServer.URL,
		AppID:          501,
		InstallationID: 601,
		PrivateKeyPEM:  testPrivateKeyPEM(t),
		WebhookSecret:  "alpha-secret-2",
		SelectedRepos:  []string{"alpha-org/repo-a"},
	})
	if err != nil {
		t.Fatalf("save staged replacement config: %v", err)
	}

	if err := service.PromoteStagedApp(ctx); err != nil {
		t.Fatalf("promote staged replacement config: %v", err)
	}

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status after promote: %v", err)
	}
	if len(status.ActiveConfigs) != 2 {
		t.Fatalf("expected promoted and unrelated routes to remain active, got %#v", status.ActiveConfigs)
	}
	if !reflect.DeepEqual([]int64{status.ActiveConfigs[0].ID, status.ActiveConfigs[1].ID}, []int64{staged.Config.ID, unrelated.Config.ID}) {
		t.Fatalf("expected status to show promoted route first and unrelated second, got %#v", status.ActiveConfigs)
	}
	if status.ActiveConfig == nil || status.ActiveConfig.ID != staged.Config.ID {
		t.Fatalf("expected compatibility active config to point at promoted route, got %#v", status.ActiveConfig)
	}
	if status.StagedConfig != nil {
		t.Fatalf("expected staged config to be cleared after promote, got %#v", status.StagedConfig)
	}

	loadedOriginal, err := service.store.FindGitHubConfigByID(ctx, original.Config.ID)
	if err != nil {
		t.Fatalf("reload original config: %v", err)
	}
	if loadedOriginal.IsActive {
		t.Fatalf("expected original config to be retired after promote, got %#v", loadedOriginal)
	}

	loadedUnrelated, err := service.store.FindGitHubConfigByID(ctx, unrelated.Config.ID)
	if err != nil {
		t.Fatalf("reload unrelated config: %v", err)
	}
	if !loadedUnrelated.IsActive {
		t.Fatalf("expected unrelated config to remain active after promote, got %#v", loadedUnrelated)
	}
}

func TestServicePromoteStagedAppRejectsLegacyWebhookSecretCollisionUntilCleared(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		Defaults: Config{
			Name:                            "env-gh-app",
			Tags:                            []string{"env", "fallback"},
			APIBaseURL:                      "https://api.github.com",
			AppID:                           900,
			InstallationID:                  901,
			PrivateKeyPEM:                   testPrivateKeyPEM(t),
			WebhookSecret:                   "collision-secret",
			SelectedRepos:                   []string{"env-org/repo"},
			AccountLogin:                    "env-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"env-org/repo"},
		},
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}

	privateKeyCiphertext, err := service.encrypt(testPrivateKeyPEM(t))
	if err != nil {
		t.Fatalf("encrypt private key: %v", err)
	}
	webhookSecretCiphertext, err := service.encrypt("collision-secret")
	if err != nil {
		t.Fatalf("encrypt webhook secret: %v", err)
	}

	if _, err := service.store.SaveActiveGitHubConfig(ctx, store.GitHubConfig{
		Name:                            "legacy-active",
		Tags:                            []string{"active"},
		APIBaseURL:                      "https://api.github.com",
		AuthMode:                        store.GitHubAuthModeApp,
		AppID:                           111,
		InstallationID:                  222,
		PrivateKeyCiphertext:            privateKeyCiphertext,
		WebhookSecretCiphertext:         webhookSecretCiphertext,
		SelectedRepos:                   []string{"active-org/repo-a"},
		AccountLogin:                    "active-org",
		AccountType:                     "Organization",
		InstallationState:               "active",
		InstallationRepositorySelection: "selected",
		InstallationRepositories:        []string{"active-org/repo-a"},
	}); err != nil {
		t.Fatalf("seed legacy active config: %v", err)
	}

	stagedRecord, err := service.store.SaveStagedGitHubConfig(ctx, store.GitHubConfig{
		Name:                            "legacy-staged",
		Tags:                            []string{"staged"},
		APIBaseURL:                      "https://api.github.com",
		AuthMode:                        store.GitHubAuthModeApp,
		AppID:                           123,
		InstallationID:                  456,
		PrivateKeyCiphertext:            privateKeyCiphertext,
		WebhookSecretCiphertext:         webhookSecretCiphertext,
		SelectedRepos:                   []string{"staged-org/repo-a"},
		AccountLogin:                    "staged-org",
		AccountType:                     "Organization",
		InstallationState:               "active",
		InstallationRepositorySelection: "selected",
		InstallationRepositories:        []string{"staged-org/repo-a"},
	})
	if err != nil {
		t.Fatalf("seed legacy staged config: %v", err)
	}

	err = service.PromoteStagedApp(ctx)
	if err == nil {
		t.Fatalf("expected promotion to reject legacy collision with active/env secrets")
	}
	if !strings.Contains(err.Error(), "cannot be promoted") {
		t.Fatalf("expected promotion collision error, got %v", err)
	}

	if err := service.Clear(ctx); err != nil {
		t.Fatalf("clear active configs: %v", err)
	}

	err = service.PromoteStagedApp(ctx)
	if err == nil {
		t.Fatalf("expected env fallback collision to still block promotion after clearing active configs")
	}

	envClearedService, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new env-cleared github service: %v", err)
	}

	if err := envClearedService.PromoteStagedApp(ctx); err != nil {
		t.Fatalf("promote staged app after clearing collisions: %v", err)
	}

	status, err := envClearedService.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status after successful promote: %v", err)
	}
	if status.StagedConfig != nil {
		t.Fatalf("expected staged config to be cleared after successful promote, got %#v", status.StagedConfig)
	}
	if status.ActiveConfig == nil || status.ActiveConfig.ID != stagedRecord.ID || !status.ActiveConfig.IsActive {
		t.Fatalf("expected staged config to become active after clearing collisions, got %#v", status.ActiveConfig)
	}
}

func TestServiceManifestRoundTripStoresPendingForSession(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	manifestPrivateKey := testPrivateKeyPEM(t)

	originalExchange := exchangeManifestCode
	exchangeManifestCode = func(context.Context, string, string) (ManifestConversion, error) {
		return ManifestConversion{
			AppID:         777,
			Name:          "OhoCI-localhost-1234",
			Slug:          "ohoci-localhost-1234",
			HTMLURL:       "https://github.com/apps/ohoci-localhost-1234",
			PrivateKeyPEM: manifestPrivateKey,
			WebhookSecret: "manifest-webhook-secret",
		}, nil
	}
	t.Cleanup(func() {
		exchangeManifestCode = originalExchange
	})

	start, err := service.StartManifest("session-token", "")
	if err != nil {
		t.Fatalf("start manifest: %v", err)
	}

	parsedStartURL, err := url.Parse(start.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	state := parsedStartURL.Query().Get("state")
	if strings.TrimSpace(state) == "" {
		t.Fatalf("expected signed manifest state in redirect url, got %q", start.RedirectURL)
	}

	launch, err := service.ManifestLaunch("session-token", state)
	if err != nil {
		t.Fatalf("manifest launch: %v", err)
	}
	if launch.PostURL != "https://github.com/settings/apps/new?state="+url.QueryEscape(state) {
		t.Fatalf("unexpected post url: %q", launch.PostURL)
	}
	var manifest struct {
		DefaultEvents []string `json:"default_events"`
	}
	if err := json.Unmarshal([]byte(launch.ManifestJSON), &manifest); err != nil {
		t.Fatalf("unmarshal manifest json: %v", err)
	}
	if !reflect.DeepEqual(manifest.DefaultEvents, []string{"workflow_job"}) {
		t.Fatalf("expected manifest to request only explicit workflow_job opt-in and rely on GitHub default installation lifecycle events, got %#v", manifest.DefaultEvents)
	}
	if !strings.Contains(launch.ManifestJSON, "\"administration\":\"write\"") || !strings.Contains(launch.ManifestJSON, "\"actions\":\"read\"") {
		t.Fatalf("expected manifest to include required permissions, got %s", launch.ManifestJSON)
	}

	if _, err := service.ManifestLaunch("different-session", state); err == nil {
		t.Fatalf("expected manifest launch to reject another session")
	}

	pending, err := service.CompleteManifest(ctx, "session-token", state, "manifest-code")
	if err != nil {
		t.Fatalf("complete manifest: %v", err)
	}
	installURL, err := url.Parse(pending.InstallURL)
	if err != nil {
		t.Fatalf("parse install url: %v", err)
	}
	if pending.AppID != 777 ||
		installURL.Scheme != "https" ||
		installURL.Host != "github.com" ||
		installURL.Path != "/apps/ohoci-localhost-1234/installations/new" ||
		strings.TrimSpace(installURL.Query().Get("state")) == "" ||
		pending.OwnerTarget != githubManifestOwnerTargetPersonal ||
		pending.TransferURL != "" {
		t.Fatalf("unexpected pending manifest: %#v", pending)
	}
	if pending.AppSettingsURL != "https://github.com/settings/apps/ohoci-localhost-1234" {
		t.Fatalf("unexpected app settings url: %#v", pending)
	}

	loadedPending, err := service.PendingManifest(ctx, "session-token")
	if err != nil {
		t.Fatalf("pending manifest: %v", err)
	}
	if loadedPending == nil || loadedPending.AppID != pending.AppID || loadedPending.WebhookSecret != pending.WebhookSecret {
		t.Fatalf("expected pending manifest for session, got %#v", loadedPending)
	}

	if err := service.ClearPendingManifest(ctx, "session-token"); err != nil {
		t.Fatalf("clear pending manifest: %v", err)
	}
	loadedPending, err = service.PendingManifest(ctx, "session-token")
	if err != nil {
		t.Fatalf("pending manifest after clear: %v", err)
	}
	if loadedPending != nil {
		t.Fatalf("expected pending manifest to be cleared, got %#v", loadedPending)
	}
}

func TestServiceManifestRoundTripLaunchesDirectlyIntoOrganization(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	manifestPrivateKey := testPrivateKeyPEM(t)

	originalExchange := exchangeManifestCode
	exchangeManifestCode = func(context.Context, string, string) (ManifestConversion, error) {
		return ManifestConversion{
			AppID:         888,
			Name:          "OhoCI-localhost-5678",
			Slug:          "ohoci-localhost-5678",
			HTMLURL:       "https://github.com/apps/ohoci-localhost-5678",
			PrivateKeyPEM: manifestPrivateKey,
			WebhookSecret: "manifest-webhook-secret",
		}, nil
	}
	t.Cleanup(func() {
		exchangeManifestCode = originalExchange
	})

	start, err := service.StartManifestWithInput("session-token", ManifestStartInput{
		OwnerTarget:      githubManifestOwnerTargetOrganization,
		OrganizationSlug: "Example-Org",
	})
	if err != nil {
		t.Fatalf("start manifest: %v", err)
	}

	parsedStartURL, err := url.Parse(start.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	state := parsedStartURL.Query().Get("state")
	if strings.TrimSpace(state) == "" {
		t.Fatalf("expected signed manifest state in redirect url, got %q", start.RedirectURL)
	}

	launch, err := service.ManifestLaunch("session-token", state)
	if err != nil {
		t.Fatalf("manifest launch: %v", err)
	}
	if launch.PostURL != "https://github.com/organizations/example-org/settings/apps/new?state="+url.QueryEscape(state) {
		t.Fatalf("expected organization github.com manifest launch, got %q", launch.PostURL)
	}

	pending, err := service.CompleteManifest(ctx, "session-token", state, "manifest-code")
	if err != nil {
		t.Fatalf("complete manifest: %v", err)
	}
	if pending.OwnerTarget != githubManifestOwnerTargetOrganization {
		t.Fatalf("expected organization owner target in pending manifest, got %#v", pending)
	}
	if pending.TransferURL != "" {
		t.Fatalf("expected transfer url to stay empty, got %#v", pending)
	}
	if pending.AppSettingsURL != "https://github.com/settings/apps/ohoci-localhost-5678" {
		t.Fatalf("unexpected app settings url: %#v", pending)
	}
}

func TestServiceStartManifestRejectsMissingOrMalformedOrganizationSlug(t *testing.T) {
	db, err := store.Open(context.Background(), "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	testCases := []struct {
		name        string
		slug        string
		expectedErr string
	}{
		{name: "missing", slug: "", expectedErr: "required"},
		{name: "malformed", slug: "example_org", expectedErr: "single hyphens"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := service.StartManifestWithInput("session-token", ManifestStartInput{
				OwnerTarget:      githubManifestOwnerTargetOrganization,
				OrganizationSlug: testCase.slug,
			})
			if err == nil || !strings.Contains(err.Error(), testCase.expectedErr) {
				t.Fatalf("expected manifest start to fail with %q, got %v", testCase.expectedErr, err)
			}
		})
	}
}

func TestServiceValidateManifestInstallReturnAcceptsSessionPendingFallback(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	manifestPrivateKey := testPrivateKeyPEM(t)

	validationAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/app/installations" {
			http.NotFound(w, r)
			return
		}
		writePayload(t, w, []map[string]any{
			{
				"id":                   321654,
				"repository_selection": "selected",
				"html_url":             "https://github.com/organizations/example/settings/installations/321654",
				"app_slug":             "ohoci-localhost-1234",
				"account": map[string]any{
					"login": "example",
					"type":  "Organization",
				},
			},
		})
	}))
	defer validationAPI.Close()

	restoreGitHubAPI := rewriteGitHubAPITransport(t, validationAPI.URL)
	defer restoreGitHubAPI()

	originalExchange := exchangeManifestCode
	exchangeManifestCode = func(context.Context, string, string) (ManifestConversion, error) {
		return ManifestConversion{
			AppID:         777,
			Name:          "OhoCI-localhost-1234",
			Slug:          "ohoci-localhost-1234",
			HTMLURL:       "https://github.com/apps/ohoci-localhost-1234",
			PrivateKeyPEM: manifestPrivateKey,
			WebhookSecret: "manifest-webhook-secret",
		}, nil
	}
	t.Cleanup(func() {
		exchangeManifestCode = originalExchange
	})

	start, err := service.StartManifest("session-token", "")
	if err != nil {
		t.Fatalf("start manifest: %v", err)
	}

	parsedStartURL, err := url.Parse(start.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	state := parsedStartURL.Query().Get("state")
	if strings.TrimSpace(state) == "" {
		t.Fatalf("expected signed state in redirect url, got %q", start.RedirectURL)
	}

	pending, err := service.CompleteManifest(ctx, "session-token", state, "manifest-code")
	if err != nil {
		t.Fatalf("complete manifest: %v", err)
	}
	installURL, err := url.Parse(pending.InstallURL)
	if err != nil {
		t.Fatalf("parse install url: %v", err)
	}
	installState := installURL.Query().Get("state")
	if strings.TrimSpace(installState) == "" {
		t.Fatalf("expected install state in install url, got %q", pending.InstallURL)
	}

	if err := service.ValidateManifestInstallReturn(ctx, "session-token", installState, "321654"); err != nil {
		t.Fatalf("validate install return with state: %v", err)
	}
	if err := service.ValidateManifestInstallReturn(ctx, "session-token", "", "321654"); err != nil {
		t.Fatalf("validate install return with pending fallback: %v", err)
	}
	if err := service.ValidateManifestInstallReturn(ctx, "session-token", "", ""); err == nil {
		t.Fatalf("expected install return without installation id to fail")
	}
	if err := service.ValidateManifestInstallReturn(ctx, "session-token", "", "999999"); err == nil {
		t.Fatalf("expected unverified installation id to fail")
	}

	if err := service.ClearPendingManifest(ctx, "session-token"); err != nil {
		t.Fatalf("clear pending manifest: %v", err)
	}
	if err := service.ValidateManifestInstallReturn(ctx, "session-token", "", "321654"); err == nil {
		t.Fatalf("expected install return without state or pending draft to fail")
	}
}

func TestServicePendingManifestPersistsAcrossServiceRestart(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	manifestPrivateKey := testPrivateKeyPEM(t)

	originalExchange := exchangeManifestCode
	exchangeManifestCode = func(context.Context, string, string) (ManifestConversion, error) {
		return ManifestConversion{
			AppID:         777,
			Name:          "OhoCI-localhost-1234",
			Slug:          "ohoci-localhost-1234",
			HTMLURL:       "https://github.com/apps/ohoci-localhost-1234",
			PrivateKeyPEM: manifestPrivateKey,
			WebhookSecret: "manifest-webhook-secret",
		}, nil
	}
	t.Cleanup(func() {
		exchangeManifestCode = originalExchange
	})

	start, err := service.StartManifest("session-token", "")
	if err != nil {
		t.Fatalf("start manifest: %v", err)
	}
	redirectURL, err := url.Parse(start.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	state := redirectURL.Query().Get("state")
	if _, err := service.CompleteManifest(ctx, "session-token", state, "manifest-code"); err != nil {
		t.Fatalf("complete manifest: %v", err)
	}

	restarted, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new restarted service: %v", err)
	}

	pending, err := restarted.PendingManifest(ctx, "session-token")
	if err != nil {
		t.Fatalf("load pending manifest after restart: %v", err)
	}
	if pending == nil || pending.AppID != 777 || pending.WebhookSecret != "manifest-webhook-secret" {
		t.Fatalf("expected pending manifest to survive restart, got %#v", pending)
	}
}

func TestServiceDiscoverInstallationsAutoSelectsSingleInstallation(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service, err := NewService(db, ServiceOptions{
		EncryptionKey: "top-secret",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	server := newGitHubAppServer(t, gitHubAppFixtureOptions{
		appID:          123,
		installationID: 456,
		accountLogin:   "example",
		accountType:    "Organization",
		appInstallations: []AppInstallation{
			{
				ID:                  456,
				AccountLogin:        "example",
				AccountType:         "Organization",
				RepositorySelection: "selected",
				HTMLURL:             "https://github.com/organizations/example/settings/installations/456",
				AppSlug:             "ohoci-example",
			},
		},
	})
	defer server.Close()

	result, err := service.DiscoverInstallations(ctx, Input{
		APIBaseURL:    server.URL,
		AppID:         123,
		PrivateKeyPEM: testPrivateKeyPEM(t),
	})
	if err != nil {
		t.Fatalf("discover installations: %v", err)
	}
	if result.AutoInstallationID != 456 || len(result.Installations) != 1 {
		t.Fatalf("expected auto-selected installation, got %#v", result)
	}
}

type gitHubAppFixtureOptions struct {
	appID               int64
	installationID      int64
	accountLogin        string
	accountType         string
	repositorySelection string
	repositories        []Repository
	appInstallations    []AppInstallation
}

func newGitHubAppServer(t *testing.T, options gitHubAppFixtureOptions) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app":
			writePayload(t, w, map[string]any{"id": options.appID})
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/"+int64Text(options.installationID)+"/access_tokens":
			writePayload(t, w, map[string]any{"token": "installation-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations/"+int64Text(options.installationID):
			writePayload(t, w, map[string]any{
				"account": map[string]any{
					"login": options.accountLogin,
					"type":  options.accountType,
				},
				"repository_selection": options.repositorySelection,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations":
			installations := options.appInstallations
			if len(installations) == 0 && options.installationID > 0 {
				installations = []AppInstallation{
					{
						ID:                  options.installationID,
						AccountLogin:        options.accountLogin,
						AccountType:         options.accountType,
						RepositorySelection: options.repositorySelection,
						HTMLURL:             "https://github.com/organizations/" + options.accountLogin + "/settings/installations/" + int64Text(options.installationID),
					},
				}
			}
			payload := make([]map[string]any, 0, len(installations))
			for _, installation := range installations {
				payload = append(payload, map[string]any{
					"id":                   installation.ID,
					"repository_selection": installation.RepositorySelection,
					"html_url":             installation.HTMLURL,
					"app_slug":             installation.AppSlug,
					"account": map[string]any{
						"login": installation.AccountLogin,
						"type":  installation.AccountType,
					},
				})
			}
			writePayload(t, w, payload)
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			repositories := make([]map[string]any, 0, len(options.repositories))
			for _, repository := range options.repositories {
				repositories = append(repositories, map[string]any{
					"full_name": repository.FullName,
					"name":      repository.Name,
					"private":   repository.Private,
					"owner":     map[string]any{"login": repository.Owner},
					"permissions": map[string]any{
						"admin": repository.Admin,
					},
				})
			}
			writePayload(t, w, map[string]any{"repositories": repositories})
		default:
			http.NotFound(w, r)
		}
	}))
}

func rewriteGitHubAPITransport(t *testing.T, targetURL string) func() {
	t.Helper()
	parsed, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}

	originalTransport := http.DefaultTransport
	baseTransport, ok := originalTransport.(*http.Transport)
	if !ok {
		t.Fatalf("expected default transport to be *http.Transport, got %T", originalTransport)
	}
	http.DefaultTransport = baseTransport.Clone()
	rewriter := &githubAPIRewriteTransport{
		target: parsed,
		next:   http.DefaultTransport,
	}
	http.DefaultTransport = rewriter
	return func() {
		http.DefaultTransport = originalTransport
	}
}

type githubAPIRewriteTransport struct {
	target *url.URL
	next   http.RoundTripper
}

func (t *githubAPIRewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if strings.EqualFold(request.URL.Host, "api.github.com") {
		cloned := request.Clone(request.Context())
		cloned.URL.Scheme = t.target.Scheme
		cloned.URL.Host = t.target.Host
		cloned.Host = t.target.Host
		return t.next.RoundTrip(cloned)
	}
	return t.next.RoundTrip(request)
}

func int64Text(value int64) string {
	return strconv.FormatInt(value, 10)
}

func writePayload(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode payload: %v", err)
	}
}

func signFixture(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func testPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	bytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: bytes}
	return string(pem.EncodeToMemory(block))
}
