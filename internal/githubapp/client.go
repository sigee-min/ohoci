package githubapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	Name                            string
	Tags                            []string
	APIBaseURL                      string
	AuthMode                        string
	WebhookSecret                   string
	SelectedRepos                   []string
	AppID                           int64
	InstallationID                  int64
	PrivateKeyPEM                   string
	AllowedOrg                      string
	AllowedRepos                    []string
	AccountLogin                    string
	AccountType                     string
	InstallationState               string
	InstallationRepositorySelection string
	InstallationRepositories        []string
}

type RunnerRegistrationToken struct {
	Token     string
	ExpiresAt time.Time
}

type RepositoryRunner struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Busy   bool   `json:"busy"`
}

type Repository struct {
	FullName string `json:"fullName"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	Private  bool   `json:"private"`
	Admin    bool   `json:"admin"`
}

type InstallationDiscovery struct {
	AccountLogin        string       `json:"accountLogin"`
	AccountType         string       `json:"accountType"`
	RepositorySelection string       `json:"repositorySelection"`
	Repositories        []Repository `json:"repositories"`
}

type AppInstallation struct {
	ID                  int64  `json:"id"`
	AccountLogin        string `json:"accountLogin"`
	AccountType         string `json:"accountType"`
	RepositorySelection string `json:"repositorySelection"`
	HTMLURL             string `json:"htmlUrl"`
	AppSlug             string `json:"appSlug"`
}

type ManifestConversion struct {
	AppID         int64  `json:"appId"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	HTMLURL       string `json:"htmlUrl"`
	PrivateKeyPEM string `json:"privateKeyPem"`
	WebhookSecret string `json:"webhookSecret"`
}

type Client struct {
	cfg        Config
	httpClient *http.Client
	allowed    map[string]struct{}
	privateKey *rsa.PrivateKey
}

func New(cfg Config) (*Client, error) {
	apiBaseURL := strings.TrimSpace(cfg.APIBaseURL)
	if apiBaseURL == "" {
		apiBaseURL = "https://api.github.com"
	}
	if cfg.AppID <= 0 {
		return nil, fmt.Errorf("github app id is required")
	}
	if strings.TrimSpace(cfg.PrivateKeyPEM) == "" {
		return nil, fmt.Errorf("github app private key is required")
	}

	selected := cfg.SelectedRepos
	if len(cfg.AllowedRepos) > 0 {
		selected = cfg.AllowedRepos
	}
	selected = normalizeRepoNames(selected)
	allowed := make(map[string]struct{}, len(selected))
	for _, repo := range selected {
		allowed[strings.ToLower(strings.TrimSpace(repo))] = struct{}{}
	}

	key, err := parsePrivateKey(strings.TrimSpace(cfg.PrivateKeyPEM))
	if err != nil {
		return nil, err
	}

	return &Client{
		cfg: Config{
			Name:                            strings.TrimSpace(cfg.Name),
			Tags:                            normalizeRepoNames(cfg.Tags),
			APIBaseURL:                      apiBaseURL,
			AuthMode:                        "app",
			WebhookSecret:                   strings.TrimSpace(cfg.WebhookSecret),
			SelectedRepos:                   normalizeRepoNames(cfg.SelectedRepos),
			AppID:                           cfg.AppID,
			InstallationID:                  cfg.InstallationID,
			PrivateKeyPEM:                   strings.TrimSpace(cfg.PrivateKeyPEM),
			AllowedOrg:                      strings.TrimSpace(cfg.AllowedOrg),
			AllowedRepos:                    normalizeRepoNames(cfg.AllowedRepos),
			AccountLogin:                    strings.TrimSpace(cfg.AccountLogin),
			AccountType:                     strings.TrimSpace(cfg.AccountType),
			InstallationState:               strings.ToLower(strings.TrimSpace(cfg.InstallationState)),
			InstallationRepositorySelection: strings.ToLower(strings.TrimSpace(cfg.InstallationRepositorySelection)),
			InstallationRepositories:        normalizeRepoNames(cfg.InstallationRepositories),
		},
		httpClient: &http.Client{Timeout: 15 * time.Second},
		allowed:    allowed,
		privateKey: key,
	}, nil
}

func (c *Client) ValidateWebhookSignature(body []byte, signature string) bool {
	signature = strings.TrimSpace(strings.TrimPrefix(signature, "sha256="))
	mac := hmac.New(sha256.New, []byte(c.cfg.WebhookSecret))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (c *Client) AuthMode() string {
	return "app"
}

func (c *Client) InstallationID() int64 {
	return c.cfg.InstallationID
}

func (c *Client) RepositoryAllowed(owner, repo string) bool {
	if strings.TrimSpace(c.cfg.AllowedOrg) != "" && !strings.EqualFold(strings.TrimSpace(owner), c.cfg.AllowedOrg) {
		return false
	}
	if len(c.allowed) == 0 {
		return strings.TrimSpace(c.cfg.AllowedOrg) != ""
	}
	_, ok := c.allowed[strings.ToLower(strings.TrimSpace(owner)+"/"+strings.TrimSpace(repo))]
	return ok
}

func (c *Client) CreateRepoRunnerToken(ctx context.Context, installationID int64, owner, repo string) (RunnerRegistrationToken, error) {
	responseBody, err := c.doRunnerJSON(ctx, installationID, http.MethodPost, fmt.Sprintf("/repos/%s/%s/actions/runners/registration-token", owner, repo), map[string]any{})
	if err != nil {
		return RunnerRegistrationToken{}, err
	}
	var payload struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return RunnerRegistrationToken{}, err
	}
	return RunnerRegistrationToken{Token: payload.Token, ExpiresAt: payload.ExpiresAt.UTC()}, nil
}

func (c *Client) ListRepoRunners(ctx context.Context, installationID int64, owner, repo string) ([]RepositoryRunner, error) {
	responseBody, err := c.doRunnerJSON(ctx, installationID, http.MethodGet, fmt.Sprintf("/repos/%s/%s/actions/runners", owner, repo), nil)
	if err != nil {
		return nil, err
	}
	return decodeRepoRunners(responseBody)
}

func (c *Client) FindRepoRunnerByName(ctx context.Context, installationID int64, owner, repo, runnerName string) (*RepositoryRunner, error) {
	name := strings.TrimSpace(runnerName)
	if name == "" {
		return nil, nil
	}

	query := url.Values{}
	query.Set("name", name)
	responseBody, err := c.doRunnerJSON(
		ctx,
		installationID,
		http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/actions/runners?%s", owner, repo, query.Encode()),
		nil,
	)
	if err != nil {
		return nil, err
	}

	runners, err := decodeRepoRunners(responseBody)
	if err != nil {
		return nil, err
	}
	for _, runner := range runners {
		if strings.EqualFold(strings.TrimSpace(runner.Name), name) {
			match := runner
			return &match, nil
		}
	}
	return nil, nil
}

func (c *Client) DeleteRepoRunner(ctx context.Context, installationID int64, owner, repo string, runnerID int64) error {
	_, err := c.doRunnerJSON(ctx, installationID, http.MethodDelete, fmt.Sprintf("/repos/%s/%s/actions/runners/%d", owner, repo, runnerID), nil)
	return err
}

func (c *Client) TestConnection(ctx context.Context) error {
	request, err := c.newAppRequest(ctx, http.MethodGet, "/app", nil)
	if err != nil {
		return err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 300 {
		return fmt.Errorf("github app metadata request failed: %s", strings.TrimSpace(string(body)))
	}
	_, err = c.installationToken(ctx, c.cfg.InstallationID)
	return err
}

func (c *Client) DiscoverInstallation(ctx context.Context) (InstallationDiscovery, error) {
	if c.effectiveInstallationID(0) <= 0 {
		return InstallationDiscovery{}, fmt.Errorf("github installation id is required")
	}
	request, err := c.newAppRequest(ctx, http.MethodGet, "/app", nil)
	if err != nil {
		return InstallationDiscovery{}, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return InstallationDiscovery{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return InstallationDiscovery{}, err
	}
	if response.StatusCode >= 300 {
		return InstallationDiscovery{}, fmt.Errorf("github app metadata request failed: %s", strings.TrimSpace(string(body)))
	}

	detailBody, err := c.doAppJSON(ctx, http.MethodGet, fmt.Sprintf("/app/installations/%d", c.cfg.InstallationID), nil)
	if err != nil {
		return InstallationDiscovery{}, err
	}
	var detail struct {
		Account struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
		RepositorySelection string `json:"repository_selection"`
	}
	if err := json.Unmarshal(detailBody, &detail); err != nil {
		return InstallationDiscovery{}, err
	}

	repositories, err := c.listInstallationRepositories(ctx)
	if err != nil {
		return InstallationDiscovery{}, err
	}
	return InstallationDiscovery{
		AccountLogin:        strings.TrimSpace(detail.Account.Login),
		AccountType:         strings.TrimSpace(detail.Account.Type),
		RepositorySelection: strings.ToLower(strings.TrimSpace(detail.RepositorySelection)),
		Repositories:        repositories,
	}, nil
}

func (c *Client) ListInstallations(ctx context.Context) ([]AppInstallation, error) {
	installations := []AppInstallation{}
	for page := 1; ; page++ {
		responseBody, err := c.doAppJSON(ctx, http.MethodGet, fmt.Sprintf("/app/installations?per_page=100&page=%d", page), nil)
		if err != nil {
			return nil, err
		}

		var payload []struct {
			ID                  int64  `json:"id"`
			RepositorySelection string `json:"repository_selection"`
			HTMLURL             string `json:"html_url"`
			AppSlug             string `json:"app_slug"`
			Account             struct {
				Login string `json:"login"`
				Type  string `json:"type"`
			} `json:"account"`
		}
		if err := json.Unmarshal(responseBody, &payload); err != nil {
			return nil, err
		}

		for _, item := range payload {
			installations = append(installations, AppInstallation{
				ID:                  item.ID,
				AccountLogin:        strings.TrimSpace(item.Account.Login),
				AccountType:         strings.TrimSpace(item.Account.Type),
				RepositorySelection: strings.ToLower(strings.TrimSpace(item.RepositorySelection)),
				HTMLURL:             strings.TrimSpace(item.HTMLURL),
				AppSlug:             strings.TrimSpace(item.AppSlug),
			})
		}

		if len(payload) < 100 {
			break
		}
	}

	slices.SortFunc(installations, func(left, right AppInstallation) int {
		if cmp := strings.Compare(strings.ToLower(left.AccountLogin), strings.ToLower(right.AccountLogin)); cmp != 0 {
			return cmp
		}
		return strings.Compare(left.HTMLURL, right.HTMLURL)
	})
	return installations, nil
}

func (c *Client) FindInstallation(ctx context.Context, installationID int64) (*AppInstallation, error) {
	if installationID <= 0 {
		return nil, fmt.Errorf("github installation id is required")
	}
	installations, err := c.ListInstallations(ctx)
	if err != nil {
		return nil, err
	}
	for _, installation := range installations {
		if installation.ID == installationID {
			match := installation
			return &match, nil
		}
	}
	return nil, nil
}

func GenerateWebhookSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func decodeRepoRunners(body []byte) ([]RepositoryRunner, error) {
	var payload struct {
		Runners []RepositoryRunner `json:"runners"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload.Runners, nil
}

func normalizeRepoNames(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		items = append(items, normalized)
	}
	slices.Sort(items)
	return slices.Compact(items)
}

func ExchangeManifestCode(ctx context.Context, apiBaseURL, code string) (ManifestConversion, error) {
	normalizedCode := strings.TrimSpace(code)
	if normalizedCode == "" {
		return ManifestConversion{}, fmt.Errorf("github manifest code is required")
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(normalizeAPIBaseURL(apiBaseURL), "/")+"/app-manifests/"+url.PathEscape(normalizedCode)+"/conversions",
		bytes.NewReader([]byte("{}")),
	)
	if err != nil {
		return ManifestConversion{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return ManifestConversion{}, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return ManifestConversion{}, err
	}
	if response.StatusCode >= 300 {
		return ManifestConversion{}, fmt.Errorf("github manifest conversion failed: %s", strings.TrimSpace(string(body)))
	}

	var payload struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		Slug          string `json:"slug"`
		HTMLURL       string `json:"html_url"`
		PrivateKeyPEM string `json:"pem"`
		WebhookSecret string `json:"webhook_secret"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ManifestConversion{}, err
	}

	return ManifestConversion{
		AppID:         payload.ID,
		Name:          strings.TrimSpace(payload.Name),
		Slug:          strings.TrimSpace(payload.Slug),
		HTMLURL:       strings.TrimSpace(payload.HTMLURL),
		PrivateKeyPEM: strings.TrimSpace(payload.PrivateKeyPEM),
		WebhookSecret: strings.TrimSpace(payload.WebhookSecret),
	}, nil
}

func uniqueOwners(repositories []Repository) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		owner := strings.TrimSpace(repository.Owner)
		if owner == "" {
			continue
		}
		key := strings.ToLower(owner)
		if _, exists := set[key]; exists {
			continue
		}
		set[key] = struct{}{}
		out = append(out, owner)
	}
	slices.Sort(out)
	return out
}

func (c *Client) doRunnerJSON(ctx context.Context, installationID int64, method, path string, body any) ([]byte, error) {
	return c.doInstallationJSON(ctx, installationID, method, path, body)
}

func (c *Client) doInstallationJSON(ctx context.Context, installationID int64, method, path string, body any) ([]byte, error) {
	token, err := c.installationToken(ctx, c.effectiveInstallationID(installationID))
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.cfg.APIBaseURL, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("github api %s %s failed: %s", method, path, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

func (c *Client) doAppJSON(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := c.newAppRequest(ctx, method, path, nil)
	if err != nil {
		return nil, err
	}
	if reader != nil {
		request.Body = io.NopCloser(reader)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("github api %s %s failed: %s", method, path, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

func (c *Client) listInstallationRepositories(ctx context.Context) ([]Repository, error) {
	type installationRepositoriesResponse struct {
		Repositories []struct {
			FullName    string `json:"full_name"`
			Name        string `json:"name"`
			Private     bool   `json:"private"`
			Permissions struct {
				Admin bool `json:"admin"`
			} `json:"permissions"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repositories"`
	}
	repositories := []Repository{}
	for page := 1; ; page++ {
		path := fmt.Sprintf("/installation/repositories?per_page=100&page=%d", page)
		responseBody, err := c.doInstallationJSON(ctx, c.cfg.InstallationID, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var payload installationRepositoriesResponse
		if err := json.Unmarshal(responseBody, &payload); err != nil {
			return nil, err
		}
		for _, item := range payload.Repositories {
			repositories = append(repositories, Repository{
				FullName: strings.TrimSpace(item.FullName),
				Name:     strings.TrimSpace(item.Name),
				Private:  item.Private,
				Admin:    item.Permissions.Admin,
				Owner:    strings.TrimSpace(item.Owner.Login),
			})
		}
		if len(payload.Repositories) < 100 {
			break
		}
	}
	slices.SortFunc(repositories, func(left, right Repository) int {
		return strings.Compare(strings.ToLower(left.FullName), strings.ToLower(right.FullName))
	})
	return repositories, nil
}

func (c *Client) installationToken(ctx context.Context, installationID int64) (string, error) {
	installationID = c.effectiveInstallationID(installationID)
	if installationID == 0 {
		return "", fmt.Errorf("github installation id is required")
	}
	request, err := c.newAppRequest(ctx, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", installationID), []byte("{}"))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode >= 300 {
		return "", fmt.Errorf("github app installation token failed: %s", strings.TrimSpace(string(body)))
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	return payload.Token, nil
}

func (c *Client) effectiveInstallationID(requested int64) int64 {
	if c.cfg.InstallationID > 0 {
		return c.cfg.InstallationID
	}
	return requested
}

func (c *Client) newAppRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	signed, err := c.signedAppJWT()
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.cfg.APIBaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+signed)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return request, nil
}

func (c *Client) signedAppJWT() (string, error) {
	if c.privateKey == nil {
		return "", fmt.Errorf("github app private key is required")
	}
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": strconv.FormatInt(c.cfg.AppID, 10),
	})
	return token.SignedString(c.privateKey)
}

func parsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(raw)))
	if block == nil {
		return nil, fmt.Errorf("github app private key must be PEM encoded")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("github app private key must be RSA")
	}
	return key, nil
}
