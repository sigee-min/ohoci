package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"ohoci/internal/oci"
	"ohoci/internal/store"
)

type staticCatalogController struct {
	*oci.FakeController
	catalog oci.CatalogResponse
	err     error
}

func newStaticCatalogController(catalog oci.CatalogResponse) *staticCatalogController {
	return &staticCatalogController{
		FakeController: &oci.FakeController{Instances: map[string]oci.Instance{}},
		catalog:        catalog,
	}
}

func (c *staticCatalogController) ListCatalog(_ context.Context, _ oci.CatalogRequest) (oci.CatalogResponse, error) {
	if c.err != nil {
		return oci.CatalogResponse{}, c.err
	}
	return c.catalog, nil
}

func TestPolicySaveRejectsShapeWithUnknownProcessorDescription(t *testing.T) {
	ctx := context.Background()
	controller := newStaticCatalogController(oci.CatalogResponse{
		Shapes: []oci.CatalogShape{
			{
				Shape:                "VM.Standard.Custom.Flex",
				ProcessorDescription: "Mystery Accelerator",
				IsFlexible:           true,
				OCPUMin:              1,
				OCPUMax:              4,
				MemoryMinGB:          8,
				MemoryMaxGB:          64,
			},
		},
	})
	handler, cfg, db, _ := newBackendTestHandler(t, backendTestOptions{
		githubDefaults: readyGitHubDefaults,
		ociController:  controller,
	})
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
		Shape:      "VM.Standard.Custom.Flex",
		OCPU:       2,
		MemoryGB:   16,
		MaxRunners: 1,
		TTLMinutes: 30,
		Enabled:    true,
	}, cfg.SessionCookieName, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown processor description, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "cannot be used for runners") {
		t.Fatalf("expected runner architecture validation error, got %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "processor description") {
		t.Fatalf("expected processor description detail, got %s", response.Body.String())
	}
}
