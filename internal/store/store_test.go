package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

func TestResolveDatabaseUsesSQLitePathWhenDatabaseURLEmpty(t *testing.T) {
	driver, dsn, err := resolveDatabase("", "./state/ohoci.db")
	if err != nil {
		t.Fatalf("resolve database: %v", err)
	}
	if driver != "sqlite" {
		t.Fatalf("expected sqlite driver, got %q", driver)
	}
	if dsn != filepath.Clean("./state/ohoci.db") {
		t.Fatalf("expected cleaned sqlite path, got %q", dsn)
	}
}

func TestResolveDatabaseUsesMySQLWhenDatabaseURLProvided(t *testing.T) {
	driver, dsn, err := resolveDatabase("mysql://user:secret@db.example:3306/ohoci?tls=skip-verify", "./state/ohoci.db")
	if err != nil {
		t.Fatalf("resolve database: %v", err)
	}
	if driver != "mysql" {
		t.Fatalf("expected mysql driver, got %q", driver)
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	if cfg.User != "user" || cfg.Passwd != "secret" {
		t.Fatalf("unexpected mysql credentials: %#v", cfg)
	}
	if cfg.Net != "tcp" || cfg.Addr != "db.example:3306" || cfg.DBName != "ohoci" {
		t.Fatalf("unexpected mysql target: %#v", cfg)
	}
	if !strings.Contains(dsn, "tls=skip-verify") || !strings.Contains(dsn, "parseTime=true") {
		t.Fatalf("unexpected mysql dsn: %q", dsn)
	}
}

func TestResolveDatabaseRejectsNonMySQLSchemes(t *testing.T) {
	testCases := []struct {
		name        string
		databaseURL string
	}{
		{name: "sqlite", databaseURL: "sqlite:///tmp/ohoci.db"},
		{name: "postgres", databaseURL: "postgres://db.example/ohoci"},
		{name: "missing scheme", databaseURL: "/tmp/ohoci.db"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			driver, dsn, err := resolveDatabase(tc.databaseURL, "./state/ohoci.db")
			if err == nil {
				t.Fatalf("expected error, got driver=%q dsn=%q", driver, dsn)
			}
		})
	}
}

func TestOpenCreatesSQLiteParentDirectories(t *testing.T) {
	ctx := context.Background()
	sqlitePath := filepath.Join(t.TempDir(), "state", "sqlite", "ohoci.db")

	if _, err := os.Stat(filepath.Dir(sqlitePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected parent directory to not exist before open, got %v", err)
	}

	db, err := Open(ctx, "", sqlitePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := os.Stat(filepath.Dir(sqlitePath)); err != nil {
		t.Fatalf("stat parent directory: %v", err)
	}
	if _, err := os.Stat(sqlitePath); err != nil {
		t.Fatalf("stat sqlite file: %v", err)
	}
}

func TestPolicyPersistsSubnetOCID(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	created, err := db.CreatePolicy(ctx, Policy{
		Labels:     []string{"oci", "cpu"},
		SubnetOCID: "ocid1.subnet.oc1..policy",
		Shape:      "VM.Standard.A1.Flex",
		OCPU:       2,
		MemoryGB:   8,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if created.SubnetOCID != "ocid1.subnet.oc1..policy" {
		t.Fatalf("unexpected created subnet OCID: %q", created.SubnetOCID)
	}

	created.SubnetOCID = "ocid1.subnet.oc1..updated"
	updated, err := db.UpdatePolicy(ctx, created.ID, created)
	if err != nil {
		t.Fatalf("update policy: %v", err)
	}
	if updated.SubnetOCID != "ocid1.subnet.oc1..updated" {
		t.Fatalf("unexpected updated subnet OCID: %q", updated.SubnetOCID)
	}

	items, err := db.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("list policies: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(items))
	}
	if items[0].SubnetOCID != "ocid1.subnet.oc1..updated" {
		t.Fatalf("unexpected listed subnet OCID: %q", items[0].SubnetOCID)
	}
}

func TestFindLatestRunnerByJobIDAndPreserveRunnerRegistration(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	policyRecord, err := db.CreatePolicy(ctx, Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.A1.Flex",
		OCPU:       2,
		MemoryGB:   8,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	jobRecord, err := db.UpsertJob(ctx, Job{
		GitHubJobID:    101,
		DeliveryID:     "delivery-101",
		InstallationID: 1,
		RepoOwner:      "example",
		RepoName:       "repo",
		RunID:          101,
		RunAttempt:     1,
		Status:         "queued",
		Labels:         []string{"oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	first, err := db.CreateRunner(ctx, Runner{
		PolicyID:       policyRecord.ID,
		JobID:          jobRecord.ID,
		InstallationID: 1,
		InstanceOCID:   "ocid1.instance.oc1..first",
		GitHubRunnerID: 42,
		RepoOwner:      "example",
		RepoName:       "repo",
		RunnerName:     "runner-first",
		Status:         "launching",
		Labels:         []string{"self-hosted", "oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("create first runner: %v", err)
	}
	time.Sleep(time.Millisecond)
	second, err := db.CreateRunner(ctx, Runner{
		PolicyID:       policyRecord.ID,
		JobID:          jobRecord.ID,
		InstallationID: 1,
		InstanceOCID:   "ocid1.instance.oc1..second",
		RepoOwner:      "example",
		RepoName:       "repo",
		RunnerName:     "runner-second",
		Status:         "launching",
		Labels:         []string{"self-hosted", "oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("create second runner: %v", err)
	}

	latest, err := db.FindLatestRunnerByJobID(ctx, jobRecord.ID)
	if err != nil {
		t.Fatalf("find latest runner: %v", err)
	}
	if latest.ID != second.ID {
		t.Fatalf("expected latest runner %d, got %d", second.ID, latest.ID)
	}

	if err := db.UpdateRunnerStatus(ctx, first.ID, "in_progress", 0, nil); err != nil {
		t.Fatalf("update runner: %v", err)
	}
	updated, err := db.FindRunnerByID(ctx, first.ID)
	if err != nil {
		t.Fatalf("find runner by id: %v", err)
	}
	if updated.GitHubRunnerID != 42 {
		t.Fatalf("expected GitHub runner ID to be preserved, got %d", updated.GitHubRunnerID)
	}
	if updated.Status != "in_progress" {
		t.Fatalf("expected runner status to update, got %q", updated.Status)
	}
}

func TestOCIRuntimeSettingsPersistAndClear(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	saved, err := db.SaveOCIRuntimeSettings(ctx, OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..example",
		NSGOCIDs:           []string{"ocid1.nsg.oc1..b", "ocid1.nsg.oc1..a", "ocid1.nsg.oc1..a"},
		ImageOCID:          "ocid1.image.oc1..example",
		AssignPublicIP:     true,
	})
	if err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}
	if len(saved.NSGOCIDs) != 2 {
		t.Fatalf("expected normalized NSG IDs, got %#v", saved.NSGOCIDs)
	}

	loaded, err := db.FindOCIRuntimeSettings(ctx)
	if err != nil {
		t.Fatalf("find runtime settings: %v", err)
	}
	if loaded.SubnetOCID != "ocid1.subnet.oc1..example" || !loaded.AssignPublicIP {
		t.Fatalf("unexpected loaded runtime settings: %#v", loaded)
	}

	if err := db.ClearOCIRuntimeSettings(ctx); err != nil {
		t.Fatalf("clear runtime settings: %v", err)
	}
	if _, err := db.FindOCIRuntimeSettings(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after clear, got %v", err)
	}
}

func TestGitHubConfigPersistActiveAndStaged(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	saved, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		APIBaseURL:                      "https://api.github.com",
		AuthMode:                        GitHubAuthModeApp,
		AppID:                           100,
		InstallationID:                  200,
		PrivateKeyCiphertext:            "cipher-key",
		WebhookSecretCiphertext:         "cipher-secret",
		AllowedOrg:                      "example",
		SelectedRepos:                   []string{"example/repo-b", "example/repo-a", "example/repo-a"},
		AccountLogin:                    "example-org",
		AccountType:                     "Organization",
		InstallationState:               "active",
		InstallationRepositorySelection: "selected",
		InstallationRepositories:        []string{"example/repo-a", "example/repo-b"},
	})
	if err != nil {
		t.Fatalf("save github config: %v", err)
	}
	if len(saved.SelectedRepos) != 2 {
		t.Fatalf("expected normalized selected repos, got %#v", saved.SelectedRepos)
	}
	if saved.AuthMode != GitHubAuthModeApp || !saved.IsActive || saved.IsStaged {
		t.Fatalf("expected active app config, got %#v", saved)
	}

	staged, err := db.SaveStagedGitHubConfig(ctx, GitHubConfig{
		APIBaseURL:                      "https://api.github.com",
		AuthMode:                        GitHubAuthModeApp,
		AppID:                           101,
		InstallationID:                  202,
		PrivateKeyCiphertext:            "cipher-key",
		WebhookSecretCiphertext:         "cipher-staged-secret",
		AllowedOrg:                      "example",
		SelectedRepos:                   []string{"example/repo-c"},
		AccountLogin:                    "app-org",
		AccountType:                     "Organization",
		InstallationState:               "active",
		InstallationRepositorySelection: "selected",
		InstallationRepositories:        []string{"app-org/repo-c"},
	})
	if err != nil {
		t.Fatalf("save staged github config: %v", err)
	}
	if staged.AuthMode != GitHubAuthModeApp || staged.IsActive || !staged.IsStaged {
		t.Fatalf("expected staged app config, got %#v", staged)
	}

	loadedActive, err := db.FindActiveGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("find active github config: %v", err)
	}
	if loadedActive.ID != saved.ID || loadedActive.AuthMode != GitHubAuthModeApp || !loadedActive.IsActive {
		t.Fatalf("unexpected active github config: %#v", loadedActive)
	}
	if loadedActive.AccountLogin != "example-org" || loadedActive.AccountType != "Organization" || loadedActive.AllowedOrg != "example" {
		t.Fatalf("unexpected active github config details: %#v", loadedActive)
	}
	if loadedActive.AppID != 100 || loadedActive.InstallationID != 200 || loadedActive.InstallationState != "active" {
		t.Fatalf("unexpected active github installation details: %#v", loadedActive)
	}

	loadedStaged, err := db.FindStagedGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("find staged github config: %v", err)
	}
	if loadedStaged.ID != staged.ID || loadedStaged.AuthMode != GitHubAuthModeApp || !loadedStaged.IsStaged {
		t.Fatalf("unexpected staged github config: %#v", loadedStaged)
	}
	if loadedStaged.AppID != 101 || loadedStaged.InstallationID != 202 || loadedStaged.AllowedOrg != "example" {
		t.Fatalf("unexpected staged github config details: %#v", loadedStaged)
	}
	if loadedStaged.InstallationState != "active" || loadedStaged.InstallationRepositorySelection != "selected" || len(loadedStaged.InstallationRepositories) != 1 {
		t.Fatalf("unexpected staged installation metadata: %#v", loadedStaged)
	}

	if err := db.UpdateGitHubConfigInstallation(ctx, loadedStaged.ID, GitHubConfig{
		AccountLogin:                    "app-org",
		AccountType:                     "Organization",
		InstallationState:               "suspended",
		InstallationRepositorySelection: "selected",
		InstallationRepositories:        []string{"app-org/repo-c"},
		LastTestError:                   "installation suspended",
	}); err != nil {
		t.Fatalf("update github installation metadata: %v", err)
	}
	updatedStaged, err := db.FindStagedGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("find updated staged github config: %v", err)
	}
	if updatedStaged.InstallationState != "suspended" || updatedStaged.LastTestError != "installation suspended" {
		t.Fatalf("unexpected updated staged installation metadata: %#v", updatedStaged)
	}

	promoted, err := db.PromoteStagedGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("promote staged github config: %v", err)
	}
	if promoted.ID != staged.ID || promoted.AuthMode != GitHubAuthModeApp || !promoted.IsActive || promoted.IsStaged {
		t.Fatalf("unexpected promoted github config: %#v", promoted)
	}

	loadedActive, err = db.FindActiveGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("find promoted active github config: %v", err)
	}
	if loadedActive.ID != staged.ID || loadedActive.AuthMode != GitHubAuthModeApp || !loadedActive.IsActive || loadedActive.IsStaged {
		t.Fatalf("unexpected promoted active github config: %#v", loadedActive)
	}
	if loadedActive.AppID != 101 || loadedActive.InstallationID != 202 || loadedActive.AllowedOrg != "example" {
		t.Fatalf("unexpected promoted active github config details: %#v", loadedActive)
	}

	if _, err := db.FindStagedGitHubConfig(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected staged config to be cleared after promote, got %v", err)
	}

	if err := db.ClearStagedGitHubConfig(ctx); err != nil {
		t.Fatalf("clear staged github config: %v", err)
	}
	if _, err := db.FindStagedGitHubConfig(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after clearing staged config, got %v", err)
	}
	if _, err := db.FindActiveGitHubConfig(ctx); err != nil {
		t.Fatalf("expected active config to remain after clearing staged config: %v", err)
	}

	if err := db.ClearActiveGitHubConfig(ctx); err != nil {
		t.Fatalf("clear active github config: %v", err)
	}
	if _, err := db.FindActiveGitHubConfig(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after clear, got %v", err)
	}
}

func TestGitHubConfigSaveActiveSupersedesMatchingIdentityOnly(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	original, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		Name:                    "alpha-old",
		APIBaseURL:              " https://api.github.com/ ",
		AuthMode:                GitHubAuthModeApp,
		AppID:                   100,
		InstallationID:          200,
		PrivateKeyCiphertext:    "cipher-key-1",
		WebhookSecretCiphertext: "cipher-secret-1",
		SelectedRepos:           []string{"alpha/repo"},
	})
	if err != nil {
		t.Fatalf("save original active config: %v", err)
	}

	unrelated, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		Name:                    "beta-other-app",
		APIBaseURL:              "",
		AuthMode:                GitHubAuthModeApp,
		AppID:                   101,
		InstallationID:          200,
		PrivateKeyCiphertext:    "cipher-key-2",
		WebhookSecretCiphertext: "cipher-secret-2",
		SelectedRepos:           []string{"beta/repo"},
	})
	if err != nil {
		t.Fatalf("save unrelated active config: %v", err)
	}

	replacement, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		Name:                    "alpha-new",
		APIBaseURL:              "https://api.github.com",
		AppID:                   100,
		InstallationID:          200,
		PrivateKeyCiphertext:    "cipher-key-3",
		WebhookSecretCiphertext: "cipher-secret-3",
		SelectedRepos:           []string{"alpha/repo"},
	})
	if err != nil {
		t.Fatalf("save replacement active config: %v", err)
	}

	activeItems, err := db.ListActiveGitHubConfigs(ctx)
	if err != nil {
		t.Fatalf("list active configs after supersession: %v", err)
	}
	activeIDs := make([]int64, 0, len(activeItems))
	for _, item := range activeItems {
		activeIDs = append(activeIDs, item.ID)
	}
	if !reflect.DeepEqual(activeIDs, []int64{replacement.ID, unrelated.ID}) {
		t.Fatalf("expected replacement and unrelated configs to remain active, got %#v", activeItems)
	}

	loadedOriginal, err := db.FindGitHubConfigByID(ctx, original.ID)
	if err != nil {
		t.Fatalf("reload original config: %v", err)
	}
	if loadedOriginal.IsActive {
		t.Fatalf("expected original config to be retired, got %#v", loadedOriginal)
	}

	loadedUnrelated, err := db.FindGitHubConfigByID(ctx, unrelated.ID)
	if err != nil {
		t.Fatalf("reload unrelated config: %v", err)
	}
	if !loadedUnrelated.IsActive {
		t.Fatalf("expected unrelated config to remain active, got %#v", loadedUnrelated)
	}

	loadedReplacement, err := db.FindGitHubConfigByID(ctx, replacement.ID)
	if err != nil {
		t.Fatalf("reload replacement config: %v", err)
	}
	if !loadedReplacement.IsActive {
		t.Fatalf("expected replacement config to be active, got %#v", loadedReplacement)
	}
}

func TestGitHubConfigPromoteStagedSupersedesMatchingIdentityOnly(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	original, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		Name:                    "alpha-old",
		APIBaseURL:              "",
		AuthMode:                GitHubAuthModeApp,
		AppID:                   500,
		InstallationID:          600,
		PrivateKeyCiphertext:    "cipher-key-1",
		WebhookSecretCiphertext: "cipher-secret-1",
		SelectedRepos:           []string{"alpha/repo"},
	})
	if err != nil {
		t.Fatalf("save original active config: %v", err)
	}

	unrelated, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		Name:                    "alpha-other-base",
		APIBaseURL:              "https://ghe.example/api/v3/",
		AuthMode:                GitHubAuthModeApp,
		AppID:                   500,
		InstallationID:          600,
		PrivateKeyCiphertext:    "cipher-key-2",
		WebhookSecretCiphertext: "cipher-secret-2",
		SelectedRepos:           []string{"alpha/repo"},
	})
	if err != nil {
		t.Fatalf("save unrelated active config: %v", err)
	}

	staged, err := db.SaveStagedGitHubConfig(ctx, GitHubConfig{
		Name:                    "alpha-rotated",
		APIBaseURL:              "https://api.github.com/",
		AuthMode:                GitHubAuthModeApp,
		AppID:                   500,
		InstallationID:          600,
		PrivateKeyCiphertext:    "cipher-key-3",
		WebhookSecretCiphertext: "cipher-secret-3",
		SelectedRepos:           []string{"alpha/repo"},
	})
	if err != nil {
		t.Fatalf("save staged config: %v", err)
	}

	promoted, err := db.PromoteStagedGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("promote staged config: %v", err)
	}
	if promoted.ID != staged.ID || !promoted.IsActive || promoted.IsStaged {
		t.Fatalf("expected staged config to become the active replacement, got %#v", promoted)
	}

	activeItems, err := db.ListActiveGitHubConfigs(ctx)
	if err != nil {
		t.Fatalf("list active configs after promote: %v", err)
	}
	activeIDs := make([]int64, 0, len(activeItems))
	for _, item := range activeItems {
		activeIDs = append(activeIDs, item.ID)
	}
	if !reflect.DeepEqual(activeIDs, []int64{promoted.ID, unrelated.ID}) {
		t.Fatalf("expected promoted and unrelated configs to remain active, got %#v", activeItems)
	}

	loadedOriginal, err := db.FindGitHubConfigByID(ctx, original.ID)
	if err != nil {
		t.Fatalf("reload original config: %v", err)
	}
	if loadedOriginal.IsActive {
		t.Fatalf("expected original config to be retired after promote, got %#v", loadedOriginal)
	}

	loadedUnrelated, err := db.FindGitHubConfigByID(ctx, unrelated.ID)
	if err != nil {
		t.Fatalf("reload unrelated config: %v", err)
	}
	if !loadedUnrelated.IsActive {
		t.Fatalf("expected unrelated config to remain active after promote, got %#v", loadedUnrelated)
	}
}

func TestGitHubConfigFindActiveByRoutePrefersExactIdentityOverInstallationOnly(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	original, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		Name:                    "alpha-old",
		APIBaseURL:              " https://api.github.com/ ",
		AuthMode:                GitHubAuthModeApp,
		AppID:                   700,
		InstallationID:          800,
		PrivateKeyCiphertext:    "cipher-key-1",
		WebhookSecretCiphertext: "cipher-secret-1",
		SelectedRepos:           []string{"alpha/repo"},
	})
	if err != nil {
		t.Fatalf("save original config: %v", err)
	}

	replacement, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		Name:                    "alpha-new",
		APIBaseURL:              "https://api.github.com",
		AuthMode:                GitHubAuthModeApp,
		AppID:                   700,
		InstallationID:          800,
		PrivateKeyCiphertext:    "cipher-key-2",
		WebhookSecretCiphertext: "cipher-secret-2",
		SelectedRepos:           []string{"alpha/repo"},
	})
	if err != nil {
		t.Fatalf("save replacement config: %v", err)
	}

	unrelated, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		Name:                    "beta-same-installation",
		APIBaseURL:              "https://ghe.example/api/v3",
		AuthMode:                GitHubAuthModeApp,
		AppID:                   701,
		InstallationID:          800,
		PrivateKeyCiphertext:    "cipher-key-3",
		WebhookSecretCiphertext: "cipher-secret-3",
		SelectedRepos:           []string{"beta/repo"},
	})
	if err != nil {
		t.Fatalf("save unrelated config: %v", err)
	}

	resolved, err := db.FindActiveGitHubConfigByRoute(ctx, original.APIBaseURL, original.AppID, original.InstallationID)
	if err != nil {
		t.Fatalf("resolve active config by route: %v", err)
	}
	if resolved.ID != replacement.ID {
		t.Fatalf("expected exact route replacement %d, got %#v", replacement.ID, resolved)
	}
	if resolved.ID == unrelated.ID {
		t.Fatalf("expected route lookup to ignore same-installation unrelated config, got %#v", resolved)
	}
}

func TestGitHubRouteInstallationStatusUpsertAndLookup(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	firstTestedAt := time.Now().Add(-2 * time.Minute).UTC()
	if err := db.UpsertGitHubRouteInstallationStatus(ctx, GitHubRouteInstallationStatus{
		APIBaseURL:                      " https://api.github.com/ ",
		AppID:                           901,
		InstallationID:                  902,
		AccountLogin:                    "env-org",
		AccountType:                     "Organization",
		InstallationState:               "suspended",
		InstallationRepositorySelection: "selected",
		InstallationRepositories:        []string{"env-org/repo-a"},
		LastTestedAt:                    &firstTestedAt,
		LastTestError:                   "initial state",
	}); err != nil {
		t.Fatalf("upsert initial route installation status: %v", err)
	}

	secondTestedAt := time.Now().UTC()
	if err := db.UpsertGitHubRouteInstallationStatus(ctx, GitHubRouteInstallationStatus{
		APIBaseURL:                      "https://api.github.com",
		AppID:                           901,
		InstallationID:                  902,
		AccountLogin:                    "env-org",
		AccountType:                     "Organization",
		InstallationState:               "active",
		InstallationRepositorySelection: "selected",
		InstallationRepositories:        []string{"env-org/repo-b", "env-org/repo-a"},
		LastTestedAt:                    &secondTestedAt,
		LastTestError:                   "",
	}); err != nil {
		t.Fatalf("upsert replacement route installation status: %v", err)
	}

	loaded, err := db.FindGitHubRouteInstallationStatus(ctx, "https://api.github.com/", 901, 902)
	if err != nil {
		t.Fatalf("find route installation status: %v", err)
	}
	if loaded.APIBaseURL != "https://api.github.com" || loaded.InstallationState != "active" {
		t.Fatalf("expected normalized active route status, got %#v", loaded)
	}
	if !reflect.DeepEqual(loaded.InstallationRepositories, []string{"env-org/repo-a", "env-org/repo-b"}) {
		t.Fatalf("unexpected route installation repositories: %#v", loaded.InstallationRepositories)
	}
	if loaded.LastTestedAt == nil || !loaded.LastTestedAt.Equal(secondTestedAt) {
		t.Fatalf("expected latest last_tested_at to persist, got %#v", loaded.LastTestedAt)
	}
	if loaded.LastTestError != "" {
		t.Fatalf("expected latest last_test_error to overwrite previous value, got %#v", loaded)
	}
}

func TestGitHubConfigAllowsMultipleActiveConfigsAndAuditMetadata(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	first, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		Name:                    "alpha-prod",
		Tags:                    []string{"prod", "alpha"},
		APIBaseURL:              "https://api.github.com",
		AuthMode:                GitHubAuthModeApp,
		AppID:                   100,
		InstallationID:          200,
		PrivateKeyCiphertext:    "cipher-key-1",
		WebhookSecretCiphertext: "cipher-secret-1",
		SelectedRepos:           []string{"alpha/repo"},
	})
	if err != nil {
		t.Fatalf("save first active config: %v", err)
	}

	second, err := db.SaveActiveGitHubConfig(ctx, GitHubConfig{
		Name:                    "beta-stage",
		Tags:                    []string{"staging", "beta"},
		APIBaseURL:              "https://api.github.com",
		AuthMode:                GitHubAuthModeApp,
		AppID:                   101,
		InstallationID:          201,
		PrivateKeyCiphertext:    "cipher-key-2",
		WebhookSecretCiphertext: "cipher-secret-2",
		SelectedRepos:           []string{"beta/repo"},
	})
	if err != nil {
		t.Fatalf("save second active config: %v", err)
	}

	activeItems, err := db.ListActiveGitHubConfigs(ctx)
	if err != nil {
		t.Fatalf("list active configs: %v", err)
	}
	if len(activeItems) != 2 {
		t.Fatalf("expected two active configs, got %#v", activeItems)
	}
	if activeItems[0].ID != second.ID || activeItems[0].Name != "beta-stage" || activeItems[1].ID != first.ID {
		t.Fatalf("expected active configs ordered newest-first, got %#v", activeItems)
	}
	if !reflect.DeepEqual(activeItems[0].Tags, []string{"beta", "staging"}) {
		t.Fatalf("expected normalized audit tags on second config, got %#v", activeItems[0].Tags)
	}

	compatibility, err := db.FindActiveGitHubConfig(ctx)
	if err != nil {
		t.Fatalf("find compatibility active config: %v", err)
	}
	if compatibility.ID != second.ID {
		t.Fatalf("expected latest active config in compatibility lookup, got %#v", compatibility)
	}

	resolvedFirst, err := db.FindActiveGitHubConfigByInstallationID(ctx, 200)
	if err != nil {
		t.Fatalf("find first config by installation id: %v", err)
	}
	if resolvedFirst.ID != first.ID || resolvedFirst.Name != "alpha-prod" {
		t.Fatalf("expected first config by installation id, got %#v", resolvedFirst)
	}
}

func TestJobAndRunnerPersistGitHubTraceSnapshots(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	policyRecord, err := db.CreatePolicy(ctx, Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.A1.Flex",
		OCPU:       2,
		MemoryGB:   8,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}

	jobRecord, err := db.UpsertJob(ctx, Job{
		GitHubJobID:      909,
		DeliveryID:       "delivery-909",
		InstallationID:   321,
		GitHubConfigID:   88,
		GitHubConfigName: "alpha-prod",
		GitHubConfigTags: []string{"prod", "alpha", "alpha"},
		RepoOwner:        "example",
		RepoName:         "repo",
		RunID:            909,
		RunAttempt:       1,
		Status:           "queued",
		Labels:           []string{"oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("upsert job: %v", err)
	}
	if jobRecord.GitHubConfigID != 88 || jobRecord.GitHubConfigName != "alpha-prod" || !reflect.DeepEqual(jobRecord.GitHubConfigTags, []string{"alpha", "prod"}) {
		t.Fatalf("expected persisted job trace snapshot, got %#v", jobRecord)
	}

	jobRecord, err = db.UpsertJob(ctx, Job{
		GitHubJobID:    909,
		DeliveryID:     "delivery-909-b",
		InstallationID: 321,
		RepoOwner:      "example",
		RepoName:       "repo",
		RunID:          909,
		RunAttempt:     2,
		Status:         "in_progress",
		Labels:         []string{"oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("update job without trace fields: %v", err)
	}
	if jobRecord.GitHubConfigID != 88 || jobRecord.GitHubConfigName != "alpha-prod" || !reflect.DeepEqual(jobRecord.GitHubConfigTags, []string{"alpha", "prod"}) {
		t.Fatalf("expected existing job trace snapshot to be preserved, got %#v", jobRecord)
	}

	jobRecord, err = db.UpsertJob(ctx, Job{
		GitHubJobID:      909,
		DeliveryID:       "delivery-909-c",
		InstallationID:   654,
		GitHubConfigID:   99,
		GitHubConfigName: "beta-stage",
		GitHubConfigTags: []string{"staging", "beta"},
		RepoOwner:        "example",
		RepoName:         "repo",
		RunID:            909,
		RunAttempt:       3,
		Status:           "completed",
		Labels:           []string{"oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("update job with conflicting trace fields: %v", err)
	}
	if jobRecord.GitHubConfigID != 88 || jobRecord.GitHubConfigName != "alpha-prod" || !reflect.DeepEqual(jobRecord.GitHubConfigTags, []string{"alpha", "prod"}) {
		t.Fatalf("expected first non-empty job trace snapshot to remain frozen, got %#v", jobRecord)
	}

	runnerRecord, err := db.CreateRunner(ctx, Runner{
		PolicyID:         policyRecord.ID,
		JobID:            jobRecord.ID,
		InstallationID:   321,
		GitHubConfigID:   jobRecord.GitHubConfigID,
		GitHubConfigName: jobRecord.GitHubConfigName,
		GitHubConfigTags: jobRecord.GitHubConfigTags,
		InstanceOCID:     "ocid1.instance.oc1..trace",
		RepoOwner:        "example",
		RepoName:         "repo",
		RunnerName:       "runner-trace",
		Status:           "launching",
		Labels:           []string{"self-hosted", "oci", "cpu"},
	})
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}
	if runnerRecord.GitHubConfigID != 88 || runnerRecord.GitHubConfigName != "alpha-prod" || !reflect.DeepEqual(runnerRecord.GitHubConfigTags, []string{"alpha", "prod"}) {
		t.Fatalf("expected persisted runner trace snapshot, got %#v", runnerRecord)
	}
}

func TestGitHubPendingManifestPersistsAndExpiresBySessionBinding(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	createdAt := time.Now().UTC().Add(-5 * time.Minute).Round(time.Second)
	expiresAt := createdAt.Add(time.Hour)
	saved, err := db.SaveGitHubPendingManifest(ctx, GitHubPendingManifest{
		SessionBinding:          "binding-1",
		AppID:                   999,
		AppName:                 "OhoCI-localhost-999",
		AppSlug:                 "ohoci-localhost-999",
		AppSettingsURL:          "https://github.com/settings/apps/ohoci-localhost-999",
		TransferURL:             "",
		InstallURL:              "https://github.com/apps/ohoci-localhost-999/installations/new?state=abc",
		OwnerTarget:             "personal",
		PrivateKeyCiphertext:    "cipher-private",
		WebhookSecretCiphertext: "cipher-webhook",
		CreatedAt:               createdAt,
		ExpiresAt:               expiresAt,
	})
	if err != nil {
		t.Fatalf("save github pending manifest: %v", err)
	}
	if saved.SessionBinding != "binding-1" || saved.AppID != 999 || !saved.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected saved github pending manifest: %#v", saved)
	}

	loaded, err := db.FindGitHubPendingManifestBySessionBinding(ctx, "binding-1", createdAt.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("find github pending manifest: %v", err)
	}
	if loaded.SessionBinding != "binding-1" || loaded.PrivateKeyCiphertext != "cipher-private" || loaded.WebhookSecretCiphertext != "cipher-webhook" {
		t.Fatalf("unexpected loaded github pending manifest: %#v", loaded)
	}

	replaced, err := db.SaveGitHubPendingManifest(ctx, GitHubPendingManifest{
		SessionBinding:          "binding-1",
		AppID:                   1000,
		AppName:                 "OhoCI-localhost-1000",
		AppSlug:                 "ohoci-localhost-1000",
		AppSettingsURL:          "https://github.com/settings/apps/ohoci-localhost-1000",
		TransferURL:             "https://github.com/settings/apps/ohoci-localhost-1000/advanced",
		InstallURL:              "https://github.com/apps/ohoci-localhost-1000/installations/new?state=xyz",
		OwnerTarget:             "organization",
		PrivateKeyCiphertext:    "cipher-private-2",
		WebhookSecretCiphertext: "cipher-webhook-2",
		CreatedAt:               createdAt.Add(10 * time.Minute),
		ExpiresAt:               expiresAt.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("replace github pending manifest: %v", err)
	}
	if replaced.AppID != 1000 || replaced.OwnerTarget != "organization" {
		t.Fatalf("expected replacement github pending manifest, got %#v", replaced)
	}

	if _, err := db.FindGitHubPendingManifestBySessionBinding(ctx, "binding-1", expiresAt.Add(11*time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected expired github pending manifest to disappear, got %v", err)
	}

	if err := db.DeleteGitHubPendingManifestBySessionBinding(ctx, "binding-1"); err != nil {
		t.Fatalf("delete github pending manifest: %v", err)
	}
}
