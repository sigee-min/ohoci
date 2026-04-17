package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type OCIRuntimeSettings struct {
	ID                 int64     `json:"id"`
	CompartmentOCID    string    `json:"compartmentOcid"`
	AvailabilityDomain string    `json:"availabilityDomain"`
	SubnetOCID         string    `json:"subnetOcid"`
	NSGOCIDs           []string  `json:"nsgOcids"`
	ImageOCID          string    `json:"imageOcid"`
	AssignPublicIP     bool      `json:"assignPublicIp"`
	CacheCompatEnabled bool      `json:"cacheCompatEnabled"`
	CacheBucketName    string    `json:"cacheBucketName"`
	CacheObjectPrefix  string    `json:"cacheObjectPrefix"`
	CacheRetentionDays int       `json:"cacheRetentionDays"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func (s *Store) SaveOCIRuntimeSettings(ctx context.Context, settings OCIRuntimeSettings) (OCIRuntimeSettings, error) {
	now := s.now().UTC()
	nsgJSON, err := json.Marshal(normalizeStrings(settings.NSGOCIDs))
	if err != nil {
		return OCIRuntimeSettings{}, err
	}
	existing, err := s.FindOCIRuntimeSettings(ctx)
	switch {
	case err == nil:
		_, err = s.db.ExecContext(
			ctx,
			`UPDATE oci_runtime_settings
			 SET compartment_ocid = ?, availability_domain = ?, subnet_ocid = ?, nsg_ocids_json = ?, image_ocid = ?, assign_public_ip = ?, cache_compat_enabled = ?, cache_bucket_name = ?, cache_object_prefix = ?, cache_retention_days = ?, updated_at = ?
			 WHERE id = ?`,
			settings.CompartmentOCID,
			settings.AvailabilityDomain,
			settings.SubnetOCID,
			string(nsgJSON),
			settings.ImageOCID,
			boolAsInt(settings.AssignPublicIP),
			boolAsInt(settings.CacheCompatEnabled),
			settings.CacheBucketName,
			settings.CacheObjectPrefix,
			settings.CacheRetentionDays,
			now,
			existing.ID,
		)
		if err != nil {
			return OCIRuntimeSettings{}, err
		}
		return s.FindOCIRuntimeSettings(ctx)
	case !errors.Is(err, ErrNotFound):
		return OCIRuntimeSettings{}, err
	}

	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO oci_runtime_settings (compartment_ocid, availability_domain, subnet_ocid, nsg_ocids_json, image_ocid, assign_public_ip, cache_compat_enabled, cache_bucket_name, cache_object_prefix, cache_retention_days, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		settings.CompartmentOCID,
		settings.AvailabilityDomain,
		settings.SubnetOCID,
		string(nsgJSON),
		settings.ImageOCID,
		boolAsInt(settings.AssignPublicIP),
		boolAsInt(settings.CacheCompatEnabled),
		settings.CacheBucketName,
		settings.CacheObjectPrefix,
		settings.CacheRetentionDays,
		now,
		now,
	)
	if err != nil {
		return OCIRuntimeSettings{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return OCIRuntimeSettings{}, err
	}
	return s.findOCIRuntimeSettingsByID(ctx, id)
}

func (s *Store) FindOCIRuntimeSettings(ctx context.Context) (OCIRuntimeSettings, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, compartment_ocid, availability_domain, subnet_ocid, nsg_ocids_json, image_ocid, assign_public_ip, cache_compat_enabled, cache_bucket_name, cache_object_prefix, cache_retention_days, created_at, updated_at
		 FROM oci_runtime_settings
		 ORDER BY id DESC
		 LIMIT 1`,
	)
	return scanOCIRuntimeSettings(row)
}

func (s *Store) findOCIRuntimeSettingsByID(ctx context.Context, id int64) (OCIRuntimeSettings, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, compartment_ocid, availability_domain, subnet_ocid, nsg_ocids_json, image_ocid, assign_public_ip, cache_compat_enabled, cache_bucket_name, cache_object_prefix, cache_retention_days, created_at, updated_at
		 FROM oci_runtime_settings
		 WHERE id = ?`,
		id,
	)
	return scanOCIRuntimeSettings(row)
}

func scanOCIRuntimeSettings(scanner interface{ Scan(dest ...any) error }) (OCIRuntimeSettings, error) {
	var settings OCIRuntimeSettings
	var nsgJSON string
	var assignPublicIP int
	var cacheCompatEnabled int
	if err := scanner.Scan(
		&settings.ID,
		&settings.CompartmentOCID,
		&settings.AvailabilityDomain,
		&settings.SubnetOCID,
		&nsgJSON,
		&settings.ImageOCID,
		&assignPublicIP,
		&cacheCompatEnabled,
		&settings.CacheBucketName,
		&settings.CacheObjectPrefix,
		&settings.CacheRetentionDays,
		&settings.CreatedAt,
		&settings.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OCIRuntimeSettings{}, ErrNotFound
		}
		return OCIRuntimeSettings{}, err
	}
	_ = json.Unmarshal([]byte(nsgJSON), &settings.NSGOCIDs)
	settings.NSGOCIDs = normalizeStrings(settings.NSGOCIDs)
	settings.AssignPublicIP = assignPublicIP == 1
	settings.CacheCompatEnabled = cacheCompatEnabled == 1
	return settings, nil
}

func (s *Store) ClearOCIRuntimeSettings(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oci_runtime_settings`)
	return err
}
