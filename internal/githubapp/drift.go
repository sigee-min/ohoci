package githubapp

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"ohoci/internal/store"
)

type DriftIssue struct {
	Code                 string   `json:"code"`
	Severity             string   `json:"severity"`
	ConfigID             int64    `json:"configId,omitempty"`
	Source               string   `json:"source"`
	APIBaseURL           string   `json:"apiBaseUrl,omitempty"`
	AppID                int64    `json:"appId,omitempty"`
	InstallationID       int64    `json:"installationId,omitempty"`
	InstallationState    string   `json:"installationState,omitempty"`
	MissingSelectedRepos []string `json:"missingSelectedRepos,omitempty"`
	NewlyVisibleRepos    []string `json:"newlyVisibleRepos,omitempty"`
	Message              string   `json:"message"`
}

type DriftStatus struct {
	GeneratedAt   time.Time    `json:"generatedAt"`
	Severity      string       `json:"severity"`
	ActiveConfigs []View       `json:"activeConfigs,omitempty"`
	StagedConfig  *View        `json:"stagedConfig,omitempty"`
	Issues        []DriftIssue `json:"issues"`
}

func (s *Service) CurrentDrift(ctx context.Context) (DriftStatus, error) {
	activeConfigs, err := s.listResolvedActiveConfigs(ctx)
	if err != nil {
		return DriftStatus{}, err
	}
	status := DriftStatus{
		GeneratedAt:   time.Now().UTC(),
		Severity:      "ok",
		ActiveConfigs: make([]View, 0, len(activeConfigs)),
		Issues:        []DriftIssue{},
	}

	issues := []DriftIssue{}
	activeByRoute := map[string]View{}
	for _, item := range activeConfigs {
		status.ActiveConfigs = append(status.ActiveConfigs, item.view)
		activeByRoute[routeKey(item.view.APIBaseURL, item.view.AppID, item.view.InstallationID)] = item.view
		issues = append(issues, viewDriftIssues("active", item.view)...)
	}

	stagedRecord, err := s.store.FindStagedGitHubConfig(ctx)
	switch {
	case err == nil:
		view := viewFromStored(stagedRecord)
		status.StagedConfig = &view
		issues = append(issues, viewDriftIssues("staged", view)...)
		if activeView, ok := activeByRoute[routeKey(view.APIBaseURL, view.AppID, view.InstallationID)]; ok {
			if !slices.Equal(normalizeRepoNames(activeView.SelectedRepos), normalizeRepoNames(view.SelectedRepos)) {
				issues = append(issues, DriftIssue{
					Code:           "staged_active_repo_scope_mismatch",
					Severity:       "warning",
					ConfigID:       view.ID,
					Source:         "staged",
					APIBaseURL:     view.APIBaseURL,
					AppID:          view.AppID,
					InstallationID: view.InstallationID,
					Message:        "staged and active route selections do not match",
				})
			}
		}
	case err != nil && !isStoreNotFound(err):
		return DriftStatus{}, err
	}

	status.Issues = issues
	status.Severity = driftSeverity(issues)
	return status, nil
}

func (s *Service) ReconcileDrift(ctx context.Context) (DriftStatus, error) {
	activeConfigs, err := s.store.ListActiveGitHubConfigs(ctx)
	if err != nil {
		return DriftStatus{}, err
	}
	for _, record := range activeConfigs {
		cfg, _, resolveErr := s.configFromRecord(record)
		if resolveErr != nil {
			continue
		}
		client, err := New(cfg)
		if err != nil {
			continue
		}
		_ = s.RefreshInstallationSnapshot(ctx, &record, client, record.AccountLogin, record.AccountType, record.InstallationRepositorySelection)
	}
	if stagedRecord, err := s.store.FindStagedGitHubConfig(ctx); err == nil {
		cfg, _, resolveErr := s.configFromRecord(stagedRecord)
		if resolveErr == nil {
			if client, err := New(cfg); err == nil {
				_ = s.RefreshInstallationSnapshot(ctx, &stagedRecord, client, stagedRecord.AccountLogin, stagedRecord.AccountType, stagedRecord.InstallationRepositorySelection)
			}
		}
	}
	return s.CurrentDrift(ctx)
}

func viewDriftIssues(source string, view View) []DriftIssue {
	issues := []DriftIssue{}
	missingSelected := normalizeRepoNames(view.InstallationMissing)
	newlyVisible := subtractRepos(view.InstallationRepositories, view.SelectedRepos)
	state := strings.ToLower(strings.TrimSpace(view.InstallationState))
	if state != "" && state != "active" {
		issues = append(issues, DriftIssue{
			Code:              "installation_not_active",
			Severity:          "critical",
			ConfigID:          view.ID,
			Source:            source,
			APIBaseURL:        view.APIBaseURL,
			AppID:             view.AppID,
			InstallationID:    view.InstallationID,
			InstallationState: view.InstallationState,
			Message:           "installation is not active",
		})
	}
	if len(missingSelected) > 0 {
		issues = append(issues, DriftIssue{
			Code:                 "selected_repos_missing_from_installation",
			Severity:             "critical",
			ConfigID:             view.ID,
			Source:               source,
			APIBaseURL:           view.APIBaseURL,
			AppID:                view.AppID,
			InstallationID:       view.InstallationID,
			InstallationState:    view.InstallationState,
			MissingSelectedRepos: missingSelected,
			Message:              "selected repositories are missing from the GitHub installation",
		})
	}
	if len(newlyVisible) > 0 {
		issues = append(issues, DriftIssue{
			Code:              "new_repositories_visible",
			Severity:          "warning",
			ConfigID:          view.ID,
			Source:            source,
			APIBaseURL:        view.APIBaseURL,
			AppID:             view.AppID,
			InstallationID:    view.InstallationID,
			InstallationState: view.InstallationState,
			NewlyVisibleRepos: newlyVisible,
			Message:           "installation can see repositories that are not locally selected",
		})
	}
	return issues
}

func subtractRepos(left, right []string) []string {
	normalizedRight := normalizeRepoNames(right)
	out := []string{}
	for _, item := range normalizeRepoNames(left) {
		if slices.Contains(normalizedRight, item) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func driftSeverity(issues []DriftIssue) string {
	severity := "ok"
	for _, issue := range issues {
		switch issue.Severity {
		case "critical":
			return "critical"
		case "warning":
			severity = "warning"
		}
	}
	return severity
}

func routeKey(apiBaseURL string, appID, installationID int64) string {
	return strings.ToLower(strings.TrimSpace(apiBaseURL)) + "|" + strings.TrimSpace(int64String(appID)) + "|" + strings.TrimSpace(int64String(installationID))
}

func int64String(value int64) string {
	if value <= 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func isStoreNotFound(err error) bool {
	return err == store.ErrNotFound
}
