package ociruntime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"ohoci/internal/oci"
	"ohoci/internal/store"
)

type Defaults struct {
	CompartmentID      string
	AvailabilityDomain string
	SubnetID           string
	NSGIDs             []string
	ImageID            string
	AssignPublicIP     bool
	CacheCompatEnabled bool
	CacheBucketName    string
	CacheObjectPrefix  string
	CacheRetentionDays int
}

type Input struct {
	CompartmentOCID    string   `json:"compartmentOcid"`
	AvailabilityDomain string   `json:"availabilityDomain"`
	SubnetOCID         string   `json:"subnetOcid"`
	NSGOCIDs           []string `json:"nsgOcids"`
	ImageOCID          string   `json:"imageOcid"`
	AssignPublicIP     bool     `json:"assignPublicIp"`
	CacheCompatEnabled bool     `json:"cacheCompatEnabled"`
	CacheBucketName    string   `json:"cacheBucketName"`
	CacheObjectPrefix  string   `json:"cacheObjectPrefix"`
	CacheRetentionDays int      `json:"cacheRetentionDays"`
}

type Status struct {
	Source            string                    `json:"source"`
	OverrideSettings  *store.OCIRuntimeSettings `json:"overrideSettings,omitempty"`
	EffectiveSettings store.OCIRuntimeSettings  `json:"effectiveSettings"`
	Ready             bool                      `json:"ready"`
	Missing           []string                  `json:"missing,omitempty"`
}

type Service struct {
	store    *store.Store
	defaults Defaults
	catalog  oci.Controller
}

func New(s *store.Store, defaults Defaults) *Service {
	return &Service{store: s, defaults: defaults}
}

func (s *Service) SetCatalogController(controller oci.Controller) {
	s.catalog = controller
}

func (s *Service) CurrentStatus(ctx context.Context) (Status, error) {
	effective := s.defaultSettings()
	status := Status{Source: "env", EffectiveSettings: effective}

	override, err := s.store.FindOCIRuntimeSettings(ctx)
	switch {
	case err == nil:
		effective = mergeSettings(effective, override)
		status.Source = "cms"
		status.OverrideSettings = &override
		status.EffectiveSettings = effective
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return Status{}, err
	}

	status.Ready, status.Missing = readiness(effective)
	return status, nil
}

func (s *Service) Save(ctx context.Context, input Input) (Status, error) {
	settings := sanitizeInput(input)
	if err := s.validateSettings(ctx, settings); err != nil {
		return Status{}, err
	}

	_, err := s.store.SaveOCIRuntimeSettings(ctx, settings)
	if err != nil {
		return Status{}, err
	}
	return s.CurrentStatus(ctx)
}

func (s *Service) Clear(ctx context.Context) error {
	return s.store.ClearOCIRuntimeSettings(ctx)
}

func (s *Service) RuntimeStatus(ctx context.Context) (bool, []string, error) {
	status, err := s.CurrentStatus(ctx)
	if err != nil {
		return false, nil, err
	}
	return status.Ready, status.Missing, nil
}

func (s *Service) ResolveRuntimeConfig(ctx context.Context) (oci.RuntimeConfig, error) {
	status, err := s.CurrentStatus(ctx)
	if err != nil {
		return oci.RuntimeConfig{}, err
	}
	return oci.RuntimeConfig{
		CompartmentID:      status.EffectiveSettings.CompartmentOCID,
		AvailabilityDomain: status.EffectiveSettings.AvailabilityDomain,
		SubnetID:           status.EffectiveSettings.SubnetOCID,
		NSGIDs:             append([]string(nil), status.EffectiveSettings.NSGOCIDs...),
		ImageID:            status.EffectiveSettings.ImageOCID,
		AssignPublicIP:     status.EffectiveSettings.AssignPublicIP,
	}, nil
}

func sanitizeInput(input Input) store.OCIRuntimeSettings {
	return store.OCIRuntimeSettings{
		CompartmentOCID:    strings.TrimSpace(input.CompartmentOCID),
		AvailabilityDomain: strings.TrimSpace(input.AvailabilityDomain),
		SubnetOCID:         strings.TrimSpace(input.SubnetOCID),
		NSGOCIDs:           normalizeStrings(input.NSGOCIDs),
		ImageOCID:          strings.TrimSpace(input.ImageOCID),
		AssignPublicIP:     input.AssignPublicIP,
		CacheCompatEnabled: input.CacheCompatEnabled,
		CacheBucketName:    strings.TrimSpace(input.CacheBucketName),
		CacheObjectPrefix:  strings.TrimSpace(input.CacheObjectPrefix),
		CacheRetentionDays: input.CacheRetentionDays,
	}
}

func (s *Service) validateSettings(ctx context.Context, settings store.OCIRuntimeSettings) error {
	if s.catalog == nil {
		return nil
	}

	effective := mergeSettings(s.defaultSettings(), settings)
	if ready, missing := readiness(effective); !ready {
		return fmt.Errorf("runtime settings are incomplete: missing %s", strings.Join(missing, ", "))
	}

	_, err := s.catalog.ListCatalog(ctx, oci.CatalogRequest{
		CompartmentOCID:    effective.CompartmentOCID,
		AvailabilityDomain: effective.AvailabilityDomain,
		SubnetOCID:         effective.SubnetOCID,
		ImageOCID:          effective.ImageOCID,
	})
	if err != nil {
		return fmt.Errorf("runtime settings failed OCI catalog validation: %w", err)
	}
	if effective.CacheCompatEnabled {
		if strings.TrimSpace(effective.CacheBucketName) == "" {
			return fmt.Errorf("cache bucket name is required when cache compatibility is enabled")
		}
		if effective.CacheRetentionDays <= 0 {
			return fmt.Errorf("cache retention days must be greater than zero")
		}
	}
	return nil
}

func (s *Service) defaultSettings() store.OCIRuntimeSettings {
	return store.OCIRuntimeSettings{
		CompartmentOCID:    strings.TrimSpace(s.defaults.CompartmentID),
		AvailabilityDomain: strings.TrimSpace(s.defaults.AvailabilityDomain),
		SubnetOCID:         strings.TrimSpace(s.defaults.SubnetID),
		NSGOCIDs:           normalizeStrings(s.defaults.NSGIDs),
		ImageOCID:          strings.TrimSpace(s.defaults.ImageID),
		AssignPublicIP:     s.defaults.AssignPublicIP,
		CacheCompatEnabled: s.defaults.CacheCompatEnabled,
		CacheBucketName:    strings.TrimSpace(s.defaults.CacheBucketName),
		CacheObjectPrefix:  strings.TrimSpace(s.defaults.CacheObjectPrefix),
		CacheRetentionDays: s.defaults.CacheRetentionDays,
	}
}

func mergeSettings(base, override store.OCIRuntimeSettings) store.OCIRuntimeSettings {
	merged := base
	if value := strings.TrimSpace(override.CompartmentOCID); value != "" {
		merged.CompartmentOCID = value
	}
	if value := strings.TrimSpace(override.AvailabilityDomain); value != "" {
		merged.AvailabilityDomain = value
	}
	if value := strings.TrimSpace(override.SubnetOCID); value != "" {
		merged.SubnetOCID = value
	}
	if values := normalizeStrings(override.NSGOCIDs); len(values) > 0 {
		merged.NSGOCIDs = values
	}
	if value := strings.TrimSpace(override.ImageOCID); value != "" {
		merged.ImageOCID = value
	}
	merged.AssignPublicIP = override.AssignPublicIP
	merged.CacheCompatEnabled = override.CacheCompatEnabled
	if value := strings.TrimSpace(override.CacheBucketName); value != "" {
		merged.CacheBucketName = value
	}
	if value := strings.TrimSpace(override.CacheObjectPrefix); value != "" {
		merged.CacheObjectPrefix = value
	}
	if override.CacheRetentionDays > 0 {
		merged.CacheRetentionDays = override.CacheRetentionDays
	}
	merged.ID = override.ID
	merged.CreatedAt = override.CreatedAt
	merged.UpdatedAt = override.UpdatedAt
	return merged
}

func readiness(settings store.OCIRuntimeSettings) (bool, []string) {
	missing := []string{}
	if strings.TrimSpace(settings.CompartmentOCID) == "" {
		missing = append(missing, "OHOCI_OCI_COMPARTMENT_OCID")
	}
	if strings.TrimSpace(settings.AvailabilityDomain) == "" {
		missing = append(missing, "OHOCI_OCI_AVAILABILITY_DOMAIN")
	}
	if strings.TrimSpace(settings.SubnetOCID) == "" {
		missing = append(missing, "OHOCI_OCI_SUBNET_OCID")
	}
	if strings.TrimSpace(settings.ImageOCID) == "" {
		missing = append(missing, "OHOCI_OCI_IMAGE_OCID")
	}
	return len(missing) == 0, missing
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	set := map[string]struct{}{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, exists := set[value]; exists {
			continue
		}
		set[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
