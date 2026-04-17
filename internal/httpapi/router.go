package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"ohoci/internal/admission"
	"ohoci/internal/auth"
	"ohoci/internal/cachecompat"
	"ohoci/internal/cleanup"
	"ohoci/internal/config"
	"ohoci/internal/githubapp"
	"ohoci/internal/oci"
	"ohoci/internal/ocibilling"
	"ohoci/internal/ocicredentials"
	"ohoci/internal/ociruntime"
	"ohoci/internal/runnerimages"
	"ohoci/internal/runnerlaunch"
	"ohoci/internal/session"
	"ohoci/internal/setup"
	"ohoci/internal/store"
	"ohoci/internal/warmpool"
)

type Dependencies struct {
	Config         config.Config
	Store          *store.Store
	Auth           *auth.Service
	Sessions       *session.Service
	GitHub         *githubapp.Service
	OCI            oci.Controller
	OCIBilling     *ocibilling.Service
	OCICredentials *ocicredentials.Service
	OCIRuntime     *ociruntime.Service
	RunnerImages   *runnerimages.Service
	Admission      *admission.Service
	RunnerLaunch   *runnerlaunch.Service
	WarmPool       *warmpool.Service
	CacheCompat    *cachecompat.Service
	Cleanup        *cleanup.Service
	Setup          *setup.Service
}

func New(deps Dependencies) http.Handler {
	rateLimiters := newAPIRateLimiters(deps.Store)
	mux := http.NewServeMux()
	mux.Handle("/api/v1/auth/login", jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		var payload struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := decodeJSON(r, &payload); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		token, sessionView, err := deps.Auth.Login(r.Context(), payload.Username, payload.Password, clientIP(r))
		if err != nil {
			return writeStatus(w, http.StatusUnauthorized, map[string]any{"error": err.Error(), "session": sessionView})
		}
		http.SetCookie(w, buildCookie(deps.Config, token, sessionView.ExpiresAt))
		return writeJSON(w, http.StatusOK, map[string]any{"session": sessionView})
	}))
	mux.Handle("/api/v1/auth/logout", jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		_ = deps.Auth.Logout(r.Context(), readSessionCookie(r, deps.Config.SessionCookieName))
		http.SetCookie(w, expiredCookie(deps.Config.SessionCookieName))
		return writeJSON(w, http.StatusOK, map[string]any{"success": true})
	}))
	mux.Handle("/api/v1/auth/session", jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		sessionView, err := deps.Auth.SessionFromToken(r.Context(), readSessionCookie(r, deps.Config.SessionCookieName))
		if err != nil {
			return writeStatus(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, map[string]any{"session": sessionView})
	}))
	mux.Handle("/api/v1/auth/change-password", jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		var payload struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if err := decodeJSON(r, &payload); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		if err := deps.Auth.ChangePassword(r.Context(), readSessionCookie(r, deps.Config.SessionCookieName), payload.CurrentPassword, payload.NewPassword); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, map[string]any{"success": true})
	}))
	mux.Handle("/api/internal/cache/_apis/artifactcache/cache", cacheCompatRestoreHandler(deps))
	mux.Handle("/api/internal/cache/_apis/artifactcache/caches", cacheCompatReserveHandler(deps))
	mux.Handle("/api/internal/cache/_apis/artifactcache/caches/", cacheCompatUploadHandler(deps))
	mux.Handle("/api/internal/cache/_apis/artifactcache/artifacts/", cacheCompatArtifactHandler(deps))
	setupStatusHandler := requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		if deps.Setup == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "setup service is not configured"})
		}
		status, err := deps.Setup.CurrentStatus(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, status)
	}))
	mux.Handle("/api/v1/setup", setupStatusHandler)
	mux.Handle("/api/v1/setup/status", setupStatusHandler)
	mux.Handle("/api/v1/policies/compatibility-check", policyCompatibilityCheckHandler(deps))
	mux.Handle("/api/v1/policies", requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		switch r.Method {
		case http.MethodGet:
			items, err := deps.Store.ListPolicies(r.Context())
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, map[string]any{"items": items})
		case http.MethodPost:
			var payload store.Policy
			if err := decodeJSON(r, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			if err := validatePolicyAgainstRuntimeCatalog(r.Context(), deps, payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			item, err := deps.Store.CreatePolicy(r.Context(), payload)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusCreated, item)
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}))))
	mux.Handle("/api/v1/policies/", requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		id, err := idFromPath(r.URL.Path)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		switch r.Method {
		case http.MethodPut:
			var payload store.Policy
			if err := decodeJSON(r, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			if err := validatePolicyAgainstRuntimeCatalog(r.Context(), deps, payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			item, err := deps.Store.UpdatePolicy(r.Context(), id, payload)
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, item)
		case http.MethodDelete:
			if err := deps.Store.DeletePolicy(r.Context(), id); err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, map[string]any{"success": true})
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}))))
	mux.Handle("/api/v1/jobs", requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		items, err := listJobItems(r.Context(), deps)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}))))
	mux.Handle("/api/v1/jobs/", jobDiagnosticsHandler(deps))
	mux.Handle("/api/v1/runners", requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		items, err := deps.Store.ListRunners(r.Context(), 100)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"items": items})
	}))))
	mux.Handle("/api/v1/events", requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		events, err := deps.Store.ListEvents(r.Context(), 100)
		if err != nil {
			return err
		}
		logs, err := deps.Store.ListEventLogs(r.Context(), 200)
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"events": events, "logs": logs})
	}))))
	mux.Handle("/api/v1/runner-images", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.RunnerImages == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "runner image service is not configured"})
		}
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		snapshot, err := deps.RunnerImages.Snapshot(r.Context())
		if err != nil {
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, snapshot)
	})))
	mux.Handle("/api/v1/runner-images/recipes", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.RunnerImages == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "runner image service is not configured"})
		}
		switch r.Method {
		case http.MethodGet:
			snapshot, err := deps.RunnerImages.Snapshot(r.Context())
			if err != nil {
				return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusOK, map[string]any{"items": snapshot.Recipes})
		case http.MethodPost:
			var payload runnerimages.RecipeInput
			if err := decodeJSON(r, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			item, err := deps.RunnerImages.SaveRecipe(r.Context(), 0, payload)
			if err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusCreated, item)
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})))
	mux.Handle("/api/v1/runner-images/recipes/", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.RunnerImages == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "runner image service is not configured"})
		}
		id, err := idFromPath(r.URL.Path)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid recipe id"})
		}
		switch r.Method {
		case http.MethodPut:
			var payload runnerimages.RecipeInput
			if err := decodeJSON(r, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			item, err := deps.RunnerImages.SaveRecipe(r.Context(), id, payload)
			if err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusOK, item)
		case http.MethodDelete:
			if err := deps.RunnerImages.DeleteRecipe(r.Context(), id); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusOK, map[string]any{"success": true})
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})))
	mux.Handle("/api/v1/runner-images/builds", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.RunnerImages == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "runner image service is not configured"})
		}
		switch r.Method {
		case http.MethodGet:
			snapshot, err := deps.RunnerImages.Snapshot(r.Context())
			if err != nil {
				return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusOK, map[string]any{"items": snapshot.Builds})
		case http.MethodPost:
			var payload struct {
				RecipeID int64 `json:"recipeId"`
			}
			if err := decodeJSON(r, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			item, err := deps.RunnerImages.CreateBuild(r.Context(), payload.RecipeID)
			if err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusCreated, item)
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})))
	mux.Handle("/api/v1/runner-images/builds/", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.RunnerImages == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "runner image service is not configured"})
		}
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/promote") {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		base := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/runner-images/builds/"), "/promote")
		buildID, err := strconv.ParseInt(strings.Trim(base, "/"), 10, 64)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid build id"})
		}
		item, err := deps.RunnerImages.PromoteBuild(r.Context(), buildID)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, item)
	})))
	mux.Handle("/api/v1/runner-images/discovery", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.RunnerImages == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "runner image service is not configured"})
		}
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		snapshot, err := deps.RunnerImages.Snapshot(r.Context())
		if err != nil {
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, map[string]any{"items": snapshot.Resources, "preflight": snapshot.Preflight})
	})))
	mux.Handle("/api/v1/runner-images/reconcile", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.RunnerImages == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "runner image service is not configured"})
		}
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		result, err := deps.RunnerImages.RunOnce(r.Context())
		if err != nil {
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, result)
	})))
	mux.Handle("/api/v1/billing/policies", billingPoliciesHandler(deps))
	mux.Handle("/api/v1/billing/guardrails", billingGuardrailsHandler(deps))
	mux.Handle("/api/v1/runners/", requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/terminate") {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		base := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/runners/"), "/terminate")
		runnerID, err := strconv.ParseInt(strings.Trim(base, "/"), 10, 64)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid runner id"})
		}
		item, err := deps.Store.FindRunnerByID(r.Context(), runnerID)
		if err != nil {
			return writeStatus(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
		}
		if err := deps.OCI.TerminateInstance(r.Context(), item.InstanceOCID); err != nil {
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		_ = deps.Store.UpdateRunnerStatus(r.Context(), item.ID, "terminating", item.GitHubRunnerID, nil)
		return writeJSON(w, http.StatusOK, map[string]any{"success": true})
	}))))
	mux.Handle("/api/v1/system/cleanup", requireUser(deps, true, requireSetupReady(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		result, err := deps.Cleanup.RunOnce(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, result)
	}))))
	mux.Handle("/api/v1/oci/catalog", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		var payload oci.CatalogRequest
		if err := decodeJSON(r, &payload); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		if strings.TrimSpace(payload.CompartmentOCID) == "" {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "compartmentOcid is required"})
		}
		catalog, err := deps.OCI.ListCatalog(r.Context(), payload)
		if err != nil {
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, catalog)
	})))
	mux.Handle("/api/v1/oci/subnets", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		items, err := deps.OCI.ListSubnetCandidates(r.Context())
		if err != nil {
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		defaultSubnetID := deps.Config.OCISubnetID
		if deps.OCIRuntime != nil {
			status, statusErr := deps.OCIRuntime.CurrentStatus(r.Context())
			if statusErr == nil {
				defaultSubnetID = status.EffectiveSettings.SubnetOCID
			}
		}
		return writeJSON(w, http.StatusOK, map[string]any{"items": items, "defaultSubnetId": defaultSubnetID})
	})))
	mux.Handle("/api/v1/oci/runtime", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.OCIRuntime == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "OCI runtime service is not configured"})
		}
		switch r.Method {
		case http.MethodGet:
			status, err := deps.OCIRuntime.CurrentStatus(r.Context())
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, status)
		case http.MethodPut:
			var payload ociruntime.Input
			if err := decodeJSON(r, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			status, err := deps.OCIRuntime.Save(r.Context(), payload)
			if err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusOK, status)
		case http.MethodDelete:
			if err := deps.OCIRuntime.Clear(r.Context()); err != nil {
				return err
			}
			status, err := deps.OCIRuntime.CurrentStatus(r.Context())
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": status})
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})))
	mux.Handle("/api/v1/oci/auth", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.OCICredentials == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "OCI credential service is not configured"})
		}
		switch r.Method {
		case http.MethodGet:
			status, err := deps.OCICredentials.CurrentStatus(r.Context())
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, status)
		case http.MethodPost:
			var payload ocicredentials.Input
			if err := decodeJSON(r, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			result, err := deps.OCICredentials.Save(r.Context(), payload)
			if err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusOK, result)
		case http.MethodDelete:
			if err := deps.OCICredentials.Clear(r.Context()); err != nil {
				return err
			}
			status, err := deps.OCICredentials.CurrentStatus(r.Context())
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": status})
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})))
	mux.Handle("/api/v1/oci/auth/test", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.OCICredentials == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "OCI credential service is not configured"})
		}
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		var payload ocicredentials.Input
		if err := decodeJSON(r, &payload); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		result, err := deps.OCICredentials.Test(r.Context(), payload)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, result)
	})))
	mux.Handle("/api/v1/oci/auth/inspect", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.OCICredentials == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "OCI credential service is not configured"})
		}
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		var payload ocicredentials.InspectInput
		if err := decodeJSON(r, &payload); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		result, err := deps.OCICredentials.Inspect(r.Context(), payload)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, result)
	})))
	mux.Handle("/api/v1/github/config/manifest/start", githubManifestStartHandler(deps))
	mux.Handle("/api/v1/github/config/manifest/launch", githubManifestLaunchHandler(deps))
	mux.Handle("/api/v1/github/config/manifest/callback", githubManifestCallbackHandler(deps))
	mux.Handle("/api/v1/github/config/manifest/pending", githubManifestPendingHandler(deps))
	mux.Handle("/api/v1/github/config/installations/discover", githubInstallationDiscoveryHandler(deps))
	mux.Handle("/api/v1/github/drift/reconcile", githubDriftReconcileHandler(deps))
	mux.Handle("/api/v1/github/drift", githubDriftHandler(deps))
	mux.Handle("/api/v1/github/config", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.GitHub == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "GitHub config service is not configured"})
		}
		switch r.Method {
		case http.MethodGet:
			status, err := deps.GitHub.CurrentStatus(r.Context())
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, status)
		case http.MethodPost:
			var payload githubapp.Input
			if err := decodeJSON(r, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			result, err := deps.GitHub.Save(r.Context(), payload)
			if err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusOK, result)
		case http.MethodDelete:
			if err := deps.GitHub.Clear(r.Context()); err != nil {
				return err
			}
			status, err := deps.GitHub.CurrentStatus(r.Context())
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": status})
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})))
	mux.Handle("/api/v1/github/config/staged", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.GitHub == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "GitHub config service is not configured"})
		}
		switch r.Method {
		case http.MethodPost:
			var payload githubapp.Input
			if err := decodeJSON(r, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			result, err := deps.GitHub.SaveStagedApp(r.Context(), payload)
			if err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusOK, result)
		case http.MethodDelete:
			if err := deps.GitHub.ClearStaged(r.Context()); err != nil {
				return err
			}
			status, err := deps.GitHub.CurrentStatus(r.Context())
			if err != nil {
				return err
			}
			return writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": status})
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})))
	mux.Handle("/api/v1/github/config/staged/promote", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.GitHub == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "GitHub config service is not configured"})
		}
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		if err := deps.GitHub.PromoteStagedApp(r.Context()); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		status, err := deps.GitHub.CurrentStatus(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": status})
	})))
	mux.Handle("/api/v1/github/config/test", requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.GitHub == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "GitHub config service is not configured"})
		}
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		var payload githubapp.Input
		if err := decodeJSON(r, &payload); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		result, err := deps.GitHub.Test(r.Context(), payload)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, result)
	})))
	mux.Handle("/api/v1/github/webhook", jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		}
		eventType := strings.TrimSpace(r.Header.Get("X-GitHub-Event"))
		resolution, err := deps.GitHub.ResolveWebhookSource(r.Context(), eventType, body, r.Header.Get("X-Hub-Signature-256"))
		if err != nil {
			if errors.Is(err, githubapp.ErrNotConfigured) {
				return writeStatus(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			}
			return err
		}
		if resolution.Source == githubapp.WebhookSourceUnknown {
			return writeStatus(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		}
		deliveryID := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery"))
		switch eventType {
		case "workflow_job":
			if resolution.Source == githubapp.WebhookSourceStaged {
				return writeJSON(w, http.StatusAccepted, map[string]any{"ignored": true, "eventType": eventType, "source": string(resolution.Source)})
			}
			var payload workflowJobEvent
			if err := json.Unmarshal(body, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid json payload"})
			}
			event := store.Event{
				DeliveryID:  deliveryID,
				EventType:   eventType,
				Action:      payload.Action,
				RepoOwner:   payload.Repository.Owner.Login,
				RepoName:    payload.Repository.Name,
				PayloadJSON: string(body),
			}
			created, err := deps.Store.CreateEventDelivery(r.Context(), event)
			if err != nil {
				return err
			}
			if !created {
				return writeJSON(w, http.StatusAccepted, map[string]any{"duplicate": true})
			}
			if err := processWorkflowJob(r.Context(), deps, resolution, deliveryID, payload); err != nil {
				_ = deps.Store.AddEventLog(r.Context(), store.EventLog{
					DeliveryID: deliveryID,
					Level:      "error",
					Message:    err.Error(),
				})
				return writeStatus(w, http.StatusAccepted, map[string]any{"processed": false, "error": err.Error()})
			}
			_ = deps.Store.MarkEventProcessed(r.Context(), deliveryID)
			return writeJSON(w, http.StatusAccepted, map[string]any{"processed": true})
		case "installation", "installation_repositories":
			if resolution.Client == nil || resolution.Client.AuthMode() != store.GitHubAuthModeApp {
				return writeJSON(w, http.StatusAccepted, map[string]any{"ignored": true, "eventType": eventType, "source": string(resolution.Source)})
			}
			var payload installationWebhookEvent
			if err := json.Unmarshal(body, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid json payload"})
			}
			event := store.Event{
				DeliveryID:  deliveryID,
				EventType:   eventType,
				Action:      payload.Action,
				RepoOwner:   payload.Installation.Account.Login,
				RepoName:    eventType,
				PayloadJSON: string(body),
			}
			created, err := deps.Store.CreateEventDelivery(r.Context(), event)
			if err != nil {
				return err
			}
			if !created {
				return writeJSON(w, http.StatusAccepted, map[string]any{"duplicate": true})
			}
			if err := processInstallationWebhook(r.Context(), deps, resolution, eventType, payload); err != nil {
				_ = deps.Store.AddEventLog(r.Context(), store.EventLog{
					DeliveryID: deliveryID,
					Level:      "error",
					Message:    err.Error(),
				})
				return writeStatus(w, http.StatusAccepted, map[string]any{"processed": false, "error": err.Error()})
			}
			_ = deps.Store.MarkEventProcessed(r.Context(), deliveryID)
			return writeJSON(w, http.StatusAccepted, map[string]any{"processed": true})
		default:
			return writeJSON(w, http.StatusAccepted, map[string]any{"ignored": true, "eventType": eventType, "source": string(resolution.Source)})
		}
	}))
	mux.Handle("/", spaHandler(deps.Config.UIDir))
	return withRecovery(withSecurityHeaders(deps.Config, withIngressControls(deps.Config, withRateLimits(rateLimiters, mux))))
}
