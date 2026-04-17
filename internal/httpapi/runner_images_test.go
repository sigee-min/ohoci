package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"ohoci/internal/runnerimages"
	"ohoci/internal/store"
)

func TestRunnerImagesEndpointsSupportBakeDiscoveryAndPromote(t *testing.T) {
	ctx := context.Background()
	handler, cfg, db, controller := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..ad1",
		ImageOCID:          "ocid1.image.oc1..ubuntu",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}

	recipeResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/runner-images/recipes", runnerimages.RecipeInput{
		Name:             "node22",
		Description:      "Node runner image",
		BaseImageOCID:    "ocid1.image.oc1..node22-base",
		Shape:            "VM.Standard.E4.Flex",
		OCPU:             2,
		MemoryGB:         16,
		ImageDisplayName: "ohoci-node22",
		SetupCommands:    []string{"sudo apt-get update", "sudo apt-get install -y nodejs"},
		VerifyCommands:   []string{"node --version"},
	}, cfg.SessionCookieName, token)
	if recipeResponse.Code != http.StatusCreated {
		t.Fatalf("expected recipe create success, got %d: %s", recipeResponse.Code, recipeResponse.Body.String())
	}
	var recipe store.RunnerImageRecipe
	if err := json.Unmarshal(recipeResponse.Body.Bytes(), &recipe); err != nil {
		t.Fatalf("decode recipe: %v", err)
	}

	buildResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/runner-images/builds", map[string]any{
		"recipeId": recipe.ID,
	}, cfg.SessionCookieName, token)
	if buildResponse.Code != http.StatusCreated {
		t.Fatalf("expected build create success, got %d: %s", buildResponse.Code, buildResponse.Body.String())
	}
	var build store.RunnerImageBuild
	if err := json.Unmarshal(buildResponse.Body.Bytes(), &build); err != nil {
		t.Fatalf("decode build: %v", err)
	}

	for i := 0; i < 3; i++ {
		reconcileResponse := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/runner-images/reconcile", nil, cfg.SessionCookieName, token)
		if reconcileResponse.Code != http.StatusOK {
			t.Fatalf("expected reconcile success, got %d: %s", reconcileResponse.Code, reconcileResponse.Body.String())
		}
	}

	discoveryResponse := performJSONRequest(t, handler.handler, http.MethodGet, "/api/v1/runner-images/discovery", nil, cfg.SessionCookieName, token)
	if discoveryResponse.Code != http.StatusOK {
		t.Fatalf("expected discovery success, got %d: %s", discoveryResponse.Code, discoveryResponse.Body.String())
	}
	var discovery struct {
		Items []runnerimages.DiscoveredResource `json:"items"`
	}
	if err := json.Unmarshal(discoveryResponse.Body.Bytes(), &discovery); err != nil {
		t.Fatalf("decode discovery: %v", err)
	}
	if len(discovery.Items) < 2 {
		t.Fatalf("expected bake instance and image in discovery, got %#v", discovery.Items)
	}

	build, err := db.FindRunnerImageBuildByID(ctx, build.ID)
	if err != nil {
		t.Fatalf("find build after reconcile: %v", err)
	}
	if build.Status != "available" || build.ImageOCID == "" {
		t.Fatalf("expected available build with image, got %#v", build)
	}

	promoteResponse := performJSONRequest(
		t,
		handler.handler,
		http.MethodPost,
		"/api/v1/runner-images/builds/"+jsonNumber(build.ID)+"/promote",
		nil,
		cfg.SessionCookieName,
		token,
	)
	if promoteResponse.Code != http.StatusOK {
		t.Fatalf("expected promote success, got %d: %s", promoteResponse.Code, promoteResponse.Body.String())
	}
	runtimeSettings, err := db.FindOCIRuntimeSettings(ctx)
	if err != nil {
		t.Fatalf("find runtime settings: %v", err)
	}
	if runtimeSettings.ImageOCID != build.ImageOCID {
		t.Fatalf("expected promoted image to become runtime default, got %#v", runtimeSettings)
	}
	if len(controller.Images) == 0 {
		t.Fatalf("expected fake controller to capture an image")
	}
}

func jsonNumber(value int64) string {
	return strconv.FormatInt(value, 10)
}
