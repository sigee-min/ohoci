package setup

import (
	"context"

	"ohoci/internal/githubapp"
	"ohoci/internal/ocicredentials"
	"ohoci/internal/ociruntime"
)

type Status struct {
	Ready      bool                  `json:"ready"`
	Blockers   []string              `json:"blockers,omitempty"`
	GitHub     githubapp.Status      `json:"github"`
	OCIAuth    ocicredentials.Status `json:"ociAuth"`
	OCIRuntime ociruntime.Status     `json:"ociRuntime"`
}

type Service struct {
	github     *githubapp.Service
	ociAuth    *ocicredentials.Service
	ociRuntime *ociruntime.Service
}

func New(github *githubapp.Service, ociAuth *ocicredentials.Service, ociRuntime *ociruntime.Service) *Service {
	return &Service{
		github:     github,
		ociAuth:    ociAuth,
		ociRuntime: ociRuntime,
	}
}

func (s *Service) CurrentStatus(ctx context.Context) (Status, error) {
	status := Status{}
	if s.github != nil {
		githubStatus, err := s.github.CurrentStatus(ctx)
		if err != nil {
			return Status{}, err
		}
		status.GitHub = githubStatus
		if !githubStatus.Ready {
			status.Blockers = append(status.Blockers, "github")
		}
	} else {
		status.Blockers = append(status.Blockers, "github")
	}
	if s.ociAuth != nil {
		ociAuthStatus, err := s.ociAuth.CurrentStatus(ctx)
		if err != nil {
			return Status{}, err
		}
		status.OCIAuth = ociAuthStatus
	}
	if s.ociRuntime != nil {
		ociRuntimeStatus, err := s.ociRuntime.CurrentStatus(ctx)
		if err != nil {
			return Status{}, err
		}
		status.OCIRuntime = ociRuntimeStatus
		if !ociRuntimeStatus.Ready {
			status.Blockers = append(status.Blockers, "ociRuntime")
		}
	} else {
		status.Blockers = append(status.Blockers, "ociRuntime")
	}
	status.Ready = len(status.Blockers) == 0
	return status, nil
}
