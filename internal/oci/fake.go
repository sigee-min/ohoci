package oci

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

type FakeController struct {
	Instances        map[string]Instance
	Images           map[string]Image
	ConsoleOutputs   map[string]string
	ManagedResources []ManagedResource
	LaunchRequests   []LaunchRequest
	defaultRuntime   RuntimeConfig
	runtimeResolver  RuntimeResolver
}

var fakeCatalogValidatedAt = time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)

func (f *FakeController) LaunchInstance(_ context.Context, req LaunchRequest) (Instance, error) {
	id := fmt.Sprintf("ocid1.instance.oc1..fake-%d", time.Now().UnixNano())
	instance := Instance{ID: id, DisplayName: req.DisplayName, State: "RUNNING"}
	if f.Instances == nil {
		f.Instances = map[string]Instance{}
	}
	if f.ConsoleOutputs == nil {
		f.ConsoleOutputs = map[string]string{}
	}
	f.LaunchRequests = append(f.LaunchRequests, req)
	f.Instances[id] = instance
	f.ManagedResources = appendManagedResource(f.ManagedResources, ManagedResource{
		ID:          id,
		Kind:        firstNonEmpty(req.FreeformTags[ManagedFreeformTagKeyResourceKind], "instance"),
		DisplayName: req.DisplayName,
		State:       instance.State,
		Tags:        req.FreeformTags,
	})
	if strings.Contains(req.UserData, "OHOCI_IMAGE_BAKE_RESULT_BEGIN") {
		instance.State = "STOPPED"
		f.Instances[id] = instance
		f.ConsoleOutputs[id] = fakeBakeConsoleOutput()
	}
	return instance, nil
}

func (f *FakeController) GetInstance(_ context.Context, instanceID string) (Instance, error) {
	instance, ok := f.Instances[instanceID]
	if !ok {
		return Instance{}, fmt.Errorf("instance not found")
	}
	return instance, nil
}

func (f *FakeController) TerminateInstance(_ context.Context, instanceID string) error {
	instance, ok := f.Instances[instanceID]
	if !ok {
		return nil
	}
	instance.State = "TERMINATED"
	f.Instances[instanceID] = instance
	f.ManagedResources = updateManagedResourceState(f.ManagedResources, instanceID, instance.State)
	return nil
}

func (f *FakeController) CreateImage(_ context.Context, req CreateImageRequest) (Image, error) {
	id := fmt.Sprintf("ocid1.image.oc1..fake-%d", time.Now().UnixNano())
	image := Image{ID: id, DisplayName: req.DisplayName, State: "AVAILABLE"}
	if f.Images == nil {
		f.Images = map[string]Image{}
	}
	f.Images[id] = image
	f.ManagedResources = appendManagedResource(f.ManagedResources, ManagedResource{
		ID:          id,
		Kind:        firstNonEmpty(req.FreeformTags[ManagedFreeformTagKeyResourceKind], "image"),
		DisplayName: req.DisplayName,
		State:       image.State,
		Tags:        req.FreeformTags,
	})
	return image, nil
}

func (f *FakeController) GetImage(_ context.Context, imageID string) (Image, error) {
	image, ok := f.Images[imageID]
	if !ok {
		return Image{}, fmt.Errorf("image not found")
	}
	return image, nil
}

func (f *FakeController) CaptureConsoleOutput(_ context.Context, instanceID string) (string, error) {
	if output, ok := f.ConsoleOutputs[instanceID]; ok {
		return output, nil
	}
	return "", nil
}

func (f *FakeController) DiscoverManagedResources(_ context.Context) (ManagedResourceDiscovery, error) {
	items := make([]ManagedResource, 0, len(f.ManagedResources))
	items = append(items, f.ManagedResources...)
	return ManagedResourceDiscovery{Items: items}, nil
}

func (f *FakeController) ListSubnetCandidates(ctx context.Context) ([]SubnetCandidate, error) {
	id := strings.TrimSpace(f.defaultRuntime.SubnetID)
	if f.runtimeResolver != nil {
		runtime, err := f.runtimeResolver.ResolveRuntimeConfig(ctx)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(runtime.SubnetID) != "" {
			id = strings.TrimSpace(runtime.SubnetID)
		}
	}
	if id == "" {
		id = "ocid1.subnet.oc1..fake"
	}
	return []SubnetCandidate{
		{
			ID:                     id,
			DisplayName:            "Fake default subnet",
			CidrBlock:              "10.0.0.0/24",
			ProhibitPublicIPOnVnic: true,
			HasDefaultRouteToNAT:   true,
			IsCurrentDefault:       true,
			IsRecommended:          true,
			Recommendation:         "Fake OCI mode default subnet",
		},
	}, nil
}

func (f *FakeController) ListCatalog(_ context.Context, req CatalogRequest) (CatalogResponse, error) {
	req, err := normalizeCatalogRequest(req)
	if err != nil {
		return CatalogResponse{}, err
	}

	availabilityDomains := []string{
		"AD-1",
		"AD-2",
	}
	if req.AvailabilityDomain != "" && !catalogAvailabilityDomainsContain(availabilityDomains, req.AvailabilityDomain) {
		return CatalogResponse{}, fmt.Errorf("selected availability domain %q is not available in compartment %s", req.AvailabilityDomain, req.CompartmentOCID)
	}

	subnets := []CatalogSubnet{
		{
			ID:          "ocid1.subnet.oc1..regional",
			DisplayName: "Regional Runner Subnet",
			CidrBlock:   "10.0.0.0/24",
		},
		{
			ID:                 "ocid1.subnet.oc1..ad1",
			DisplayName:        "AD-1 Runner Subnet",
			CidrBlock:          "10.0.1.0/24",
			AvailabilityDomain: "AD-1",
		},
		{
			ID:                 "ocid1.subnet.oc1..ad2",
			DisplayName:        "AD-2 Runner Subnet",
			CidrBlock:          "10.0.2.0/24",
			AvailabilityDomain: "AD-2",
		},
	}
	filteredSubnets := make([]CatalogSubnet, 0, len(subnets))
	for _, subnet := range subnets {
		if req.AvailabilityDomain != "" && subnet.AvailabilityDomain != "" && !strings.EqualFold(subnet.AvailabilityDomain, req.AvailabilityDomain) {
			continue
		}
		filteredSubnets = append(filteredSubnets, subnet)
	}
	if len(filteredSubnets) == 0 {
		return CatalogResponse{}, fmt.Errorf("no available subnets found for compartment %s%s", req.CompartmentOCID, catalogAvailabilityDomainSuffix(req.AvailabilityDomain))
	}
	if req.SubnetOCID != "" && !catalogSubnetsContain(filteredSubnets, req.SubnetOCID) {
		return CatalogResponse{}, fmt.Errorf("selected subnet %q is not available in compartment %s%s", req.SubnetOCID, req.CompartmentOCID, catalogAvailabilityDomainSuffix(req.AvailabilityDomain))
	}

	images := []CatalogImage{
		{
			ID:                     "ocid1.image.oc1..oraclelinux",
			DisplayName:            "Oracle Linux 9",
			OperatingSystem:        "Oracle Linux",
			OperatingSystemVersion: "9",
			TimeCreated:            fakeCatalogValidatedAt.Add(-48 * time.Hour),
		},
		{
			ID:                     "ocid1.image.oc1..ubuntu",
			DisplayName:            "Ubuntu 22.04",
			OperatingSystem:        "Canonical Ubuntu",
			OperatingSystemVersion: "22.04",
			TimeCreated:            fakeCatalogValidatedAt.Add(-24 * time.Hour),
		},
	}
	for _, image := range f.Images {
		images = append(images, CatalogImage{
			ID:                     image.ID,
			DisplayName:            image.DisplayName,
			OperatingSystem:        "Custom",
			OperatingSystemVersion: "Prepared",
			TimeCreated:            fakeCatalogValidatedAt.Add(-12 * time.Hour),
		})
	}
	slices.SortFunc(images, func(a, b CatalogImage) int {
		switch {
		case a.TimeCreated.After(b.TimeCreated):
			return -1
		case a.TimeCreated.Before(b.TimeCreated):
			return 1
		}
		return strings.Compare(strings.ToLower(a.DisplayName), strings.ToLower(b.DisplayName))
	})
	if req.ImageOCID != "" && !catalogImagesContain(images, req.ImageOCID) {
		return CatalogResponse{}, fmt.Errorf("selected image %q is not available in compartment %s", req.ImageOCID, req.CompartmentOCID)
	}

	shapes := []CatalogShape{}
	if req.AvailabilityDomain != "" && req.ImageOCID != "" {
		shapes = []CatalogShape{
			{
				Shape:                  "VM.Standard.E2.1.Micro",
				ProcessorDescription:   "Intel Xeon",
				IsFlexible:             false,
				DefaultOCPU:            1,
				DefaultMemoryGB:        1,
				OCPUMin:                1,
				OCPUMax:                1,
				MemoryMinGB:            1,
				MemoryMaxGB:            1,
				MemoryDefaultPerOCPUGB: 1,
				MemoryMinPerOCPUGB:     1,
				MemoryMaxPerOCPUGB:     1,
			},
			{
				Shape:                  "VM.Standard.E4.Flex",
				ProcessorDescription:   "AMD EPYC",
				IsFlexible:             true,
				DefaultOCPU:            1,
				DefaultMemoryGB:        16,
				OCPUMin:                1,
				OCPUMax:                8,
				MemoryMinGB:            16,
				MemoryMaxGB:            128,
				MemoryDefaultPerOCPUGB: 16,
				MemoryMinPerOCPUGB:     1,
				MemoryMaxPerOCPUGB:     64,
			},
		}
	}

	return CatalogResponse{
		AvailabilityDomains: availabilityDomains,
		Subnets:             filteredSubnets,
		Images:              images,
		Shapes:              shapes,
		SourceRegion:        "fake-region-1",
		ValidatedAt:         fakeCatalogValidatedAt,
	}, nil
}

func appendManagedResource(items []ManagedResource, item ManagedResource) []ManagedResource {
	for index, existing := range items {
		if existing.ID != item.ID {
			continue
		}
		items[index] = item
		return items
	}
	return append(items, item)
}

func updateManagedResourceState(items []ManagedResource, id, state string) []ManagedResource {
	for index, item := range items {
		if item.ID != id {
			continue
		}
		item.State = state
		items[index] = item
		return items
	}
	return items
}

func fakeBakeConsoleOutput() string {
	return strings.Join([]string{
		"OHOCI_IMAGE_BAKE_PHASE:provisioning",
		"OHOCI_IMAGE_BAKE_PHASE:verifying",
		"OHOCI_IMAGE_BAKE_RESULT_BEGIN",
		`{"success":true,"summary":"setup and verify commands passed","setupExitCode":0,"verifyExitCode":0}`,
		"OHOCI_IMAGE_BAKE_RESULT_END",
	}, "\n")
}
