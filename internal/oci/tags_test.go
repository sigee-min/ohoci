package oci

import "testing"

func TestBuildManagedTagsIncludeOhoCIOwnershipMarkers(t *testing.T) {
	tags := BuildManagedTags("billing", ManagedTagInput{
		ResourceKind: "runner_image",
		RecipeID:     7,
		RecipeName:   "node22",
		BuildID:      11,
		RunnerName:   "ohoci-runner-1",
	})

	if got := tags.Freeform[ManagedFreeformTagKeyManaged]; got != "true" {
		t.Fatalf("expected managed freeform tag, got %q", got)
	}
	if got := tags.Freeform[ManagedFreeformTagKeyController]; got != "ohoci" {
		t.Fatalf("expected controller freeform tag, got %q", got)
	}
	if got := tags.Freeform[ManagedFreeformTagKeyResourceKind]; got != "runner_image" {
		t.Fatalf("expected resource kind freeform tag, got %q", got)
	}
	if got := tags.Freeform[ManagedFreeformTagKeyRecipeID]; got != "7" {
		t.Fatalf("expected recipe id freeform tag, got %q", got)
	}
	if got := tags.Freeform[ManagedFreeformTagKeyBuildID]; got != "11" {
		t.Fatalf("expected build id freeform tag, got %q", got)
	}
	if got := tags.Defined[ManagedDefinedTagKeyManaged]; got != "true" {
		t.Fatalf("expected managed defined tag, got %q", got)
	}
	if got := tags.Defined[ManagedDefinedTagKeyController]; got != "ohoci" {
		t.Fatalf("expected controller defined tag, got %q", got)
	}
}

func TestBuildLaunchBillingTagsCarryManagedOwnershipMarkers(t *testing.T) {
	tags := BuildLaunchBillingTags("billing", LaunchBillingTagInput{
		PolicyID:         3,
		PolicyLabel:      "bench",
		RepoOwner:        "example",
		RepoName:         "repo",
		WorkflowJobID:    101,
		WorkflowRunID:    202,
		RunnerName:       "ohoci-runner-2",
		GitHubConfigID:   88,
		GitHubConfigName: "beta-stage",
		GitHubConfigTags: []string{"staging", "beta", "beta"},
	})

	if got := tags.Freeform[ManagedFreeformTagKeyManaged]; got != "true" {
		t.Fatalf("expected managed marker on launch tags, got %q", got)
	}
	if got := tags.Freeform[ManagedFreeformTagKeyController]; got != "ohoci" {
		t.Fatalf("expected controller marker on launch tags, got %q", got)
	}
	if got := tags.Freeform[ManagedFreeformTagKeyResourceKind]; got != "github_runner_instance" {
		t.Fatalf("expected runner instance resource kind, got %q", got)
	}
	if got := tags.Freeform[BillingFreeformTagKeyWorkflowJobID]; got != "101" {
		t.Fatalf("expected workflow job billing tag, got %q", got)
	}
	if got := tags.Defined[ManagedDefinedTagKeyRunnerName]; got != "ohoci-runner-2" {
		t.Fatalf("expected runner name on defined tags, got %q", got)
	}
	if got := tags.Freeform[AuditFreeformTagKeyGitHubConfigID]; got != "88" {
		t.Fatalf("expected github config id on freeform tags, got %q", got)
	}
	if got := tags.Freeform[AuditFreeformTagKeyGitHubAppName]; got != "beta-stage" {
		t.Fatalf("expected github app name on freeform tags, got %q", got)
	}
	if got := tags.Freeform[AuditFreeformTagKeyGitHubAppTags]; got != "beta,staging" {
		t.Fatalf("expected github app tags on freeform tags, got %q", got)
	}
	if got := tags.Defined[AuditDefinedTagKeyGitHubConfigID]; got != "88" {
		t.Fatalf("expected github config id on defined tags, got %q", got)
	}
	if got := tags.Defined[AuditDefinedTagKeyGitHubAppName]; got != "beta-stage" {
		t.Fatalf("expected github app name on defined tags, got %q", got)
	}
	if got := tags.Defined[AuditDefinedTagKeyGitHubAppTags]; got != "beta,staging" {
		t.Fatalf("expected github app tags on defined tags, got %q", got)
	}
}
