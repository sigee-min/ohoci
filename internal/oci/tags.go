package oci

import (
	"slices"
	"strconv"
	"strings"
)

const (
	ManagedDefinedTagKeyManaged      = "managed"
	ManagedDefinedTagKeyController   = "controller"
	ManagedDefinedTagKeyResourceKind = "resource_kind"
	ManagedDefinedTagKeyRecipeID     = "recipe_id"
	ManagedDefinedTagKeyRecipeName   = "recipe_name"
	ManagedDefinedTagKeyBuildID      = "build_id"
	ManagedDefinedTagKeyRunnerName   = "runner_name"

	ManagedFreeformTagKeyManaged      = "ohoci_managed"
	ManagedFreeformTagKeyController   = "ohoci_controller"
	ManagedFreeformTagKeyResourceKind = "ohoci_resource_kind"
	ManagedFreeformTagKeyRecipeID     = "ohoci_recipe_id"
	ManagedFreeformTagKeyRecipeName   = "ohoci_recipe_name"
	ManagedFreeformTagKeyBuildID      = "ohoci_build_id"
	ManagedFreeformTagKeyRunnerName   = "ohoci_runner_name"

	BillingDefinedTagKeyPolicyID      = "policy_id"
	BillingDefinedTagKeyPolicyLabel   = "policy_label"
	BillingDefinedTagKeyRepoOwner     = "repo_owner"
	BillingDefinedTagKeyRepoName      = "repo_name"
	BillingDefinedTagKeyWorkflowJobID = "workflow_job_id"
	BillingDefinedTagKeyWorkflowRunID = "workflow_run_id"
	AuditDefinedTagKeyGitHubConfigID  = "github_config_id"
	AuditDefinedTagKeyGitHubAppName   = "github_app_name"
	AuditDefinedTagKeyGitHubAppTags   = "github_app_tags"

	BillingFreeformTagKeyPolicyID      = "ohoci_policy_id"
	BillingFreeformTagKeyPolicyLabel   = "ohoci_policy_label"
	BillingFreeformTagKeyRepoOwner     = "ohoci_repo_owner"
	BillingFreeformTagKeyRepoName      = "ohoci_repo_name"
	BillingFreeformTagKeyWorkflowJobID = "ohoci_workflow_job_id"
	BillingFreeformTagKeyWorkflowRunID = "ohoci_workflow_run_id"
	AuditFreeformTagKeyGitHubConfigID  = "ohoci_github_config_id"
	AuditFreeformTagKeyGitHubAppName   = "ohoci_github_app_name"
	AuditFreeformTagKeyGitHubAppTags   = "ohoci_github_app_tags"
)

type LaunchBillingTagInput struct {
	PolicyID         int64
	PolicyLabel      string
	RepoOwner        string
	RepoName         string
	WorkflowJobID    int64
	WorkflowRunID    int64
	RunnerName       string
	GitHubConfigID   int64
	GitHubConfigName string
	GitHubConfigTags []string
}

type LaunchBillingTags struct {
	Freeform map[string]string
	Defined  map[string]string
}

type ManagedTagInput struct {
	ResourceKind string
	RecipeID     int64
	RecipeName   string
	BuildID      int64
	RunnerName   string
}

type ManagedTags struct {
	Freeform map[string]string
	Defined  map[string]string
}

func BuildLaunchBillingTags(namespace string, input LaunchBillingTagInput) LaunchBillingTags {
	managed := BuildManagedTags(namespace, ManagedTagInput{
		ResourceKind: "github_runner_instance",
		RunnerName:   input.RunnerName,
	})
	definedValues := map[string]string{
		BillingDefinedTagKeyPolicyID:      int64TagValue(input.PolicyID),
		BillingDefinedTagKeyPolicyLabel:   strings.TrimSpace(input.PolicyLabel),
		BillingDefinedTagKeyRepoOwner:     strings.TrimSpace(input.RepoOwner),
		BillingDefinedTagKeyRepoName:      strings.TrimSpace(input.RepoName),
		BillingDefinedTagKeyWorkflowJobID: int64TagValue(input.WorkflowJobID),
		BillingDefinedTagKeyWorkflowRunID: int64TagValue(input.WorkflowRunID),
	}
	freeformValues := map[string]string{
		BillingFreeformTagKeyPolicyID:      int64TagValue(input.PolicyID),
		BillingFreeformTagKeyPolicyLabel:   strings.TrimSpace(input.PolicyLabel),
		BillingFreeformTagKeyRepoOwner:     strings.TrimSpace(input.RepoOwner),
		BillingFreeformTagKeyRepoName:      strings.TrimSpace(input.RepoName),
		BillingFreeformTagKeyWorkflowJobID: int64TagValue(input.WorkflowJobID),
		BillingFreeformTagKeyWorkflowRunID: int64TagValue(input.WorkflowRunID),
		AuditFreeformTagKeyGitHubConfigID:  int64TagValue(input.GitHubConfigID),
		AuditFreeformTagKeyGitHubAppName:   strings.TrimSpace(input.GitHubConfigName),
		AuditFreeformTagKeyGitHubAppTags:   stringSliceTagValue(input.GitHubConfigTags),
	}
	auditDefinedValues := map[string]string{
		AuditDefinedTagKeyGitHubConfigID: int64TagValue(input.GitHubConfigID),
		AuditDefinedTagKeyGitHubAppName:  strings.TrimSpace(input.GitHubConfigName),
		AuditDefinedTagKeyGitHubAppTags:  stringSliceTagValue(input.GitHubConfigTags),
	}

	tags := LaunchBillingTags{
		Freeform: mergeTagMaps(managed.Freeform, normalizeTagValues(freeformValues)),
	}
	if strings.TrimSpace(namespace) != "" {
		tags.Defined = mergeTagMaps(mergeTagMaps(managed.Defined, normalizeTagValues(definedValues)), normalizeTagValues(auditDefinedValues))
	}
	return tags
}

func BuildManagedTags(namespace string, input ManagedTagInput) ManagedTags {
	definedValues := map[string]string{
		ManagedDefinedTagKeyManaged:      "true",
		ManagedDefinedTagKeyController:   "ohoci",
		ManagedDefinedTagKeyResourceKind: strings.TrimSpace(input.ResourceKind),
		ManagedDefinedTagKeyRecipeID:     int64TagValue(input.RecipeID),
		ManagedDefinedTagKeyRecipeName:   strings.TrimSpace(input.RecipeName),
		ManagedDefinedTagKeyBuildID:      int64TagValue(input.BuildID),
		ManagedDefinedTagKeyRunnerName:   strings.TrimSpace(input.RunnerName),
	}
	freeformValues := map[string]string{
		ManagedFreeformTagKeyManaged:      "true",
		ManagedFreeformTagKeyController:   "ohoci",
		ManagedFreeformTagKeyResourceKind: strings.TrimSpace(input.ResourceKind),
		ManagedFreeformTagKeyRecipeID:     int64TagValue(input.RecipeID),
		ManagedFreeformTagKeyRecipeName:   strings.TrimSpace(input.RecipeName),
		ManagedFreeformTagKeyBuildID:      int64TagValue(input.BuildID),
		ManagedFreeformTagKeyRunnerName:   strings.TrimSpace(input.RunnerName),
	}
	tags := ManagedTags{Freeform: normalizeTagValues(freeformValues)}
	if strings.TrimSpace(namespace) != "" {
		tags.Defined = normalizeTagValues(definedValues)
	}
	return tags
}

func normalizeTagValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, raw := range values {
		key = strings.TrimSpace(key)
		value := strings.TrimSpace(raw)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeTagMaps(base, extra map[string]string) map[string]string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func int64TagValue(value int64) string {
	if value <= 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func stringSliceTagValue(values []string) string {
	normalized := normalizeTagSlice(values)
	if len(normalized) == 0 {
		return ""
	}
	return strings.Join(normalized, ",")
}

func normalizeTagSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return out
}
