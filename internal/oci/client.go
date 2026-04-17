package oci

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

type Config struct {
	AuthMode            string
	Runtime             RuntimeConfig
	BillingTagNamespace string
	RunnerDownloadBase  string
	RunnerVersion       string
	RunnerUser          string
	RunnerWorkDir       string
}

type LaunchRequest struct {
	DisplayName  string
	SubnetID     string
	ImageID      string
	Shape        string
	OCPU         int
	MemoryGB     int
	Spot         bool
	UserData     string
	FreeformTags map[string]string
	DefinedTags  map[string]string
}

type Instance struct {
	ID          string
	DisplayName string
	State       string
}

type Image struct {
	ID          string
	DisplayName string
	State       string
}

type CreateImageRequest struct {
	InstanceID   string
	DisplayName  string
	FreeformTags map[string]string
	DefinedTags  map[string]string
}

type ManagedResource struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	DisplayName string            `json:"displayName"`
	State       string            `json:"state"`
	Tags        map[string]string `json:"tags,omitempty"`
}

type ManagedResourceDiscovery struct {
	Items []ManagedResource `json:"items"`
}

type SubnetCandidate struct {
	ID                        string `json:"id"`
	DisplayName               string `json:"displayName"`
	CidrBlock                 string `json:"cidrBlock"`
	AvailabilityDomain        string `json:"availabilityDomain,omitempty"`
	ProhibitPublicIPOnVnic    bool   `json:"prohibitPublicIpOnVnic"`
	HasDefaultRouteToNAT      bool   `json:"hasDefaultRouteToNat"`
	HasDefaultRouteToInternet bool   `json:"hasDefaultRouteToInternet"`
	IsCurrentDefault          bool   `json:"isCurrentDefault"`
	IsRecommended             bool   `json:"isRecommended"`
	Recommendation            string `json:"recommendation"`
}

type RuntimeConfig struct {
	CompartmentID      string
	AvailabilityDomain string
	SubnetID           string
	NSGIDs             []string
	ImageID            string
	AssignPublicIP     bool
}

type Controller interface {
	LaunchInstance(ctx context.Context, req LaunchRequest) (Instance, error)
	GetInstance(ctx context.Context, instanceID string) (Instance, error)
	TerminateInstance(ctx context.Context, instanceID string) error
	CreateImage(ctx context.Context, req CreateImageRequest) (Image, error)
	GetImage(ctx context.Context, imageID string) (Image, error)
	CaptureConsoleOutput(ctx context.Context, instanceID string) (string, error)
	DiscoverManagedResources(ctx context.Context) (ManagedResourceDiscovery, error)
	ListSubnetCandidates(ctx context.Context) ([]SubnetCandidate, error)
	ListCatalog(ctx context.Context, req CatalogRequest) (CatalogResponse, error)
}

type ProviderResolver interface {
	ResolveProvider(ctx context.Context) (common.ConfigurationProvider, bool, error)
}

type RuntimeResolver interface {
	ResolveRuntimeConfig(ctx context.Context) (RuntimeConfig, error)
}

type OCIController struct {
	cfg                       Config
	providerResolver          ProviderResolver
	runtimeResolver           RuntimeResolver
	fake                      *FakeController
	instancePrincipalProvider func() (common.ConfigurationProvider, error)
	computeClientFactory      func(common.ConfigurationProvider) (core.ComputeClient, error)
	networkClientFactory      func(common.ConfigurationProvider) (core.VirtualNetworkClient, error)
	identityClientFactory     func(common.ConfigurationProvider) (identity.IdentityClient, error)
}

func New(_ context.Context, cfg Config, resolver ProviderResolver, runtimeResolver RuntimeResolver) (Controller, error) {
	fake := &FakeController{
		Instances:       map[string]Instance{},
		defaultRuntime:  cfg.Runtime,
		runtimeResolver: runtimeResolver,
	}
	return &OCIController{
		cfg:                       cfg,
		providerResolver:          resolver,
		runtimeResolver:           runtimeResolver,
		fake:                      fake,
		instancePrincipalProvider: auth.InstancePrincipalConfigurationProvider,
		computeClientFactory:      core.NewComputeClientWithConfigurationProvider,
		networkClientFactory:      core.NewVirtualNetworkClientWithConfigurationProvider,
		identityClientFactory:     identity.NewIdentityClientWithConfigurationProvider,
	}, nil
}
