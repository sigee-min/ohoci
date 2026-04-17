package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment            string
	HTTPAddress            string
	PublicBaseURL          string
	UIDir                  string
	SessionCookieName      string
	SessionSecret          string
	DataEncryptionKey      string
	SessionTTL             time.Duration
	AuthLockoutAttempts    int
	AuthLockoutDuration    time.Duration
	CleanupInterval        time.Duration
	DatabaseURL            string
	SQLitePath             string
	GitHubAPIBaseURL       string
	GitHubAppName          string
	GitHubAppTags          []string
	GitHubAppID            int64
	GitHubInstallationID   int64
	GitHubAppPrivateKey    string
	GitHubWebhookSecret    string
	GitHubAllowedRepos     []string
	TrustedProxyCIDRs      []string
	AdminAllowCIDRs        []string
	WebhookAllowCIDRs      []string
	OCIAuthMode            string
	OCICompartmentID       string
	OCIAvailabilityDomain  string
	OCISubnetID            string
	OCINSGIDs              []string
	OCIImageID             string
	OCIAssignPublicIP      bool
	OCIBillingTagNamespace string
	CacheCompatEnabled     bool
	CacheBucketName        string
	CacheObjectPrefix      string
	CacheRetentionDays     int
	RunnerDownloadBaseURL  string
	RunnerVersion          string
	RunnerUser             string
	RunnerWorkDirectory    string
	loadError              error
}

func Load() Config {
	environment := valueOrDefault("OHOCI_ENV", "local")
	sessionSecret, sessionSecretErr := valueOrFile("OHOCI_SESSION_SECRET", "OHOCI_SESSION_SECRET_FILE")
	dataEncryptionKey, dataEncryptionKeyErr := valueOrFile("OHOCI_DATA_ENCRYPTION_KEY", "OHOCI_DATA_ENCRYPTION_KEY_FILE")
	githubAppPrivateKey, githubAppPrivateKeyErr := valueOrFile("OHOCI_GITHUB_APP_PRIVATE_KEY", "OHOCI_GITHUB_APP_PRIVATE_KEY_FILE")
	githubWebhookSecret, githubWebhookSecretErr := valueOrFile("OHOCI_GITHUB_WEBHOOK_SECRET", "OHOCI_GITHUB_WEBHOOK_SECRET_FILE")
	if strings.TrimSpace(dataEncryptionKey) == "" {
		dataEncryptionKey = sessionSecret
	}
	return Config{
		Environment:            environment,
		HTTPAddress:            valueOrDefault("OHOCI_HTTP_ADDRESS", ":8080"),
		PublicBaseURL:          valueOrDefault("OHOCI_PUBLIC_BASE_URL", "http://localhost:8080"),
		UIDir:                  valueOrDefault("OHOCI_UI_DIR", "./web/dist"),
		SessionCookieName:      valueOrDefault("OHOCI_SESSION_COOKIE_NAME", "ohoci_session"),
		SessionSecret:          sessionSecret,
		DataEncryptionKey:      dataEncryptionKey,
		SessionTTL:             durationValue("OHOCI_SESSION_TTL", 12*time.Hour),
		AuthLockoutAttempts:    intValue("OHOCI_AUTH_LOCKOUT_ATTEMPTS", 15),
		AuthLockoutDuration:    durationValue("OHOCI_AUTH_LOCKOUT_DURATION", 5*time.Minute),
		CleanupInterval:        durationValue("OHOCI_CLEANUP_INTERVAL", time.Minute),
		DatabaseURL:            strings.TrimSpace(os.Getenv("OHOCI_DATABASE_URL")),
		SQLitePath:             valueOrDefault("OHOCI_SQLITE_PATH", "./ohoci.local.db"),
		GitHubAPIBaseURL:       valueOrDefault("OHOCI_GITHUB_API_BASE_URL", "https://api.github.com"),
		GitHubAppName:          strings.TrimSpace(os.Getenv("OHOCI_GITHUB_APP_NAME")),
		GitHubAppTags:          csvValues(os.Getenv("OHOCI_GITHUB_APP_TAGS")),
		GitHubAppID:            int64Value("OHOCI_GITHUB_APP_ID", 0),
		GitHubInstallationID:   int64Value("OHOCI_GITHUB_INSTALLATION_ID", 0),
		GitHubAppPrivateKey:    githubAppPrivateKey,
		GitHubWebhookSecret:    githubWebhookSecret,
		GitHubAllowedRepos:     csvValues(os.Getenv("OHOCI_GITHUB_ALLOWED_REPOS")),
		TrustedProxyCIDRs:      csvValues(valueOrDefault("OHOCI_TRUSTED_PROXY_CIDRS", "0.0.0.0/0")),
		AdminAllowCIDRs:        csvValues(valueOrDefault("OHOCI_ADMIN_ALLOW_CIDRS", "0.0.0.0/0")),
		WebhookAllowCIDRs:      csvValues(valueOrDefault("OHOCI_WEBHOOK_ALLOW_CIDRS", "0.0.0.0/0")),
		OCIAuthMode:            valueOrDefault("OHOCI_OCI_AUTH_MODE", defaultOCIAuthMode(environment)),
		OCICompartmentID:       strings.TrimSpace(os.Getenv("OHOCI_OCI_COMPARTMENT_OCID")),
		OCIAvailabilityDomain:  strings.TrimSpace(os.Getenv("OHOCI_OCI_AVAILABILITY_DOMAIN")),
		OCISubnetID:            strings.TrimSpace(os.Getenv("OHOCI_OCI_SUBNET_OCID")),
		OCINSGIDs:              csvValues(os.Getenv("OHOCI_OCI_NSG_OCIDS")),
		OCIImageID:             strings.TrimSpace(os.Getenv("OHOCI_OCI_IMAGE_OCID")),
		OCIAssignPublicIP:      boolValue("OHOCI_OCI_ASSIGN_PUBLIC_IP", false),
		OCIBillingTagNamespace: strings.TrimSpace(os.Getenv("OHOCI_OCI_BILLING_TAG_NAMESPACE")),
		CacheCompatEnabled:     boolValue("OHOCI_CACHE_COMPAT_ENABLED", false),
		CacheBucketName:        strings.TrimSpace(os.Getenv("OHOCI_CACHE_BUCKET_NAME")),
		CacheObjectPrefix:      strings.TrimSpace(os.Getenv("OHOCI_CACHE_OBJECT_PREFIX")),
		CacheRetentionDays:     intValue("OHOCI_CACHE_RETENTION_DAYS", 7),
		RunnerDownloadBaseURL:  valueOrDefault("OHOCI_RUNNER_DOWNLOAD_BASE_URL", "https://github.com/actions/runner/releases/download"),
		RunnerVersion:          valueOrDefault("OHOCI_RUNNER_VERSION", "2.327.1"),
		RunnerUser:             valueOrDefault("OHOCI_RUNNER_USER", "runner"),
		RunnerWorkDirectory:    valueOrDefault("OHOCI_RUNNER_WORKDIR", "/home/runner/actions-runner"),
		loadError:              firstError(sessionSecretErr, dataEncryptionKeyErr, githubAppPrivateKeyErr, githubWebhookSecretErr),
	}
}

func (c Config) Validate() error {
	if c.loadError != nil {
		return c.loadError
	}
	if c.SessionSecret == "" {
		return fmt.Errorf("OHOCI_SESSION_SECRET is required")
	}
	if _, err := url.Parse(c.PublicBaseURL); err != nil {
		return fmt.Errorf("OHOCI_PUBLIC_BASE_URL must be a valid URL: %w", err)
	}
	if c.DatabaseURL == "" && c.SQLitePath == "" {
		return fmt.Errorf("either OHOCI_DATABASE_URL or OHOCI_SQLITE_PATH is required")
	}
	if c.CacheCompatEnabled && c.CacheRetentionDays <= 0 {
		return fmt.Errorf("OHOCI_CACHE_RETENTION_DAYS must be greater than zero")
	}
	if err := validateCIDRList("OHOCI_TRUSTED_PROXY_CIDRS", c.TrustedProxyCIDRs); err != nil {
		return err
	}
	if err := validateCIDRList("OHOCI_ADMIN_ALLOW_CIDRS", c.AdminAllowCIDRs); err != nil {
		return err
	}
	if err := validateCIDRList("OHOCI_WEBHOOK_ALLOW_CIDRS", c.WebhookAllowCIDRs); err != nil {
		return err
	}
	switch c.OCIAuthMode {
	case "fake":
	case "instance_principal":
	default:
		return fmt.Errorf("OHOCI_OCI_AUTH_MODE must be fake or instance_principal")
	}
	return nil
}

func defaultOCIAuthMode(environment string) string {
	if strings.EqualFold(environment, "local") {
		return "fake"
	}
	return "instance_principal"
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func durationValue(key string, fallback time.Duration) time.Duration {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := time.ParseDuration(value)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func valueOrFile(key, fileKey string) (string, error) {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value, nil
	}
	path := strings.TrimSpace(os.Getenv(fileKey))
	if path == "" {
		return "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s from %s: %w", key, fileKey, err)
	}
	return strings.TrimSpace(string(content)), nil
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func int64Value(key string, fallback int64) int64 {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func intValue(key string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func boolValue(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func csvValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		values = append(values, value)
	}
	return values
}

func validateCIDRList(name string, cidrs []string) error {
	for _, cidr := range cidrs {
		if _, err := netip.ParsePrefix(strings.TrimSpace(cidr)); err != nil {
			return fmt.Errorf("%s must contain valid CIDRs: %w", name, err)
		}
	}
	return nil
}
