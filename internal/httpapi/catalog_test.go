package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"ohoci/internal/githubapp"
	"ohoci/internal/oci"
	"ohoci/internal/store"
)

type catalogErrorController struct {
	err error
}

var readyGitHubDefaults = githubapp.Config{
	APIBaseURL:                      "https://api.github.com",
	AppID:                           123,
	InstallationID:                  456,
	PrivateKeyPEM:                   testPrivateKey,
	WebhookSecret:                   "webhook-secret",
	SelectedRepos:                   []string{"example/repo"},
	AccountLogin:                    "example-org",
	AccountType:                     "Organization",
	InstallationState:               "active",
	InstallationRepositorySelection: "selected",
	InstallationRepositories:        []string{"example/repo"},
}

func (c catalogErrorController) LaunchInstance(_ context.Context, _ oci.LaunchRequest) (oci.Instance, error) {
	return oci.Instance{}, c.err
}

func (c catalogErrorController) GetInstance(_ context.Context, _ string) (oci.Instance, error) {
	return oci.Instance{}, c.err
}

func (c catalogErrorController) TerminateInstance(_ context.Context, _ string) error {
	return c.err
}

func (c catalogErrorController) CreateImage(_ context.Context, _ oci.CreateImageRequest) (oci.Image, error) {
	return oci.Image{}, c.err
}

func (c catalogErrorController) GetImage(_ context.Context, _ string) (oci.Image, error) {
	return oci.Image{}, c.err
}

func (c catalogErrorController) CaptureConsoleOutput(_ context.Context, _ string) (string, error) {
	return "", c.err
}

func (c catalogErrorController) DiscoverManagedResources(_ context.Context) (oci.ManagedResourceDiscovery, error) {
	return oci.ManagedResourceDiscovery{}, c.err
}

func (c catalogErrorController) ListSubnetCandidates(_ context.Context) ([]oci.SubnetCandidate, error) {
	return nil, c.err
}

func (c catalogErrorController) ListCatalog(_ context.Context, _ oci.CatalogRequest) (oci.CatalogResponse, error) {
	return oci.CatalogResponse{}, c.err
}

func TestOCICatalogReturnsFixtureCatalog(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/oci/catalog", oci.CatalogRequest{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		ImageOCID:          "ocid1.image.oc1..ubuntu",
		SubnetOCID:         "ocid1.subnet.oc1..ad1",
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected catalog success, got %d: %s", response.Code, response.Body.String())
	}

	var catalog oci.CatalogResponse
	if err := json.Unmarshal(response.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("decode catalog response: %v", err)
	}
	if catalog.SourceRegion != "fake-region-1" {
		t.Fatalf("expected fake source region, got %q", catalog.SourceRegion)
	}
	expectedValidatedAt := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	if !catalog.ValidatedAt.Equal(expectedValidatedAt) {
		t.Fatalf("unexpected validatedAt: %s", catalog.ValidatedAt)
	}
	if len(catalog.AvailabilityDomains) != 2 {
		t.Fatalf("expected 2 availability domains, got %d", len(catalog.AvailabilityDomains))
	}
	if len(catalog.Subnets) != 2 {
		t.Fatalf("expected AD-specific plus regional subnet, got %d", len(catalog.Subnets))
	}
	if len(catalog.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(catalog.Images))
	}
	if len(catalog.Shapes) != 2 {
		t.Fatalf("expected 2 shapes, got %d", len(catalog.Shapes))
	}
	if catalog.Subnets[0].ID != "ocid1.subnet.oc1..regional" || catalog.Subnets[1].ID != "ocid1.subnet.oc1..ad1" {
		t.Fatalf("unexpected subnet ordering/filtering: %#v", catalog.Subnets)
	}
}

func TestOCICatalogRequiresCompartment(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/oci/catalog", map[string]any{
		"availabilityDomain": "AD-1",
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing compartment, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "compartmentOcid is required") {
		t.Fatalf("expected explicit compartment error, got %s", response.Body.String())
	}
}

func TestOCICatalogReturnsExplicitSelectionError(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/oci/catalog", oci.CatalogRequest{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..ad2",
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for invalid selected subnet, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "selected subnet") {
		t.Fatalf("expected explicit selected subnet error, got %s", response.Body.String())
	}
}

func TestOCICatalogPropagatesEmptyInventoryErrors(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{
		ociController: catalogErrorController{err: errors.New("no available images found for compartment ocid1.compartment.oc1..empty")},
	})
	token := authenticatedToken(t, handler.auth)

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/oci/catalog", oci.CatalogRequest{
		CompartmentOCID: "ocid1.compartment.oc1..empty",
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for empty inventory, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "no available images found") {
		t.Fatalf("expected empty inventory detail, got %s", response.Body.String())
	}
}

func TestOCICatalogPropagatesPermissionErrors(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{
		ociController: catalogErrorController{err: errors.New("NotAuthorizedOrNotFound: operation not allowed")},
	})
	token := authenticatedToken(t, handler.auth)

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/oci/catalog", oci.CatalogRequest{
		CompartmentOCID: "ocid1.compartment.oc1..restricted",
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for permission failure, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "NotAuthorizedOrNotFound") {
		t.Fatalf("expected permission detail, got %s", response.Body.String())
	}
}

func TestOCIRuntimeSaveRejectsUnavailableCatalogValues(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	response := performJSONRequest(t, handler.handler, http.MethodPut, "/api/v1/oci/runtime", map[string]any{
		"compartmentOcid":    "ocid1.compartment.oc1..example",
		"availabilityDomain": "AD-1",
		"subnetOcid":         "ocid1.subnet.oc1..ad2",
		"imageOcid":          "ocid1.image.oc1..ubuntu",
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid runtime selection, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "selected subnet") {
		t.Fatalf("expected selected subnet error, got %s", response.Body.String())
	}
}

func TestPolicySaveRejectsUnavailableShape(t *testing.T) {
	ctx := context.Background()
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{githubDefaults: readyGitHubDefaults})
	token := authenticatedToken(t, handler.auth)

	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..ad1",
		ImageOCID:          "ocid1.image.oc1..ubuntu",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/policies", store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.DoesNotExist",
		OCPU:       1,
		MemoryGB:   1,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unavailable shape, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "stale or unavailable") {
		t.Fatalf("expected stale shape error, got %s", response.Body.String())
	}
}

func TestPolicySaveRejectsOutOfBoundsFlexibleShape(t *testing.T) {
	ctx := context.Background()
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{githubDefaults: readyGitHubDefaults})
	token := authenticatedToken(t, handler.auth)

	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..ad1",
		ImageOCID:          "ocid1.image.oc1..ubuntu",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/policies", store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E4.Flex",
		OCPU:       9,
		MemoryGB:   16,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for out-of-bounds flexible shape, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "OCPU must be 8 or less") {
		t.Fatalf("expected OCPU bounds error, got %s", response.Body.String())
	}
}

func TestPolicySaveRejectsFixedShapeOverrides(t *testing.T) {
	ctx := context.Background()
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{githubDefaults: readyGitHubDefaults})
	token := authenticatedToken(t, handler.auth)

	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..ad1",
		ImageOCID:          "ocid1.image.oc1..ubuntu",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}

	response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/policies", store.Policy{
		Labels:     []string{"oci", "cpu"},
		Shape:      "VM.Standard.E2.1.Micro",
		OCPU:       2,
		MemoryGB:   2,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for fixed shape override, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "must use 1 OCPU and 1 GB") {
		t.Fatalf("expected fixed shape default error, got %s", response.Body.String())
	}
}
