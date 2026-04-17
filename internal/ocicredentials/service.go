package ocicredentials

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ohoci/internal/store"

	"github.com/oracle/oci-go-sdk/v65/common"
)

func New(s *store.Store, cfg Config) (*Service, error) {
	if s == nil {
		return nil, fmt.Errorf("store is required")
	}
	keyMaterial := strings.TrimSpace(cfg.EncryptionKey)
	if keyMaterial == "" {
		return nil, fmt.Errorf("data encryption key is required")
	}
	tester := cfg.Tester
	if tester == nil {
		tester = identityConnectionTester{}
	}
	return &Service{
		store:                 s,
		key:                   sha256Sum([]byte(keyMaterial)),
		defaultMode:           normalizeDefaultMode(cfg.DefaultMode),
		runtime:               cfg.Runtime,
		runtimeStatusProvider: cfg.RuntimeStatusProvider,
		tester:                tester,
	}, nil
}

func (s *Service) CurrentStatus(ctx context.Context) (Status, error) {
	status := Status{
		DefaultMode: s.defaultMode,
	}
	var err error
	status.RuntimeConfigReady, status.RuntimeConfigMissing, err = s.runtimeStatus(ctx)
	if err != nil {
		return Status{}, err
	}
	credential, err := s.store.FindActiveOCICredential(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			status.EffectiveMode = s.defaultMode
			return status, nil
		}
		return Status{}, err
	}
	sanitized := sanitizeCredential(credential)
	status.EffectiveMode = "api_key"
	status.ActiveCredential = &sanitized
	return status, nil
}

func (s *Service) Inspect(_ context.Context, input InspectInput) (InspectResult, error) {
	parsed, err := inspectCredential(input)
	if err != nil {
		return InspectResult{}, err
	}
	profiles, _, err := parseOCIProfiles(input.ConfigText)
	if err != nil {
		return InspectResult{}, err
	}
	selectedProfile := profiles[parsed.ProfileName]
	return InspectResult{
		Profiles:              append([]string(nil), parsed.Profiles...),
		SelectedProfile:       parsed.ProfileName,
		SuggestedName:         parsed.Name,
		TenancyOCID:           parsed.TenancyOCID,
		UserOCID:              parsed.UserOCID,
		Fingerprint:           parsed.Fingerprint,
		Region:                parsed.Region,
		HasEmbeddedPrivateKey: strings.TrimSpace(selectedProfile["key_content"]) != "",
		HasPassphrase:         strings.TrimSpace(firstNonEmpty(selectedProfile["pass_phrase"], selectedProfile["passphrase"])) != "",
	}, nil
}

func (s *Service) Test(ctx context.Context, input Input) (TestResult, error) {
	parsed, err := parseInput(input)
	if err != nil {
		return TestResult{}, err
	}
	provider, err := parsed.configurationProvider()
	if err != nil {
		return TestResult{}, err
	}
	if ok, err := common.IsConfigurationProviderValid(provider); err != nil {
		return TestResult{}, err
	} else if !ok {
		return TestResult{}, fmt.Errorf("OCI configuration provider is invalid")
	}

	regions, availabilityDomains, err := s.tester.Test(ctx, provider, parsed.TenancyOCID)
	if err != nil {
		return TestResult{}, err
	}
	now := time.Now().UTC()
	runtimeReady, runtimeMissing, err := s.runtimeStatus(ctx)
	if err != nil {
		return TestResult{}, err
	}
	return TestResult{
		EffectiveMode: "api_key",
		Credential: sanitizeCredential(store.OCICredential{
			Name:         parsed.Name,
			ProfileName:  parsed.ProfileName,
			TenancyOCID:  parsed.TenancyOCID,
			UserOCID:     parsed.UserOCID,
			Fingerprint:  parsed.Fingerprint,
			Region:       parsed.Region,
			LastTestedAt: &now,
		}),
		RegionSubscriptions:  regions,
		AvailabilityDomains:  availabilityDomains,
		Message:              fmt.Sprintf("Validated OCI API-key auth for profile %s", parsed.ProfileName),
		RuntimeConfigReady:   runtimeReady,
		RuntimeConfigMissing: runtimeMissing,
	}, nil
}

func (s *Service) Save(ctx context.Context, input Input) (TestResult, error) {
	parsed, err := parseInput(input)
	if err != nil {
		return TestResult{}, err
	}
	provider, err := parsed.configurationProvider()
	if err != nil {
		return TestResult{}, err
	}
	if ok, err := common.IsConfigurationProviderValid(provider); err != nil {
		return TestResult{}, err
	} else if !ok {
		return TestResult{}, fmt.Errorf("OCI configuration provider is invalid")
	}

	regions, availabilityDomains, err := s.tester.Test(ctx, provider, parsed.TenancyOCID)
	if err != nil {
		return TestResult{}, err
	}
	privateKeyCiphertext, err := s.encrypt(parsed.PrivateKeyPEM)
	if err != nil {
		return TestResult{}, err
	}
	passphraseCiphertext := ""
	if strings.TrimSpace(parsed.Passphrase) != "" {
		passphraseCiphertext, err = s.encrypt(parsed.Passphrase)
		if err != nil {
			return TestResult{}, err
		}
	}
	now := time.Now().UTC()
	record, err := s.store.SaveActiveOCICredential(ctx, store.OCICredential{
		Name:                 parsed.Name,
		ProfileName:          parsed.ProfileName,
		TenancyOCID:          parsed.TenancyOCID,
		UserOCID:             parsed.UserOCID,
		Fingerprint:          parsed.Fingerprint,
		Region:               parsed.Region,
		PrivateKeyCiphertext: privateKeyCiphertext,
		PassphraseCiphertext: passphraseCiphertext,
		IsActive:             true,
		LastTestedAt:         &now,
	})
	if err != nil {
		return TestResult{}, err
	}
	runtimeReady, runtimeMissing, err := s.runtimeStatus(ctx)
	if err != nil {
		return TestResult{}, err
	}
	return TestResult{
		EffectiveMode:        "api_key",
		Credential:           sanitizeCredential(record),
		RegionSubscriptions:  regions,
		AvailabilityDomains:  availabilityDomains,
		Message:              fmt.Sprintf("Stored and activated OCI API-key auth for profile %s", parsed.ProfileName),
		RuntimeConfigReady:   runtimeReady,
		RuntimeConfigMissing: runtimeMissing,
	}, nil
}

func (s *Service) Clear(ctx context.Context) error {
	return s.store.ClearActiveOCICredential(ctx)
}

func (s *Service) ResolveProvider(ctx context.Context) (common.ConfigurationProvider, bool, error) {
	record, err := s.store.FindActiveOCICredential(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	privateKeyPEM, err := s.decrypt(record.PrivateKeyCiphertext)
	if err != nil {
		return nil, true, err
	}
	passphrase := ""
	if strings.TrimSpace(record.PassphraseCiphertext) != "" {
		passphrase, err = s.decrypt(record.PassphraseCiphertext)
		if err != nil {
			return nil, true, err
		}
	}
	provider := common.NewRawConfigurationProvider(
		record.TenancyOCID,
		record.UserOCID,
		record.Region,
		record.Fingerprint,
		privateKeyPEM,
		optionalString(passphrase),
	)
	if ok, err := common.IsConfigurationProviderValid(provider); err != nil {
		return nil, true, err
	} else if !ok {
		return nil, true, fmt.Errorf("stored OCI credential is invalid")
	}
	return provider, true, nil
}

func (s *Service) runtimeStatus(ctx context.Context) (bool, []string, error) {
	if s.runtimeStatusProvider != nil {
		return s.runtimeStatusProvider.RuntimeStatus(ctx)
	}
	ready, missing := runtimeStatusFromConfig(s.runtime)
	return ready, missing, nil
}
