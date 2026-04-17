package oci

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func (c *OCIController) LaunchInstance(ctx context.Context, req LaunchRequest) (Instance, error) {
	mode, provider, err := c.resolveProvider(ctx)
	if err != nil {
		return Instance{}, err
	}
	if mode == "fake" {
		return c.fake.LaunchInstance(ctx, req)
	}
	runtime, err := c.runtimeConfig(ctx)
	if err != nil {
		return Instance{}, err
	}
	client, err := c.computeClientFactory(provider)
	if err != nil {
		return Instance{}, err
	}
	metadata := map[string]string{
		"user_data": base64.StdEncoding.EncodeToString([]byte(req.UserData)),
	}
	subnetID := strings.TrimSpace(req.SubnetID)
	if subnetID == "" {
		subnetID = strings.TrimSpace(runtime.SubnetID)
	}
	imageID := strings.TrimSpace(req.ImageID)
	if imageID == "" {
		imageID = strings.TrimSpace(runtime.ImageID)
	}
	details := core.LaunchInstanceDetails{
		AvailabilityDomain: common.String(runtime.AvailabilityDomain),
		CompartmentId:      common.String(runtime.CompartmentID),
		DisplayName:        common.String(req.DisplayName),
		Shape:              common.String(req.Shape),
		Metadata:           metadata,
		CreateVnicDetails: &core.CreateVnicDetails{
			SubnetId:       common.String(subnetID),
			NsgIds:         runtime.NSGIDs,
			AssignPublicIp: common.Bool(runtime.AssignPublicIP),
		},
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId: common.String(imageID),
		},
	}
	if len(req.FreeformTags) > 0 {
		details.FreeformTags = req.FreeformTags
	}
	if strings.TrimSpace(c.cfg.BillingTagNamespace) != "" && len(req.DefinedTags) > 0 {
		details.DefinedTags = map[string]map[string]interface{}{
			strings.TrimSpace(c.cfg.BillingTagNamespace): stringMapToInterfaceMap(req.DefinedTags),
		}
	}
	if req.OCPU > 0 || req.MemoryGB > 0 {
		details.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus:       common.Float32(float32(req.OCPU)),
			MemoryInGBs: common.Float32(float32(req.MemoryGB)),
		}
	}
	if req.Spot {
		details.PreemptibleInstanceConfig = &core.PreemptibleInstanceConfigDetails{
			PreemptionAction: core.TerminatePreemptionAction{
				PreserveBootVolume: common.Bool(false),
			},
		}
	}
	response, err := client.LaunchInstance(ctx, core.LaunchInstanceRequest{LaunchInstanceDetails: details})
	if err != nil {
		return Instance{}, err
	}
	return Instance{
		ID:          valueOrEmpty(response.Instance.Id),
		DisplayName: valueOrEmpty(response.Instance.DisplayName),
		State:       string(response.Instance.LifecycleState),
	}, nil
}

func (c *OCIController) GetInstance(ctx context.Context, instanceID string) (Instance, error) {
	mode, provider, err := c.resolveProvider(ctx)
	if err != nil {
		return Instance{}, err
	}
	if mode == "fake" {
		return c.fake.GetInstance(ctx, instanceID)
	}
	client, err := c.computeClientFactory(provider)
	if err != nil {
		return Instance{}, err
	}
	response, err := client.GetInstance(ctx, core.GetInstanceRequest{InstanceId: common.String(strings.TrimSpace(instanceID))})
	if err != nil {
		return Instance{}, err
	}
	return Instance{
		ID:          valueOrEmpty(response.Instance.Id),
		DisplayName: valueOrEmpty(response.Instance.DisplayName),
		State:       string(response.Instance.LifecycleState),
	}, nil
}

func (c *OCIController) TerminateInstance(ctx context.Context, instanceID string) error {
	mode, provider, err := c.resolveProvider(ctx)
	if err != nil {
		return err
	}
	if mode == "fake" {
		return c.fake.TerminateInstance(ctx, instanceID)
	}
	client, err := c.computeClientFactory(provider)
	if err != nil {
		return err
	}
	_, err = client.TerminateInstance(ctx, core.TerminateInstanceRequest{
		InstanceId:         common.String(strings.TrimSpace(instanceID)),
		PreserveBootVolume: common.Bool(false),
	})
	return err
}

func (c *OCIController) resolveProvider(ctx context.Context) (string, common.ConfigurationProvider, error) {
	if c.providerResolver != nil {
		provider, ok, err := c.providerResolver.ResolveProvider(ctx)
		if err != nil {
			return "", nil, err
		}
		if ok {
			return "api_key", provider, nil
		}
	}
	if strings.EqualFold(c.cfg.AuthMode, "fake") {
		return "fake", nil, nil
	}
	provider, err := c.instancePrincipalProvider()
	if err != nil {
		return "", nil, err
	}
	return "instance_principal", provider, nil
}

func (c *OCIController) runtimeConfig(ctx context.Context) (RuntimeConfig, error) {
	if c.runtimeResolver != nil {
		return c.runtimeResolver.ResolveRuntimeConfig(ctx)
	}
	return c.cfg.Runtime, nil
}

func stringMapToInterfaceMap(values map[string]string) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
