package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type OCICredential struct {
	ID                   int64      `json:"id"`
	Name                 string     `json:"name"`
	ProfileName          string     `json:"profileName,omitempty"`
	TenancyOCID          string     `json:"tenancyOcid"`
	UserOCID             string     `json:"userOcid"`
	Fingerprint          string     `json:"fingerprint"`
	Region               string     `json:"region"`
	PrivateKeyCiphertext string     `json:"-"`
	PassphraseCiphertext string     `json:"-"`
	IsActive             bool       `json:"isActive"`
	LastTestedAt         *time.Time `json:"lastTestedAt,omitempty"`
	LastTestError        string     `json:"lastTestError,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

func (s *Store) SaveActiveOCICredential(ctx context.Context, credential OCICredential) (OCICredential, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OCICredential{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `UPDATE oci_credentials SET is_active = 0, updated_at = ? WHERE is_active = 1`, now); err != nil {
		return OCICredential{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO oci_credentials (name, profile_name, tenancy_ocid, user_ocid, fingerprint, region, private_key_ciphertext, passphrase_ciphertext, is_active, last_tested_at, last_test_error, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(credential.Name),
		nullableString(credential.ProfileName),
		strings.TrimSpace(credential.TenancyOCID),
		strings.TrimSpace(credential.UserOCID),
		strings.TrimSpace(credential.Fingerprint),
		strings.TrimSpace(credential.Region),
		strings.TrimSpace(credential.PrivateKeyCiphertext),
		strings.TrimSpace(credential.PassphraseCiphertext),
		boolAsInt(true),
		credential.LastTestedAt,
		strings.TrimSpace(credential.LastTestError),
		now,
		now,
	)
	if err != nil {
		return OCICredential{}, err
	}
	if _, err = result.LastInsertId(); err != nil {
		return OCICredential{}, err
	}
	if err = tx.Commit(); err != nil {
		return OCICredential{}, err
	}
	return s.FindActiveOCICredential(ctx)
}

func (s *Store) FindActiveOCICredential(ctx context.Context) (OCICredential, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, profile_name, tenancy_ocid, user_ocid, fingerprint, region, private_key_ciphertext, passphrase_ciphertext, is_active, last_tested_at, last_test_error, created_at, updated_at FROM oci_credentials WHERE is_active = 1 ORDER BY id DESC LIMIT 1`)
	return scanOCICredential(row)
}

func (s *Store) ClearActiveOCICredential(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE oci_credentials SET is_active = 0, updated_at = ? WHERE is_active = 1`, s.now().UTC())
	return err
}

func scanOCICredential(scanner interface{ Scan(dest ...any) error }) (OCICredential, error) {
	var credential OCICredential
	var profile sql.NullString
	var lastTested sql.NullTime
	var active int
	if err := scanner.Scan(
		&credential.ID,
		&credential.Name,
		&profile,
		&credential.TenancyOCID,
		&credential.UserOCID,
		&credential.Fingerprint,
		&credential.Region,
		&credential.PrivateKeyCiphertext,
		&credential.PassphraseCiphertext,
		&active,
		&lastTested,
		&credential.LastTestError,
		&credential.CreatedAt,
		&credential.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OCICredential{}, ErrNotFound
		}
		return OCICredential{}, err
	}
	if profile.Valid {
		credential.ProfileName = profile.String
	}
	if lastTested.Valid {
		value := lastTested.Time.UTC()
		credential.LastTestedAt = &value
	}
	credential.IsActive = active == 1
	return credential, nil
}
