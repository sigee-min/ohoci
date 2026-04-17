package warmpool

import (
	"context"
	"errors"
	"strings"

	"ohoci/internal/githubapp"
	"ohoci/internal/runnerlaunch"
	"ohoci/internal/store"
)

type Service struct {
	store    *store.Store
	github   *githubapp.Service
	launcher *runnerlaunch.Service
}

type Result struct {
	Launched int `json:"launched"`
	Updated  int `json:"updated"`
}

func New(storeDB *store.Store, github *githubapp.Service, launcher *runnerlaunch.Service) *Service {
	return &Service{
		store:    storeDB,
		github:   github,
		launcher: launcher,
	}
}

func (s *Service) RunOnce(ctx context.Context) (Result, error) {
	result := Result{}
	if s.store == nil || s.github == nil || s.launcher == nil {
		return result, nil
	}
	activeWarmRunners, err := s.store.ListActiveWarmRunners(ctx)
	if err != nil {
		return result, err
	}
	for _, runner := range activeWarmRunners {
		if strings.EqualFold(strings.TrimSpace(runner.WarmState), "warm_idle") || strings.EqualFold(strings.TrimSpace(runner.WarmState), "reserved") || strings.EqualFold(strings.TrimSpace(runner.WarmState), "consumed") {
			continue
		}
		client, _, err := s.github.ResolveClientForRepository(ctx, runner.RepoOwner, runner.RepoName)
		if err != nil || client == nil {
			continue
		}
		match, err := client.FindRepoRunnerByName(ctx, runner.InstallationID, runner.RepoOwner, runner.RepoName, runner.RunnerName)
		if err != nil || match == nil || match.Busy {
			continue
		}
		if err := s.store.UpdateRunnerStatus(ctx, runner.ID, "warm_idle", match.ID, nil); err != nil {
			return result, err
		}
		if err := s.store.UpdateRunnerWarmState(ctx, runner.ID, 0, "warm_idle"); err != nil {
			return result, err
		}
		result.Updated++
	}

	policies, err := s.store.ListPolicies(ctx)
	if err != nil {
		return result, err
	}
	for _, policy := range policies {
		if !policy.Enabled || !policy.WarmEnabled || policy.WarmMinIdle <= 0 {
			continue
		}
		for _, target := range policy.WarmRepoAllowlist {
			owner, repo, ok := splitRepo(target)
			if !ok {
				continue
			}
			if _, err := s.store.FindActiveWarmRunnerByTarget(ctx, policy.ID, owner, repo); err == nil {
				continue
			} else if err != nil && !errors.Is(err, store.ErrNotFound) {
				return result, err
			}
			activeCount, err := s.store.CountActiveRunnersForPolicy(ctx, policy.ID)
			if err != nil {
				return result, err
			}
			if activeCount >= policy.MaxRunners {
				continue
			}
			client, record, err := s.github.ResolveClientForRepository(ctx, owner, repo)
			if err != nil || client == nil || record == nil {
				continue
			}
			if _, err := s.launcher.Launch(ctx, client, runnerlaunch.Input{
				Policy:           policy,
				RepoOwner:        owner,
				RepoName:         repo,
				InstallationID:   record.InstallationID,
				JobID:            0,
				GitHubConfigID:   record.ID,
				GitHubConfigName: record.Name,
				GitHubConfigTags: append([]string(nil), record.Tags...),
				RequestedLabels:  append([]string(nil), policy.Labels...),
				Source:           "warm",
				WarmState:        "warming",
			}); err != nil {
				continue
			}
			result.Launched++
		}
	}
	return result, nil
}

func splitRepo(value string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(value), "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSpace(parts[1])
	if owner == "" || repo == "" {
		return "", "", false
	}
	return owner, repo, true
}
