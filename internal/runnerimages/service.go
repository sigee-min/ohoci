package runnerimages

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"ohoci/internal/oci"
	"ohoci/internal/ociruntime"
	"ohoci/internal/store"
)

var bakeResultPattern = regexp.MustCompile(`OHOCI_IMAGE_BAKE_RESULT_BEGIN\s*(\{.*?\})\s*OHOCI_IMAGE_BAKE_RESULT_END`)
var bakePhasePattern = regexp.MustCompile(`OHOCI_IMAGE_BAKE_PHASE:([a-z_]+)`)

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type Preflight struct {
	Ready   bool     `json:"ready"`
	Blocked bool     `json:"blocked"`
	Status  string   `json:"status"`
	Summary string   `json:"summary"`
	Missing []string `json:"missing,omitempty"`
	Checks  []Check  `json:"checks,omitempty"`
}

type DiscoveredResource struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Name     string            `json:"name"`
	Status   string            `json:"status"`
	Tags     map[string]string `json:"tags,omitempty"`
	Tracked  bool              `json:"tracked"`
	BuildID  string            `json:"buildId,omitempty"`
	RecipeID string            `json:"recipeId,omitempty"`
}

type ImageSelection struct {
	Name           string `json:"name"`
	ImageReference string `json:"imageReference"`
	RecipeName     string `json:"recipeName,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

type BuildView struct {
	store.RunnerImageBuild
	Summary    string `json:"summary,omitempty"`
	LogExcerpt string `json:"logExcerpt,omitempty"`
	CanPromote bool   `json:"canPromote"`
	Promoted   bool   `json:"promoted"`
}

type Snapshot struct {
	Preflight     Preflight                 `json:"preflight"`
	Recipes       []store.RunnerImageRecipe `json:"recipes"`
	Builds        []BuildView               `json:"builds"`
	Resources     []DiscoveredResource      `json:"resources"`
	DefaultImage  *ImageSelection           `json:"defaultImage,omitempty"`
	PromotedImage *ImageSelection           `json:"promotedImage,omitempty"`
}

type RecipeInput struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	BaseImageOCID    string   `json:"baseImageOcid"`
	SubnetOCID       string   `json:"subnetOcid"`
	Shape            string   `json:"shape"`
	OCPU             int      `json:"ocpu"`
	MemoryGB         int      `json:"memoryGb"`
	ImageDisplayName string   `json:"imageDisplayName"`
	SetupCommands    []string `json:"setupCommands"`
	VerifyCommands   []string `json:"verifyCommands"`
}

type RunResult struct {
	Checked int `json:"checked"`
	Updated int `json:"updated"`
}

type Service struct {
	store   *store.Store
	oci     oci.Controller
	runtime *ociruntime.Service
	now     func() time.Time
}

func New(s *store.Store, ociClient oci.Controller, runtime *ociruntime.Service) *Service {
	return &Service{
		store:   s,
		oci:     ociClient,
		runtime: runtime,
		now:     time.Now,
	}
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	recipes, err := s.store.ListRunnerImageRecipes(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	builds, err := s.store.ListRunnerImageBuilds(ctx, 100)
	if err != nil {
		return Snapshot{}, err
	}
	preflight, defaultImage, promotedImage, err := s.preflight(ctx, recipes)
	if err != nil {
		return Snapshot{}, err
	}
	resources := []DiscoveredResource{}
	if preflight.Ready {
		resources, err = s.discovery(ctx, builds)
		if err != nil {
			return Snapshot{}, err
		}
	}
	items := make([]BuildView, 0, len(builds))
	for _, build := range builds {
		items = append(items, buildViewFromRecord(build))
	}
	return Snapshot{
		Preflight:     preflight,
		Recipes:       recipes,
		Builds:        items,
		Resources:     resources,
		DefaultImage:  defaultImage,
		PromotedImage: promotedImage,
	}, nil
}

func (s *Service) SaveRecipe(ctx context.Context, id int64, input RecipeInput) (store.RunnerImageRecipe, error) {
	recipe, err := normalizeRecipeInput(input)
	if err != nil {
		return store.RunnerImageRecipe{}, err
	}
	if id > 0 {
		return s.store.UpdateRunnerImageRecipe(ctx, id, recipe)
	}
	return s.store.CreateRunnerImageRecipe(ctx, recipe)
}

func (s *Service) DeleteRecipe(ctx context.Context, id int64) error {
	return s.store.DeleteRunnerImageRecipe(ctx, id)
}

func (s *Service) CreateBuild(ctx context.Context, recipeID int64) (store.RunnerImageBuild, error) {
	recipe, err := s.store.FindRunnerImageRecipeByID(ctx, recipeID)
	if err != nil {
		return store.RunnerImageBuild{}, err
	}
	build, err := s.store.CreateRunnerImageBuild(ctx, store.RunnerImageBuild{
		RecipeID:         recipe.ID,
		RecipeName:       recipe.Name,
		Status:           "queued",
		StatusMessage:    "Build queued.",
		BaseImageOCID:    recipe.BaseImageOCID,
		SubnetOCID:       recipe.SubnetOCID,
		Shape:            recipe.Shape,
		OCPU:             recipe.OCPU,
		MemoryGB:         recipe.MemoryGB,
		ImageDisplayName: recipe.ImageDisplayName,
		SetupCommands:    recipe.SetupCommands,
		VerifyCommands:   recipe.VerifyCommands,
	})
	if err != nil {
		return store.RunnerImageBuild{}, err
	}
	_, _ = s.RunOnce(ctx)
	return s.store.FindRunnerImageBuildByID(ctx, build.ID)
}

func (s *Service) PromoteBuild(ctx context.Context, buildID int64) (store.RunnerImageBuild, error) {
	build, err := s.store.FindRunnerImageBuildByID(ctx, buildID)
	if err != nil {
		return store.RunnerImageBuild{}, err
	}
	if strings.TrimSpace(build.ImageOCID) == "" {
		return store.RunnerImageBuild{}, fmt.Errorf("build does not have a captured image yet")
	}
	status, err := s.runtime.CurrentStatus(ctx)
	if err != nil {
		return store.RunnerImageBuild{}, err
	}
	_, err = s.runtime.Save(ctx, ociruntime.Input{
		CompartmentOCID:    status.EffectiveSettings.CompartmentOCID,
		AvailabilityDomain: status.EffectiveSettings.AvailabilityDomain,
		SubnetOCID:         status.EffectiveSettings.SubnetOCID,
		NSGOCIDs:           append([]string(nil), status.EffectiveSettings.NSGOCIDs...),
		ImageOCID:          build.ImageOCID,
		AssignPublicIP:     status.EffectiveSettings.AssignPublicIP,
	})
	if err != nil {
		return store.RunnerImageBuild{}, err
	}
	now := s.now().UTC()
	build.PromotedAt = &now
	build.Status = "promoted"
	build.StatusMessage = "Image promoted to the default runtime."
	if err := s.store.UpdateRunnerImageBuild(ctx, build); err != nil {
		return store.RunnerImageBuild{}, err
	}
	if err := s.store.MarkRunnerImageRecipePromoted(ctx, build.RecipeID, build.ID, build.ImageOCID); err != nil {
		return store.RunnerImageBuild{}, err
	}
	return s.store.FindRunnerImageBuildByID(ctx, build.ID)
}

func (s *Service) RunOnce(ctx context.Context) (RunResult, error) {
	builds, err := s.store.ListPendingRunnerImageBuilds(ctx, 100)
	if err != nil {
		return RunResult{}, err
	}
	result := RunResult{Checked: len(builds)}
	for _, build := range builds {
		updated, err := s.reconcileBuild(ctx, build)
		if err != nil {
			build.Status = "failed"
			build.ErrorMessage = err.Error()
			now := s.now().UTC()
			build.CompletedAt = &now
			_ = s.store.UpdateRunnerImageBuild(ctx, build)
			result.Updated++
			continue
		}
		if updated {
			result.Updated++
		}
	}
	return result, nil
}

func (s *Service) reconcileBuild(ctx context.Context, build store.RunnerImageBuild) (bool, error) {
	switch build.Status {
	case "queued":
		return s.startBuild(ctx, build)
	case "available", "failed", "promoted":
		return false, nil
	default:
		return s.advanceBuild(ctx, build)
	}
}

func (s *Service) startBuild(ctx context.Context, build store.RunnerImageBuild) (bool, error) {
	runtime, err := s.runtime.ResolveRuntimeConfig(ctx)
	if err != nil {
		return false, err
	}
	subnetID := strings.TrimSpace(build.SubnetOCID)
	if subnetID == "" {
		subnetID = strings.TrimSpace(runtime.SubnetID)
	}
	userData := oci.BuildRunnerImageBakeCloudInit(oci.RunnerImageBakeCloudInitInput{
		SetupCommands:  build.SetupCommands,
		VerifyCommands: build.VerifyCommands,
	})
	tags := oci.BuildManagedTags("", oci.ManagedTagInput{
		ResourceKind: "runner_image_bake_instance",
		RecipeID:     build.RecipeID,
		RecipeName:   build.RecipeName,
		BuildID:      build.ID,
	})
	instance, err := s.oci.LaunchInstance(ctx, oci.LaunchRequest{
		DisplayName:  bakeDisplayName(build),
		SubnetID:     subnetID,
		ImageID:      build.BaseImageOCID,
		Shape:        build.Shape,
		OCPU:         build.OCPU,
		MemoryGB:     build.MemoryGB,
		UserData:     userData,
		FreeformTags: tags.Freeform,
		DefinedTags:  tags.Defined,
	})
	if err != nil {
		return false, err
	}
	now := s.now().UTC()
	build.Status = "launching"
	build.StatusMessage = "Bake instance launched."
	build.InstanceOCID = instance.ID
	build.LaunchedAt = &now
	return true, s.store.UpdateRunnerImageBuild(ctx, build)
}

func (s *Service) advanceBuild(ctx context.Context, build store.RunnerImageBuild) (bool, error) {
	instance, err := s.oci.GetInstance(ctx, build.InstanceOCID)
	if err != nil {
		return false, err
	}
	consoleOutput, _ := s.oci.CaptureConsoleOutput(ctx, build.InstanceOCID)
	phase := parseBakePhase(consoleOutput)
	if phase != "" && build.Status != phase {
		build.Status = phase
		build.StatusMessage = "Bake phase updated."
		if err := s.store.UpdateRunnerImageBuild(ctx, build); err != nil {
			return false, err
		}
	}
	result, hasResult := parseBakeResult(consoleOutput)
	if hasResult && !result.Success {
		build.Status = "failed"
		build.ErrorMessage = result.Summary
		build.StatusMessage = "Bake verification failed."
		now := s.now().UTC()
		build.CompletedAt = &now
		_ = s.oci.TerminateInstance(ctx, build.InstanceOCID)
		return true, s.store.UpdateRunnerImageBuild(ctx, build)
	}
	if hasResult && result.Success && strings.TrimSpace(build.ImageOCID) == "" {
		tags := oci.BuildManagedTags("", oci.ManagedTagInput{
			ResourceKind: "runner_image",
			RecipeID:     build.RecipeID,
			RecipeName:   build.RecipeName,
			BuildID:      build.ID,
		})
		image, err := s.oci.CreateImage(ctx, oci.CreateImageRequest{
			InstanceID:   build.InstanceOCID,
			DisplayName:  capturedImageDisplayName(build),
			FreeformTags: tags.Freeform,
			DefinedTags:  tags.Defined,
		})
		if err != nil {
			return false, err
		}
		build.ImageOCID = image.ID
		build.Status = "creating_image"
		build.StatusMessage = result.Summary
		return true, s.store.UpdateRunnerImageBuild(ctx, build)
	}
	if strings.TrimSpace(build.ImageOCID) != "" {
		image, err := s.oci.GetImage(ctx, build.ImageOCID)
		if err != nil {
			return false, err
		}
		if strings.EqualFold(strings.TrimSpace(image.State), "AVAILABLE") {
			now := s.now().UTC()
			build.Status = "available"
			build.StatusMessage = "Runner image is ready."
			build.CompletedAt = &now
			_ = s.oci.TerminateInstance(ctx, build.InstanceOCID)
			return true, s.store.UpdateRunnerImageBuild(ctx, build)
		}
		build.Status = "creating_image"
		build.StatusMessage = fmt.Sprintf("Image state: %s", strings.ToLower(strings.TrimSpace(image.State)))
		return true, s.store.UpdateRunnerImageBuild(ctx, build)
	}
	if !hasResult && isTerminalInstanceState(instance.State) {
		now := s.now().UTC()
		build.Status = "failed"
		build.ErrorMessage = "Bake instance stopped before a success marker was recorded."
		build.CompletedAt = &now
		return true, s.store.UpdateRunnerImageBuild(ctx, build)
	}
	return false, nil
}

func (s *Service) preflight(ctx context.Context, recipes []store.RunnerImageRecipe) (Preflight, *ImageSelection, *ImageSelection, error) {
	status, err := s.runtime.CurrentStatus(ctx)
	if err != nil {
		return Preflight{}, nil, nil, err
	}
	checks := []Check{
		{Name: "OCI runtime", Status: ternaryStatus(status.Ready), Detail: strings.Join(status.Missing, ", ")},
		{Name: "Saved recipes", Status: ternaryStatus(len(recipes) > 0), Detail: strconv.Itoa(len(recipes))},
	}
	preflight := Preflight{
		Ready:   status.Ready,
		Blocked: !status.Ready,
		Status:  ternaryStatus(status.Ready),
		Summary: ternarySummary(status.Ready, "Runner image builds can launch.", "Complete OCI runtime setup before baking images."),
		Missing: append([]string(nil), status.Missing...),
		Checks:  checks,
	}
	defaultImage := &ImageSelection{
		Name:           "Current default image",
		ImageReference: status.EffectiveSettings.ImageOCID,
	}
	var promotedImage *ImageSelection
	for _, recipe := range recipes {
		if strings.TrimSpace(recipe.PromotedImageOCID) == "" {
			continue
		}
		promotedImage = &ImageSelection{
			Name:           recipe.ImageDisplayName,
			ImageReference: recipe.PromotedImageOCID,
			RecipeName:     recipe.Name,
			UpdatedAt:      recipe.UpdatedAt.UTC().Format(time.RFC3339),
		}
		break
	}
	return preflight, defaultImage, promotedImage, nil
}

func (s *Service) discovery(ctx context.Context, builds []store.RunnerImageBuild) ([]DiscoveredResource, error) {
	discovery, err := s.oci.DiscoverManagedResources(ctx)
	if err != nil {
		return nil, err
	}
	tracked := map[string]struct{}{}
	runners, err := s.store.ListRunners(ctx, 200)
	if err == nil {
		for _, runner := range runners {
			tracked[runner.InstanceOCID] = struct{}{}
		}
	}
	for _, build := range builds {
		if strings.TrimSpace(build.InstanceOCID) != "" {
			tracked[build.InstanceOCID] = struct{}{}
		}
		if strings.TrimSpace(build.ImageOCID) != "" {
			tracked[build.ImageOCID] = struct{}{}
		}
	}
	items := make([]DiscoveredResource, 0, len(discovery.Items))
	for _, item := range discovery.Items {
		_, isTracked := tracked[item.ID]
		items = append(items, DiscoveredResource{
			ID:       item.ID,
			Kind:     item.Kind,
			Name:     item.DisplayName,
			Status:   item.State,
			Tags:     item.Tags,
			Tracked:  isTracked,
			BuildID:  firstNonEmpty(item.Tags[oci.ManagedFreeformTagKeyBuildID], item.Tags[oci.ManagedDefinedTagKeyBuildID]),
			RecipeID: firstNonEmpty(item.Tags[oci.ManagedFreeformTagKeyRecipeID], item.Tags[oci.ManagedDefinedTagKeyRecipeID]),
		})
	}
	slices.SortFunc(items, func(a, b DiscoveredResource) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return items, nil
}

func normalizeRecipeInput(input RecipeInput) (store.RunnerImageRecipe, error) {
	recipe := store.RunnerImageRecipe{
		Name:             strings.TrimSpace(input.Name),
		Description:      strings.TrimSpace(input.Description),
		BaseImageOCID:    strings.TrimSpace(input.BaseImageOCID),
		SubnetOCID:       strings.TrimSpace(input.SubnetOCID),
		Shape:            strings.TrimSpace(input.Shape),
		OCPU:             input.OCPU,
		MemoryGB:         input.MemoryGB,
		ImageDisplayName: strings.TrimSpace(input.ImageDisplayName),
		SetupCommands:    normalizeCommands(input.SetupCommands),
		VerifyCommands:   normalizeCommands(input.VerifyCommands),
	}
	if recipe.Name == "" {
		return store.RunnerImageRecipe{}, fmt.Errorf("name is required")
	}
	if recipe.BaseImageOCID == "" {
		return store.RunnerImageRecipe{}, fmt.Errorf("baseImageOcid is required")
	}
	if recipe.Shape == "" {
		return store.RunnerImageRecipe{}, fmt.Errorf("shape is required")
	}
	if recipe.OCPU <= 0 || recipe.MemoryGB <= 0 {
		return store.RunnerImageRecipe{}, fmt.Errorf("ocpu and memoryGb must be positive")
	}
	if len(recipe.SetupCommands) == 0 {
		return store.RunnerImageRecipe{}, fmt.Errorf("setupCommands are required")
	}
	if len(recipe.VerifyCommands) == 0 {
		return store.RunnerImageRecipe{}, fmt.Errorf("verifyCommands are required")
	}
	if recipe.ImageDisplayName == "" {
		recipe.ImageDisplayName = "ohoci-" + recipe.Name
	}
	return recipe, nil
}

func normalizeCommands(values []string) []string {
	out := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func buildViewFromRecord(build store.RunnerImageBuild) BuildView {
	summary := strings.TrimSpace(build.StatusMessage)
	if strings.TrimSpace(build.ErrorMessage) != "" {
		summary = strings.TrimSpace(build.ErrorMessage)
	}
	return BuildView{
		RunnerImageBuild: build,
		Summary:          summary,
		LogExcerpt:       summary,
		CanPromote:       strings.EqualFold(strings.TrimSpace(build.Status), "available"),
		Promoted:         build.PromotedAt != nil || strings.EqualFold(strings.TrimSpace(build.Status), "promoted"),
	}
}

func bakeDisplayName(build store.RunnerImageBuild) string {
	if strings.TrimSpace(build.ImageDisplayName) != "" {
		return strings.TrimSpace(build.ImageDisplayName) + "-builder"
	}
	return fmt.Sprintf("ohoci-image-bake-%d", build.ID)
}

func capturedImageDisplayName(build store.RunnerImageBuild) string {
	if strings.TrimSpace(build.ImageDisplayName) != "" {
		return strings.TrimSpace(build.ImageDisplayName)
	}
	return fmt.Sprintf("ohoci-image-%d", build.ID)
}

type bakeResult struct {
	Success        bool   `json:"success"`
	Summary        string `json:"summary"`
	SetupExitCode  int    `json:"setupExitCode"`
	VerifyExitCode int    `json:"verifyExitCode"`
}

func parseBakeResult(value string) (bakeResult, bool) {
	matches := bakeResultPattern.FindStringSubmatch(value)
	if len(matches) != 2 {
		return bakeResult{}, false
	}
	var result bakeResult
	if err := json.Unmarshal([]byte(matches[1]), &result); err != nil {
		return bakeResult{}, false
	}
	return result, true
}

func parseBakePhase(value string) string {
	matches := bakePhasePattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return ""
	}
	switch strings.TrimSpace(matches[len(matches)-1][1]) {
	case "provisioning":
		return "provisioning"
	case "verifying":
		return "verifying"
	default:
		return ""
	}
}

func isTerminalInstanceState(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "STOPPED", "TERMINATED", "FAILED":
		return true
	default:
		return false
	}
}

func ternaryStatus(value bool) string {
	if value {
		return "ready"
	}
	return "blocked"
}

func ternarySummary(value bool, onTrue, onFalse string) string {
	if value {
		return onTrue
	}
	return onFalse
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
