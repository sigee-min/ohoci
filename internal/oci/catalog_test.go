package oci

import (
	"context"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func TestListCatalogUsesFakeModeFixtures(t *testing.T) {
	controller, err := New(context.Background(), Config{AuthMode: "fake"}, nil, nil)
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}

	catalog, err := controller.ListCatalog(context.Background(), CatalogRequest{
		CompartmentOCID:    "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		ImageOCID:          "ocid1.image.oc1..ubuntu",
		SubnetOCID:         "ocid1.subnet.oc1..ad1",
	})
	if err != nil {
		t.Fatalf("list catalog: %v", err)
	}
	if catalog.SourceRegion != "fake-region-1" {
		t.Fatalf("expected fake source region, got %q", catalog.SourceRegion)
	}
	if !catalog.ValidatedAt.Equal(fakeCatalogValidatedAt) {
		t.Fatalf("expected deterministic fake validatedAt, got %s", catalog.ValidatedAt)
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
		t.Fatalf("unexpected subnet filtering: %#v", catalog.Subnets)
	}
}

func TestCatalogShapeFromOCIFlexibleBounds(t *testing.T) {
	shape := catalogShapeFromOCI(core.Shape{
		Shape:                common.String("VM.Standard.E4.Flex"),
		ProcessorDescription: common.String("AMD EPYC"),
		IsFlexible:           common.Bool(true),
		Ocpus:                common.Float32(1),
		MemoryInGBs:          common.Float32(16),
		OcpuOptions: &core.ShapeOcpuOptions{
			Min: common.Float32(1),
			Max: common.Float32(8),
		},
		MemoryOptions: &core.ShapeMemoryOptions{
			MinInGBs:            common.Float32(16),
			MaxInGBs:            common.Float32(128),
			DefaultPerOcpuInGBs: common.Float32(16),
			MinPerOcpuInGBs:     common.Float32(1),
			MaxPerOcpuInGBs:     common.Float32(64),
		},
	})

	if !shape.IsFlexible {
		t.Fatalf("expected flexible shape")
	}
	if shape.ProcessorDescription != "AMD EPYC" {
		t.Fatalf("unexpected processor description: %#v", shape)
	}
	if shape.OCPUMin != 1 || shape.OCPUMax != 8 {
		t.Fatalf("unexpected OCPU bounds: %#v", shape)
	}
	if shape.MemoryMinGB != 16 || shape.MemoryMaxGB != 128 {
		t.Fatalf("unexpected memory bounds: %#v", shape)
	}
	if shape.MemoryMinPerOCPUGB != 1 || shape.MemoryMaxPerOCPUGB != 64 {
		t.Fatalf("unexpected per-OCPU bounds: %#v", shape)
	}
}

func TestDeriveRunnerArchFromProcessorDescription(t *testing.T) {
	cases := []struct {
		name        string
		description string
		expected    string
		wantErr     bool
	}{
		{name: "ampere", description: "Ampere Altra", expected: "arm64"},
		{name: "arm", description: "Arm Neoverse", expected: "arm64"},
		{name: "intel", description: "Intel Xeon", expected: "x64"},
		{name: "amd", description: "AMD EPYC", expected: "x64"},
		{name: "empty", description: "", wantErr: true},
		{name: "unknown", description: "Mystery Processor", wantErr: true},
		{name: "ambiguous", description: "Ampere and AMD", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arch, err := DeriveRunnerArchFromProcessorDescription(tc.description)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got arch %q", arch)
				}
				return
			}
			if err != nil {
				t.Fatalf("derive runner arch: %v", err)
			}
			if arch != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, arch)
			}
		})
	}
}
