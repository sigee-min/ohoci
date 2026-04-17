package cleanup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ohoci/internal/githubapp"
	"ohoci/internal/oci"
	"ohoci/internal/session"
	"ohoci/internal/store"
)

type Result struct {
	Checked    int `json:"checked"`
	Terminated int `json:"terminated"`
}

type Service struct {
	store    *store.Store
	oci      oci.Controller
	github   *githubapp.Service
	sessions *session.Service
	now      func() time.Time
}

func New(s *store.Store, ociClient oci.Controller, githubClient *githubapp.Service, sessions *session.Service) *Service {
	return &Service{store: s, oci: ociClient, github: githubClient, sessions: sessions, now: time.Now}
}

func (s *Service) RunOnce(ctx context.Context) (Result, error) {
	if err := s.sessions.GC(ctx); err != nil {
		return Result{}, err
	}
	candidates, err := s.store.ListCleanupCandidates(ctx, s.now().UTC())
	if err != nil {
		return Result{}, err
	}
	result := Result{Checked: len(candidates)}
	for _, runner := range candidates {
		githubClient, err := s.resolveRunnerClient(ctx, runner)
		if err != nil && !errors.Is(err, githubapp.ErrNotConfigured) {
			return Result{}, err
		}
		githubRunnerID, err := s.syncGitHubRunnerID(ctx, githubClient, runner)
		if err != nil {
			_ = s.store.AddEventLog(ctx, store.EventLog{
				Level:   "warn",
				Message: fmt.Sprintf("sync GitHub runner for %s failed: %v", runner.RunnerName, err),
			})
			githubRunnerID = runner.GitHubRunnerID
		}
		if githubRunnerID > 0 && githubRunnerID != runner.GitHubRunnerID {
			_ = s.store.UpdateRunnerStatus(ctx, runner.ID, runner.Status, githubRunnerID, nil)
			runner.GitHubRunnerID = githubRunnerID
		}
		instance, err := s.oci.GetInstance(ctx, runner.InstanceOCID)
		if err == nil {
			state := strings.ToUpper(strings.TrimSpace(instance.State))
			if state == "TERMINATED" || state == "STOPPED" || state == "FAILED" {
				if githubClient != nil && runner.GitHubRunnerID > 0 {
					if err := githubClient.DeleteRepoRunner(ctx, runner.InstallationID, runner.RepoOwner, runner.RepoName, runner.GitHubRunnerID); err != nil {
						_ = s.store.AddEventLog(ctx, store.EventLog{
							Level:   "warn",
							Message: fmt.Sprintf("delete GitHub runner %s failed after OCI terminal state: %v", runner.RunnerName, err),
						})
					}
				}
				if err := s.store.MarkRunnerTerminated(ctx, runner.ID); err == nil {
					result.Terminated++
				}
				_ = s.store.AddEventLog(ctx, store.EventLog{
					Level:      "info",
					DeliveryID: "",
					Message:    fmt.Sprintf("runner %s already in terminal OCI state %s", runner.RunnerName, state),
				})
				continue
			}
		}
		if err := s.oci.TerminateInstance(ctx, runner.InstanceOCID); err != nil {
			_ = s.store.AddEventLog(ctx, store.EventLog{
				Level:   "error",
				Message: fmt.Sprintf("terminate runner %s failed: %v", runner.RunnerName, err),
			})
			continue
		}
		if !isTerminalWorkflowRunnerStatus(runner.Status) {
			_ = s.store.UpdateRunnerStatus(ctx, runner.ID, "terminating", runner.GitHubRunnerID, nil)
		}
		result.Terminated++
		_ = s.store.AddEventLog(ctx, store.EventLog{
			Level:   "info",
			Message: fmt.Sprintf("terminate requested for runner %s", runner.RunnerName),
		})
		if githubClient != nil && runner.GitHubRunnerID > 0 {
			_ = githubClient.DeleteRepoRunner(ctx, runner.InstallationID, runner.RepoOwner, runner.RepoName, runner.GitHubRunnerID)
		}
	}
	return result, nil
}

func isTerminalWorkflowRunnerStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func (s *Service) resolveRunnerClient(ctx context.Context, runner store.Runner) (*githubapp.Client, error) {
	if s.github == nil {
		return nil, githubapp.ErrNotConfigured
	}
	return s.github.ResolveRunnerClient(ctx, runner.GitHubConfigID, runner.InstallationID)
}

func (s *Service) syncGitHubRunnerID(ctx context.Context, githubClient *githubapp.Client, runner store.Runner) (int64, error) {
	if runner.GitHubRunnerID > 0 {
		return runner.GitHubRunnerID, nil
	}
	if githubClient == nil {
		return 0, nil
	}
	match, err := githubClient.FindRepoRunnerByName(ctx, runner.InstallationID, runner.RepoOwner, runner.RepoName, runner.RunnerName)
	if err != nil {
		return 0, err
	}
	if match == nil {
		return 0, nil
	}
	return match.ID, nil
}
