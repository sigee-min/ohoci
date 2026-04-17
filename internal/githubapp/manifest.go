package githubapp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ohoci/internal/store"
)

const (
	defaultGitHubAPIBaseURL  = "https://api.github.com"
	githubManifestStateTTL   = time.Hour
	githubManifestPendingTTL = time.Hour

	githubManifestOwnerTargetPersonal     = "personal"
	githubManifestOwnerTargetOrganization = "organization"
)

type ManifestStartInput struct {
	APIBaseURL       string `json:"apiBaseUrl"`
	OwnerTarget      string `json:"ownerTarget"`
	OrganizationSlug string `json:"organizationSlug,omitempty"`
}

type ManifestStart struct {
	RedirectURL string `json:"redirectUrl"`
}

type ManifestLaunch struct {
	PostURL      string
	State        string
	ManifestJSON string
}

type PendingManifest struct {
	AppID          int64     `json:"appId"`
	AppName        string    `json:"appName"`
	AppSlug        string    `json:"appSlug"`
	AppSettingsURL string    `json:"appSettingsUrl"`
	TransferURL    string    `json:"transferUrl,omitempty"`
	InstallURL     string    `json:"installUrl"`
	OwnerTarget    string    `json:"ownerTarget"`
	PrivateKeyPEM  string    `json:"privateKeyPem"`
	WebhookSecret  string    `json:"webhookSecret"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type InstallationLookup struct {
	Installations      []AppInstallation `json:"installations"`
	AutoInstallationID int64             `json:"autoInstallationId,omitempty"`
}

type manifestStateClaims struct {
	Purpose          string `json:"purpose"`
	SessionBinding   string `json:"sessionBinding"`
	Nonce            string `json:"nonce"`
	ExpiresAtUnix    int64  `json:"expiresAtUnix"`
	OwnerTarget      string `json:"ownerTarget,omitempty"`
	OrganizationSlug string `json:"organizationSlug,omitempty"`
}

var exchangeManifestCode = ExchangeManifestCode
var gitHubOrganizationSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func normalizeAPIBaseURL(value string) string {
	normalized := strings.TrimRight(strings.TrimSpace(value), "/")
	if normalized == "" {
		return defaultGitHubAPIBaseURL
	}
	return normalized
}

func ManifestHelperDisabledReason(apiBaseURL string) string {
	if normalizeAPIBaseURL(apiBaseURL) != defaultGitHubAPIBaseURL {
		return "GitHub App manifest helper is only available for github.com. Leave GitHub API URL empty or set it to https://api.github.com."
	}
	return ""
}

func (s *Service) StartManifest(sessionToken, apiBaseURL string) (ManifestStart, error) {
	return s.StartManifestWithInput(sessionToken, ManifestStartInput{
		APIBaseURL: apiBaseURL,
	})
}

func (s *Service) StartManifestWithInput(sessionToken string, input ManifestStartInput) (ManifestStart, error) {
	if reason := ManifestHelperDisabledReason(input.APIBaseURL); reason != "" {
		return ManifestStart{}, fmt.Errorf("%s", reason)
	}
	if strings.TrimSpace(s.appRootURL()) == "" {
		return ManifestStart{}, fmt.Errorf("github app manifest helper requires a public base URL")
	}

	ownerTarget, err := normalizeManifestOwnerTarget(input.OwnerTarget)
	if err != nil {
		return ManifestStart{}, err
	}
	organizationSlug, err := normalizeManifestOrganizationSlug(ownerTarget, input.OrganizationSlug)
	if err != nil {
		return ManifestStart{}, err
	}

	state, err := s.newManifestStateTokenForOwner(sessionToken, ownerTarget, organizationSlug)
	if err != nil {
		return ManifestStart{}, err
	}

	return ManifestStart{
		RedirectURL: s.appRootURL() + "/api/v1/github/config/manifest/launch?state=" + url.QueryEscape(state),
	}, nil
}

func (s *Service) ManifestLaunch(sessionToken, state string) (ManifestLaunch, error) {
	claims, err := s.parseManifestStateToken(state)
	if err != nil {
		return ManifestLaunch{}, err
	}
	if err := s.validateManifestSession(claims, sessionToken); err != nil {
		return ManifestLaunch{}, err
	}

	manifestPayload, err := json.Marshal(map[string]any{
		"name":        s.manifestAppName(claims.Nonce),
		"url":         s.appRootURL(),
		"setup_url":   s.manifestSetupURL(),
		"description": "OhoCI GitHub Actions runner control plane",
		"public":      false,
		"hook_attributes": map[string]any{
			"url":    s.webhookURL(),
			"active": true,
		},
		"redirect_url": s.appRootURL() + "/api/v1/github/config/manifest/callback",
		"default_permissions": map[string]string{
			"actions":        "read",
			"administration": "write",
			"metadata":       "read",
		},
		// GitHub Apps always receive installation and installation_repositories
		// lifecycle events. The manifest only needs to opt into workflow_job.
		"default_events": []string{
			"workflow_job",
		},
	})
	if err != nil {
		return ManifestLaunch{}, err
	}

	return ManifestLaunch{
		PostURL:      manifestCreateURL(claims.OwnerTarget, claims.OrganizationSlug, state),
		State:        state,
		ManifestJSON: string(manifestPayload),
	}, nil
}

func (s *Service) CompleteManifest(ctx context.Context, sessionToken, state, code string) (PendingManifest, error) {
	claims, err := s.parseManifestStateToken(state)
	if err != nil {
		return PendingManifest{}, err
	}
	if err := s.validateManifestSession(claims, sessionToken); err != nil {
		return PendingManifest{}, err
	}

	conversion, err := exchangeManifestCode(ctx, defaultGitHubAPIBaseURL, code)
	if err != nil {
		return PendingManifest{}, err
	}

	installState, err := s.newManifestStateTokenForOwner(sessionToken, claims.OwnerTarget, claims.OrganizationSlug)
	if err != nil {
		return PendingManifest{}, err
	}

	appSettingsURL := manifestAppSettingsURL(conversion.Slug, conversion.HTMLURL)

	expiresAt := s.now().UTC().Add(githubManifestPendingTTL)
	result := PendingManifest{
		AppID:          conversion.AppID,
		AppName:        conversion.Name,
		AppSlug:        conversion.Slug,
		AppSettingsURL: appSettingsURL,
		TransferURL:    "",
		InstallURL:     manifestInstallURL(conversion.Slug, conversion.HTMLURL, installState),
		OwnerTarget:    claims.OwnerTarget,
		PrivateKeyPEM:  conversion.PrivateKeyPEM,
		WebhookSecret:  conversion.WebhookSecret,
		CreatedAt:      s.now().UTC(),
		ExpiresAt:      expiresAt,
	}

	privateKeyCiphertext, err := s.encrypt(result.PrivateKeyPEM)
	if err != nil {
		return PendingManifest{}, err
	}
	webhookSecretCiphertext, err := s.encrypt(result.WebhookSecret)
	if err != nil {
		return PendingManifest{}, err
	}
	if _, err := s.store.SaveGitHubPendingManifest(ctx, store.GitHubPendingManifest{
		SessionBinding:          s.sessionBinding(sessionToken),
		AppID:                   result.AppID,
		AppName:                 result.AppName,
		AppSlug:                 result.AppSlug,
		AppSettingsURL:          result.AppSettingsURL,
		TransferURL:             result.TransferURL,
		InstallURL:              result.InstallURL,
		OwnerTarget:             result.OwnerTarget,
		PrivateKeyCiphertext:    privateKeyCiphertext,
		WebhookSecretCiphertext: webhookSecretCiphertext,
		CreatedAt:               result.CreatedAt,
		ExpiresAt:               result.ExpiresAt,
	}); err != nil {
		return PendingManifest{}, err
	}

	return result, nil
}

func (s *Service) PendingManifest(ctx context.Context, sessionToken string) (*PendingManifest, error) {
	binding := s.sessionBinding(sessionToken)
	if binding == "" {
		return nil, nil
	}

	record, err := s.store.FindGitHubPendingManifestBySessionBinding(ctx, binding, s.now().UTC())
	switch {
	case err == nil:
	case err == store.ErrNotFound:
		return nil, nil
	default:
		return nil, err
	}
	result, err := s.pendingManifestFromRecord(record)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) ValidateManifestState(sessionToken, state string) error {
	claims, err := s.parseManifestStateToken(state)
	if err != nil {
		return err
	}
	return s.validateManifestSession(claims, sessionToken)
}

func (s *Service) ValidateManifestInstallReturn(ctx context.Context, sessionToken, state, installationID string) error {
	parsedInstallationID, err := strconv.ParseInt(strings.TrimSpace(installationID), 10, 64)
	if err != nil || parsedInstallationID <= 0 {
		return fmt.Errorf("github installation id is required")
	}

	pending, err := s.pendingManifestForInstallReturn(ctx, sessionToken, state)
	if err != nil {
		return err
	}
	client, err := New(normalizeConfig(Config{
		APIBaseURL:    defaultGitHubAPIBaseURL,
		AppID:         pending.AppID,
		PrivateKeyPEM: pending.PrivateKeyPEM,
	}))
	if err != nil {
		return err
	}
	installation, err := client.FindInstallation(ctx, parsedInstallationID)
	if err != nil {
		return err
	}
	if installation == nil {
		return fmt.Errorf("github installation id is not associated with the pending app draft")
	}
	return nil
}

func (s *Service) ClearPendingManifest(ctx context.Context, sessionToken string) error {
	binding := s.sessionBinding(sessionToken)
	if binding == "" {
		return nil
	}
	return s.store.DeleteGitHubPendingManifestBySessionBinding(ctx, binding)
}

func (s *Service) DiscoverInstallations(ctx context.Context, input Input) (InstallationLookup, error) {
	cfg := normalizeConfig(Config{
		APIBaseURL:     input.APIBaseURL,
		AppID:          input.AppID,
		PrivateKeyPEM:  input.PrivateKeyPEM,
		SelectedRepos:  input.SelectedRepos,
		WebhookSecret:  input.WebhookSecret,
		InstallationID: input.InstallationID,
	})
	if cfg.AppID <= 0 {
		return InstallationLookup{}, fmt.Errorf("github app id is required")
	}
	if strings.TrimSpace(cfg.PrivateKeyPEM) == "" {
		return InstallationLookup{}, fmt.Errorf("github app private key is required")
	}

	client, err := New(cfg)
	if err != nil {
		return InstallationLookup{}, err
	}
	installations, err := client.ListInstallations(ctx)
	if err != nil {
		return InstallationLookup{}, err
	}

	result := InstallationLookup{
		Installations: append([]AppInstallation(nil), installations...),
	}
	if len(installations) == 1 {
		result.AutoInstallationID = installations[0].ID
	}
	return result, nil
}

func (s *Service) newManifestStateToken(sessionToken string) (string, error) {
	return s.newManifestStateTokenForOwner(sessionToken, githubManifestOwnerTargetPersonal, "")
}

func (s *Service) newManifestStateTokenForOwner(sessionToken, ownerTarget, organizationSlug string) (string, error) {
	if strings.TrimSpace(sessionToken) == "" {
		return "", fmt.Errorf("authenticated session is required")
	}
	normalizedOwnerTarget, err := normalizeManifestOwnerTarget(ownerTarget)
	if err != nil {
		return "", err
	}
	normalizedOrganizationSlug, err := normalizeManifestOrganizationSlug(normalizedOwnerTarget, organizationSlug)
	if err != nil {
		return "", err
	}
	nonce, err := randomHex(8)
	if err != nil {
		return "", err
	}
	claims := manifestStateClaims{
		Purpose:          "github-manifest",
		SessionBinding:   s.sessionBinding(sessionToken),
		Nonce:            nonce,
		ExpiresAtUnix:    s.now().UTC().Add(githubManifestStateTTL).Unix(),
		OwnerTarget:      normalizedOwnerTarget,
		OrganizationSlug: normalizedOrganizationSlug,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signature := s.signManifestPayload(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *Service) parseManifestStateToken(value string) (manifestStateClaims, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 2 {
		return manifestStateClaims{}, fmt.Errorf("invalid github manifest state")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return manifestStateClaims{}, fmt.Errorf("invalid github manifest state")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return manifestStateClaims{}, fmt.Errorf("invalid github manifest state")
	}
	expected := s.signManifestPayload(payload)
	if !hmac.Equal(signature, expected) {
		return manifestStateClaims{}, fmt.Errorf("invalid github manifest state")
	}

	var claims manifestStateClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return manifestStateClaims{}, fmt.Errorf("invalid github manifest state")
	}
	if claims.Purpose != "github-manifest" {
		return manifestStateClaims{}, fmt.Errorf("invalid github manifest state")
	}
	if claims.ExpiresAtUnix <= s.now().UTC().Unix() {
		return manifestStateClaims{}, fmt.Errorf("github manifest flow expired")
	}
	if strings.TrimSpace(claims.SessionBinding) == "" {
		return manifestStateClaims{}, fmt.Errorf("invalid github manifest state")
	}
	claims.OwnerTarget, err = normalizeManifestOwnerTarget(claims.OwnerTarget)
	if err != nil {
		return manifestStateClaims{}, fmt.Errorf("invalid github manifest state")
	}
	claims.OrganizationSlug, err = normalizeManifestOrganizationSlug(claims.OwnerTarget, claims.OrganizationSlug)
	if err != nil {
		return manifestStateClaims{}, fmt.Errorf("invalid github manifest state")
	}
	return claims, nil
}

func (s *Service) validateManifestSession(claims manifestStateClaims, sessionToken string) error {
	if strings.TrimSpace(sessionToken) == "" {
		return fmt.Errorf("authenticated session is required")
	}
	if claims.SessionBinding != s.sessionBinding(sessionToken) {
		return fmt.Errorf("github manifest flow does not match the current session")
	}
	return nil
}

func (s *Service) signManifestPayload(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func (s *Service) sessionBinding(sessionToken string) string {
	trimmed := strings.TrimSpace(sessionToken)
	if trimmed == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte(trimmed))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) pendingManifestForInstallReturn(ctx context.Context, sessionToken, state string) (*PendingManifest, error) {
	if strings.TrimSpace(state) != "" {
		if err := s.ValidateManifestState(sessionToken, state); err != nil {
			return nil, err
		}
	}

	pending, err := s.PendingManifest(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	if pending == nil {
		return nil, fmt.Errorf("github manifest install return requires a pending draft in the current session")
	}
	return pending, nil
}

func (s *Service) pendingManifestFromRecord(record store.GitHubPendingManifest) (PendingManifest, error) {
	privateKey, err := s.decrypt(record.PrivateKeyCiphertext)
	if err != nil {
		return PendingManifest{}, err
	}
	webhookSecret, err := s.decrypt(record.WebhookSecretCiphertext)
	if err != nil {
		return PendingManifest{}, err
	}
	return PendingManifest{
		AppID:          record.AppID,
		AppName:        record.AppName,
		AppSlug:        record.AppSlug,
		AppSettingsURL: record.AppSettingsURL,
		TransferURL:    "",
		InstallURL:     record.InstallURL,
		OwnerTarget:    record.OwnerTarget,
		PrivateKeyPEM:  privateKey,
		WebhookSecret:  webhookSecret,
		CreatedAt:      record.CreatedAt,
		ExpiresAt:      record.ExpiresAt,
	}, nil
}

func (s *Service) appRootURL() string {
	return strings.TrimRight(strings.TrimSpace(s.publicBaseURL), "/")
}

func (s *Service) manifestSetupURL() string {
	return s.appRootURL() + "/api/v1/github/config/manifest/callback?source=install"
}

func (s *Service) manifestAppName(nonce string) string {
	host := sanitizeHostLabel(s.appRootURL())
	name := "OhoCI"
	if host != "" {
		name += "-" + host
	}
	if strings.TrimSpace(nonce) != "" {
		name += "-" + strings.ToLower(strings.TrimSpace(nonce))
	}
	if len(name) > 34 {
		return name[:34]
	}
	return name
}

func sanitizeHostLabel(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(host))
	lastDash := false
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func manifestInstallURL(slug, htmlURL, state string) string {
	baseURL := ""
	normalizedSlug := strings.TrimSpace(slug)
	if normalizedSlug != "" {
		baseURL = "https://github.com/apps/" + normalizedSlug + "/installations/new"
	} else {
		normalizedHTMLURL := strings.TrimRight(strings.TrimSpace(htmlURL), "/")
		if normalizedHTMLURL == "" {
			return ""
		}
		baseURL = normalizedHTMLURL + "/installations/new"
	}

	normalizedState := strings.TrimSpace(state)
	if normalizedState == "" {
		return baseURL
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	values := parsed.Query()
	values.Set("state", normalizedState)
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func manifestAppSettingsURL(slug, htmlURL string) string {
	normalizedSlug := strings.TrimSpace(slug)
	if normalizedSlug != "" {
		return "https://github.com/settings/apps/" + url.PathEscape(normalizedSlug)
	}

	normalizedHTMLURL := strings.TrimRight(strings.TrimSpace(htmlURL), "/")
	if normalizedHTMLURL == "" {
		return ""
	}

	parsed, err := url.Parse(normalizedHTMLURL)
	if err != nil {
		return normalizedHTMLURL
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	switch {
	case len(parts) == 2 && parts[0] == "apps" && parts[1] != "":
		parsed.Path = "/settings/apps/" + parts[1]
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	case len(parts) >= 3 && parts[0] == "settings" && parts[1] == "apps" && parts[2] != "":
		parsed.Path = "/settings/apps/" + parts[2]
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	default:
		return normalizedHTMLURL
	}
}

func manifestCreateURL(ownerTarget, organizationSlug, state string) string {
	baseURL := "https://github.com/settings/apps/new"
	if ownerTarget == githubManifestOwnerTargetOrganization {
		baseURL = "https://github.com/organizations/" + url.PathEscape(organizationSlug) + "/settings/apps/new"
	}

	normalizedState := strings.TrimSpace(state)
	if normalizedState == "" {
		return baseURL
	}
	return baseURL + "?state=" + url.QueryEscape(normalizedState)
}

func normalizeManifestOwnerTarget(ownerTarget string) (string, error) {
	switch normalizedOwnerTarget := strings.ToLower(strings.TrimSpace(ownerTarget)); normalizedOwnerTarget {
	case "", githubManifestOwnerTargetPersonal:
		return githubManifestOwnerTargetPersonal, nil
	case githubManifestOwnerTargetOrganization:
		return githubManifestOwnerTargetOrganization, nil
	default:
		return "", fmt.Errorf("github app owner target must be personal or organization")
	}
}

func normalizeManifestOrganizationSlug(ownerTarget, organizationSlug string) (string, error) {
	if ownerTarget != githubManifestOwnerTargetOrganization {
		return "", nil
	}

	normalizedOrganizationSlug := strings.ToLower(strings.TrimSpace(organizationSlug))
	switch {
	case normalizedOrganizationSlug == "":
		return "", fmt.Errorf("github organization slug is required for organization apps")
	case len(normalizedOrganizationSlug) > 39:
		return "", fmt.Errorf("github organization slug must be 39 characters or fewer")
	case !gitHubOrganizationSlugPattern.MatchString(normalizedOrganizationSlug):
		return "", fmt.Errorf("github organization slug must use only letters, numbers, and single hyphens")
	default:
		return normalizedOrganizationSlug, nil
	}
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
