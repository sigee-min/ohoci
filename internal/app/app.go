package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"ohoci/internal/admission"
	"ohoci/internal/auth"
	"ohoci/internal/cachecompat"
	"ohoci/internal/cleanup"
	"ohoci/internal/config"
	"ohoci/internal/githubapp"
	"ohoci/internal/httpapi"
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

type Instance struct {
	Config  config.Config
	Handler http.Handler
	store   *store.Store
}

func New() (*Instance, error) {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DatabaseURL, cfg.SQLitePath)
	if err != nil {
		return nil, err
	}
	sessions := session.New(db, cfg.SessionSecret, cfg.SessionTTL)
	authService := auth.NewWithPolicy(db, sessions, auth.Policy{
		LockoutAttempts: cfg.AuthLockoutAttempts,
		LockoutDuration: cfg.AuthLockoutDuration,
	})
	githubService, err := githubapp.NewService(db, githubapp.ServiceOptions{
		Defaults: githubapp.Config{
			Name:           cfg.GitHubAppName,
			Tags:           cfg.GitHubAppTags,
			APIBaseURL:     cfg.GitHubAPIBaseURL,
			AppID:          cfg.GitHubAppID,
			InstallationID: cfg.GitHubInstallationID,
			PrivateKeyPEM:  cfg.GitHubAppPrivateKey,
			WebhookSecret:  cfg.GitHubWebhookSecret,
			SelectedRepos:  cfg.GitHubAllowedRepos,
		},
		EncryptionKey: cfg.DataEncryptionKey,
		PublicBaseURL: cfg.PublicBaseURL,
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	ociRuntimeService := ociruntime.New(db, ociruntime.Defaults{
		CompartmentID:      cfg.OCICompartmentID,
		AvailabilityDomain: cfg.OCIAvailabilityDomain,
		SubnetID:           cfg.OCISubnetID,
		NSGIDs:             cfg.OCINSGIDs,
		ImageID:            cfg.OCIImageID,
		AssignPublicIP:     cfg.OCIAssignPublicIP,
		CacheCompatEnabled: cfg.CacheCompatEnabled,
		CacheBucketName:    cfg.CacheBucketName,
		CacheObjectPrefix:  cfg.CacheObjectPrefix,
		CacheRetentionDays: cfg.CacheRetentionDays,
	})
	ociCredentialService, err := ocicredentials.New(db, ocicredentials.Config{
		DefaultMode:   cfg.OCIAuthMode,
		EncryptionKey: cfg.DataEncryptionKey,
		Runtime: ocicredentials.RuntimeConfig{
			CompartmentID:      cfg.OCICompartmentID,
			AvailabilityDomain: cfg.OCIAvailabilityDomain,
			SubnetID:           cfg.OCISubnetID,
			ImageID:            cfg.OCIImageID,
		},
		RuntimeStatusProvider: ociRuntimeService,
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	ociClient, err := oci.New(ctx, oci.Config{
		AuthMode: cfg.OCIAuthMode,
		Runtime: oci.RuntimeConfig{
			CompartmentID:      cfg.OCICompartmentID,
			AvailabilityDomain: cfg.OCIAvailabilityDomain,
			SubnetID:           cfg.OCISubnetID,
			NSGIDs:             cfg.OCINSGIDs,
			ImageID:            cfg.OCIImageID,
			AssignPublicIP:     cfg.OCIAssignPublicIP,
		},
		BillingTagNamespace: cfg.OCIBillingTagNamespace,
		RunnerDownloadBase:  cfg.RunnerDownloadBaseURL,
		RunnerVersion:       cfg.RunnerVersion,
		RunnerUser:          cfg.RunnerUser,
		RunnerWorkDir:       cfg.RunnerWorkDirectory,
	}, ociCredentialService, ociRuntimeService)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	ociRuntimeService.SetCatalogController(ociClient)
	ociBillingService, err := ocibilling.New(db, ocibilling.Config{
		DefaultMode:         cfg.OCIAuthMode,
		BillingTagNamespace: cfg.OCIBillingTagNamespace,
		ProviderResolver:    ociCredentialService,
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	cleanupService := cleanup.New(db, ociClient, githubService, sessions)
	runnerImageService := runnerimages.New(db, ociClient, ociRuntimeService)
	setupService := setup.New(githubService, ociCredentialService, ociRuntimeService)
	runnerLaunchService := runnerlaunch.New(cfg, db, ociClient, ociRuntimeService)
	admissionService := admission.New(db, githubService, setupService, ociBillingService)
	warmPoolService := warmpool.New(db, githubService, runnerLaunchService)
	cacheCompatService := cachecompat.New(db, ociRuntimeService, cachecompat.NewOCIBlobStore(ociCredentialService), cfg.PublicBaseURL, cfg.DataEncryptionKey)
	handler := httpapi.New(httpapi.Dependencies{
		Config:         cfg,
		Store:          db,
		Auth:           authService,
		Sessions:       sessions,
		GitHub:         githubService,
		OCI:            ociClient,
		OCIBilling:     ociBillingService,
		OCICredentials: ociCredentialService,
		OCIRuntime:     ociRuntimeService,
		RunnerImages:   runnerImageService,
		Admission:      admissionService,
		RunnerLaunch:   runnerLaunchService,
		WarmPool:       warmPoolService,
		CacheCompat:    cacheCompatService,
		Cleanup:        cleanupService,
		Setup:          setupService,
	})
	go startCleanupLoop(cleanupService, cfg.CleanupInterval)
	go startRunnerImageLoop(runnerImageService, 5*time.Second)
	go startWarmPoolLoop(db, warmPoolService, 15*time.Second)
	go startBillingSnapshotLoop(db, ociBillingService, 15*time.Minute)
	go startGitHubDriftLoop(db, githubService, 15*time.Minute)
	return &Instance{
		Config:  cfg,
		Handler: handler,
		store:   db,
	}, nil
}

func (i *Instance) Close() error {
	return i.store.Close()
}

func startCleanupLoop(service *cleanup.Service, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		_, _ = service.RunOnce(context.Background())
	}
}

func startRunnerImageLoop(service *runnerimages.Service, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		_, _ = service.RunOnce(context.Background())
	}
}

func startWarmPoolLoop(storeDB *store.Store, service *warmpool.Service, interval time.Duration) {
	if service == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := service.RunOnce(context.Background()); err != nil && storeDB != nil {
			_ = storeDB.AddEventLog(context.Background(), store.EventLog{
				Level:   "warn",
				Message: "warm pool reconcile failed: " + err.Error(),
			})
		}
		<-ticker.C
	}
}

func startBillingSnapshotLoop(storeDB *store.Store, service *ocibilling.Service, interval time.Duration) {
	if service == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := service.RefreshPolicySnapshots(context.Background(), ocibilling.DefaultBudgetWindowDays); err != nil && storeDB != nil {
			_ = storeDB.AddEventLog(context.Background(), store.EventLog{
				Level:   "warn",
				Message: "billing snapshot refresh failed: " + err.Error(),
			})
		}
		<-ticker.C
	}
}

func startGitHubDriftLoop(storeDB *store.Store, service *githubapp.Service, interval time.Duration) {
	if service == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastFingerprint := ""
	for {
		status, err := service.ReconcileDrift(context.Background())
		if err != nil {
			if storeDB != nil {
				_ = storeDB.AddEventLog(context.Background(), store.EventLog{
					Level:   "warn",
					Message: "github drift reconcile failed: " + err.Error(),
				})
			}
			<-ticker.C
			continue
		}
		fingerprint := driftFingerprint(status)
		if storeDB != nil && fingerprint != "" && lastFingerprint != fingerprint && (lastFingerprint != "" || status.Severity != "ok") {
			detailsJSON, _ := json.Marshal(status)
			level := "info"
			if status.Severity == "critical" {
				level = "warn"
			}
			_ = storeDB.AddEventLog(context.Background(), store.EventLog{
				Level:       level,
				Message:     "github drift state changed",
				DetailsJSON: string(detailsJSON),
			})
		}
		lastFingerprint = fingerprint
		<-ticker.C
	}
}

func driftFingerprint(status githubapp.DriftStatus) string {
	payload := struct {
		Severity string                 `json:"severity"`
		Issues   []githubapp.DriftIssue `json:"issues"`
	}{
		Severity: status.Severity,
		Issues:   append([]githubapp.DriftIssue(nil), status.Issues...),
	}
	out, _ := json.Marshal(payload)
	return string(out)
}
