package oci

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func (c *OCIController) CreateImage(ctx context.Context, req CreateImageRequest) (Image, error) {
	mode, provider, err := c.resolveProvider(ctx)
	if err != nil {
		return Image{}, err
	}
	if mode == "fake" {
		return c.fake.CreateImage(ctx, req)
	}
	runtime, err := c.runtimeConfig(ctx)
	if err != nil {
		return Image{}, err
	}
	client, err := c.computeClientFactory(provider)
	if err != nil {
		return Image{}, err
	}
	details := core.CreateImageDetails{
		CompartmentId: common.String(strings.TrimSpace(runtime.CompartmentID)),
		InstanceId:    common.String(strings.TrimSpace(req.InstanceID)),
		DisplayName:   common.String(strings.TrimSpace(req.DisplayName)),
		FreeformTags:  req.FreeformTags,
	}
	if strings.TrimSpace(c.cfg.BillingTagNamespace) != "" && len(req.DefinedTags) > 0 {
		details.DefinedTags = map[string]map[string]interface{}{
			strings.TrimSpace(c.cfg.BillingTagNamespace): stringMapToInterfaceMap(req.DefinedTags),
		}
	}
	response, err := client.CreateImage(ctx, core.CreateImageRequest{CreateImageDetails: details})
	if err != nil {
		return Image{}, err
	}
	return Image{
		ID:          valueOrEmpty(response.Image.Id),
		DisplayName: valueOrEmpty(response.Image.DisplayName),
		State:       string(response.Image.LifecycleState),
	}, nil
}

func (c *OCIController) GetImage(ctx context.Context, imageID string) (Image, error) {
	mode, provider, err := c.resolveProvider(ctx)
	if err != nil {
		return Image{}, err
	}
	if mode == "fake" {
		return c.fake.GetImage(ctx, imageID)
	}
	client, err := c.computeClientFactory(provider)
	if err != nil {
		return Image{}, err
	}
	response, err := client.GetImage(ctx, core.GetImageRequest{ImageId: common.String(strings.TrimSpace(imageID))})
	if err != nil {
		return Image{}, err
	}
	return Image{
		ID:          valueOrEmpty(response.Image.Id),
		DisplayName: valueOrEmpty(response.Image.DisplayName),
		State:       string(response.Image.LifecycleState),
	}, nil
}

func (c *OCIController) CaptureConsoleOutput(ctx context.Context, instanceID string) (string, error) {
	mode, provider, err := c.resolveProvider(ctx)
	if err != nil {
		return "", err
	}
	if mode == "fake" {
		return c.fake.CaptureConsoleOutput(ctx, instanceID)
	}
	client, err := c.computeClientFactory(provider)
	if err != nil {
		return "", err
	}
	capture, err := client.CaptureConsoleHistory(ctx, core.CaptureConsoleHistoryRequest{
		CaptureConsoleHistoryDetails: core.CaptureConsoleHistoryDetails{
			InstanceId:  common.String(strings.TrimSpace(instanceID)),
			DisplayName: common.String("ohoci-console-capture"),
			FreeformTags: map[string]string{
				ManagedFreeformTagKeyManaged:      "true",
				ManagedFreeformTagKeyController:   "ohoci",
				ManagedFreeformTagKeyResourceKind: "console_capture",
			},
		},
	})
	if err != nil {
		return "", err
	}
	consoleID := valueOrEmpty(capture.ConsoleHistory.Id)
	if consoleID == "" {
		return "", fmt.Errorf("console history id not returned")
	}
	for attempt := 0; attempt < 10; attempt++ {
		status, err := client.GetConsoleHistory(ctx, core.GetConsoleHistoryRequest{
			InstanceConsoleHistoryId: common.String(consoleID),
		})
		if err != nil {
			return "", err
		}
		switch status.ConsoleHistory.LifecycleState {
		case core.ConsoleHistoryLifecycleStateSucceeded:
			content, err := client.GetConsoleHistoryContent(ctx, core.GetConsoleHistoryContentRequest{
				InstanceConsoleHistoryId: common.String(consoleID),
			})
			if err != nil {
				return "", err
			}
			return valueOrEmpty(content.Value), nil
		case core.ConsoleHistoryLifecycleStateFailed:
			return "", fmt.Errorf("console capture failed")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("console capture timed out")
}

func (c *OCIController) DiscoverManagedResources(ctx context.Context) (ManagedResourceDiscovery, error) {
	mode, provider, err := c.resolveProvider(ctx)
	if err != nil {
		return ManagedResourceDiscovery{}, err
	}
	if mode == "fake" {
		return c.fake.DiscoverManagedResources(ctx)
	}
	runtime, err := c.runtimeConfig(ctx)
	if err != nil {
		return ManagedResourceDiscovery{}, err
	}
	client, err := c.computeClientFactory(provider)
	if err != nil {
		return ManagedResourceDiscovery{}, err
	}
	compartmentID := strings.TrimSpace(runtime.CompartmentID)
	items := []ManagedResource{}

	instanceReq := core.ListInstancesRequest{CompartmentId: common.String(compartmentID)}
	for {
		response, err := client.ListInstances(ctx, instanceReq)
		if err != nil {
			return ManagedResourceDiscovery{}, err
		}
		for _, instance := range response.Items {
			tags := extractManagedTags(c.cfg.BillingTagNamespace, instance.FreeformTags, instance.DefinedTags)
			if len(tags) == 0 {
				continue
			}
			items = append(items, ManagedResource{
				ID:          valueOrEmpty(instance.Id),
				Kind:        firstNonEmpty(tags[ManagedFreeformTagKeyResourceKind], tags[ManagedDefinedTagKeyResourceKind], "instance"),
				DisplayName: valueOrEmpty(instance.DisplayName),
				State:       string(instance.LifecycleState),
				Tags:        tags,
			})
		}
		if strings.TrimSpace(valueOrEmpty(response.OpcNextPage)) == "" {
			break
		}
		instanceReq.Page = response.OpcNextPage
	}

	imageReq := core.ListImagesRequest{CompartmentId: common.String(compartmentID)}
	for {
		response, err := client.ListImages(ctx, imageReq)
		if err != nil {
			return ManagedResourceDiscovery{}, err
		}
		for _, image := range response.Items {
			tags := extractManagedTags(c.cfg.BillingTagNamespace, image.FreeformTags, image.DefinedTags)
			if len(tags) == 0 {
				continue
			}
			items = append(items, ManagedResource{
				ID:          valueOrEmpty(image.Id),
				Kind:        firstNonEmpty(tags[ManagedFreeformTagKeyResourceKind], tags[ManagedDefinedTagKeyResourceKind], "image"),
				DisplayName: valueOrEmpty(image.DisplayName),
				State:       string(image.LifecycleState),
				Tags:        tags,
			})
		}
		if strings.TrimSpace(valueOrEmpty(response.OpcNextPage)) == "" {
			break
		}
		imageReq.Page = response.OpcNextPage
	}

	return ManagedResourceDiscovery{Items: items}, nil
}

func extractManagedTags(namespace string, freeform map[string]string, defined map[string]map[string]interface{}) map[string]string {
	tags := map[string]string{}
	for key, value := range freeform {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		tags[key] = value
	}
	if scoped := defined[strings.TrimSpace(namespace)]; len(scoped) > 0 {
		for key, rawValue := range scoped {
			key = strings.TrimSpace(key)
			value := strings.TrimSpace(fmt.Sprint(rawValue))
			if key == "" || value == "" {
				continue
			}
			tags[key] = value
		}
	}
	if !strings.EqualFold(tags[ManagedFreeformTagKeyManaged], "true") && !strings.EqualFold(tags[ManagedDefinedTagKeyManaged], "true") {
		return nil
	}
	if !strings.EqualFold(firstNonEmpty(tags[ManagedFreeformTagKeyController], tags[ManagedDefinedTagKeyController]), "ohoci") {
		return nil
	}
	return tags
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
