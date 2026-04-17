package httpapi

import (
	"context"
	"fmt"
	"strings"

	"ohoci/internal/oci"
	"ohoci/internal/store"
)

func validatePolicyAgainstRuntimeCatalog(ctx context.Context, deps Dependencies, policy store.Policy) error {
	if err := validateProductizedPolicy(policy); err != nil {
		return err
	}
	selectedShape, err := currentRuntimeCatalogShape(ctx, deps, policy.Shape, "saving policies")
	if err != nil {
		return err
	}
	if _, err := runnerArchForCatalogShape(selectedShape); err != nil {
		return err
	}
	if !selectedShape.IsFlexible {
		return validateFixedShapePolicy(policy, selectedShape)
	}
	return validateFlexibleShapePolicy(policy, selectedShape)
}

func validateProductizedPolicy(policy store.Policy) error {
	if policy.WarmMinIdle < 0 || policy.WarmMinIdle > 1 {
		return fmt.Errorf("warmMinIdle must be 0 or 1 in v1")
	}
	if policy.WarmEnabled && policy.WarmMinIdle > 0 && len(policy.WarmRepoAllowlist) == 0 {
		return fmt.Errorf("warmRepoAllowlist must include at least one repository when warm capacity is enabled")
	}
	if policy.BudgetWindowDays > 0 && policy.BudgetWindowDays != 7 {
		return fmt.Errorf("budgetWindowDays must stay fixed at 7 in v1")
	}
	return nil
}

func currentRuntimeCatalogShape(ctx context.Context, deps Dependencies, shapeName, action string) (oci.CatalogShape, error) {
	settings, err := currentReadyRuntimeSettings(ctx, deps, action)
	if err != nil {
		return oci.CatalogShape{}, err
	}

	catalog, err := deps.OCI.ListCatalog(ctx, oci.CatalogRequest{
		CompartmentOCID:    settings.CompartmentOCID,
		AvailabilityDomain: settings.AvailabilityDomain,
		ImageOCID:          settings.ImageOCID,
	})
	if err != nil {
		return oci.CatalogShape{}, fmt.Errorf("Settings must be fixed first before %s: %w", action, err)
	}
	return selectCatalogShape(shapeName, catalog.Shapes)
}

func currentReadyRuntimeSettings(ctx context.Context, deps Dependencies, action string) (store.OCIRuntimeSettings, error) {
	if deps.OCIRuntime == nil {
		return store.OCIRuntimeSettings{}, fmt.Errorf("OCI runtime service is not configured")
	}
	if deps.OCI == nil {
		return store.OCIRuntimeSettings{}, fmt.Errorf("OCI controller is not configured")
	}

	runtimeStatus, err := deps.OCIRuntime.CurrentStatus(ctx)
	if err != nil {
		return store.OCIRuntimeSettings{}, err
	}
	if !runtimeStatus.Ready {
		if len(runtimeStatus.Missing) == 0 {
			return store.OCIRuntimeSettings{}, fmt.Errorf("Settings must be fixed first before %s", action)
		}
		return store.OCIRuntimeSettings{}, fmt.Errorf("Settings must be fixed first before %s: missing %s", action, strings.Join(runtimeStatus.Missing, ", "))
	}
	return runtimeStatus.EffectiveSettings, nil
}

func runnerArchForCatalogShape(shape oci.CatalogShape) (string, error) {
	runnerArch, err := oci.DeriveRunnerArchFromProcessorDescription(shape.ProcessorDescription)
	if err != nil {
		return "", fmt.Errorf("selected shape %q cannot be used for runners: %w", shape.Shape, err)
	}
	return runnerArch, nil
}

func selectCatalogShape(shapeName string, shapes []oci.CatalogShape) (oci.CatalogShape, error) {
	shapeName = strings.TrimSpace(shapeName)
	if shapeName == "" {
		return oci.CatalogShape{}, fmt.Errorf("shape is required")
	}

	for _, item := range shapes {
		if strings.EqualFold(item.Shape, shapeName) {
			return item, nil
		}
	}
	return oci.CatalogShape{}, fmt.Errorf("selected shape %q is stale or unavailable", shapeName)
}

func validateFixedShapePolicy(policy store.Policy, shape oci.CatalogShape) error {
	if shape.DefaultOCPU <= 0 || shape.DefaultMemoryGB <= 0 {
		return fmt.Errorf("fixed shape %q is missing default OCPU or memory metadata", shape.Shape)
	}
	if float32(policy.OCPU) != shape.DefaultOCPU || float32(policy.MemoryGB) != shape.DefaultMemoryGB {
		return fmt.Errorf("fixed shape %q must use %g OCPU and %g GB", shape.Shape, shape.DefaultOCPU, shape.DefaultMemoryGB)
	}
	return nil
}

func validateFlexibleShapePolicy(policy store.Policy, shape oci.CatalogShape) error {
	ocpu := float32(policy.OCPU)
	memoryGB := float32(policy.MemoryGB)
	if policy.OCPU <= 0 {
		return fmt.Errorf("OCPU must be greater than zero for %s", shape.Shape)
	}
	if policy.MemoryGB <= 0 {
		return fmt.Errorf("memory must be greater than zero for %s", shape.Shape)
	}
	if shape.OCPUMin > 0 && ocpu < shape.OCPUMin {
		return fmt.Errorf("OCPU must be at least %g for %s", shape.OCPUMin, shape.Shape)
	}
	if shape.OCPUMax > 0 && ocpu > shape.OCPUMax {
		return fmt.Errorf("OCPU must be %g or less for %s", shape.OCPUMax, shape.Shape)
	}
	if shape.MemoryMinGB > 0 && memoryGB < shape.MemoryMinGB {
		return fmt.Errorf("memory must be at least %g GB for %s", shape.MemoryMinGB, shape.Shape)
	}
	if shape.MemoryMaxGB > 0 && memoryGB > shape.MemoryMaxGB {
		return fmt.Errorf("memory must be %g GB or less for %s", shape.MemoryMaxGB, shape.Shape)
	}

	memoryPerOCPU := memoryGB / ocpu
	if shape.MemoryMinPerOCPUGB > 0 && memoryPerOCPU < shape.MemoryMinPerOCPUGB {
		return fmt.Errorf("memory must stay above %g GB per OCPU for %s", shape.MemoryMinPerOCPUGB, shape.Shape)
	}
	if shape.MemoryMaxPerOCPUGB > 0 && memoryPerOCPU > shape.MemoryMaxPerOCPUGB {
		return fmt.Errorf("memory must stay below %g GB per OCPU for %s", shape.MemoryMaxPerOCPUGB, shape.Shape)
	}
	return nil
}
