package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ohoci/internal/auth"
	"ohoci/internal/cleanup"
	"ohoci/internal/config"
	"ohoci/internal/githubapp"
	"ohoci/internal/oci"
	"ohoci/internal/ocibilling"
	"ohoci/internal/ocicredentials"
	"ohoci/internal/ociruntime"
	"ohoci/internal/runnerimages"
	"ohoci/internal/session"
	setupsvc "ohoci/internal/setup"
	"ohoci/internal/store"
)

func TestSetupGateBlocksDashboardButAllowsSetupEndpoints(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	setupResponse := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/setup", nil, cfg.SessionCookieName, token)
	if setupResponse.Code != http.StatusOK {
		t.Fatalf("expected setup endpoint to stay available, got %d: %s", setupResponse.Code, setupResponse.Body.String())
	}
	var setupStatus setupsvc.Status
	if err := json.Unmarshal(setupResponse.Body.Bytes(), &setupStatus); err != nil {
		t.Fatalf("decode setup status: %v", err)
	}
	if setupStatus.Ready {
		t.Fatalf("expected setup to be incomplete, got %#v", setupStatus)
	}

	jobsResponse := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/jobs", nil, cfg.SessionCookieName, token)
	if jobsResponse.Code != http.StatusForbidden {
		t.Fatalf("expected jobs endpoint to be gated, got %d: %s", jobsResponse.Code, jobsResponse.Body.String())
	}

	runtimeResponse := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/oci/runtime", nil, cfg.SessionCookieName, token)
	if runtimeResponse.Code != http.StatusOK {
		t.Fatalf("expected setup endpoint /oci/runtime to stay available, got %d: %s", runtimeResponse.Code, runtimeResponse.Body.String())
	}
}

func TestOCIAuthInspectParsesUploadedConfig(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/oci/auth/inspect", ocicredentials.InspectInput{
		ConfigText: `
[DEFAULT]
user=ocid1.user.oc1..user
fingerprint=11:22:33:44
tenancy=ocid1.tenancy.oc1..tenancy
region=ap-seoul-1
`,
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected inspect success, got %d: %s", response.Code, response.Body.String())
	}
	var result ocicredentials.InspectResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode inspect result: %v", err)
	}
	if result.SelectedProfile != "DEFAULT" || result.Region != "ap-seoul-1" {
		t.Fatalf("unexpected inspect result: %#v", result)
	}
	if result.HasEmbeddedPrivateKey {
		t.Fatalf("expected inspect to avoid requiring a private key upload")
	}
}

func TestGitHubConfigSaveClearAndWebhookUseCMSOverride(t *testing.T) {
	ctx := context.Background()
	activeAppAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app":
			writeJSONBody(t, w, map[string]any{"id": 123})
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/456/access_tokens":
			writeJSONBody(t, w, map[string]any{"token": "installation-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations/456":
			writeJSONBody(t, w, map[string]any{
				"account": map[string]any{
					"login": "example",
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
		default:
			http.NotFound(w, r)
		}
	}))
	defer activeAppAPI.Close()

	stagedAppAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app":
			writeJSONBody(t, w, map[string]any{"id": 987})
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/654/access_tokens":
			writeJSONBody(t, w, map[string]any{"token": "staged-installation-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations/654":
			writeJSONBody(t, w, map[string]any{
				"account": map[string]any{
					"login": "staged-org",
					"type":  "Organization",
				},
				"repository_selection": "selected",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			writeJSONBody(t, w, map[string]any{
				"repositories": []map[string]any{
					{
						"full_name":   "staged-org/repo-a",
						"name":        "repo-a",
						"private":     true,
						"owner":       map[string]any{"login": "staged-org"},
						"permissions": map[string]any{"admin": true},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer stagedAppAPI.Close()

	handler, cfg, db, controller := newBackendTestHandler(t, backendTestOptions{
		githubDefaults: githubapp.Config{
			APIBaseURL:                      "https://invalid.example.test",
			AppID:                           111,
			InstallationID:                  222,
			PrivateKeyPEM:                   testPrivateKey,
			WebhookSecret:                   "env-secret",
			SelectedRepos:                   []string{"wrong/repo"},
			AccountLogin:                    "env-org",
			AccountType:                     "Organization",
			InstallationState:               "active",
			InstallationRepositorySelection: "selected",
			InstallationRepositories:        []string{"wrong/repo"},
		},
	})
	token := authenticatedToken(t, handler.auth)

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
	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..example",
		ImageOCID:          "ocid1.image.oc1..ubuntu",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}

	testResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/test", githubapp.Input{
		APIBaseURL:     activeAppAPI.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		SelectedRepos:  []string{"example/repo"},
	}, cfg.SessionCookieName, token)
	if testResponse.Code != http.StatusOK {
		t.Fatalf("expected github config test success, got %d: %s", testResponse.Code, testResponse.Body.String())
	}
	var testResult githubapp.TestResult
	if err := json.Unmarshal(testResponse.Body.Bytes(), &testResult); err != nil {
		t.Fatalf("decode github config test result: %v", err)
	}
	if testResult.Config.AuthMode != store.GitHubAuthModeApp || !testResult.Config.InstallationReady {
		t.Fatalf("expected app test result, got %#v", testResult.Config)
	}

	saveResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config", githubapp.Input{
		APIBaseURL:     activeAppAPI.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "cms-active-secret",
		SelectedRepos:  []string{"example/repo"},
	}, cfg.SessionCookieName, token)
	if saveResponse.Code != http.StatusOK {
		t.Fatalf("expected github config save success, got %d: %s", saveResponse.Code, saveResponse.Body.String())
	}

	statusResponse := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/github/config", nil, cfg.SessionCookieName, token)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("expected github status success, got %d: %s", statusResponse.Code, statusResponse.Body.String())
	}
	var status githubapp.Status
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode github status: %v", err)
	}
	if status.Source != "cms" || !status.Ready {
		t.Fatalf("expected cms github status, got %#v", status)
	}
	if status.ActiveConfig == nil || status.ActiveConfig.AuthMode != store.GitHubAuthModeApp {
		t.Fatalf("expected active app config after save, got %#v", status.ActiveConfig)
	}

	stageResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/staged", githubapp.Input{
		APIBaseURL:     stagedAppAPI.URL,
		AppID:          987,
		InstallationID: 654,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "cms-staged-secret",
		SelectedRepos:  []string{"staged-org/repo-a"},
	}, cfg.SessionCookieName, token)
	if stageResponse.Code != http.StatusOK {
		t.Fatalf("expected staged github config save success, got %d: %s", stageResponse.Code, stageResponse.Body.String())
	}
	var stageResult githubapp.TestResult
	if err := json.Unmarshal(stageResponse.Body.Bytes(), &stageResult); err != nil {
		t.Fatalf("decode staged github result: %v", err)
	}
	if stageResult.Config.AuthMode != store.GitHubAuthModeApp || !stageResult.Config.IsStaged {
		t.Fatalf("expected staged app result, got %#v", stageResult.Config)
	}
	if !stageResult.Config.InstallationReady || stageResult.Config.InstallationState != "active" {
		t.Fatalf("expected staged app installation metadata, got %#v", stageResult.Config)
	}

	statusResponse = performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/github/config", nil, cfg.SessionCookieName, token)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("expected github status success after staged save, got %d: %s", statusResponse.Code, statusResponse.Body.String())
	}
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode github status after staged save: %v", err)
	}
	if status.ActiveConfig == nil || status.ActiveConfig.AuthMode != store.GitHubAuthModeApp {
		t.Fatalf("expected active app config to remain active, got %#v", status.ActiveConfig)
	}
	if status.StagedConfig == nil || status.StagedConfig.AuthMode != store.GitHubAuthModeApp {
		t.Fatalf("expected staged app config to be visible, got %#v", status.StagedConfig)
	}
	if !status.StagedConfig.InstallationReady || len(status.StagedConfig.InstallationRepositories) != 1 {
		t.Fatalf("expected staged app installation scope in status, got %#v", status.StagedConfig)
	}
	if status.EffectiveConfig.AuthMode != store.GitHubAuthModeApp {
		t.Fatalf("expected active app config to remain effective, got %#v", status.EffectiveConfig)
	}

	clearStagedResponse := performJSONRequest(t, handler.handler, http.MethodDelete, "/api/v1/github/config/staged", nil, cfg.SessionCookieName, token)
	if clearStagedResponse.Code != http.StatusOK {
		t.Fatalf("expected staged github clear success, got %d: %s", clearStagedResponse.Code, clearStagedResponse.Body.String())
	}
	var clearStagedPayload struct {
		Success bool             `json:"success"`
		Status  githubapp.Status `json:"status"`
	}
	if err := json.Unmarshal(clearStagedResponse.Body.Bytes(), &clearStagedPayload); err != nil {
		t.Fatalf("decode staged clear response: %v", err)
	}
	if !clearStagedPayload.Success {
		t.Fatalf("expected staged clear success payload, got %#v", clearStagedPayload)
	}
	if clearStagedPayload.Status.ActiveConfig == nil || clearStagedPayload.Status.ActiveConfig.AuthMode != store.GitHubAuthModeApp {
		t.Fatalf("expected active app config to remain after staged clear, got %#v", clearStagedPayload.Status.ActiveConfig)
	}
	if clearStagedPayload.Status.StagedConfig != nil {
		t.Fatalf("expected staged config to be removed, got %#v", clearStagedPayload.Status.StagedConfig)
	}

	webhookResponse := sendWebhookWithSecret(t, handler.handler, webhookFixture{
		Action:     "queued",
		DeliveryID: "delivery-cms-override",
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
	}, "cms-active-secret")
	if webhookResponse.Code != http.StatusAccepted {
		t.Fatalf("expected webhook acceptance via CMS config, got %d: %s", webhookResponse.Code, webhookResponse.Body.String())
	}
	if len(controller.LaunchRequests) != 1 {
		t.Fatalf("expected runner launch through CMS GitHub config, got %d", len(controller.LaunchRequests))
	}

	clearResponse := performJSONRequest(t, handler.handler, http.MethodDelete, "/api/v1/github/config", nil, cfg.SessionCookieName, token)
	if clearResponse.Code != http.StatusOK {
		t.Fatalf("expected github clear success, got %d: %s", clearResponse.Code, clearResponse.Body.String())
	}
	statusResponse = performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/github/config", nil, cfg.SessionCookieName, token)
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode github status after clear: %v", err)
	}
	if status.Source != "env" {
		t.Fatalf("expected env fallback after clear, got %#v", status)
	}
}

func TestGitHubConfigStagedPromoteCutoverRoute(t *testing.T) {
	ctx := context.Background()
	activeAppAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer activeAppAPI.Close()

	stagedAppAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app":
			writeJSONBody(t, w, map[string]any{"id": 321})
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/654/access_tokens":
			writeJSONBody(t, w, map[string]any{"token": "staged-installation-token"})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations/654":
			writeJSONBody(t, w, map[string]any{
				"account": map[string]any{
					"login": "staged-org",
					"type":  "Organization",
				},
				"repository_selection": "selected",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			writeJSONBody(t, w, map[string]any{
				"repositories": []map[string]any{
					{
						"full_name":   "staged-org/repo-a",
						"name":        "repo-a",
						"private":     true,
						"owner":       map[string]any{"login": "staged-org"},
						"permissions": map[string]any{"admin": true},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer stagedAppAPI.Close()

	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

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
	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..example",
		ImageOCID:          "ocid1.image.oc1..ubuntu",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}

	if saveResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config", githubapp.Input{
		APIBaseURL:     activeAppAPI.URL,
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "active-webhook-secret",
		SelectedRepos:  []string{"app-org/repo-a"},
	}, cfg.SessionCookieName, token); saveResponse.Code != http.StatusOK {
		t.Fatalf("expected github config save success, got %d: %s", saveResponse.Code, saveResponse.Body.String())
	}
	if stageResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/staged", githubapp.Input{
		APIBaseURL:     stagedAppAPI.URL,
		AppID:          321,
		InstallationID: 654,
		PrivateKeyPEM:  testPrivateKey,
		WebhookSecret:  "staged-webhook-secret",
		SelectedRepos:  []string{"staged-org/repo-a"},
	}, cfg.SessionCookieName, token); stageResponse.Code != http.StatusOK {
		t.Fatalf("expected staged github config save success, got %d: %s", stageResponse.Code, stageResponse.Body.String())
	}

	promoteResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/staged/promote", nil, cfg.SessionCookieName, token)
	if promoteResponse.Code != http.StatusOK {
		t.Fatalf("expected staged promote success, got %d: %s", promoteResponse.Code, promoteResponse.Body.String())
	}
	var promotePayload struct {
		Success bool             `json:"success"`
		Status  githubapp.Status `json:"status"`
	}
	if err := json.Unmarshal(promoteResponse.Body.Bytes(), &promotePayload); err != nil {
		t.Fatalf("decode promote response: %v", err)
	}
	if !promotePayload.Success {
		t.Fatalf("expected promote success payload, got %#v", promotePayload)
	}
	if promotePayload.Status.ActiveConfig == nil || promotePayload.Status.ActiveConfig.AuthMode != store.GitHubAuthModeApp {
		t.Fatalf("expected promoted app config to become active, got %#v", promotePayload.Status)
	}
	if promotePayload.Status.ActiveConfig.AppID != 321 || promotePayload.Status.ActiveConfig.InstallationID != 654 {
		t.Fatalf("expected promoted staged app config to become active, got %#v", promotePayload.Status.ActiveConfig)
	}
	if promotePayload.Status.StagedConfig != nil {
		t.Fatalf("expected staged config to be cleared on promote, got %#v", promotePayload.Status)
	}
	if promotePayload.Status.EffectiveConfig.AuthMode != store.GitHubAuthModeApp {
		t.Fatalf("expected promoted app config to be effective, got %#v", promotePayload.Status)
	}
}

func TestGitHubManifestHelperFlowStoresPendingDraftWithPersonalOwnerByDefault(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	conversionAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app-manifests/manifest-code/conversions":
			writeJSONBody(t, w, map[string]any{
				"id":             999,
				"name":           "OhoCI-localhost-999",
				"slug":           "ohoci-localhost-999",
				"html_url":       "https://github.com/apps/ohoci-localhost-999",
				"pem":            testPrivateKey,
				"webhook_secret": "manifest-secret",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations":
			writeJSONBody(t, w, []map[string]any{
				{
					"id":                   321654,
					"repository_selection": "selected",
					"html_url":             "https://github.com/organizations/example/settings/installations/321654",
					"app_slug":             "ohoci-localhost-999",
					"account": map[string]any{
						"login": "example",
						"type":  "Organization",
					},
				},
			})
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer conversionAPI.Close()

	restoreGitHubAPI := rewriteGitHubAPITransport(t, conversionAPI.URL)
	defer restoreGitHubAPI()

	startResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/manifest/start", map[string]any{}, cfg.SessionCookieName, token)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("expected manifest start success, got %d: %s", startResponse.Code, startResponse.Body.String())
	}
	var startPayload struct {
		RedirectURL string `json:"redirectUrl"`
	}
	if err := json.Unmarshal(startResponse.Body.Bytes(), &startPayload); err != nil {
		t.Fatalf("decode start payload: %v", err)
	}
	redirectURL, err := url.Parse(startPayload.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	state := redirectURL.Query().Get("state")
	if strings.TrimSpace(state) == "" {
		t.Fatalf("expected signed state in redirect url, got %q", startPayload.RedirectURL)
	}

	launchResponse := performRequest(t, handler.handler, http.MethodGet, redirectURL.RequestURI(), nil, cfg.SessionCookieName, token)
	if launchResponse.Code != http.StatusOK {
		t.Fatalf("expected manifest launch page, got %d: %s", launchResponse.Code, launchResponse.Body.String())
	}
	if !strings.Contains(launchResponse.Body.String(), "https://github.com/settings/apps/new?state=") {
		t.Fatalf("expected launch page to post to GitHub, got %s", launchResponse.Body.String())
	}

	callbackResponse := performRequest(
		t,
		handler.handler,
		http.MethodGet,
		"/api/v1/github/config/manifest/callback?code=manifest-code&state="+url.QueryEscape(state),
		nil,
		cfg.SessionCookieName,
		token,
	)
	if callbackResponse.Code != http.StatusFound {
		t.Fatalf("expected callback redirect, got %d: %s", callbackResponse.Code, callbackResponse.Body.String())
	}
	callbackLocation := callbackResponse.Header().Get("Location")
	callbackURL, err := url.Parse(callbackLocation)
	if err != nil {
		t.Fatalf("parse callback redirect: %v", err)
	}
	if callbackURL.Scheme != "https" || callbackURL.Host != "github.com" || callbackURL.Path != "/apps/ohoci-localhost-999/installations/new" {
		t.Fatalf("unexpected callback redirect: %q", callbackLocation)
	}
	installState := callbackURL.Query().Get("state")
	if strings.TrimSpace(installState) == "" {
		t.Fatalf("expected install redirect to preserve state, got %q", callbackLocation)
	}

	completeResponse := performRequest(
		t,
		handler.handler,
		http.MethodGet,
		"/api/v1/github/config/manifest/callback?source=install&installation_id=321654&state="+url.QueryEscape(installState),
		nil,
		cfg.SessionCookieName,
		token,
	)
	if completeResponse.Code != http.StatusFound {
		t.Fatalf("expected install completion redirect, got %d: %s", completeResponse.Code, completeResponse.Body.String())
	}
	completeLocation := completeResponse.Header().Get("Location")
	completeURL, err := url.Parse(completeLocation)
	if err != nil {
		t.Fatalf("parse install completion redirect: %v", err)
	}
	if completeURL.Scheme != "http" || completeURL.Host != "localhost:8080" || completeURL.Path != "/" {
		t.Fatalf("unexpected install completion redirect: %q", completeLocation)
	}
	if completeURL.Query().Get("github_manifest") != "installed" {
		t.Fatalf("expected install completion success marker, got %q", completeLocation)
	}
	if completeURL.Query().Get("github_installation_id") != "321654" {
		t.Fatalf("expected installation id on return redirect, got %q", completeLocation)
	}

	pendingResponse := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/github/config/manifest/pending", nil, cfg.SessionCookieName, token)
	if pendingResponse.Code != http.StatusOK {
		t.Fatalf("expected pending manifest success, got %d: %s", pendingResponse.Code, pendingResponse.Body.String())
	}
	var pendingPayload struct {
		Pending *githubapp.PendingManifest `json:"pending"`
	}
	if err := json.Unmarshal(pendingResponse.Body.Bytes(), &pendingPayload); err != nil {
		t.Fatalf("decode pending payload: %v", err)
	}
	if pendingPayload.Pending == nil || pendingPayload.Pending.AppID != 999 {
		t.Fatalf("expected pending manifest credentials, got %#v", pendingPayload.Pending)
	}
	if pendingPayload.Pending.OwnerTarget != "personal" {
		t.Fatalf("expected personal owner context in pending manifest, got %#v", pendingPayload.Pending)
	}
	if pendingPayload.Pending.InstallURL != callbackLocation {
		t.Fatalf("unexpected install url: %#v", pendingPayload.Pending)
	}
	if pendingPayload.Pending.TransferURL != "" {
		t.Fatalf("expected personal draft to skip transfer guidance, got %#v", pendingPayload.Pending)
	}

	clearResponse := performJSONRequest(t, handler.handler, http.MethodDelete, "/api/v1/github/config/manifest/pending", nil, cfg.SessionCookieName, token)
	if clearResponse.Code != http.StatusOK {
		t.Fatalf("expected pending manifest clear success, got %d: %s", clearResponse.Code, clearResponse.Body.String())
	}
}

func TestGitHubManifestHelperFlowLaunchesOrganizationDraftDirectly(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	conversionAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app-manifests/manifest-code/conversions":
			writeJSONBody(t, w, map[string]any{
				"id":             999,
				"name":           "OhoCI-localhost-999",
				"slug":           "ohoci-localhost-999",
				"html_url":       "https://github.com/apps/ohoci-localhost-999",
				"pem":            testPrivateKey,
				"webhook_secret": "manifest-secret",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations":
			writeJSONBody(t, w, []map[string]any{
				{
					"id":                   321654,
					"repository_selection": "selected",
					"html_url":             "https://github.com/organizations/example/settings/installations/321654",
					"app_slug":             "ohoci-localhost-999",
					"account": map[string]any{
						"login": "example",
						"type":  "Organization",
					},
				},
			})
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer conversionAPI.Close()

	restoreGitHubAPI := rewriteGitHubAPITransport(t, conversionAPI.URL)
	defer restoreGitHubAPI()

	startResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/manifest/start", map[string]any{
		"ownerTarget":      "organization",
		"organizationSlug": "example-org",
	}, cfg.SessionCookieName, token)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("expected manifest start success, got %d: %s", startResponse.Code, startResponse.Body.String())
	}
	var startPayload struct {
		RedirectURL string `json:"redirectUrl"`
	}
	if err := json.Unmarshal(startResponse.Body.Bytes(), &startPayload); err != nil {
		t.Fatalf("decode start payload: %v", err)
	}
	redirectURL, err := url.Parse(startPayload.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	state := redirectURL.Query().Get("state")
	if strings.TrimSpace(state) == "" {
		t.Fatalf("expected signed state in redirect url, got %q", startPayload.RedirectURL)
	}

	launchResponse := performRequest(t, handler.handler, http.MethodGet, redirectURL.RequestURI(), nil, cfg.SessionCookieName, token)
	if launchResponse.Code != http.StatusOK {
		t.Fatalf("expected manifest launch page, got %d: %s", launchResponse.Code, launchResponse.Body.String())
	}
	if !strings.Contains(launchResponse.Body.String(), "https://github.com/organizations/example-org/settings/apps/new?state=") {
		t.Fatalf("expected organization github.com launch path, got %s", launchResponse.Body.String())
	}

	callbackResponse := performRequest(
		t,
		handler.handler,
		http.MethodGet,
		"/api/v1/github/config/manifest/callback?code=manifest-code&state="+url.QueryEscape(state),
		nil,
		cfg.SessionCookieName,
		token,
	)
	if callbackResponse.Code != http.StatusFound {
		t.Fatalf("expected callback redirect, got %d: %s", callbackResponse.Code, callbackResponse.Body.String())
	}
	callbackLocation := callbackResponse.Header().Get("Location")
	callbackURL, err := url.Parse(callbackLocation)
	if err != nil {
		t.Fatalf("parse callback redirect: %v", err)
	}
	if callbackURL.Scheme != "https" || callbackURL.Host != "github.com" || callbackURL.Path != "/apps/ohoci-localhost-999/installations/new" {
		t.Fatalf("unexpected callback redirect: %q", callbackLocation)
	}
	installState := callbackURL.Query().Get("state")
	if strings.TrimSpace(installState) == "" {
		t.Fatalf("expected install redirect to preserve state, got %q", callbackLocation)
	}

	completeResponse := performRequest(
		t,
		handler.handler,
		http.MethodGet,
		"/api/v1/github/config/manifest/callback?source=install&installation_id=321654&state="+url.QueryEscape(installState),
		nil,
		cfg.SessionCookieName,
		token,
	)
	if completeResponse.Code != http.StatusFound {
		t.Fatalf("expected install completion redirect, got %d: %s", completeResponse.Code, completeResponse.Body.String())
	}
	completeLocation := completeResponse.Header().Get("Location")
	completeURL, err := url.Parse(completeLocation)
	if err != nil {
		t.Fatalf("parse install completion redirect: %v", err)
	}
	if completeURL.Scheme != "http" || completeURL.Host != "localhost:8080" || completeURL.Path != "/" {
		t.Fatalf("unexpected install completion redirect: %q", completeLocation)
	}
	if completeURL.Query().Get("github_manifest") != "installed" {
		t.Fatalf("expected install completion success marker, got %q", completeLocation)
	}
	if completeURL.Query().Get("github_installation_id") != "321654" {
		t.Fatalf("expected installation id on return redirect, got %q", completeLocation)
	}

	pendingResponse := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/github/config/manifest/pending", nil, cfg.SessionCookieName, token)
	if pendingResponse.Code != http.StatusOK {
		t.Fatalf("expected pending manifest success, got %d: %s", pendingResponse.Code, pendingResponse.Body.String())
	}
	var pendingPayload struct {
		Pending *githubapp.PendingManifest `json:"pending"`
	}
	if err := json.Unmarshal(pendingResponse.Body.Bytes(), &pendingPayload); err != nil {
		t.Fatalf("decode pending payload: %v", err)
	}
	if pendingPayload.Pending == nil {
		t.Fatalf("expected pending manifest payload, got %#v", pendingPayload.Pending)
	}
	if pendingPayload.Pending.OwnerTarget != "organization" {
		t.Fatalf("expected org owner context in pending manifest, got %#v", pendingPayload.Pending)
	}
	if pendingPayload.Pending.TransferURL != "" {
		t.Fatalf("expected organization draft to skip transfer guidance, got %#v", pendingPayload.Pending)
	}
	if pendingPayload.Pending.InstallURL != callbackLocation {
		t.Fatalf("expected install url in pending manifest, got %#v", pendingPayload.Pending)
	}
}

func TestGitHubManifestStartRejectsMissingOrMalformedOrganizationSlug(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	missingResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/manifest/start", map[string]any{
		"ownerTarget": "organization",
	}, cfg.SessionCookieName, token)
	if missingResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected missing organization slug to be rejected, got %d: %s", missingResponse.Code, missingResponse.Body.String())
	}
	if !strings.Contains(missingResponse.Body.String(), "organization slug is required") {
		t.Fatalf("expected missing slug error, got %s", missingResponse.Body.String())
	}

	malformedResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/manifest/start", map[string]any{
		"ownerTarget":      "organization",
		"organizationSlug": "example_org",
	}, cfg.SessionCookieName, token)
	if malformedResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected malformed organization slug to be rejected, got %d: %s", malformedResponse.Code, malformedResponse.Body.String())
	}
	if !strings.Contains(malformedResponse.Body.String(), "single hyphens") {
		t.Fatalf("expected malformed slug error, got %s", malformedResponse.Body.String())
	}
}

func TestGitHubManifestStartRejectsNonDefaultAPIBaseURL(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/manifest/start", map[string]any{
		"apiBaseUrl": "https://ghe.example.test/api/v3",
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected manifest helper rejection for GHES-like URL, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "only available for github.com") {
		t.Fatalf("expected clear GHES helper error, got %s", response.Body.String())
	}
}

func TestGitHubManifestInstallReturnWithoutStateRequiresPendingDraft(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	response := performRequest(
		t,
		handler.handler,
		http.MethodGet,
		"/api/v1/github/config/manifest/callback?source=install&installation_id=321654",
		nil,
		cfg.SessionCookieName,
		token,
	)
	if response.Code != http.StatusFound {
		t.Fatalf("expected install return redirect, got %d: %s", response.Code, response.Body.String())
	}

	location := response.Header().Get("Location")
	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if redirectURL.Scheme != "http" || redirectURL.Host != "localhost:8080" || redirectURL.Path != "/" {
		t.Fatalf("unexpected redirect: %q", location)
	}
	if redirectURL.Query().Get("github_manifest") != "failed" {
		t.Fatalf("expected failed marker without pending draft, got %q", location)
	}
}

func TestGitHubManifestInstallReturnFailsWhenGitHubDoesNotConfirmInstallation(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	conversionAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app-manifests/manifest-code/conversions":
			writeJSONBody(t, w, map[string]any{
				"id":             999,
				"name":           "OhoCI-localhost-999",
				"slug":           "ohoci-localhost-999",
				"html_url":       "https://github.com/apps/ohoci-localhost-999",
				"pem":            testPrivateKey,
				"webhook_secret": "manifest-secret",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations":
			writeJSONBody(t, w, []map[string]any{})
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer conversionAPI.Close()

	restoreGitHubAPI := rewriteGitHubAPITransport(t, conversionAPI.URL)
	defer restoreGitHubAPI()

	startResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/manifest/start", map[string]any{}, cfg.SessionCookieName, token)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("expected manifest start success, got %d: %s", startResponse.Code, startResponse.Body.String())
	}
	var startPayload struct {
		RedirectURL string `json:"redirectUrl"`
	}
	if err := json.Unmarshal(startResponse.Body.Bytes(), &startPayload); err != nil {
		t.Fatalf("decode start payload: %v", err)
	}
	redirectURL, err := url.Parse(startPayload.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	state := redirectURL.Query().Get("state")

	callbackResponse := performRequest(
		t,
		handler.handler,
		http.MethodGet,
		"/api/v1/github/config/manifest/callback?code=manifest-code&state="+url.QueryEscape(state),
		nil,
		cfg.SessionCookieName,
		token,
	)
	if callbackResponse.Code != http.StatusFound {
		t.Fatalf("expected callback redirect, got %d: %s", callbackResponse.Code, callbackResponse.Body.String())
	}
	callbackURL, err := url.Parse(callbackResponse.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse callback redirect: %v", err)
	}
	installState := callbackURL.Query().Get("state")

	completeResponse := performRequest(
		t,
		handler.handler,
		http.MethodGet,
		"/api/v1/github/config/manifest/callback?source=install&installation_id=321654&state="+url.QueryEscape(installState),
		nil,
		cfg.SessionCookieName,
		token,
	)
	if completeResponse.Code != http.StatusFound {
		t.Fatalf("expected install completion redirect, got %d: %s", completeResponse.Code, completeResponse.Body.String())
	}
	completeURL, err := url.Parse(completeResponse.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse install completion redirect: %v", err)
	}
	if completeURL.Query().Get("github_manifest") != "failed" {
		t.Fatalf("expected failed marker for unverified installation, got %q", completeURL.String())
	}
	if completeURL.Query().Get("github_installation_id") != "" {
		t.Fatalf("expected install id to be omitted on failure, got %q", completeURL.String())
	}
}

func TestGitHubInstallationDiscoveryEndpointHandlesSingleAndMultipleInstallations(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	singleInstallAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app":
			writeJSONBody(t, w, map[string]any{"id": 111})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations":
			writeJSONBody(t, w, []map[string]any{
				{
					"id":                   444,
					"repository_selection": "selected",
					"html_url":             "https://github.com/organizations/example/settings/installations/444",
					"app_slug":             "ohoci-example",
					"account": map[string]any{
						"login": "example",
						"type":  "Organization",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer singleInstallAPI.Close()

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/installations/discover", githubapp.Input{
		APIBaseURL:    singleInstallAPI.URL,
		AppID:         111,
		PrivateKeyPEM: testPrivateKey,
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected installation discovery success, got %d: %s", response.Code, response.Body.String())
	}
	var singlePayload githubapp.InstallationLookup
	if err := json.Unmarshal(response.Body.Bytes(), &singlePayload); err != nil {
		t.Fatalf("decode single installation payload: %v", err)
	}
	if singlePayload.AutoInstallationID != 444 || len(singlePayload.Installations) != 1 {
		t.Fatalf("expected single installation auto-fill, got %#v", singlePayload)
	}

	multiInstallAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app":
			writeJSONBody(t, w, map[string]any{"id": 111})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations":
			writeJSONBody(t, w, []map[string]any{
				{
					"id":                   444,
					"repository_selection": "selected",
					"html_url":             "https://github.com/organizations/example/settings/installations/444",
					"app_slug":             "ohoci-example",
					"account":              map[string]any{"login": "example", "type": "Organization"},
				},
				{
					"id":                   555,
					"repository_selection": "all",
					"html_url":             "https://github.com/users/demo/settings/installations/555",
					"app_slug":             "ohoci-example",
					"account":              map[string]any{"login": "demo", "type": "User"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer multiInstallAPI.Close()

	response = performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/github/config/installations/discover", githubapp.Input{
		APIBaseURL:    multiInstallAPI.URL,
		AppID:         111,
		PrivateKeyPEM: testPrivateKey,
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected multi-installation discovery success, got %d: %s", response.Code, response.Body.String())
	}
	var multiPayload githubapp.InstallationLookup
	if err := json.Unmarshal(response.Body.Bytes(), &multiPayload); err != nil {
		t.Fatalf("decode multi installation payload: %v", err)
	}
	if multiPayload.AutoInstallationID != 0 || len(multiPayload.Installations) != 2 {
		t.Fatalf("expected installation choices without auto-fill, got %#v", multiPayload)
	}
}

type backendTestOptions struct {
	githubDefaults      githubapp.Config
	ociController       oci.Controller
	billingTagNamespace string
}

type backendTestHandler struct {
	handler http.Handler
	auth    *auth.Service
}

func newBackendTestHandler(t *testing.T, options backendTestOptions) (backendTestHandler, config.Config, *store.Store, *oci.FakeController) {
	t.Helper()
	return newBackendTestHandlerWithSQLitePath(t, t.TempDir()+"/ohoci.db", options)
}

func newBackendTestHandlerWithSQLitePath(t *testing.T, sqlitePath string, options backendTestOptions) (backendTestHandler, config.Config, *store.Store, *oci.FakeController) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, "", sqlitePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := config.Config{
		SessionCookieName:      "ohoci_session",
		SessionSecret:          "secret",
		DataEncryptionKey:      "top-secret",
		PublicBaseURL:          "http://localhost:8080",
		OCIAuthMode:            "fake",
		OCIBillingTagNamespace: options.billingTagNamespace,
		RunnerDownloadBaseURL:  "https://github.com/actions/runner/releases/download",
		RunnerVersion:          "2.325.0",
		RunnerUser:             "runner",
		RunnerWorkDirectory:    "/home/runner/actions-runner",
	}

	sessions := session.New(db, cfg.SessionSecret, time.Hour)
	authService := auth.New(db, sessions)
	githubService, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults:      options.githubDefaults,
		EncryptionKey: cfg.DataEncryptionKey,
		PublicBaseURL: cfg.PublicBaseURL,
	})
	if err != nil {
		t.Fatalf("new github service: %v", err)
	}
	ociRuntimeService := ociruntime.New(db, ociruntime.Defaults{})
	ociCredentialService, err := ocicredentials.New(db, ocicredentials.Config{
		DefaultMode:           "fake",
		EncryptionKey:         cfg.DataEncryptionKey,
		RuntimeStatusProvider: ociRuntimeService,
	})
	if err != nil {
		t.Fatalf("new oci credential service: %v", err)
	}
	fakeController := &oci.FakeController{Instances: map[string]oci.Instance{}}
	controller := options.ociController
	if controller == nil {
		controller = fakeController
	}
	ociRuntimeService.SetCatalogController(controller)
	ociBillingService, err := ocibilling.New(db, ocibilling.Config{
		DefaultMode:         cfg.OCIAuthMode,
		BillingTagNamespace: cfg.OCIBillingTagNamespace,
		ProviderResolver:    ociCredentialService,
	})
	if err != nil {
		t.Fatalf("new oci billing service: %v", err)
	}
	cleanupService := cleanup.New(db, controller, githubService, sessions)
	runnerImageService := runnerimages.New(db, controller, ociRuntimeService)
	setupService := setupsvc.New(githubService, ociCredentialService, ociRuntimeService)
	handler := New(Dependencies{
		Config:         cfg,
		Store:          db,
		Auth:           authService,
		Sessions:       sessions,
		GitHub:         githubService,
		OCI:            controller,
		OCIBilling:     ociBillingService,
		OCICredentials: ociCredentialService,
		OCIRuntime:     ociRuntimeService,
		RunnerImages:   runnerImageService,
		Cleanup:        cleanupService,
		Setup:          setupService,
	})
	return backendTestHandler{handler: handler, auth: authService}, cfg, db, fakeController
}

func authenticatedToken(t *testing.T, authService *auth.Service) string {
	t.Helper()
	ctx := context.Background()
	token, _, err := authService.Login(ctx, "admin", "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := authService.ChangePassword(ctx, token, "admin", "super-secret-password"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	return token
}

func performJSONRequest(t *testing.T, handler http.Handler, method, path string, payload any, cookieName, token string) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func performRequest(t *testing.T, handler http.Handler, method, path string, body []byte, cookieName, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		request.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
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
