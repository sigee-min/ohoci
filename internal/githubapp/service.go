package githubapp

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"ohoci/internal/store"
)

var ErrNotConfigured = errors.New("github configuration is not ready")

const envSyntheticConfigIDBase int64 = 1 << 62

type ServiceOptions struct {
	Defaults      Config
	EncryptionKey string
	PublicBaseURL string
}

type Input struct {
	APIBaseURL     string   `json:"apiBaseUrl"`
	AppID          int64    `json:"appId"`
	InstallationID int64    `json:"installationId"`
	Name           string   `json:"name"`
	Tags           []string `json:"tags"`
	PrivateKeyPEM  string   `json:"privateKeyPem"`
	WebhookSecret  string   `json:"webhookSecret"`
	SelectedRepos  []string `json:"selectedRepos"`
}

type View struct {
	ID                              int64      `json:"id,omitempty"`
	Name                            string     `json:"name,omitempty"`
	Tags                            []string   `json:"tags,omitempty"`
	APIBaseURL                      string     `json:"apiBaseUrl"`
	AuthMode                        string     `json:"authMode"`
	AppID                           int64      `json:"appId,omitempty"`
	InstallationID                  int64      `json:"installationId,omitempty"`
	AccountLogin                    string     `json:"accountLogin"`
	AccountType                     string     `json:"accountType"`
	SelectedRepos                   []string   `json:"selectedRepos"`
	InstallationState               string     `json:"installationState,omitempty"`
	InstallationRepositorySelection string     `json:"installationRepositorySelection,omitempty"`
	InstallationRepositories        []string   `json:"installationRepositories,omitempty"`
	InstallationReady               bool       `json:"installationReady,omitempty"`
	InstallationMissing             []string   `json:"installationMissing,omitempty"`
	InstallationError               string     `json:"installationError,omitempty"`
	IsActive                        bool       `json:"isActive,omitempty"`
	IsStaged                        bool       `json:"isStaged,omitempty"`
	LastTestedAt                    *time.Time `json:"lastTestedAt,omitempty"`
	CreatedAt                       time.Time  `json:"createdAt,omitempty"`
	UpdatedAt                       time.Time  `json:"updatedAt,omitempty"`
}

type Status struct {
	Source            string   `json:"source"`
	ActiveConfig      *View    `json:"activeConfig,omitempty"`
	ActiveConfigs     []View   `json:"activeConfigs,omitempty"`
	StagedConfig      *View    `json:"stagedConfig,omitempty"`
	EffectiveConfig   View     `json:"effectiveConfig"`
	Ready             bool     `json:"ready"`
	Missing           []string `json:"missing,omitempty"`
	Error             string   `json:"error,omitempty"`
	StagedReady       bool     `json:"stagedReady,omitempty"`
	StagedMissing     []string `json:"stagedMissing,omitempty"`
	StagedError       string   `json:"stagedError,omitempty"`
	HasAppCredentials bool     `json:"hasAppCredentials"`
	HasWebhookSecret  bool     `json:"hasWebhookSecret"`
	WebhookURL        string   `json:"webhookUrl"`
	AccountLogin      string   `json:"accountLogin,omitempty"`
	AccountType       string   `json:"accountType,omitempty"`
	SelectedRepos     []string `json:"selectedRepos,omitempty"`
}

type TestResult struct {
	Config       View         `json:"config"`
	Message      string       `json:"message"`
	AccountLogin string       `json:"accountLogin"`
	AccountType  string       `json:"accountType"`
	Owners       []string     `json:"owners"`
	Repositories []Repository `json:"repositories"`
}

type WebhookSource string

const (
	WebhookSourceUnknown WebhookSource = ""
	WebhookSourceActive  WebhookSource = "active"
	WebhookSourceStaged  WebhookSource = "staged"
)

type WebhookResolution struct {
	Source WebhookSource
	Client *Client
	Config *store.GitHubConfig
}

type resolvedStoredConfig struct {
	record  store.GitHubConfig
	cfg     Config
	view    View
	ready   bool
	missing []string
	errText string
}

type Service struct {
	store         *store.Store
	key           [32]byte
	defaults      Config
	publicBaseURL string
	now           func() time.Time
}

var generateWebhookSecret = GenerateWebhookSecret

func NewService(s *store.Store, options ServiceOptions) (*Service, error) {
	if s == nil {
		return nil, fmt.Errorf("store is required")
	}
	keyMaterial := strings.TrimSpace(options.EncryptionKey)
	if keyMaterial == "" {
		return nil, fmt.Errorf("data encryption key is required")
	}
	return &Service{
		store:         s,
		key:           sha256.Sum256([]byte(keyMaterial)),
		defaults:      normalizeConfig(options.Defaults),
		publicBaseURL: strings.TrimSpace(options.PublicBaseURL),
		now:           time.Now,
	}, nil
}

func (s *Service) CurrentStatus(ctx context.Context) (Status, error) {
	envConfig, err := s.envResolvedConfig(ctx)
	if err != nil {
		return Status{}, err
	}

	status := Status{
		Source:            "env",
		EffectiveConfig:   envConfig.view,
		Ready:             envConfig.ready,
		Missing:           append([]string(nil), envConfig.missing...),
		Error:             envConfig.errText,
		HasAppCredentials: hasAppCredentials(envConfig.cfg),
		HasWebhookSecret:  strings.TrimSpace(envConfig.cfg.WebhookSecret) != "",
		WebhookURL:        s.webhookURL(),
	}
	status.AccountLogin = status.EffectiveConfig.AccountLogin
	status.AccountType = status.EffectiveConfig.AccountType
	status.SelectedRepos = append([]string(nil), status.EffectiveConfig.SelectedRepos...)

	activeConfigs, err := s.listResolvedActiveConfigs(ctx)
	switch {
	case err != nil:
		return Status{}, err
	default:
		activeConfigs = filterRoutableResolvedConfigs(activeConfigs)
	}
	switch {
	case len(activeConfigs) == 0:
	default:
		status.Source = "cms"
		status.ActiveConfigs = make([]View, 0, len(activeConfigs))
		for _, item := range activeConfigs {
			status.ActiveConfigs = append(status.ActiveConfigs, item.view)
		}
		preferred := preferredResolvedConfig(activeConfigs)
		if preferred != nil {
			view := preferred.view
			status.ActiveConfig = &view
			status.EffectiveConfig = view
			status.HasAppCredentials = hasAppCredentials(preferred.cfg)
			status.HasWebhookSecret = strings.TrimSpace(preferred.cfg.WebhookSecret) != ""
			status.AccountLogin = view.AccountLogin
			status.AccountType = view.AccountType
			status.SelectedRepos = append([]string(nil), view.SelectedRepos...)
			status.Ready = preferred.ready
			status.Missing = append([]string(nil), preferred.missing...)
			status.Error = preferred.errText
		}
	}

	stagedRecord, err := s.store.FindStagedGitHubConfig(ctx)
	switch {
	case err == nil:
		view := viewFromStored(stagedRecord)
		cfg, _, resolveErr := s.configFromRecord(stagedRecord)
		status.StagedConfig = &view
		status.StagedReady, status.StagedMissing, status.StagedError = statusFromView(cfg, &view, resolveErr)
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return Status{}, err
	}

	return status, nil
}

func (s *Service) Test(ctx context.Context, input Input) (TestResult, error) {
	cfg, err := normalizeInputMode(input)
	if err != nil {
		return TestResult{}, err
	}
	client, err := New(cfg)
	if err != nil {
		return TestResult{}, err
	}
	discovery, err := client.DiscoverInstallation(ctx)
	if err != nil {
		return TestResult{}, err
	}
	cfg.AccountLogin = discovery.AccountLogin
	cfg.AccountType = discovery.AccountType
	cfg.InstallationState = "active"
	cfg.InstallationRepositorySelection = discovery.RepositorySelection
	cfg.InstallationRepositories = repositoryNames(discovery.Repositories)
	view := viewFromConfig(cfg, discovery.AccountLogin, discovery.AccountType)
	view.InstallationState = cfg.InstallationState
	view.InstallationRepositorySelection = cfg.InstallationRepositorySelection
	view.InstallationRepositories = append([]string(nil), cfg.InstallationRepositories...)
	view.InstallationReady, view.InstallationMissing, view.InstallationError = installationReadiness(view, "")
	return TestResult{
		Config:       view,
		Message:      testedAppMessage(cfg, discovery),
		AccountLogin: discovery.AccountLogin,
		AccountType:  discovery.AccountType,
		Owners:       uniqueOwners(discovery.Repositories),
		Repositories: append([]Repository(nil), discovery.Repositories...),
	}, nil
}

func (s *Service) Save(ctx context.Context, input Input) (TestResult, error) {
	return s.saveAppConfig(ctx, input, false)
}

func (s *Service) SaveStagedApp(ctx context.Context, input Input) (TestResult, error) {
	return s.saveAppConfig(ctx, input, true)
}

func (s *Service) Clear(ctx context.Context) error {
	return s.store.ClearActiveGitHubConfig(ctx)
}

func (s *Service) ClearStaged(ctx context.Context) error {
	return s.store.ClearStagedGitHubConfig(ctx)
}

func (s *Service) RecordInstallationStatus(ctx context.Context, record *store.GitHubConfig, state, accountLogin, accountType, repositorySelection string, repositories []string, lastTestError string) error {
	if record == nil {
		return nil
	}
	update := store.GitHubConfig{
		AccountLogin:                    accountLogin,
		AccountType:                     accountType,
		InstallationState:               state,
		InstallationRepositorySelection: repositorySelection,
		LastTestError:                   lastTestError,
	}
	if repositories == nil {
		update.InstallationRepositories = record.InstallationRepositories
	} else {
		update.InstallationRepositories = normalizeRepoNames(repositories)
	}
	now := time.Now().UTC()
	update.LastTestedAt = &now
	if s.isCurrentEnvConfigRecord(record) {
		return s.store.UpsertGitHubRouteInstallationStatus(ctx, store.GitHubRouteInstallationStatus{
			APIBaseURL:                      record.APIBaseURL,
			AppID:                           record.AppID,
			InstallationID:                  record.InstallationID,
			AccountLogin:                    update.AccountLogin,
			AccountType:                     update.AccountType,
			InstallationState:               update.InstallationState,
			InstallationRepositorySelection: update.InstallationRepositorySelection,
			InstallationRepositories:        update.InstallationRepositories,
			LastTestedAt:                    update.LastTestedAt,
			LastTestError:                   update.LastTestError,
		})
	}
	if record.ID <= 0 {
		return nil
	}
	return s.store.UpdateGitHubConfigInstallation(ctx, record.ID, update)
}

func (s *Service) RefreshInstallationSnapshot(ctx context.Context, record *store.GitHubConfig, client *Client, fallbackAccountLogin, fallbackAccountType, fallbackSelection string) error {
	if client == nil {
		return s.RecordInstallationStatus(ctx, record, "active", fallbackAccountLogin, fallbackAccountType, fallbackSelection, nil, "")
	}
	discovery, err := client.DiscoverInstallation(ctx)
	if err != nil {
		return s.RecordInstallationStatus(ctx, record, "active", fallbackAccountLogin, fallbackAccountType, fallbackSelection, nil, err.Error())
	}
	return s.RecordInstallationStatus(ctx, record, "active", discovery.AccountLogin, discovery.AccountType, discovery.RepositorySelection, repositoryNames(discovery.Repositories), "")
}

func (s *Service) PromoteStagedApp(ctx context.Context) error {
	stagedRecord, err := s.store.FindStagedGitHubConfig(ctx)
	switch {
	case err == nil:
	case errors.Is(err, store.ErrNotFound):
		return ErrNotConfigured
	default:
		return err
	}

	stagedCfg, stagedView, err := s.configFromRecord(stagedRecord)
	if err != nil {
		return err
	}
	if ready, missing, readyErr := statusFromView(stagedCfg, &stagedView, nil); !ready {
		if readyErr != "" {
			return fmt.Errorf("staged github app config is not ready: %s", readyErr)
		}
		if len(missing) > 0 {
			return fmt.Errorf("staged github app config is not ready: missing %s", strings.Join(missing, ", "))
		}
		return fmt.Errorf("staged github app config is not ready")
	}
	otherSecrets, err := s.otherWebhookSecrets(ctx, true)
	if err != nil {
		return err
	}
	if _, err := s.resolveWebhookSecret(stagedCfg.WebhookSecret, otherSecrets, true); err != nil {
		return fmt.Errorf("staged github app config cannot be promoted: %w", err)
	}

	_, err = s.store.PromoteStagedGitHubConfig(ctx)
	return err
}

func (s *Service) ResolveClient(ctx context.Context) (*Client, error) {
	cfg, err := s.activeRuntimeConfig(ctx)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

func (s *Service) ResolveStagedClient(ctx context.Context) (*Client, error) {
	cfg, err := s.stagedRuntimeConfig(ctx)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

func (s *Service) ResolveClientByInstallationID(ctx context.Context, installationID int64) (*Client, error) {
	cfg, err := s.activeRuntimeConfigForInstallationID(ctx, installationID)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

func (s *Service) ResolveClientByConfigID(ctx context.Context, configID int64) (*Client, error) {
	cfg, _, err := s.runtimeConfigByConfigID(ctx, configID)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

func (s *Service) ResolveRunnerClient(ctx context.Context, configID, installationID int64) (*Client, error) {
	if configID > 0 {
		cfg, allowInstallationFallback, err := s.runtimeConfigByConfigID(ctx, configID)
		switch {
		case err == nil:
			return New(cfg)
		case !errors.Is(err, ErrNotConfigured):
			return nil, err
		case !allowInstallationFallback:
			return nil, ErrNotConfigured
		}
	}
	if installationID <= 0 {
		return nil, ErrNotConfigured
	}
	cfg, err := s.activeRuntimeConfigForInstallationID(ctx, installationID)
	if err != nil {
		return nil, err
	}
	return New(cfg)
}

func (s *Service) ResolveClientForRepository(ctx context.Context, owner, repo string) (*Client, *store.GitHubConfig, error) {
	activeConfigs, err := s.listResolvedActiveConfigs(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, item := range activeConfigs {
		if !item.ready {
			continue
		}
		client, err := New(item.cfg)
		if err != nil {
			return nil, nil, err
		}
		if !client.RepositoryAllowed(owner, repo) {
			continue
		}
		record := item.record
		return client, &record, nil
	}
	envConfig, err := s.envResolvedConfig(ctx)
	if err != nil {
		return nil, nil, err
	}
	if envConfig.ready {
		client, err := New(envConfig.cfg)
		if err != nil {
			return nil, nil, err
		}
		if !client.RepositoryAllowed(owner, repo) {
			return nil, nil, ErrNotConfigured
		}
		record := envConfig.record
		return client, &record, nil
	}
	return nil, nil, ErrNotConfigured
}

func (s *Service) ResolveWebhookSource(ctx context.Context, eventType string, body []byte, signature string) (WebhookResolution, error) {
	payloadInstallationID, payloadHasInstallationID := webhookInstallationID(body)
	activeConfigs, err := s.listResolvedActiveConfigs(ctx)
	if err != nil {
		return WebhookResolution{}, err
	}
	activeConfigs = activeConfigsForWebhookEvent(activeConfigs, eventType)
	for _, item := range activeConfigs {
		if webhookMatchesConfig(body, signature, payloadInstallationID, payloadHasInstallationID, item.cfg) {
			client, err := New(item.cfg)
			if err != nil {
				return WebhookResolution{}, err
			}
			record := item.record
			return WebhookResolution{Source: WebhookSourceActive, Client: client, Config: &record}, nil
		}
	}

	stagedAvailable := false
	stagedRecord, stagedCfg, err := s.stagedRuntimeConfigRecord(ctx)
	switch {
	case err == nil:
		stagedAvailable = true
	case errors.Is(err, ErrNotConfigured):
		stagedAvailable = false
	default:
		return WebhookResolution{}, err
	}
	if stagedAvailable && webhookMatchesConfig(body, signature, payloadInstallationID, payloadHasInstallationID, stagedCfg) {
		client, err := New(stagedCfg)
		if err != nil {
			return WebhookResolution{}, err
		}
		record := stagedRecord
		return WebhookResolution{Source: WebhookSourceStaged, Client: client, Config: &record}, nil
	}
	envConfig, err := s.envResolvedConfig(ctx)
	if err != nil {
		return WebhookResolution{}, err
	}
	envEligible := envConfig.ready
	if strings.ToLower(strings.TrimSpace(eventType)) != "workflow_job" {
		envEligible = webhookConfigAvailable(envConfig.cfg)
	}
	if envEligible && webhookMatchesConfig(body, signature, payloadInstallationID, payloadHasInstallationID, envConfig.cfg) {
		cfg := envConfig.cfg
		client, err := New(cfg)
		if err != nil {
			return WebhookResolution{}, err
		}
		record := envConfig.record
		return WebhookResolution{Source: WebhookSourceActive, Client: client, Config: &record}, nil
	}
	if len(activeConfigs) == 0 && !stagedAvailable && !webhookConfigAvailable(envConfig.cfg) {
		return WebhookResolution{}, ErrNotConfigured
	}
	return WebhookResolution{}, nil
}

func webhookInstallationID(body []byte) (int64, bool) {
	var payload struct {
		Installation struct {
			ID int64 `json:"id"`
		} `json:"installation"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, false
	}
	if payload.Installation.ID <= 0 {
		return 0, false
	}
	return payload.Installation.ID, true
}

func webhookMatchesConfig(body []byte, signature string, payloadInstallationID int64, payloadHasInstallationID bool, cfg Config) bool {
	if !webhookSignatureMatches(body, signature, cfg.WebhookSecret) {
		return false
	}
	if !payloadHasInstallationID {
		return true
	}
	if cfg.InstallationID <= 0 {
		return false
	}
	return cfg.InstallationID == payloadInstallationID
}

func (s *Service) activeRuntimeConfig(ctx context.Context) (Config, error) {
	activeConfigs, err := s.listResolvedActiveConfigs(ctx)
	if err != nil {
		return Config{}, err
	}
	if preferred := preferredResolvedConfig(activeConfigs); preferred != nil {
		if !preferred.ready {
			if preferred.errText != "" {
				return Config{}, errors.New(preferred.errText)
			}
			return Config{}, ErrNotConfigured
		}
		return preferred.cfg, nil
	}
	envConfig, err := s.envResolvedConfig(ctx)
	if err != nil {
		return Config{}, err
	}
	if !envConfig.ready {
		if envConfig.errText != "" {
			return Config{}, errors.New(envConfig.errText)
		}
		return Config{}, ErrNotConfigured
	}
	return envConfig.cfg, nil
}

func (s *Service) stagedRuntimeConfig(ctx context.Context) (Config, error) {
	_, cfg, err := s.stagedRuntimeConfigRecord(ctx)
	return cfg, err
}

func (s *Service) stagedRuntimeConfigRecord(ctx context.Context) (store.GitHubConfig, Config, error) {
	record, err := s.store.FindStagedGitHubConfig(ctx)
	switch {
	case err == nil:
		cfg, _, resolveErr := s.configFromRecord(record)
		if resolveErr != nil {
			return store.GitHubConfig{}, Config{}, resolveErr
		}
		return record, cfg, nil
	case errors.Is(err, store.ErrNotFound):
		return store.GitHubConfig{}, Config{}, ErrNotConfigured
	default:
		return store.GitHubConfig{}, Config{}, err
	}
}

func (s *Service) activeRuntimeConfigForInstallationID(ctx context.Context, installationID int64) (Config, error) {
	if installationID > 0 {
		record, err := s.store.FindActiveGitHubConfigByInstallationID(ctx, installationID)
		switch {
		case err == nil:
			cfg, _, resolveErr := s.configFromRecord(record)
			if resolveErr != nil {
				return Config{}, resolveErr
			}
			return cfg, nil
		case errors.Is(err, store.ErrNotFound):
		default:
			return Config{}, err
		}
	}
	activeConfigs, err := s.store.ListActiveGitHubConfigs(ctx)
	if err != nil {
		return Config{}, err
	}
	cfg := normalizeConfig(s.defaults)
	envConfig, err := s.envResolvedConfig(ctx)
	if err != nil {
		return Config{}, err
	}
	cfg = envConfig.cfg
	if !envConfig.ready {
		if len(activeConfigs) > 0 {
			return Config{}, ErrNotConfigured
		}
		return Config{}, ErrNotConfigured
	}
	if installationID > 0 && cfg.InstallationID > 0 && cfg.InstallationID != installationID {
		if len(activeConfigs) > 0 {
			return Config{}, ErrNotConfigured
		}
		return Config{}, ErrNotConfigured
	}
	return cfg, nil
}

func (s *Service) activeRuntimeConfigByConfigID(ctx context.Context, configID int64) (Config, error) {
	cfg, _, err := s.runtimeConfigByConfigID(ctx, configID)
	return cfg, err
}

func (s *Service) runtimeConfigByConfigID(ctx context.Context, configID int64) (Config, bool, error) {
	if configID <= 0 {
		return Config{}, true, ErrNotConfigured
	}
	envConfig, err := s.envResolvedConfig(ctx)
	if err != nil {
		return Config{}, false, err
	}
	if syntheticID := syntheticEnvConfigID(envConfig.cfg); syntheticID > 0 && configID == syntheticID {
		if !envConfig.ready {
			if envConfig.errText != "" {
				return Config{}, false, errors.New(envConfig.errText)
			}
			return Config{}, false, ErrNotConfigured
		}
		return envConfig.cfg, false, nil
	}
	if configID >= envSyntheticConfigIDBase {
		return Config{}, false, ErrNotConfigured
	}
	record, err := s.store.FindGitHubConfigByID(ctx, configID)
	switch {
	case err == nil:
		if !record.IsActive {
			if activeRecord, routeErr := s.store.FindActiveGitHubConfigByRoute(ctx, record.APIBaseURL, record.AppID, record.InstallationID); routeErr == nil {
				record = activeRecord
			} else if errors.Is(routeErr, store.ErrNotFound) {
				if record.AppID > 0 && record.InstallationID > 0 {
					return Config{}, false, ErrNotConfigured
				}
				if record.InstallationID > 0 {
					return Config{}, true, ErrNotConfigured
				}
				return Config{}, false, ErrNotConfigured
			} else {
				return Config{}, false, routeErr
			}
		}
		cfg, _, resolveErr := s.configFromRecord(record)
		if resolveErr != nil {
			return Config{}, false, resolveErr
		}
		return cfg, false, nil
	case errors.Is(err, store.ErrNotFound):
		return Config{}, true, ErrNotConfigured
	default:
		return Config{}, false, err
	}
}

func (s *Service) saveAppConfig(ctx context.Context, input Input, staged bool) (TestResult, error) {
	cfg, err := normalizeInputMode(input)
	if err != nil {
		return TestResult{}, err
	}
	otherSecrets, err := s.otherWebhookSecrets(ctx, staged)
	if err != nil {
		return TestResult{}, err
	}
	cfg.WebhookSecret, err = s.resolveWebhookSecret(strings.TrimSpace(cfg.WebhookSecret), otherSecrets, staged)
	if err != nil {
		return TestResult{}, err
	}

	client, err := New(cfg)
	if err != nil {
		return TestResult{}, err
	}
	if err := client.TestConnection(ctx); err != nil {
		return TestResult{}, err
	}

	discovery, discoverErr := client.DiscoverInstallation(ctx)
	now := time.Now().UTC()
	accountLogin := ""
	accountType := ""
	installationState := ""
	installationRepositorySelection := ""
	installationRepositories := []string{}
	lastTestError := ""
	if discoverErr == nil {
		accountLogin = discovery.AccountLogin
		accountType = discovery.AccountType
		installationState = "active"
		installationRepositorySelection = discovery.RepositorySelection
		installationRepositories = repositoryNames(discovery.Repositories)
		cfg.AccountLogin = accountLogin
		cfg.AccountType = accountType
		cfg.InstallationState = installationState
		cfg.InstallationRepositorySelection = installationRepositorySelection
		cfg.InstallationRepositories = installationRepositories
	} else {
		lastTestError = discoverErr.Error()
	}

	view := viewFromConfig(cfg, accountLogin, accountType)
	view.IsActive = !staged
	view.IsStaged = staged
	view.InstallationState = installationState
	view.InstallationRepositorySelection = installationRepositorySelection
	view.InstallationRepositories = append([]string(nil), installationRepositories...)
	view.InstallationReady, view.InstallationMissing, view.InstallationError = installationReadiness(view, lastTestError)

	if !staged {
		if discoverErr != nil {
			return TestResult{}, discoverErr
		}
		if ready, missing, readyErr := statusFromView(cfg, &view, nil); !ready {
			if readyErr != "" {
				return TestResult{}, fmt.Errorf("active github app config is not ready: %s", readyErr)
			}
			if len(missing) > 0 {
				return TestResult{}, fmt.Errorf("active github app config is not ready: missing %s", strings.Join(missing, ", "))
			}
			return TestResult{}, fmt.Errorf("active github app config is not ready")
		}
	}

	privateKeyCiphertext, err := s.encrypt(cfg.PrivateKeyPEM)
	if err != nil {
		return TestResult{}, err
	}
	webhookSecretCiphertext, err := s.encrypt(cfg.WebhookSecret)
	if err != nil {
		return TestResult{}, err
	}
	record := store.GitHubConfig{
		Name:                            cfg.Name,
		Tags:                            cfg.Tags,
		APIBaseURL:                      cfg.APIBaseURL,
		AuthMode:                        store.GitHubAuthModeApp,
		AppID:                           cfg.AppID,
		InstallationID:                  cfg.InstallationID,
		PrivateKeyCiphertext:            privateKeyCiphertext,
		WebhookSecretCiphertext:         webhookSecretCiphertext,
		AllowedOrg:                      deriveAllowedOrg(cfg.SelectedRepos),
		SelectedRepos:                   cfg.SelectedRepos,
		AccountLogin:                    accountLogin,
		AccountType:                     accountType,
		InstallationState:               installationState,
		InstallationRepositorySelection: installationRepositorySelection,
		InstallationRepositories:        installationRepositories,
		LastTestedAt:                    &now,
		LastTestError:                   lastTestError,
		IsActive:                        !staged,
		IsStaged:                        staged,
	}

	var savedRecord store.GitHubConfig
	if staged {
		savedRecord, err = s.store.SaveStagedGitHubConfig(ctx, record)
		if err != nil {
			return TestResult{}, err
		}
	} else {
		savedRecord, err = s.store.SaveActiveGitHubConfig(ctx, record)
		if err != nil {
			return TestResult{}, err
		}
	}
	view = viewFromStored(savedRecord)
	view.InstallationReady, view.InstallationMissing, view.InstallationError = installationReadiness(view, lastTestError)

	message := savedAppMessage(cfg, discovery, discoverErr, staged)
	return TestResult{
		Config:       view,
		Message:      message,
		AccountLogin: accountLogin,
		AccountType:  accountType,
		Owners:       uniqueOwners(discovery.Repositories),
		Repositories: append([]Repository(nil), discovery.Repositories...),
	}, nil
}

func (s *Service) otherWebhookSecrets(ctx context.Context, staged bool) ([]string, error) {
	secrets := []string{}
	appendSecret := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		for _, existing := range secrets {
			if existing == trimmed {
				return
			}
		}
		secrets = append(secrets, trimmed)
	}

	activeConfigs, err := s.store.ListActiveGitHubConfigs(ctx)
	if err != nil {
		return nil, err
	}
	for _, record := range activeConfigs {
		secret, err := s.decrypt(record.WebhookSecretCiphertext)
		if err != nil {
			return nil, err
		}
		appendSecret(secret)
	}
	if staged {
		cfg := normalizeConfig(s.defaults)
		if ready, _, _ := readiness(cfg, nil); ready {
			appendSecret(cfg.WebhookSecret)
		}
		return secrets, nil
	}
	cfg := normalizeConfig(s.defaults)
	if ready, _, _ := readiness(cfg, nil); ready {
		appendSecret(cfg.WebhookSecret)
	}

	record, err := s.store.FindStagedGitHubConfig(ctx)
	switch {
	case err == nil:
		secret, err := s.decrypt(record.WebhookSecretCiphertext)
		if err != nil {
			return nil, err
		}
		appendSecret(secret)
	case errors.Is(err, store.ErrNotFound):
	default:
		return nil, err
	}
	return secrets, nil
}

func (s *Service) resolveWebhookSecret(current string, others []string, staged bool) (string, error) {
	conflicts := func(candidate string) bool {
		for _, other := range others {
			if candidate == strings.TrimSpace(other) {
				return true
			}
		}
		return false
	}
	if current != "" {
		if conflicts(current) {
			if staged {
				return "", fmt.Errorf("staged webhook secret must differ from live webhook secrets")
			}
			return "", fmt.Errorf("active webhook secret must differ from existing staged or live webhook secrets")
		}
		return current, nil
	}
	for {
		secret, err := generateWebhookSecret()
		if err != nil {
			return "", err
		}
		if !conflicts(secret) {
			return secret, nil
		}
	}
}

func (s *Service) configFromRecord(record store.GitHubConfig) (Config, View, error) {
	privateKey, err := s.decrypt(record.PrivateKeyCiphertext)
	if err != nil {
		return Config{}, View{}, err
	}
	webhookSecret, err := s.decrypt(record.WebhookSecretCiphertext)
	if err != nil {
		return Config{}, View{}, err
	}
	cfg := normalizeConfig(Config{
		Name:                            record.Name,
		Tags:                            record.Tags,
		APIBaseURL:                      record.APIBaseURL,
		AuthMode:                        normalizeAuthMode(record.AuthMode),
		AppID:                           record.AppID,
		InstallationID:                  record.InstallationID,
		PrivateKeyPEM:                   privateKey,
		WebhookSecret:                   webhookSecret,
		AllowedOrg:                      record.AllowedOrg,
		SelectedRepos:                   record.SelectedRepos,
		AccountLogin:                    record.AccountLogin,
		AccountType:                     record.AccountType,
		InstallationState:               record.InstallationState,
		InstallationRepositorySelection: record.InstallationRepositorySelection,
		InstallationRepositories:        record.InstallationRepositories,
	})
	return cfg, viewFromStored(record), nil
}

func normalizeInputMode(input Input) (Config, error) {
	cfg := normalizeConfig(Config{
		Name:           input.Name,
		Tags:           input.Tags,
		APIBaseURL:     input.APIBaseURL,
		AuthMode:       store.GitHubAuthModeApp,
		AppID:          input.AppID,
		InstallationID: input.InstallationID,
		PrivateKeyPEM:  input.PrivateKeyPEM,
		WebhookSecret:  input.WebhookSecret,
		SelectedRepos:  input.SelectedRepos,
	})
	if cfg.AppID <= 0 {
		return Config{}, fmt.Errorf("github app id is required")
	}
	if cfg.InstallationID <= 0 {
		return Config{}, fmt.Errorf("github installation id is required")
	}
	if strings.TrimSpace(cfg.PrivateKeyPEM) == "" {
		return Config{}, fmt.Errorf("github app private key is required")
	}
	return cfg, nil
}

func normalizeConfig(cfg Config) Config {
	normalized := Config{
		Name:                            strings.TrimSpace(cfg.Name),
		Tags:                            normalizeRepoNames(cfg.Tags),
		APIBaseURL:                      normalizeAPIBaseURL(cfg.APIBaseURL),
		AuthMode:                        normalizeAuthMode(cfg.AuthMode),
		AppID:                           cfg.AppID,
		InstallationID:                  cfg.InstallationID,
		PrivateKeyPEM:                   strings.TrimSpace(cfg.PrivateKeyPEM),
		WebhookSecret:                   strings.TrimSpace(cfg.WebhookSecret),
		AllowedOrg:                      strings.TrimSpace(cfg.AllowedOrg),
		SelectedRepos:                   normalizeRepoNames(cfg.SelectedRepos),
		AccountLogin:                    strings.TrimSpace(cfg.AccountLogin),
		AccountType:                     strings.TrimSpace(cfg.AccountType),
		InstallationState:               strings.ToLower(strings.TrimSpace(cfg.InstallationState)),
		InstallationRepositorySelection: strings.ToLower(strings.TrimSpace(cfg.InstallationRepositorySelection)),
		InstallationRepositories:        normalizeRepoNames(cfg.InstallationRepositories),
	}
	return normalized
}

func (s *Service) listResolvedActiveConfigs(ctx context.Context) ([]resolvedStoredConfig, error) {
	records, err := s.store.ListActiveGitHubConfigs(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]resolvedStoredConfig, 0, len(records))
	for _, record := range records {
		view := viewFromStored(record)
		cfg, _, resolveErr := s.configFromRecord(record)
		ready, missing, errText := statusFromView(cfg, &view, resolveErr)
		items = append(items, resolvedStoredConfig{
			record:  record,
			cfg:     cfg,
			view:    view,
			ready:   ready,
			missing: append([]string(nil), missing...),
			errText: errText,
		})
	}
	return items, nil
}

func (s *Service) envResolvedConfig(ctx context.Context) (resolvedStoredConfig, error) {
	cfg := normalizeConfig(s.defaults)
	record := s.syntheticEnvConfigRecord(cfg)

	if cfg.AppID > 0 && cfg.InstallationID > 0 {
		status, err := s.store.FindGitHubRouteInstallationStatus(ctx, cfg.APIBaseURL, cfg.AppID, cfg.InstallationID)
		switch {
		case err == nil:
			record.AccountLogin = status.AccountLogin
			record.AccountType = status.AccountType
			record.InstallationState = status.InstallationState
			record.InstallationRepositorySelection = status.InstallationRepositorySelection
			record.InstallationRepositories = append([]string(nil), status.InstallationRepositories...)
			record.LastTestedAt = status.LastTestedAt
			record.LastTestError = status.LastTestError
		case errors.Is(err, store.ErrNotFound):
		default:
			return resolvedStoredConfig{}, err
		}
	}

	cfg.AccountLogin = record.AccountLogin
	cfg.AccountType = record.AccountType
	cfg.InstallationState = record.InstallationState
	cfg.InstallationRepositorySelection = record.InstallationRepositorySelection
	cfg.InstallationRepositories = append([]string(nil), record.InstallationRepositories...)

	view := viewFromStored(record)
	ready, missing, errText := statusFromView(cfg, &view, nil)
	if errText == "" {
		errText = strings.TrimSpace(record.LastTestError)
	}
	return resolvedStoredConfig{
		record:  record,
		cfg:     cfg,
		view:    view,
		ready:   ready,
		missing: append([]string(nil), missing...),
		errText: errText,
	}, nil
}

func preferredResolvedConfig(items []resolvedStoredConfig) *resolvedStoredConfig {
	for i := range items {
		if items[i].ready {
			return &items[i]
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &items[0]
}

func activeConfigsForWebhookEvent(items []resolvedStoredConfig, eventType string) []resolvedStoredConfig {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "workflow_job":
		return filterRoutableResolvedConfigs(items)
	default:
		return items
	}
}

func filterRoutableResolvedConfigs(items []resolvedStoredConfig) []resolvedStoredConfig {
	routable := make([]resolvedStoredConfig, 0, len(items))
	for _, item := range items {
		if item.ready {
			routable = append(routable, item)
		}
	}
	return routable
}

func webhookConfigAvailable(cfg Config) bool {
	return cfg.InstallationID > 0 && hasAppCredentials(cfg) && strings.TrimSpace(cfg.WebhookSecret) != ""
}

func syntheticEnvConfigID(cfg Config) int64 {
	normalized := normalizeConfig(cfg)
	if normalized.APIBaseURL == "" || normalized.AppID <= 0 || normalized.InstallationID <= 0 {
		return 0
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", normalized.APIBaseURL, normalized.AppID, normalized.InstallationID)))
	suffix := binary.BigEndian.Uint64(sum[:8]) & uint64(envSyntheticConfigIDBase-1)
	if suffix == 0 {
		suffix = 1
	}
	return envSyntheticConfigIDBase | int64(suffix)
}

func (s *Service) syntheticEnvConfigRecord(cfg Config) store.GitHubConfig {
	normalized := normalizeConfig(cfg)
	return store.GitHubConfig{
		ID:                              syntheticEnvConfigID(normalized),
		Name:                            normalized.Name,
		Tags:                            normalized.Tags,
		APIBaseURL:                      normalized.APIBaseURL,
		AuthMode:                        store.GitHubAuthModeApp,
		AppID:                           normalized.AppID,
		InstallationID:                  normalized.InstallationID,
		AllowedOrg:                      normalized.AllowedOrg,
		SelectedRepos:                   normalized.SelectedRepos,
		AccountLogin:                    normalized.AccountLogin,
		AccountType:                     normalized.AccountType,
		InstallationState:               normalized.InstallationState,
		InstallationRepositorySelection: normalized.InstallationRepositorySelection,
		InstallationRepositories:        normalized.InstallationRepositories,
		IsActive:                        true,
	}
}

func (s *Service) isCurrentEnvConfigRecord(record *store.GitHubConfig) bool {
	if record == nil {
		return false
	}
	envCfg := normalizeConfig(s.defaults)
	if syntheticID := syntheticEnvConfigID(envCfg); syntheticID <= 0 || record.ID != syntheticID {
		return false
	}
	return normalizeAPIBaseURL(record.APIBaseURL) == envCfg.APIBaseURL &&
		record.AppID == envCfg.AppID &&
		record.InstallationID == envCfg.InstallationID
}

func missingFields(cfg Config) []string {
	missing := []string{}
	if cfg.AppID <= 0 {
		missing = append(missing, "appId")
	}
	if cfg.InstallationID <= 0 {
		missing = append(missing, "installationId")
	}
	if strings.TrimSpace(cfg.PrivateKeyPEM) == "" {
		missing = append(missing, "privateKeyPem")
	}
	if strings.TrimSpace(cfg.WebhookSecret) == "" {
		missing = append(missing, "webhookSecret")
	}
	if len(cfg.SelectedRepos) == 0 {
		missing = append(missing, "selectedRepos")
	}
	return missing
}

func readiness(cfg Config, resolveErr error) (bool, []string, string) {
	if resolveErr != nil {
		return false, nil, resolveErr.Error()
	}
	if missing := missingFields(cfg); len(missing) > 0 {
		return false, missing, ""
	}
	if _, err := New(cfg); err != nil {
		return false, nil, err.Error()
	}
	return true, nil, ""
}

func normalizeAuthMode(string) string {
	return store.GitHubAuthModeApp
}

func viewFromConfig(cfg Config, accountLogin, accountType string) View {
	return View{
		Name:                            strings.TrimSpace(cfg.Name),
		Tags:                            normalizeRepoNames(cfg.Tags),
		APIBaseURL:                      cfg.APIBaseURL,
		AuthMode:                        store.GitHubAuthModeApp,
		AppID:                           cfg.AppID,
		InstallationID:                  cfg.InstallationID,
		AccountLogin:                    strings.TrimSpace(accountLogin),
		AccountType:                     strings.TrimSpace(accountType),
		SelectedRepos:                   normalizeRepoNames(cfg.SelectedRepos),
		InstallationState:               strings.ToLower(strings.TrimSpace(cfg.InstallationState)),
		InstallationRepositorySelection: strings.ToLower(strings.TrimSpace(cfg.InstallationRepositorySelection)),
		InstallationRepositories:        normalizeRepoNames(cfg.InstallationRepositories),
	}
}

func viewFromStored(record store.GitHubConfig) View {
	view := viewFromConfig(Config{
		Name:                            record.Name,
		Tags:                            record.Tags,
		APIBaseURL:                      record.APIBaseURL,
		AuthMode:                        record.AuthMode,
		AppID:                           record.AppID,
		InstallationID:                  record.InstallationID,
		AllowedOrg:                      record.AllowedOrg,
		SelectedRepos:                   record.SelectedRepos,
		AccountLogin:                    record.AccountLogin,
		AccountType:                     record.AccountType,
		InstallationState:               record.InstallationState,
		InstallationRepositorySelection: record.InstallationRepositorySelection,
		InstallationRepositories:        record.InstallationRepositories,
	}, record.AccountLogin, record.AccountType)
	view.ID = record.ID
	view.IsActive = record.IsActive
	view.IsStaged = record.IsStaged
	view.LastTestedAt = record.LastTestedAt
	view.CreatedAt = record.CreatedAt
	view.UpdatedAt = record.UpdatedAt
	return view
}

func statusFromView(cfg Config, view *View, resolveErr error) (bool, []string, string) {
	ready, missing, errText := readiness(cfg, resolveErr)
	if view == nil {
		return ready, missing, errText
	}
	installationReady, installationMissing, installationError := installationReadiness(*view, view.InstallationError)
	view.InstallationReady = installationReady
	view.InstallationMissing = append([]string(nil), installationMissing...)
	view.InstallationError = installationError
	if !installationReady {
		ready = false
		missing = append(missing, installationMissing...)
		if errText == "" {
			errText = installationError
		}
	}
	view.InstallationRepositories = normalizeRepoNames(view.InstallationRepositories)
	view.InstallationRepositorySelection = strings.ToLower(strings.TrimSpace(view.InstallationRepositorySelection))
	return ready, slicesCompactStrings(missing), errText
}

func installationReadiness(view View, fallbackError string) (bool, []string, string) {
	switch strings.ToLower(strings.TrimSpace(view.InstallationState)) {
	case "active":
	case "suspended":
		return false, []string{"installationState"}, "github app installation is suspended"
	case "deleted":
		return false, []string{"installationState"}, "github app installation was removed"
	case "":
		return false, []string{"installationState"}, strings.TrimSpace(fallbackError)
	default:
		return false, []string{"installationState"}, fmt.Sprintf("unsupported installation state %q", view.InstallationState)
	}

	missing := []string{}
	if strings.TrimSpace(view.InstallationRepositorySelection) == "" {
		missing = append(missing, "installationRepositorySelection")
	}
	if len(view.InstallationRepositories) == 0 {
		missing = append(missing, "installationRepositories")
	}
	if len(view.SelectedRepos) == 0 {
		missing = append(missing, "selectedRepos")
	}
	if len(missing) > 0 {
		return false, missing, strings.TrimSpace(fallbackError)
	}

	visible := map[string]struct{}{}
	for _, repoName := range view.InstallationRepositories {
		visible[strings.ToLower(strings.TrimSpace(repoName))] = struct{}{}
	}
	selectedMissing := []string{}
	for _, repoName := range view.SelectedRepos {
		if _, ok := visible[strings.ToLower(strings.TrimSpace(repoName))]; !ok {
			selectedMissing = append(selectedMissing, repoName)
		}
	}
	if len(selectedMissing) > 0 {
		return false, []string{"selectedRepos"}, fmt.Sprintf("selected repositories are not installed: %s", strings.Join(selectedMissing, ", "))
	}
	return true, nil, ""
}

func testedAppMessage(cfg Config, discovery InstallationDiscovery) string {
	visible := len(discovery.Repositories)
	if visible == 0 {
		return fmt.Sprintf("GitHub App %d credentials validated, but the installation has no visible repositories", cfg.AppID)
	}
	if len(cfg.SelectedRepos) == 0 {
		return fmt.Sprintf("GitHub App %d can see %d repositories", cfg.AppID, visible)
	}
	return fmt.Sprintf("GitHub App %d can see %d repositories and %d are selected locally", cfg.AppID, visible, len(cfg.SelectedRepos))
}

func savedAppMessage(cfg Config, discovery InstallationDiscovery, err error, staged bool) string {
	if staged {
		if err == nil {
			if len(discovery.Repositories) == 0 {
				return fmt.Sprintf("GitHub App %d staged, but no installation-visible repositories were discovered", cfg.AppID)
			}
			return fmt.Sprintf("GitHub App %d staged with %d visible repositories", cfg.AppID, len(discovery.Repositories))
		}
		return fmt.Sprintf("GitHub App %d staged with pending installation discovery: %s", cfg.AppID, strings.TrimSpace(err.Error()))
	}
	return fmt.Sprintf("GitHub App %d saved for %d selected repositories", cfg.AppID, len(cfg.SelectedRepos))
}

func repositoryNames(repositories []Repository) []string {
	names := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		if fullName := strings.TrimSpace(repository.FullName); fullName != "" {
			names = append(names, fullName)
		}
	}
	return normalizeRepoNames(names)
}

func slicesCompactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func deriveAllowedOrg(selectedRepos []string) string {
	owners := []string{}
	seen := map[string]struct{}{}
	for _, repoName := range selectedRepos {
		owner, _, err := splitRepoName(repoName)
		if err != nil {
			continue
		}
		key := strings.ToLower(owner)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		owners = append(owners, owner)
	}
	if len(owners) != 1 {
		return ""
	}
	return owners[0]
}

func splitRepoName(value string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid repository name %q", value)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func hasAppCredentials(cfg Config) bool {
	return cfg.AppID > 0 && cfg.InstallationID > 0 && strings.TrimSpace(cfg.PrivateKeyPEM) != ""
}

func (s *Service) webhookURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(s.publicBaseURL), "/")
	if baseURL == "" {
		return ""
	}
	return baseURL + "/api/v1/github/webhook"
}

func (s *Service) encrypt(plain string) (string, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	payload := append(nonce, gcm.Seal(nil, nonce, []byte(plain), nil)...)
	return "v1:" + base64.StdEncoding.EncodeToString(payload), nil
}

func (s *Service) decrypt(ciphertext string) (string, error) {
	if strings.TrimSpace(ciphertext) == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, "v1:") {
		return "", fmt.Errorf("unsupported ciphertext format")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, "v1:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext payload is too short")
	}
	nonce := raw[:gcm.NonceSize()]
	payload := raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func webhookSignatureMatches(body []byte, signature, secret string) bool {
	signature = strings.TrimSpace(strings.TrimPrefix(signature, "sha256="))
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(secret)))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
