package oci

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

type CatalogRequest struct {
	CompartmentOCID    string `json:"compartmentOcid"`
	AvailabilityDomain string `json:"availabilityDomain,omitempty"`
	ImageOCID          string `json:"imageOcid,omitempty"`
	SubnetOCID         string `json:"subnetOcid,omitempty"`
}

type CatalogResponse struct {
	AvailabilityDomains []string        `json:"availabilityDomains"`
	Subnets             []CatalogSubnet `json:"subnets"`
	Images              []CatalogImage  `json:"images"`
	Shapes              []CatalogShape  `json:"shapes"`
	SourceRegion        string          `json:"sourceRegion"`
	ValidatedAt         time.Time       `json:"validatedAt"`
}

type CatalogSubnet struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"displayName"`
	CidrBlock          string `json:"cidrBlock"`
	AvailabilityDomain string `json:"availabilityDomain,omitempty"`
}

type CatalogImage struct {
	ID                     string    `json:"id"`
	DisplayName            string    `json:"displayName"`
	OperatingSystem        string    `json:"operatingSystem"`
	OperatingSystemVersion string    `json:"operatingSystemVersion"`
	TimeCreated            time.Time `json:"timeCreated"`
}

type CatalogShape struct {
	Shape                  string  `json:"shape"`
	ProcessorDescription   string  `json:"processorDescription,omitempty"`
	IsFlexible             bool    `json:"isFlexible"`
	DefaultOCPU            float32 `json:"defaultOcpu"`
	DefaultMemoryGB        float32 `json:"defaultMemoryGb"`
	OCPUMin                float32 `json:"ocpuMin"`
	OCPUMax                float32 `json:"ocpuMax"`
	MemoryMinGB            float32 `json:"memoryMinGb"`
	MemoryMaxGB            float32 `json:"memoryMaxGb"`
	MemoryDefaultPerOCPUGB float32 `json:"memoryDefaultPerOcpuGb"`
	MemoryMinPerOCPUGB     float32 `json:"memoryMinPerOcpuGb"`
	MemoryMaxPerOCPUGB     float32 `json:"memoryMaxPerOcpuGb"`
}

func (c *OCIController) ListCatalog(ctx context.Context, req CatalogRequest) (CatalogResponse, error) {
	req, err := normalizeCatalogRequest(req)
	if err != nil {
		return CatalogResponse{}, err
	}

	mode, provider, err := c.resolveProvider(ctx)
	if err != nil {
		return CatalogResponse{}, err
	}
	if mode == "fake" {
		return c.fake.ListCatalog(ctx, req)
	}

	sourceRegion, err := provider.Region()
	if err != nil {
		return CatalogResponse{}, fmt.Errorf("resolve source region: %w", err)
	}
	sourceRegion = strings.TrimSpace(sourceRegion)
	if sourceRegion == "" {
		return CatalogResponse{}, fmt.Errorf("OCI provider region is required for catalog discovery")
	}

	identityClient, err := c.identityClientFactory(provider)
	if err != nil {
		return CatalogResponse{}, fmt.Errorf("create OCI identity client: %w", err)
	}
	networkClient, err := c.networkClientFactory(provider)
	if err != nil {
		return CatalogResponse{}, fmt.Errorf("create OCI network client: %w", err)
	}
	computeClient, err := c.computeClientFactory(provider)
	if err != nil {
		return CatalogResponse{}, fmt.Errorf("create OCI compute client: %w", err)
	}

	tenancyOCID, err := provider.TenancyOCID()
	if err != nil {
		return CatalogResponse{}, fmt.Errorf("resolve tenancy: %w", err)
	}

	availabilityDomains, err := c.listCatalogAvailabilityDomains(ctx, identityClient, tenancyOCID)
	if err != nil {
		return CatalogResponse{}, fmt.Errorf("list availability domains: %w", err)
	}
	if len(availabilityDomains) == 0 {
		return CatalogResponse{}, fmt.Errorf("no availability domains found for compartment %s", req.CompartmentOCID)
	}
	if req.AvailabilityDomain != "" && !catalogAvailabilityDomainsContain(availabilityDomains, req.AvailabilityDomain) {
		return CatalogResponse{}, fmt.Errorf("selected availability domain %q is not available in compartment %s", req.AvailabilityDomain, req.CompartmentOCID)
	}

	subnets, err := c.listCatalogSubnets(ctx, networkClient, req)
	if err != nil {
		return CatalogResponse{}, fmt.Errorf("list subnets: %w", err)
	}
	if len(subnets) == 0 {
		return CatalogResponse{}, fmt.Errorf("no available subnets found for compartment %s%s", req.CompartmentOCID, catalogAvailabilityDomainSuffix(req.AvailabilityDomain))
	}
	if req.SubnetOCID != "" && !catalogSubnetsContain(subnets, req.SubnetOCID) {
		return CatalogResponse{}, fmt.Errorf("selected subnet %q is not available in compartment %s%s", req.SubnetOCID, req.CompartmentOCID, catalogAvailabilityDomainSuffix(req.AvailabilityDomain))
	}

	images, err := c.listCatalogImages(ctx, computeClient, req.CompartmentOCID)
	if err != nil {
		return CatalogResponse{}, fmt.Errorf("list images: %w", err)
	}
	if len(images) == 0 {
		return CatalogResponse{}, fmt.Errorf("no available images found for compartment %s", req.CompartmentOCID)
	}
	if req.ImageOCID != "" && !catalogImagesContain(images, req.ImageOCID) {
		return CatalogResponse{}, fmt.Errorf("selected image %q is not available in compartment %s", req.ImageOCID, req.CompartmentOCID)
	}

	shapes := []CatalogShape{}
	if req.AvailabilityDomain != "" && req.ImageOCID != "" {
		shapes, err = c.listCatalogShapes(ctx, computeClient, req)
		if err != nil {
			return CatalogResponse{}, fmt.Errorf("list shapes: %w", err)
		}
		if len(shapes) == 0 {
			return CatalogResponse{}, fmt.Errorf("no compatible shapes found for compartment %s%s%s", req.CompartmentOCID, catalogAvailabilityDomainSuffix(req.AvailabilityDomain), catalogImageSuffix(req.ImageOCID))
		}
	}

	return CatalogResponse{
		AvailabilityDomains: availabilityDomains,
		Subnets:             subnets,
		Images:              images,
		Shapes:              shapes,
		SourceRegion:        sourceRegion,
		ValidatedAt:         time.Now().UTC(),
	}, nil
}

func normalizeCatalogRequest(req CatalogRequest) (CatalogRequest, error) {
	req.CompartmentOCID = strings.TrimSpace(req.CompartmentOCID)
	req.AvailabilityDomain = strings.TrimSpace(req.AvailabilityDomain)
	req.ImageOCID = strings.TrimSpace(req.ImageOCID)
	req.SubnetOCID = strings.TrimSpace(req.SubnetOCID)
	if req.CompartmentOCID == "" {
		return CatalogRequest{}, fmt.Errorf("compartmentOcid is required")
	}
	return req, nil
}

func (c *OCIController) listCatalogAvailabilityDomains(ctx context.Context, client identity.IdentityClient, tenancyOCID string) ([]string, error) {
	response, err := client.ListAvailabilityDomains(ctx, identity.ListAvailabilityDomainsRequest{
		CompartmentId: common.String(strings.TrimSpace(tenancyOCID)),
	})
	if err != nil {
		return nil, err
	}

	items := make([]string, 0, len(response.Items))
	for _, item := range response.Items {
		name := valueOrEmpty(item.Name)
		if name == "" {
			continue
		}
		items = append(items, name)
	}
	slices.SortFunc(items, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return items, nil
}

func (c *OCIController) listCatalogSubnets(ctx context.Context, client core.VirtualNetworkClient, req CatalogRequest) ([]CatalogSubnet, error) {
	request := core.ListSubnetsRequest{
		CompartmentId:  common.String(req.CompartmentOCID),
		LifecycleState: core.SubnetLifecycleStateAvailable,
		SortBy:         core.ListSubnetsSortByDisplayname,
		SortOrder:      core.ListSubnetsSortOrderAsc,
		Limit:          common.Int(100),
	}

	items := []CatalogSubnet{}
	for {
		response, err := client.ListSubnets(ctx, request)
		if err != nil {
			return nil, err
		}
		for _, subnet := range response.Items {
			if !catalogSubnetMatchesAvailabilityDomain(req.AvailabilityDomain, subnet) {
				continue
			}
			items = append(items, CatalogSubnet{
				ID:                 valueOrEmpty(subnet.Id),
				DisplayName:        valueOrEmpty(subnet.DisplayName),
				CidrBlock:          valueOrEmpty(subnet.CidrBlock),
				AvailabilityDomain: valueOrEmpty(subnet.AvailabilityDomain),
			})
		}
		if response.OpcNextPage == nil || strings.TrimSpace(*response.OpcNextPage) == "" {
			break
		}
		request.Page = response.OpcNextPage
	}
	return items, nil
}

func (c *OCIController) listCatalogImages(ctx context.Context, client core.ComputeClient, compartmentOCID string) ([]CatalogImage, error) {
	request := core.ListImagesRequest{
		CompartmentId:  common.String(strings.TrimSpace(compartmentOCID)),
		LifecycleState: core.ImageLifecycleStateAvailable,
		SortBy:         core.ListImagesSortByTimecreated,
		SortOrder:      core.ListImagesSortOrderDesc,
		Limit:          common.Int(100),
	}

	items := []CatalogImage{}
	for {
		response, err := client.ListImages(ctx, request)
		if err != nil {
			return nil, err
		}
		for _, image := range response.Items {
			items = append(items, CatalogImage{
				ID:                     valueOrEmpty(image.Id),
				DisplayName:            valueOrEmpty(image.DisplayName),
				OperatingSystem:        valueOrEmpty(image.OperatingSystem),
				OperatingSystemVersion: valueOrEmpty(image.OperatingSystemVersion),
				TimeCreated:            sdkTimeValue(image.TimeCreated),
			})
		}
		if response.OpcNextPage == nil || strings.TrimSpace(*response.OpcNextPage) == "" {
			break
		}
		request.Page = response.OpcNextPage
	}

	slices.SortFunc(items, func(a, b CatalogImage) int {
		switch {
		case a.TimeCreated.After(b.TimeCreated):
			return -1
		case a.TimeCreated.Before(b.TimeCreated):
			return 1
		}
		return strings.Compare(strings.ToLower(a.DisplayName+" "+a.ID), strings.ToLower(b.DisplayName+" "+b.ID))
	})
	return items, nil
}

func (c *OCIController) listCatalogShapes(ctx context.Context, client core.ComputeClient, req CatalogRequest) ([]CatalogShape, error) {
	request := core.ListShapesRequest{
		CompartmentId: common.String(req.CompartmentOCID),
		Limit:         common.Int(100),
	}
	if req.AvailabilityDomain != "" {
		request.AvailabilityDomain = common.String(req.AvailabilityDomain)
	}
	if req.ImageOCID != "" {
		request.ImageId = common.String(req.ImageOCID)
	}

	items := []CatalogShape{}
	for {
		response, err := client.ListShapes(ctx, request)
		if err != nil {
			return nil, err
		}
		for _, shape := range response.Items {
			items = append(items, catalogShapeFromOCI(shape))
		}
		if response.OpcNextPage == nil || strings.TrimSpace(*response.OpcNextPage) == "" {
			break
		}
		request.Page = response.OpcNextPage
	}

	slices.SortFunc(items, func(a, b CatalogShape) int {
		return strings.Compare(strings.ToLower(a.Shape), strings.ToLower(b.Shape))
	})
	return items, nil
}

func catalogShapeFromOCI(shape core.Shape) CatalogShape {
	defaultOCPU := float32Value(shape.Ocpus)
	defaultMemoryGB := float32Value(shape.MemoryInGBs)
	ocpuMin := defaultOCPU
	ocpuMax := defaultOCPU
	if shape.OcpuOptions != nil {
		if value := float32Value(shape.OcpuOptions.Min); value > 0 {
			ocpuMin = value
		}
		if value := float32Value(shape.OcpuOptions.Max); value > 0 {
			ocpuMax = value
		}
	}

	memoryMinGB := defaultMemoryGB
	memoryMaxGB := defaultMemoryGB
	memoryDefaultPerOCPUGB := zeroIfNaNOrInf(memoryPerOCPU(defaultMemoryGB, defaultOCPU))
	memoryMinPerOCPUGB := memoryDefaultPerOCPUGB
	memoryMaxPerOCPUGB := memoryDefaultPerOCPUGB
	if shape.MemoryOptions != nil {
		if value := float32Value(shape.MemoryOptions.MinInGBs); value > 0 {
			memoryMinGB = value
		}
		if value := float32Value(shape.MemoryOptions.MaxInGBs); value > 0 {
			memoryMaxGB = value
		}
		if value := float32Value(shape.MemoryOptions.DefaultPerOcpuInGBs); value > 0 {
			memoryDefaultPerOCPUGB = value
		}
		if value := float32Value(shape.MemoryOptions.MinPerOcpuInGBs); value > 0 {
			memoryMinPerOCPUGB = value
		}
		if value := float32Value(shape.MemoryOptions.MaxPerOcpuInGBs); value > 0 {
			memoryMaxPerOCPUGB = value
		}
	}

	return CatalogShape{
		Shape:                  valueOrEmpty(shape.Shape),
		ProcessorDescription:   valueOrEmpty(shape.ProcessorDescription),
		IsFlexible:             pointerBool(shape.IsFlexible),
		DefaultOCPU:            defaultOCPU,
		DefaultMemoryGB:        defaultMemoryGB,
		OCPUMin:                ocpuMin,
		OCPUMax:                ocpuMax,
		MemoryMinGB:            memoryMinGB,
		MemoryMaxGB:            memoryMaxGB,
		MemoryDefaultPerOCPUGB: memoryDefaultPerOCPUGB,
		MemoryMinPerOCPUGB:     memoryMinPerOCPUGB,
		MemoryMaxPerOCPUGB:     memoryMaxPerOCPUGB,
	}
}

func DeriveRunnerArchFromProcessorDescription(processorDescription string) (string, error) {
	description := strings.TrimSpace(processorDescription)
	if description == "" {
		return "", fmt.Errorf("runner architecture could not be determined from OCI shape processor description: value is empty")
	}

	normalized := strings.ToLower(description)
	matchesArm := strings.Contains(normalized, "ampere") || strings.Contains(normalized, "arm")
	matchesX64 := strings.Contains(normalized, "intel") || strings.Contains(normalized, "amd")

	switch {
	case matchesArm && !matchesX64:
		return "arm64", nil
	case matchesX64 && !matchesArm:
		return "x64", nil
	case matchesArm && matchesX64:
		return "", fmt.Errorf("runner architecture could not be determined from OCI shape processor description %q: matched both ARM and x64 families", description)
	default:
		return "", fmt.Errorf("runner architecture could not be determined from OCI shape processor description %q", description)
	}
}

func catalogSubnetMatchesAvailabilityDomain(availabilityDomain string, subnet core.Subnet) bool {
	availabilityDomain = strings.TrimSpace(availabilityDomain)
	if availabilityDomain == "" {
		return true
	}
	subnetAvailabilityDomain := valueOrEmpty(subnet.AvailabilityDomain)
	return subnetAvailabilityDomain == "" || strings.EqualFold(subnetAvailabilityDomain, availabilityDomain)
}

func catalogAvailabilityDomainsContain(items []string, name string) bool {
	name = strings.TrimSpace(name)
	for _, item := range items {
		if strings.EqualFold(item, name) {
			return true
		}
	}
	return false
}

func catalogSubnetsContain(items []CatalogSubnet, subnetOCID string) bool {
	subnetOCID = strings.TrimSpace(subnetOCID)
	for _, item := range items {
		if strings.EqualFold(item.ID, subnetOCID) {
			return true
		}
	}
	return false
}

func catalogImagesContain(items []CatalogImage, imageOCID string) bool {
	imageOCID = strings.TrimSpace(imageOCID)
	for _, item := range items {
		if strings.EqualFold(item.ID, imageOCID) {
			return true
		}
	}
	return false
}

func catalogAvailabilityDomainSuffix(availabilityDomain string) string {
	if strings.TrimSpace(availabilityDomain) == "" {
		return ""
	}
	return fmt.Sprintf(" in availability domain %s", availabilityDomain)
}

func catalogImageSuffix(imageOCID string) string {
	if strings.TrimSpace(imageOCID) == "" {
		return ""
	}
	return fmt.Sprintf(" for image %s", imageOCID)
}

func sdkTimeValue(value *common.SDKTime) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.Time.UTC()
}

func float32Value(value *float32) float32 {
	if value == nil {
		return 0
	}
	return *value
}

func memoryPerOCPU(memoryGB, ocpu float32) float32 {
	if memoryGB <= 0 || ocpu <= 0 {
		return 0
	}
	return memoryGB / ocpu
}

func zeroIfNaNOrInf(value float32) float32 {
	if value != value {
		return 0
	}
	return value
}
