package runnerimages

import (
	"context"
	"strconv"
	"testing"

	"ohoci/internal/oci"
	"ohoci/internal/ociruntime"
	"ohoci/internal/store"
)

func TestSnapshotDiscoversManagedResourcesWithoutDatabaseState(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runtime := ociruntime.New(db, ociruntime.Defaults{})
	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..example",
		ImageOCID:          "ocid1.image.oc1..default",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}

	fake := &oci.FakeController{
		Instances: map[string]oci.Instance{},
		ManagedResources: []oci.ManagedResource{
			{
				ID:          "ocid1.image.oc1..managed",
				Kind:        "runner_image",
				DisplayName: "ohoci-node22",
				State:       "AVAILABLE",
				Tags: oci.BuildManagedTags("", oci.ManagedTagInput{
					ResourceKind: "runner_image",
					RecipeID:     7,
					RecipeName:   "node22",
					BuildID:      11,
				}).Freeform,
			},
		},
	}
	service := New(db, fake, runtime)

	snapshot, err := service.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !snapshot.Preflight.Ready {
		t.Fatalf("expected ready preflight, got %#v", snapshot.Preflight)
	}
	if len(snapshot.Resources) != 1 {
		t.Fatalf("expected 1 discovered resource, got %d", len(snapshot.Resources))
	}
	resource := snapshot.Resources[0]
	if resource.Tracked {
		t.Fatalf("expected resource to be discoverable without DB tracking")
	}
	if resource.BuildID != "11" || resource.RecipeID != "7" {
		t.Fatalf("expected ids from tags, got %#v", resource)
	}
	if snapshot.DefaultImage == nil || snapshot.DefaultImage.ImageReference != "ocid1.image.oc1..default" {
		t.Fatalf("expected current default image in snapshot, got %#v", snapshot.DefaultImage)
	}
}

func TestCreateBuildUsesRecipeBaseImageAndCreatesTaggedImage(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runtime := ociruntime.New(db, ociruntime.Defaults{})
	if _, err := db.SaveOCIRuntimeSettings(ctx, store.OCIRuntimeSettings{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..runtime",
		ImageOCID:          "ocid1.image.oc1..runtime-default",
	}); err != nil {
		t.Fatalf("save runtime settings: %v", err)
	}

	fake := &oci.FakeController{Instances: map[string]oci.Instance{}}
	service := New(db, fake, runtime)
	recipe, err := service.SaveRecipe(ctx, 0, RecipeInput{
		Name:             "node22",
		BaseImageOCID:    "ocid1.image.oc1..node22-base",
		Shape:            "VM.Standard.E4.Flex",
		OCPU:             2,
		MemoryGB:         16,
		ImageDisplayName: "ohoci-node22",
		SetupCommands:    []string{"sudo apt-get update", "sudo apt-get install -y nodejs"},
		VerifyCommands:   []string{"node --version"},
	})
	if err != nil {
		t.Fatalf("save recipe: %v", err)
	}

	build, err := service.CreateBuild(ctx, recipe.ID)
	if err != nil {
		t.Fatalf("create build: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := service.RunOnce(ctx); err != nil {
			t.Fatalf("run once %d: %v", i, err)
		}
	}

	build, err = db.FindRunnerImageBuildByID(ctx, build.ID)
	if err != nil {
		t.Fatalf("find build: %v", err)
	}
	if build.Status != "available" {
		t.Fatalf("expected available build, got %#v", build)
	}
	if build.ImageOCID == "" {
		t.Fatalf("expected captured image ocid, got %#v", build)
	}
	if len(fake.LaunchRequests) != 1 {
		t.Fatalf("expected one bake instance launch, got %d", len(fake.LaunchRequests))
	}
	if fake.LaunchRequests[0].ImageID != "ocid1.image.oc1..node22-base" {
		t.Fatalf("expected recipe base image to be used, got %#v", fake.LaunchRequests[0])
	}

	var foundImage bool
	for _, item := range fake.ManagedResources {
		if item.ID != build.ImageOCID {
			continue
		}
		foundImage = true
		if item.Tags[oci.ManagedFreeformTagKeyManaged] != "true" || item.Tags[oci.ManagedFreeformTagKeyController] != "ohoci" {
			t.Fatalf("expected tagged image resource, got %#v", item)
		}
		if item.Tags[oci.ManagedFreeformTagKeyBuildID] != strconv.FormatInt(build.ID, 10) || item.Tags[oci.ManagedFreeformTagKeyRecipeID] != strconv.FormatInt(recipe.ID, 10) {
			t.Fatalf("expected build and recipe ids on image tags, got %#v", item.Tags)
		}
	}
	if !foundImage {
		t.Fatalf("expected managed image resource for build %#v", build)
	}
}
