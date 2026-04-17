package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadReadsSecretsFromFiles(t *testing.T) {
	tempDir := t.TempDir()
	sessionPath := filepath.Join(tempDir, "session_secret")
	webhookPath := filepath.Join(tempDir, "webhook_secret")
	privateKeyPath := filepath.Join(tempDir, "github_app_private_key")

	writeTestFile(t, sessionPath, "session-secret\n")
	writeTestFile(t, webhookPath, "webhook-secret\n")
	writeTestFile(t, privateKeyPath, "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----\n")

	t.Setenv("OHOCI_SESSION_SECRET_FILE", sessionPath)
	t.Setenv("OHOCI_GITHUB_WEBHOOK_SECRET_FILE", webhookPath)
	t.Setenv("OHOCI_GITHUB_APP_PRIVATE_KEY_FILE", privateKeyPath)

	cfg := Load()
	if cfg.loadError != nil {
		t.Fatalf("unexpected load error: %v", cfg.loadError)
	}
	if cfg.SessionSecret != "session-secret" {
		t.Fatalf("unexpected session secret: %q", cfg.SessionSecret)
	}
	if cfg.GitHubWebhookSecret != "webhook-secret" {
		t.Fatalf("unexpected webhook secret: %q", cfg.GitHubWebhookSecret)
	}
	if cfg.GitHubAppPrivateKey != "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----" {
		t.Fatalf("expected github app private key to load from file, got %q", cfg.GitHubAppPrivateKey)
	}
}

func TestLoadPrefersInlineEnvOverSecretFile(t *testing.T) {
	tempDir := t.TempDir()
	sessionPath := filepath.Join(tempDir, "session_secret")
	writeTestFile(t, sessionPath, "file-session-secret")

	t.Setenv("OHOCI_SESSION_SECRET", "inline-session-secret")
	t.Setenv("OHOCI_SESSION_SECRET_FILE", sessionPath)

	cfg := Load()
	if cfg.loadError != nil {
		t.Fatalf("unexpected load error: %v", cfg.loadError)
	}
	if cfg.SessionSecret != "inline-session-secret" {
		t.Fatalf("expected inline env to win, got %q", cfg.SessionSecret)
	}
}

func TestLoadFallsBackToSessionSecretForDataEncryptionKey(t *testing.T) {
	t.Setenv("OHOCI_SESSION_SECRET", "session-secret")

	cfg := Load()
	if cfg.loadError != nil {
		t.Fatalf("unexpected load error: %v", cfg.loadError)
	}
	if cfg.DataEncryptionKey != "session-secret" {
		t.Fatalf("expected data encryption key fallback, got %q", cfg.DataEncryptionKey)
	}
}

func TestLoadReadsGitHubAppTraceMetadata(t *testing.T) {
	t.Setenv("OHOCI_GITHUB_APP_NAME", "env-gh-app")
	t.Setenv("OHOCI_GITHUB_APP_TAGS", "prod, env ,prod")

	cfg := Load()
	if cfg.GitHubAppName != "env-gh-app" {
		t.Fatalf("expected github app name, got %q", cfg.GitHubAppName)
	}
	if got := strings.Join(cfg.GitHubAppTags, ","); got != "prod,env,prod" {
		t.Fatalf("expected github app tags from csv env, got %q", got)
	}
}

func TestLoadSetsAuthLockoutDefaults(t *testing.T) {
	cfg := Load()
	if cfg.AuthLockoutAttempts != 15 {
		t.Fatalf("expected default auth lockout attempts 15, got %d", cfg.AuthLockoutAttempts)
	}
	if cfg.AuthLockoutDuration != 5*time.Minute {
		t.Fatalf("expected default auth lockout duration 5m, got %s", cfg.AuthLockoutDuration)
	}
}

func TestLoadSetsIngressCIDRDefaults(t *testing.T) {
	cfg := Load()
	if got := strings.Join(cfg.TrustedProxyCIDRs, ","); got != "0.0.0.0/0" {
		t.Fatalf("expected default trusted proxy CIDR, got %q", got)
	}
	if got := strings.Join(cfg.AdminAllowCIDRs, ","); got != "0.0.0.0/0" {
		t.Fatalf("expected default admin allow CIDR, got %q", got)
	}
	if got := strings.Join(cfg.WebhookAllowCIDRs, ","); got != "0.0.0.0/0" {
		t.Fatalf("expected default webhook allow CIDR, got %q", got)
	}
}

func TestLoadReadsAuthLockoutOverrides(t *testing.T) {
	t.Setenv("OHOCI_AUTH_LOCKOUT_ATTEMPTS", "12")
	t.Setenv("OHOCI_AUTH_LOCKOUT_DURATION", "7m")

	cfg := Load()
	if cfg.AuthLockoutAttempts != 12 {
		t.Fatalf("expected overridden auth lockout attempts, got %d", cfg.AuthLockoutAttempts)
	}
	if cfg.AuthLockoutDuration != 7*time.Minute {
		t.Fatalf("expected overridden auth lockout duration, got %s", cfg.AuthLockoutDuration)
	}
}

func TestLoadSetsDefaultUIDir(t *testing.T) {
	cfg := Load()
	if cfg.UIDir != "./web/dist" {
		t.Fatalf("expected default UI dir ./web/dist, got %q", cfg.UIDir)
	}
}

func TestLoadReadsUIDirOverride(t *testing.T) {
	t.Setenv("OHOCI_UI_DIR", "/srv/ohoci/ui")

	cfg := Load()
	if cfg.UIDir != "/srv/ohoci/ui" {
		t.Fatalf("expected overridden UI dir, got %q", cfg.UIDir)
	}
}

func TestLoadCapturesSecretFileErrors(t *testing.T) {
	t.Setenv("OHOCI_GITHUB_APP_PRIVATE_KEY_FILE", filepath.Join(t.TempDir(), "missing.txt"))

	cfg := Load()
	if cfg.loadError == nil {
		t.Fatalf("expected load error")
	}
	if !strings.Contains(cfg.loadError.Error(), "OHOCI_GITHUB_APP_PRIVATE_KEY") {
		t.Fatalf("unexpected load error: %v", cfg.loadError)
	}
}

func TestValidateRejectsUnsupportedOCIAuthMode(t *testing.T) {
	cfg := Config{
		PublicBaseURL: "http://localhost:8080",
		SessionSecret: "session-secret",
		SQLitePath:    "./ohoci.db",
		OCIAuthMode:   "api_key",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validate to reject unsupported auth mode")
	}
}

func TestValidateAllowsInstancePrincipalWithoutBootstrappedRuntimeTargets(t *testing.T) {
	cfg := Config{
		PublicBaseURL: "http://localhost:8080",
		SessionSecret: "session-secret",
		SQLitePath:    "./ohoci.db",
		OCIAuthMode:   "instance_principal",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected runtime targets to be optionally CMS-managed, got %v", err)
	}
}

func TestValidateAllowsMissingGitHubBootstrapConfig(t *testing.T) {
	cfg := Config{
		PublicBaseURL: "http://localhost:8080",
		SessionSecret: "session-secret",
		SQLitePath:    "./ohoci.db",
		OCIAuthMode:   "fake",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected GitHub bootstrap config to be optional when CMS manages it, got %v", err)
	}
}

func TestValidateRejectsInvalidIngressCIDRs(t *testing.T) {
	cfg := Config{
		PublicBaseURL:     "http://localhost:8080",
		SessionSecret:     "session-secret",
		SQLitePath:        "./ohoci.db",
		OCIAuthMode:       "fake",
		TrustedProxyCIDRs: []string{"not-a-cidr"},
		AdminAllowCIDRs:   []string{"0.0.0.0/0"},
		WebhookAllowCIDRs: []string{"0.0.0.0/0"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validate to reject invalid CIDRs")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
