package runnerlaunch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ohoci/internal/cachecompat"
	"ohoci/internal/config"
	"ohoci/internal/githubapp"
	"ohoci/internal/oci"
	"ohoci/internal/ociruntime"
	"ohoci/internal/store"
)

type Service struct {
	config     config.Config
	store      *store.Store
	oci        oci.Controller
	ociRuntime *ociruntime.Service
}

type Input struct {
	Policy           store.Policy
	RepoOwner        string
	RepoName         string
	InstallationID   int64
	JobID            int64
	GitHubJobID      int64
	RunID            int64
	GitHubConfigID   int64
	GitHubConfigName string
	GitHubConfigTags []string
	RequestedLabels  []string
	Source           string
	WarmState        string
}

type Result struct {
	Runner   store.Runner `json:"runner"`
	Instance oci.Instance `json:"instance"`
}

func New(cfg config.Config, storeDB *store.Store, ociController oci.Controller, runtimeService *ociruntime.Service) *Service {
	return &Service{
		config:     cfg,
		store:      storeDB,
		oci:        ociController,
		ociRuntime: runtimeService,
	}
}

func (s *Service) Launch(ctx context.Context, githubClient *githubapp.Client, input Input) (Result, error) {
	if githubClient == nil {
		return Result{}, fmt.Errorf("github client is required")
	}
	runtimeSettings, err := s.currentRuntimeSettings(ctx)
	if err != nil {
		return Result{}, err
	}
	shape, err := s.selectShape(ctx, runtimeSettings, input.Policy.Shape)
	if err != nil {
		return Result{}, err
	}
	runnerArch, err := oci.DeriveRunnerArchFromProcessorDescription(shape.ProcessorDescription)
	if err != nil {
		return Result{}, fmt.Errorf("selected shape %q cannot be used for runners: %w", shape.Shape, err)
	}
	registrationToken, err := githubClient.CreateRepoRunnerToken(ctx, input.InstallationID, input.RepoOwner, input.RepoName)
	if err != nil {
		return Result{}, err
	}
	workflowJobID := input.GitHubJobID
	if workflowJobID <= 0 {
		workflowJobID = input.JobID
	}
	runnerName := runnerDisplayName(input.RepoOwner, input.RepoName, workflowJobID, input.Source)
	var cacheCompatInput *oci.CloudInitCacheCompatInput
	if runtimeSettings.CacheCompatEnabled && strings.TrimSpace(runtimeSettings.CacheBucketName) != "" {
		cacheCompatInput = &oci.CloudInitCacheCompatInput{
			UpstreamBaseURL: s.config.PublicBaseURL,
			SharedSecret:    cachecompat.DeriveSharedSecret(s.config.DataEncryptionKey, input.RepoOwner, input.RepoName, runnerName),
		}
	}
	userData := oci.BuildCloudInit(oci.CloudInitInput{
		RepoOwner:          input.RepoOwner,
		RepoName:           input.RepoName,
		RunnerName:         runnerName,
		RegistrationToken:  registrationToken.Token,
		Labels:             append([]string{"self-hosted"}, input.RequestedLabels...),
		RunnerDownloadBase: s.config.RunnerDownloadBaseURL,
		RunnerVersion:      s.config.RunnerVersion,
		RunnerArch:         runnerArch,
		RunnerUser:         s.config.RunnerUser,
		RunnerWorkDir:      s.config.RunnerWorkDirectory,
		CacheCompat:        cacheCompatInput,
	})
	billingTags := oci.BuildLaunchBillingTags(s.config.OCIBillingTagNamespace, oci.LaunchBillingTagInput{
		PolicyID:         input.Policy.ID,
		PolicyLabel:      input.Policy.Label,
		RepoOwner:        input.RepoOwner,
		RepoName:         input.RepoName,
		WorkflowJobID:    workflowJobID,
		WorkflowRunID:    input.RunID,
		RunnerName:       runnerName,
		GitHubConfigID:   input.GitHubConfigID,
		GitHubConfigName: input.GitHubConfigName,
		GitHubConfigTags: input.GitHubConfigTags,
	})
	instance, err := s.oci.LaunchInstance(ctx, oci.LaunchRequest{
		DisplayName:  runnerName,
		SubnetID:     effectivePolicySubnet(input.Policy, runtimeSettings.SubnetOCID),
		Shape:        input.Policy.Shape,
		OCPU:         input.Policy.OCPU,
		MemoryGB:     input.Policy.MemoryGB,
		Spot:         input.Policy.Spot,
		UserData:     userData,
		FreeformTags: billingTags.Freeform,
		DefinedTags:  billingTags.Defined,
	})
	if err != nil {
		return Result{}, err
	}
	ttlMinutes := input.Policy.TTLMinutes
	if strings.EqualFold(strings.TrimSpace(input.Source), "warm") && input.Policy.WarmTTLMinutes > 0 {
		ttlMinutes = input.Policy.WarmTTLMinutes
	}
	expiresAt := time.Now().UTC().Add(time.Duration(ttlMinutes) * time.Minute)
	runner := store.Runner{
		PolicyID:         input.Policy.ID,
		JobID:            input.JobID,
		InstallationID:   input.InstallationID,
		GitHubConfigID:   input.GitHubConfigID,
		GitHubConfigName: input.GitHubConfigName,
		GitHubConfigTags: append([]string(nil), input.GitHubConfigTags...),
		InstanceOCID:     instance.ID,
		RepoOwner:        input.RepoOwner,
		RepoName:         input.RepoName,
		RunnerName:       runnerName,
		Status:           "launching",
		Labels:           append([]string{"self-hosted"}, input.RequestedLabels...),
		Source:           firstNonEmpty(strings.TrimSpace(input.Source), "ondemand"),
		WarmState:        strings.TrimSpace(input.WarmState),
		ExpiresAt:        &expiresAt,
	}
	if strings.EqualFold(runner.Source, "warm") {
		runner.WarmPolicyID = &input.Policy.ID
		runner.WarmRepoOwner = input.RepoOwner
		runner.WarmRepoName = input.RepoName
	}
	record, err := s.store.CreateRunner(ctx, runner)
	if err != nil {
		return Result{}, err
	}
	return Result{Runner: record, Instance: instance}, nil
}

func (s *Service) currentRuntimeSettings(ctx context.Context) (store.OCIRuntimeSettings, error) {
	if s.ociRuntime == nil {
		return store.OCIRuntimeSettings{
			CompartmentOCID:    strings.TrimSpace(s.config.OCICompartmentID),
			AvailabilityDomain: strings.TrimSpace(s.config.OCIAvailabilityDomain),
			SubnetOCID:         strings.TrimSpace(s.config.OCISubnetID),
			NSGOCIDs:           append([]string(nil), s.config.OCINSGIDs...),
			ImageOCID:          strings.TrimSpace(s.config.OCIImageID),
			AssignPublicIP:     s.config.OCIAssignPublicIP,
		}, nil
	}
	status, err := s.ociRuntime.CurrentStatus(ctx)
	if err != nil {
		return store.OCIRuntimeSettings{}, err
	}
	if !status.Ready {
		return store.OCIRuntimeSettings{}, fmt.Errorf("runtime settings are not ready")
	}
	return status.EffectiveSettings, nil
}

func (s *Service) selectShape(ctx context.Context, runtimeSettings store.OCIRuntimeSettings, shapeName string) (oci.CatalogShape, error) {
	catalog, err := s.oci.ListCatalog(ctx, oci.CatalogRequest{
		CompartmentOCID:    runtimeSettings.CompartmentOCID,
		AvailabilityDomain: runtimeSettings.AvailabilityDomain,
		ImageOCID:          runtimeSettings.ImageOCID,
	})
	if err != nil {
		return oci.CatalogShape{}, err
	}
	shapeName = strings.TrimSpace(shapeName)
	for _, item := range catalog.Shapes {
		if strings.EqualFold(item.Shape, shapeName) {
			return item, nil
		}
	}
	return oci.CatalogShape{}, fmt.Errorf("selected shape %q is stale or unavailable", shapeName)
}

func effectivePolicySubnet(policy store.Policy, defaultSubnetID string) string {
	if subnetID := strings.TrimSpace(policy.SubnetOCID); subnetID != "" {
		return subnetID
	}
	return strings.TrimSpace(defaultSubnetID)
}

func runnerDisplayName(owner, repo string, jobID int64, source string) string {
	base := strings.ToLower(strings.ReplaceAll(owner+"-"+repo, "/", "-"))
	if strings.EqualFold(strings.TrimSpace(source), "warm") && jobID <= 0 {
		return fmt.Sprintf("ohoci-%s-warm-%d", base, time.Now().Unix())
	}
	return fmt.Sprintf("ohoci-%s-%d", base, jobID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
