package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	GitHubAuthModeApp = "app"

	defaultGitHubConfigAPIBaseURL = "https://api.github.com"
)

type GitHubConfig struct {
	ID                              int64      `json:"id"`
	Name                            string     `json:"name"`
	Tags                            []string   `json:"tags"`
	APIBaseURL                      string     `json:"apiBaseUrl"`
	AuthMode                        string     `json:"authMode"`
	AppID                           int64      `json:"appId"`
	InstallationID                  int64      `json:"installationId"`
	PrivateKeyCiphertext            string     `json:"-"`
	WebhookSecretCiphertext         string     `json:"-"`
	AllowedOrg                      string     `json:"allowedOrg"`
	SelectedRepos                   []string   `json:"selectedRepos"`
	AccountLogin                    string     `json:"accountLogin"`
	AccountType                     string     `json:"accountType"`
	InstallationState               string     `json:"installationState"`
	InstallationRepositorySelection string     `json:"installationRepositorySelection"`
	InstallationRepositories        []string   `json:"installationRepositories"`
	IsActive                        bool       `json:"isActive"`
	IsStaged                        bool       `json:"isStaged"`
	LastTestedAt                    *time.Time `json:"lastTestedAt,omitempty"`
	LastTestError                   string     `json:"lastTestError,omitempty"`
	CreatedAt                       time.Time  `json:"createdAt"`
	UpdatedAt                       time.Time  `json:"updatedAt"`
}

type GitHubRouteInstallationStatus struct {
	APIBaseURL                      string     `json:"apiBaseUrl"`
	AppID                           int64      `json:"appId"`
	InstallationID                  int64      `json:"installationId"`
	AccountLogin                    string     `json:"accountLogin"`
	AccountType                     string     `json:"accountType"`
	InstallationState               string     `json:"installationState"`
	InstallationRepositorySelection string     `json:"installationRepositorySelection"`
	InstallationRepositories        []string   `json:"installationRepositories"`
	LastTestedAt                    *time.Time `json:"lastTestedAt,omitempty"`
	LastTestError                   string     `json:"lastTestError,omitempty"`
	CreatedAt                       time.Time  `json:"createdAt"`
	UpdatedAt                       time.Time  `json:"updatedAt"`
}

const githubConfigColumns = `id, name, tags_json, api_base_url, auth_mode, app_id, installation_id, private_key_ciphertext, webhook_secret_ciphertext, allowed_org, allowed_repos_json, account_login, account_type, installation_state, installation_repository_selection, installation_repositories_json, is_active, is_staged, last_tested_at, last_test_error, created_at, updated_at`
const githubRouteInstallationStatusColumns = `api_base_url, app_id, installation_id, account_login, account_type, installation_state, installation_repository_selection, installation_repositories_json, last_tested_at, last_test_error, created_at, updated_at`

type gitHubConfigRouteIdentity struct {
	APIBaseURL     string
	AppID          int64
	InstallationID int64
}

func (s *Store) SaveActiveGitHubConfig(ctx context.Context, cfg GitHubConfig) (GitHubConfig, error) {
	authMode := strings.ToLower(strings.TrimSpace(cfg.AuthMode))
	if authMode == "" {
		authMode = GitHubAuthModeApp
	}
	return s.saveGitHubConfig(ctx, cfg, authMode, true, false)
}

func (s *Store) SaveStagedGitHubConfig(ctx context.Context, cfg GitHubConfig) (GitHubConfig, error) {
	return s.saveGitHubConfig(ctx, cfg, GitHubAuthModeApp, false, true)
}

func (s *Store) PromoteStagedGitHubConfig(ctx context.Context) (GitHubConfig, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GitHubConfig{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stagedRow := tx.QueryRowContext(ctx, `SELECT `+githubConfigColumns+` FROM github_configs WHERE is_staged = 1 ORDER BY id DESC LIMIT 1`)
	staged, scanErr := scanGitHubConfig(stagedRow)
	if scanErr != nil {
		err = scanErr
		return GitHubConfig{}, err
	}

	if err = s.retireMatchingActiveGitHubConfigsTx(ctx, tx, staged, staged.ID, now); err != nil {
		return GitHubConfig{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE github_configs SET is_staged = 0, updated_at = ? WHERE is_staged = 1`, now); err != nil {
		return GitHubConfig{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE github_configs SET is_active = 1, is_staged = 0, updated_at = ? WHERE id = ?`, now, staged.ID); err != nil {
		return GitHubConfig{}, err
	}
	if err = tx.Commit(); err != nil {
		return GitHubConfig{}, err
	}
	return s.FindGitHubConfigByID(ctx, staged.ID)
}

func (s *Store) saveGitHubConfig(ctx context.Context, cfg GitHubConfig, authMode string, isActive, isStaged bool) (GitHubConfig, error) {
	now := s.now().UTC()
	tagsJSON, err := json.Marshal(normalizeStrings(cfg.Tags))
	if err != nil {
		return GitHubConfig{}, err
	}
	selectedReposJSON, err := json.Marshal(normalizeStrings(cfg.SelectedRepos))
	if err != nil {
		return GitHubConfig{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GitHubConfig{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	record := cfg
	record.AuthMode = authMode
	if isActive {
		if err = s.retireMatchingActiveGitHubConfigsTx(ctx, tx, record, 0, now); err != nil {
			return GitHubConfig{}, err
		}
	}
	if isStaged {
		if _, err = tx.ExecContext(ctx, `UPDATE github_configs SET is_staged = 0, updated_at = ? WHERE is_staged = 1`, now); err != nil {
			return GitHubConfig{}, err
		}
	}
	installationRepositoriesJSON, err := json.Marshal(normalizeStrings(cfg.InstallationRepositories))
	if err != nil {
		return GitHubConfig{}, err
	}
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO github_configs (name, tags_json, api_base_url, auth_mode, app_id, installation_id, private_key_ciphertext, webhook_secret_ciphertext, allowed_org, allowed_repos_json, account_login, account_type, installation_state, installation_repository_selection, installation_repositories_json, is_active, is_staged, last_tested_at, last_test_error, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(cfg.Name),
		string(tagsJSON),
		normalizeGitHubConfigAPIBaseURL(cfg.APIBaseURL),
		authMode,
		cfg.AppID,
		cfg.InstallationID,
		strings.TrimSpace(cfg.PrivateKeyCiphertext),
		strings.TrimSpace(cfg.WebhookSecretCiphertext),
		strings.TrimSpace(cfg.AllowedOrg),
		string(selectedReposJSON),
		strings.TrimSpace(cfg.AccountLogin),
		strings.TrimSpace(cfg.AccountType),
		strings.TrimSpace(cfg.InstallationState),
		strings.TrimSpace(cfg.InstallationRepositorySelection),
		string(installationRepositoriesJSON),
		boolAsInt(isActive),
		boolAsInt(isStaged),
		cfg.LastTestedAt,
		strings.TrimSpace(cfg.LastTestError),
		now,
		now,
	)
	if err != nil {
		return GitHubConfig{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return GitHubConfig{}, err
	}
	if err = tx.Commit(); err != nil {
		return GitHubConfig{}, err
	}
	return s.FindGitHubConfigByID(ctx, id)
}

func (s *Store) FindActiveGitHubConfig(ctx context.Context) (GitHubConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+githubConfigColumns+` FROM github_configs WHERE is_active = 1 ORDER BY id DESC LIMIT 1`)
	return scanGitHubConfig(row)
}

func (s *Store) ListActiveGitHubConfigs(ctx context.Context) ([]GitHubConfig, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+githubConfigColumns+` FROM github_configs WHERE is_active = 1 ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []GitHubConfig{}
	for rows.Next() {
		item, err := scanGitHubConfig(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) FindGitHubConfigByID(ctx context.Context, id int64) (GitHubConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+githubConfigColumns+` FROM github_configs WHERE id = ?`, id)
	return scanGitHubConfig(row)
}

func (s *Store) FindActiveGitHubConfigByInstallationID(ctx context.Context, installationID int64) (GitHubConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+githubConfigColumns+` FROM github_configs WHERE is_active = 1 AND installation_id = ? ORDER BY id DESC LIMIT 1`, installationID)
	return scanGitHubConfig(row)
}

func (s *Store) FindActiveGitHubConfigByRoute(ctx context.Context, apiBaseURL string, appID, installationID int64) (GitHubConfig, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT `+githubConfigColumns+` FROM github_configs WHERE is_active = 1 AND api_base_url = ? AND app_id = ? AND installation_id = ? ORDER BY id DESC LIMIT 1`,
		normalizeGitHubConfigAPIBaseURL(apiBaseURL),
		appID,
		installationID,
	)
	return scanGitHubConfig(row)
}

func (s *Store) FindStagedGitHubConfig(ctx context.Context) (GitHubConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+githubConfigColumns+` FROM github_configs WHERE is_staged = 1 ORDER BY id DESC LIMIT 1`)
	return scanGitHubConfig(row)
}

func (s *Store) ClearActiveGitHubConfig(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE github_configs SET is_active = 0, updated_at = ? WHERE is_active = 1`, s.now().UTC())
	return err
}

func (s *Store) ClearStagedGitHubConfig(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE github_configs SET is_staged = 0, updated_at = ? WHERE is_staged = 1`, s.now().UTC())
	return err
}

func (s *Store) UpdateGitHubConfigInstallation(ctx context.Context, id int64, cfg GitHubConfig) error {
	installationReposJSON, err := json.Marshal(normalizeStrings(cfg.InstallationRepositories))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(
		ctx,
		`UPDATE github_configs
			SET account_login = ?, account_type = ?, installation_state = ?, installation_repository_selection = ?, installation_repositories_json = ?, last_tested_at = ?, last_test_error = ?, updated_at = ?
			WHERE id = ?`,
		strings.TrimSpace(cfg.AccountLogin),
		strings.TrimSpace(cfg.AccountType),
		strings.ToLower(strings.TrimSpace(cfg.InstallationState)),
		strings.ToLower(strings.TrimSpace(cfg.InstallationRepositorySelection)),
		string(installationReposJSON),
		cfg.LastTestedAt,
		strings.TrimSpace(cfg.LastTestError),
		s.now().UTC(),
		id,
	)
	return err
}

func (s *Store) FindGitHubRouteInstallationStatus(ctx context.Context, apiBaseURL string, appID, installationID int64) (GitHubRouteInstallationStatus, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT `+githubRouteInstallationStatusColumns+` FROM github_route_installation_statuses WHERE api_base_url = ? AND app_id = ? AND installation_id = ?`,
		normalizeGitHubConfigAPIBaseURL(apiBaseURL),
		appID,
		installationID,
	)
	return scanGitHubRouteInstallationStatus(row)
}

func (s *Store) UpsertGitHubRouteInstallationStatus(ctx context.Context, status GitHubRouteInstallationStatus) error {
	if status.AppID <= 0 || status.InstallationID <= 0 {
		return nil
	}

	installationReposJSON, err := json.Marshal(normalizeStrings(status.InstallationRepositories))
	if err != nil {
		return err
	}

	now := s.now().UTC()
	normalized := GitHubRouteInstallationStatus{
		APIBaseURL:                      normalizeGitHubConfigAPIBaseURL(status.APIBaseURL),
		AppID:                           status.AppID,
		InstallationID:                  status.InstallationID,
		AccountLogin:                    strings.TrimSpace(status.AccountLogin),
		AccountType:                     strings.TrimSpace(status.AccountType),
		InstallationState:               strings.ToLower(strings.TrimSpace(status.InstallationState)),
		InstallationRepositorySelection: strings.ToLower(strings.TrimSpace(status.InstallationRepositorySelection)),
		InstallationRepositories:        normalizeStrings(status.InstallationRepositories),
		LastTestedAt:                    status.LastTestedAt,
		LastTestError:                   strings.TrimSpace(status.LastTestError),
	}

	existing, err := s.FindGitHubRouteInstallationStatus(ctx, normalized.APIBaseURL, normalized.AppID, normalized.InstallationID)
	switch {
	case err == nil:
		_, err = s.db.ExecContext(
			ctx,
			`UPDATE github_route_installation_statuses
				SET account_login = ?, account_type = ?, installation_state = ?, installation_repository_selection = ?, installation_repositories_json = ?, last_tested_at = ?, last_test_error = ?, updated_at = ?
				WHERE api_base_url = ? AND app_id = ? AND installation_id = ?`,
			normalized.AccountLogin,
			normalized.AccountType,
			normalized.InstallationState,
			normalized.InstallationRepositorySelection,
			string(installationReposJSON),
			normalized.LastTestedAt,
			normalized.LastTestError,
			now,
			normalized.APIBaseURL,
			normalized.AppID,
			normalized.InstallationID,
		)
		return err
	case !errors.Is(err, ErrNotFound):
		return err
	}

	createdAt := now
	if !existing.CreatedAt.IsZero() {
		createdAt = existing.CreatedAt
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO github_route_installation_statuses (api_base_url, app_id, installation_id, account_login, account_type, installation_state, installation_repository_selection, installation_repositories_json, last_tested_at, last_test_error, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		normalized.APIBaseURL,
		normalized.AppID,
		normalized.InstallationID,
		normalized.AccountLogin,
		normalized.AccountType,
		normalized.InstallationState,
		normalized.InstallationRepositorySelection,
		string(installationReposJSON),
		normalized.LastTestedAt,
		normalized.LastTestError,
		createdAt,
		now,
	)
	return err
}

func (s *Store) retireMatchingActiveGitHubConfigsTx(ctx context.Context, tx *sql.Tx, candidate GitHubConfig, excludeID int64, now time.Time) error {
	identity, ok := gitHubConfigRouteIdentityFor(candidate)
	if !ok {
		return nil
	}

	rows, err := tx.QueryContext(ctx, `SELECT `+githubConfigColumns+` FROM github_configs WHERE is_active = 1 AND app_id = ? AND installation_id = ?`, identity.AppID, identity.InstallationID)
	if err != nil {
		return err
	}
	defer rows.Close()

	retireIDs := make([]int64, 0)
	for rows.Next() {
		record, err := scanGitHubConfig(rows)
		if err != nil {
			return err
		}
		if record.ID == excludeID {
			continue
		}
		if !identity.matches(record) {
			continue
		}
		retireIDs = append(retireIDs, record.ID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range retireIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE github_configs SET is_active = 0, updated_at = ? WHERE id = ?`, now, id); err != nil {
			return err
		}
	}
	return nil
}

func gitHubConfigRouteIdentityFor(cfg GitHubConfig) (gitHubConfigRouteIdentity, bool) {
	authMode := strings.ToLower(strings.TrimSpace(cfg.AuthMode))
	if authMode == "" {
		authMode = GitHubAuthModeApp
	}
	if authMode != GitHubAuthModeApp || cfg.AppID <= 0 || cfg.InstallationID <= 0 {
		return gitHubConfigRouteIdentity{}, false
	}
	return gitHubConfigRouteIdentity{
		APIBaseURL:     normalizeGitHubConfigAPIBaseURL(cfg.APIBaseURL),
		AppID:          cfg.AppID,
		InstallationID: cfg.InstallationID,
	}, true
}

func (identity gitHubConfigRouteIdentity) matches(cfg GitHubConfig) bool {
	other, ok := gitHubConfigRouteIdentityFor(cfg)
	if !ok {
		return false
	}
	return identity == other
}

func normalizeGitHubConfigAPIBaseURL(value string) string {
	normalized := strings.TrimRight(strings.TrimSpace(value), "/")
	if normalized == "" {
		return defaultGitHubConfigAPIBaseURL
	}
	return normalized
}

func scanGitHubConfig(scanner interface{ Scan(dest ...any) error }) (GitHubConfig, error) {
	var cfg GitHubConfig
	var tagsJSON string
	var selectedReposJSON string
	var installationReposJSON string
	var active int
	var staged int
	var lastTested sql.NullTime
	if err := scanner.Scan(
		&cfg.ID,
		&cfg.Name,
		&tagsJSON,
		&cfg.APIBaseURL,
		&cfg.AuthMode,
		&cfg.AppID,
		&cfg.InstallationID,
		&cfg.PrivateKeyCiphertext,
		&cfg.WebhookSecretCiphertext,
		&cfg.AllowedOrg,
		&selectedReposJSON,
		&cfg.AccountLogin,
		&cfg.AccountType,
		&cfg.InstallationState,
		&cfg.InstallationRepositorySelection,
		&installationReposJSON,
		&active,
		&staged,
		&lastTested,
		&cfg.LastTestError,
		&cfg.CreatedAt,
		&cfg.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GitHubConfig{}, ErrNotFound
		}
		return GitHubConfig{}, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &cfg.Tags)
	_ = json.Unmarshal([]byte(selectedReposJSON), &cfg.SelectedRepos)
	_ = json.Unmarshal([]byte(installationReposJSON), &cfg.InstallationRepositories)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Tags = normalizeStrings(cfg.Tags)
	cfg.SelectedRepos = normalizeStrings(cfg.SelectedRepos)
	cfg.InstallationRepositories = normalizeStrings(cfg.InstallationRepositories)
	cfg.AuthMode = strings.ToLower(strings.TrimSpace(cfg.AuthMode))
	if cfg.AuthMode == "" {
		cfg.AuthMode = GitHubAuthModeApp
	}
	cfg.InstallationState = strings.ToLower(strings.TrimSpace(cfg.InstallationState))
	cfg.InstallationRepositorySelection = strings.ToLower(strings.TrimSpace(cfg.InstallationRepositorySelection))
	cfg.IsActive = active == 1
	cfg.IsStaged = staged == 1
	if lastTested.Valid {
		value := lastTested.Time.UTC()
		cfg.LastTestedAt = &value
	}
	return cfg, nil
}

func scanGitHubRouteInstallationStatus(scanner interface{ Scan(dest ...any) error }) (GitHubRouteInstallationStatus, error) {
	var status GitHubRouteInstallationStatus
	var installationReposJSON string
	var lastTested sql.NullTime
	if err := scanner.Scan(
		&status.APIBaseURL,
		&status.AppID,
		&status.InstallationID,
		&status.AccountLogin,
		&status.AccountType,
		&status.InstallationState,
		&status.InstallationRepositorySelection,
		&installationReposJSON,
		&lastTested,
		&status.LastTestError,
		&status.CreatedAt,
		&status.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GitHubRouteInstallationStatus{}, ErrNotFound
		}
		return GitHubRouteInstallationStatus{}, err
	}
	_ = json.Unmarshal([]byte(installationReposJSON), &status.InstallationRepositories)
	status.APIBaseURL = normalizeGitHubConfigAPIBaseURL(status.APIBaseURL)
	status.AccountLogin = strings.TrimSpace(status.AccountLogin)
	status.AccountType = strings.TrimSpace(status.AccountType)
	status.InstallationState = strings.ToLower(strings.TrimSpace(status.InstallationState))
	status.InstallationRepositorySelection = strings.ToLower(strings.TrimSpace(status.InstallationRepositorySelection))
	status.InstallationRepositories = normalizeStrings(status.InstallationRepositories)
	status.LastTestError = strings.TrimSpace(status.LastTestError)
	if lastTested.Valid {
		value := lastTested.Time.UTC()
		status.LastTestedAt = &value
	}
	return status, nil
}
