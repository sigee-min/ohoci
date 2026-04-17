package ociruntime

import (
	"context"
	"strings"
	"testing"

	"ohoci/internal/oci"
	"ohoci/internal/store"
)

func TestRuntimeStatusUsesDefaultsAndOverride(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service := New(db, Defaults{
		CompartmentID:      "ocid1.compartment.oc1..env",
		AvailabilityDomain: "AD-1",
		SubnetID:           "ocid1.subnet.oc1..env",
		NSGIDs:             []string{"ocid1.nsg.oc1..env"},
		ImageID:            "ocid1.image.oc1..env",
		AssignPublicIP:     false,
	})

	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status: %v", err)
	}
	if !status.Ready || status.Source != "env" {
		t.Fatalf("expected ready env status, got %#v", status)
	}

	status, err = service.Save(ctx, Input{
		SubnetOCID:     "ocid1.subnet.oc1..cms",
		NSGOCIDs:       []string{"ocid1.nsg.oc1..cms", "ocid1.nsg.oc1..cms"},
		AssignPublicIP: true,
	})
	if err != nil {
		t.Fatalf("save status: %v", err)
	}
	if status.Source != "cms" {
		t.Fatalf("expected cms source, got %q", status.Source)
	}
	if status.EffectiveSettings.SubnetOCID != "ocid1.subnet.oc1..cms" {
		t.Fatalf("expected cms subnet override, got %q", status.EffectiveSettings.SubnetOCID)
	}
	if !status.EffectiveSettings.AssignPublicIP {
		t.Fatalf("expected assign public IP override")
	}
	if len(status.EffectiveSettings.NSGOCIDs) != 1 {
		t.Fatalf("expected normalized NSG IDs, got %#v", status.EffectiveSettings.NSGOCIDs)
	}
	if status.EffectiveSettings.CompartmentOCID != "ocid1.compartment.oc1..env" {
		t.Fatalf("expected env compartment fallback, got %q", status.EffectiveSettings.CompartmentOCID)
	}
}

func TestRuntimeStatusCanClearOverride(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service := New(db, Defaults{})
	if _, err := service.Save(ctx, Input{SubnetOCID: "ocid1.subnet.oc1..cms"}); err != nil {
		t.Fatalf("save override: %v", err)
	}
	if err := service.Clear(ctx); err != nil {
		t.Fatalf("clear override: %v", err)
	}
	status, err := service.CurrentStatus(ctx)
	if err != nil {
		t.Fatalf("current status: %v", err)
	}
	if status.Source != "env" {
		t.Fatalf("expected env source after clear, got %q", status.Source)
	}
	if status.OverrideSettings != nil {
		t.Fatalf("expected nil override after clear")
	}
}

func TestRuntimeSaveRejectsUnavailableCatalogValues(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service := New(db, Defaults{})
	service.SetCatalogController(&oci.FakeController{Instances: map[string]oci.Instance{}})

	_, err = service.Save(ctx, Input{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetOCID:         "ocid1.subnet.oc1..ad2",
		ImageOCID:          "ocid1.image.oc1..ubuntu",
	})
	if err == nil {
		t.Fatalf("expected catalog validation failure")
	}
	if !strings.Contains(err.Error(), "selected subnet") {
		t.Fatalf("expected selected subnet failure, got %v", err)
	}
}
