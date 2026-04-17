package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type GitHubPendingManifest struct {
	SessionBinding          string    `json:"-"`
	AppID                   int64     `json:"appId"`
	AppName                 string    `json:"appName"`
	AppSlug                 string    `json:"appSlug"`
	AppSettingsURL          string    `json:"appSettingsUrl"`
	TransferURL             string    `json:"transferUrl,omitempty"`
	InstallURL              string    `json:"installUrl"`
	OwnerTarget             string    `json:"ownerTarget"`
	PrivateKeyCiphertext    string    `json:"-"`
	WebhookSecretCiphertext string    `json:"-"`
	CreatedAt               time.Time `json:"createdAt"`
	ExpiresAt               time.Time `json:"expiresAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

func (s *Store) SaveGitHubPendingManifest(ctx context.Context, manifest GitHubPendingManifest) (GitHubPendingManifest, error) {
	now := s.now().UTC()
	sessionBinding := strings.TrimSpace(manifest.SessionBinding)
	if sessionBinding == "" {
		return GitHubPendingManifest{}, ErrNotFound
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GitHubPendingManifest{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM github_pending_manifests WHERE expires_at <= ?`, now); err != nil {
		return GitHubPendingManifest{}, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM github_pending_manifests WHERE session_binding = ?`, sessionBinding); err != nil {
		return GitHubPendingManifest{}, err
	}
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO github_pending_manifests
			(session_binding, app_id, app_name, app_slug, app_settings_url, transfer_url, install_url, owner_target, private_key_ciphertext, webhook_secret_ciphertext, created_at, expires_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionBinding,
		manifest.AppID,
		strings.TrimSpace(manifest.AppName),
		strings.TrimSpace(manifest.AppSlug),
		strings.TrimSpace(manifest.AppSettingsURL),
		strings.TrimSpace(manifest.TransferURL),
		strings.TrimSpace(manifest.InstallURL),
		strings.TrimSpace(manifest.OwnerTarget),
		strings.TrimSpace(manifest.PrivateKeyCiphertext),
		strings.TrimSpace(manifest.WebhookSecretCiphertext),
		manifest.CreatedAt.UTC(),
		manifest.ExpiresAt.UTC(),
		now,
	); err != nil {
		return GitHubPendingManifest{}, err
	}
	if err = tx.Commit(); err != nil {
		return GitHubPendingManifest{}, err
	}
	return s.FindGitHubPendingManifestBySessionBinding(ctx, sessionBinding, now)
}

func (s *Store) FindGitHubPendingManifestBySessionBinding(ctx context.Context, sessionBinding string, now time.Time) (GitHubPendingManifest, error) {
	binding := strings.TrimSpace(sessionBinding)
	if binding == "" {
		return GitHubPendingManifest{}, ErrNotFound
	}
	if err := s.DeleteExpiredGitHubPendingManifests(ctx, now); err != nil {
		return GitHubPendingManifest{}, err
	}
	row := s.db.QueryRowContext(
		ctx,
		`SELECT session_binding, app_id, app_name, app_slug, app_settings_url, transfer_url, install_url, owner_target, private_key_ciphertext, webhook_secret_ciphertext, created_at, expires_at, updated_at
			FROM github_pending_manifests
			WHERE session_binding = ? AND expires_at > ?`,
		binding,
		now.UTC(),
	)
	return scanGitHubPendingManifest(row)
}

func (s *Store) DeleteGitHubPendingManifestBySessionBinding(ctx context.Context, sessionBinding string) error {
	binding := strings.TrimSpace(sessionBinding)
	if binding == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_pending_manifests WHERE session_binding = ?`, binding)
	return err
}

func (s *Store) DeleteExpiredGitHubPendingManifests(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_pending_manifests WHERE expires_at <= ?`, now.UTC())
	return err
}

func scanGitHubPendingManifest(scanner interface{ Scan(dest ...any) error }) (GitHubPendingManifest, error) {
	var manifest GitHubPendingManifest
	if err := scanner.Scan(
		&manifest.SessionBinding,
		&manifest.AppID,
		&manifest.AppName,
		&manifest.AppSlug,
		&manifest.AppSettingsURL,
		&manifest.TransferURL,
		&manifest.InstallURL,
		&manifest.OwnerTarget,
		&manifest.PrivateKeyCiphertext,
		&manifest.WebhookSecretCiphertext,
		&manifest.CreatedAt,
		&manifest.ExpiresAt,
		&manifest.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GitHubPendingManifest{}, ErrNotFound
		}
		return GitHubPendingManifest{}, err
	}
	manifest.SessionBinding = strings.TrimSpace(manifest.SessionBinding)
	manifest.AppName = strings.TrimSpace(manifest.AppName)
	manifest.AppSlug = strings.TrimSpace(manifest.AppSlug)
	manifest.AppSettingsURL = strings.TrimSpace(manifest.AppSettingsURL)
	manifest.TransferURL = strings.TrimSpace(manifest.TransferURL)
	manifest.InstallURL = strings.TrimSpace(manifest.InstallURL)
	manifest.OwnerTarget = strings.TrimSpace(manifest.OwnerTarget)
	manifest.PrivateKeyCiphertext = strings.TrimSpace(manifest.PrivateKeyCiphertext)
	manifest.WebhookSecretCiphertext = strings.TrimSpace(manifest.WebhookSecretCiphertext)
	manifest.CreatedAt = manifest.CreatedAt.UTC()
	manifest.ExpiresAt = manifest.ExpiresAt.UTC()
	manifest.UpdatedAt = manifest.UpdatedAt.UTC()
	return manifest, nil
}
