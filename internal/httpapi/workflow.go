package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ohoci/internal/admission"
	"ohoci/internal/githubapp"
	"ohoci/internal/runnerlaunch"
	"ohoci/internal/store"
)

func processWorkflowJob(ctx context.Context, deps Dependencies, resolution githubapp.WebhookResolution, deliveryID string, payload workflowJobEvent) error {
	githubClient := resolution.Client
	if githubClient == nil {
		return fmt.Errorf("github client is required for workflow_job processing")
	}
	jobStatus := workflowJobStatus(payload)
	installationID := payload.Installation.ID
	if installationID <= 0 && githubClient != nil && githubClient.InstallationID() > 0 {
		installationID = githubClient.InstallationID()
	}
	configID, configName, configTags := githubTraceSnapshot(resolution.Config)
	if !githubClient.RepositoryAllowed(payload.Repository.Owner.Login, payload.Repository.Name) {
		job, _ := deps.Store.UpsertJob(ctx, store.Job{
			GitHubJobID:      payload.WorkflowJob.ID,
			DeliveryID:       deliveryID,
			InstallationID:   installationID,
			GitHubConfigID:   configID,
			GitHubConfigName: configName,
			GitHubConfigTags: configTags,
			RepoOwner:        payload.Repository.Owner.Login,
			RepoName:         payload.Repository.Name,
			RunID:            payload.WorkflowJob.RunID,
			RunAttempt:       payload.WorkflowJob.RunAttempt,
			Status:           jobStatus,
			Labels:           payload.WorkflowJob.Labels,
			ErrorMessage:     "repository not allowed",
		})
		_ = deps.Store.AddEventLog(ctx, store.EventLog{
			DeliveryID:  deliveryID,
			Level:       "warn",
			Message:     fmt.Sprintf("repository %s/%s is not allowed", payload.Repository.Owner.Login, payload.Repository.Name),
			DetailsJSON: mustJSON(job),
		})
		return nil
	}

	switch payload.Action {
	case "queued":
		admissionService := resolveAdmissionService(deps)
		decision, err := admissionService.Evaluate(ctx, admission.Input{
			DeliveryID:     deliveryID,
			InstallationID: installationID,
			RepoOwner:      payload.Repository.Owner.Login,
			RepoName:       payload.Repository.Name,
			Labels:         payload.WorkflowJob.Labels,
		})
		if err != nil {
			return err
		}
		job := store.Job{
			GitHubJobID:      payload.WorkflowJob.ID,
			DeliveryID:       deliveryID,
			InstallationID:   installationID,
			GitHubConfigID:   configID,
			GitHubConfigName: configName,
			GitHubConfigTags: configTags,
			RepoOwner:        payload.Repository.Owner.Login,
			RepoName:         payload.Repository.Name,
			RunID:            payload.WorkflowJob.RunID,
			RunAttempt:       payload.WorkflowJob.RunAttempt,
			Status:           "queued",
			Labels:           payload.WorkflowJob.Labels,
		}
		if decision.MatchedPolicy != nil {
			job.MatchedPolicyID = &decision.MatchedPolicy.ID
		}
		if decision.BlockingStage != "" {
			job.ErrorMessage = admissionErrorMessage(decision)
		} else if decision.WarmCandidate != nil {
			job.Status = "waiting_warm_attach"
		} else {
			job.Status = "provisioning"
		}
		jobRecord, err := deps.Store.UpsertJob(ctx, job)
		if err != nil {
			return err
		}
		if err := upsertJobDiagnosticFromDecision(ctx, deps, jobRecord, decision); err != nil {
			return err
		}
		existingRunner, err := deps.Store.FindLatestRunnerByJobID(ctx, jobRecord.ID)
		if err == nil {
			if existingRunner.TerminatedAt == nil && !strings.EqualFold(strings.TrimSpace(existingRunner.Status), "terminated") {
				return deps.Store.AddEventLog(ctx, store.EventLog{
					DeliveryID: deliveryID,
					Level:      "info",
					Message:    fmt.Sprintf("job %d already has tracked runner %s", jobRecord.GitHubJobID, existingRunner.RunnerName),
				})
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		if decision.BlockingStage != "" {
			return deps.Store.AddEventLog(ctx, store.EventLog{
				DeliveryID:  deliveryID,
				Level:       "warn",
				Message:     admissionErrorMessage(decision),
				DetailsJSON: mustJSON(decision),
			})
		}
		if decision.WarmCandidate != nil {
			if err := deps.Store.UpdateRunnerWarmState(ctx, decision.WarmCandidate.ID, jobRecord.ID, "reserved"); err != nil {
				return err
			}
			if err := deps.Store.UpdateRunnerStatus(ctx, decision.WarmCandidate.ID, "reserved", decision.WarmCandidate.GitHubRunnerID, nil); err != nil {
				return err
			}
			if err := updateDiagnosticStage(ctx, deps, jobRecord.ID, admission.StageWarmCandidate, store.DiagnosticStage{
				State:     "passed",
				Code:      "warm_candidate_reserved",
				Message:   "warm runner reserved for queued job",
				Details:   map[string]any{"runnerId": decision.WarmCandidate.ID},
				UpdatedAt: time.Now().UTC(),
			}, func(diagnostic *store.JobDiagnostic) {
				runnerID := decision.WarmCandidate.ID
				diagnostic.RunnerID = &runnerID
				diagnostic.InstanceOCID = decision.WarmCandidate.InstanceOCID
				diagnostic.SummaryCode = decision.SummaryCode
			}); err != nil {
				return err
			}
			return deps.Store.AddEventLog(ctx, store.EventLog{
				DeliveryID:  deliveryID,
				Level:       "info",
				Message:     fmt.Sprintf("warm runner %s reserved for job %d", decision.WarmCandidate.RunnerName, jobRecord.GitHubJobID),
				DetailsJSON: mustJSON(map[string]any{"jobId": jobRecord.ID, "runnerId": decision.WarmCandidate.ID}),
			})
		}

		launchResult, err := resolveRunnerLaunchService(deps).Launch(ctx, githubClient, buildRunnerLaunchInput(jobRecord, *decision.MatchedPolicy, decision.RequestedLabels))
		if err != nil {
			jobRecord.ErrorMessage = err.Error()
			jobRecord.Status = "failed"
			_, _ = deps.Store.UpsertJob(ctx, jobRecord)
			_ = updateDiagnosticStage(ctx, deps, jobRecord.ID, admission.StageLaunchRequired, store.DiagnosticStage{
				State:     "blocked",
				Code:      "launch_failed",
				Message:   err.Error(),
				UpdatedAt: time.Now().UTC(),
			}, func(diagnostic *store.JobDiagnostic) {
				diagnostic.SummaryCode = "launch_failed"
				diagnostic.BlockingStage = admission.StageLaunchRequired
			})
			return err
		}
		if err := updateDiagnosticStage(ctx, deps, jobRecord.ID, admission.StageLaunchRequired, store.DiagnosticStage{
			State:     "passed",
			Code:      "launch_requested",
			Message:   "runner launch requested",
			Details:   map[string]any{"runnerId": launchResult.Runner.ID},
			UpdatedAt: time.Now().UTC(),
		}, func(diagnostic *store.JobDiagnostic) {
			runnerID := launchResult.Runner.ID
			diagnostic.RunnerID = &runnerID
			diagnostic.InstanceOCID = launchResult.Runner.InstanceOCID
			diagnostic.SummaryCode = "launch_requested"
		}); err != nil {
			return err
		}
		return deps.Store.AddEventLog(ctx, store.EventLog{DeliveryID: deliveryID, Level: "info", Message: fmt.Sprintf("runner %s launch requested", launchResult.Runner.RunnerName)})
	case "completed", "in_progress":
		jobRecord, err := deps.Store.UpsertJob(ctx, store.Job{
			GitHubJobID:      payload.WorkflowJob.ID,
			DeliveryID:       deliveryID,
			InstallationID:   installationID,
			GitHubConfigID:   configID,
			GitHubConfigName: configName,
			GitHubConfigTags: configTags,
			RepoOwner:        payload.Repository.Owner.Login,
			RepoName:         payload.Repository.Name,
			RunID:            payload.WorkflowJob.RunID,
			RunAttempt:       payload.WorkflowJob.RunAttempt,
			Status:           jobStatus,
			Labels:           payload.WorkflowJob.Labels,
		})
		if err != nil {
			return err
		}
		runnerMessage, err := syncWorkflowJobRunner(ctx, deps, githubClient, deliveryID, jobRecord, payload, jobStatus)
		if err != nil {
			return err
		}
		message := fmt.Sprintf("job %d updated to %s", jobRecord.GitHubJobID, jobStatus)
		if runnerMessage != "" {
			message += " (" + runnerMessage + ")"
		}
		return deps.Store.AddEventLog(ctx, store.EventLog{DeliveryID: deliveryID, Level: "info", Message: message})
	default:
		return deps.Store.AddEventLog(ctx, store.EventLog{DeliveryID: deliveryID, Level: "info", Message: fmt.Sprintf("workflow_job action %s ignored", payload.Action)})
	}
}

func buildRunnerLaunchInput(jobRecord store.Job, matchedPolicy store.Policy, requestedLabels []string) runnerlaunch.Input {
	return runnerlaunch.Input{
		Policy:           matchedPolicy,
		RepoOwner:        jobRecord.RepoOwner,
		RepoName:         jobRecord.RepoName,
		InstallationID:   jobRecord.InstallationID,
		JobID:            jobRecord.ID,
		GitHubJobID:      jobRecord.GitHubJobID,
		RunID:            jobRecord.RunID,
		GitHubConfigID:   jobRecord.GitHubConfigID,
		GitHubConfigName: jobRecord.GitHubConfigName,
		GitHubConfigTags: append([]string(nil), jobRecord.GitHubConfigTags...),
		RequestedLabels:  append([]string(nil), requestedLabels...),
		Source:           "ondemand",
	}
}

func admissionErrorMessage(decision admission.Decision) string {
	if decision.BlockingStage != "" {
		if status, ok := decision.StageStatuses[decision.BlockingStage]; ok {
			if message := strings.TrimSpace(status.Message); message != "" {
				return message
			}
		}
	}
	for _, check := range decision.PolicyChecks {
		if !check.Matched || len(check.Reasons) == 0 {
			continue
		}
		return strings.Join(check.Reasons, "; ")
	}
	if strings.TrimSpace(decision.SummaryCode) != "" {
		return strings.ReplaceAll(decision.SummaryCode, "_", " ")
	}
	return "admission blocked"
}

func resolveAdmissionService(deps Dependencies) *admission.Service {
	if deps.Admission != nil {
		return deps.Admission
	}
	return admission.New(deps.Store, deps.GitHub, deps.Setup, deps.OCIBilling)
}

func resolveRunnerLaunchService(deps Dependencies) *runnerlaunch.Service {
	if deps.RunnerLaunch != nil {
		return deps.RunnerLaunch
	}
	return runnerlaunch.New(deps.Config, deps.Store, deps.OCI, deps.OCIRuntime)
}

func syncWorkflowJobRunner(ctx context.Context, deps Dependencies, githubClient *githubapp.Client, deliveryID string, jobRecord store.Job, payload workflowJobEvent, targetStatus string) (string, error) {
	runnerRecord, err := deps.Store.FindLatestRunnerByJobID(ctx, jobRecord.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = updateDiagnosticStage(ctx, deps, jobRecord.ID, admission.StageRunnerAttachment, store.DiagnosticStage{
				State:     "blocked",
				Code:      "runner_not_tracked",
				Message:   "job has no tracked runner",
				UpdatedAt: time.Now().UTC(),
			}, func(diagnostic *store.JobDiagnostic) {
				diagnostic.SummaryCode = "runner_not_tracked"
				diagnostic.BlockingStage = admission.StageRunnerAttachment
			})
			return "no tracked runner", nil
		}
		return "", err
	}
	githubRunnerID, err := resolveGitHubRunnerID(ctx, githubClient, runnerRecord, payload)
	if err != nil {
		_ = deps.Store.AddEventLog(ctx, store.EventLog{
			DeliveryID: deliveryID,
			Level:      "warn",
			Message:    fmt.Sprintf("runner lookup for %s failed: %v", runnerRecord.RunnerName, err),
		})
		githubRunnerID = runnerRecord.GitHubRunnerID
	}
	if strings.EqualFold(strings.TrimSpace(targetStatus), "in_progress") && strings.EqualFold(strings.TrimSpace(runnerRecord.Source), "warm") {
		if err := deps.Store.UpdateRunnerWarmState(ctx, runnerRecord.ID, jobRecord.ID, "consumed"); err != nil {
			return "", err
		}
		runnerRecord.WarmState = "consumed"
		runnerRecord.JobID = jobRecord.ID
	}
	if err := deps.Store.UpdateRunnerStatus(ctx, runnerRecord.ID, targetStatus, githubRunnerID, nil); err != nil {
		return "", err
	}
	runnerRecord.GitHubRunnerID = githubRunnerID
	now := time.Now().UTC()
	registrationStage := store.DiagnosticStage{
		State:     "degraded",
		Code:      "runner_registration_pending",
		Message:   "runner status updated but GitHub runner ID is not resolved yet",
		UpdatedAt: now,
	}
	if githubRunnerID > 0 {
		registrationStage = store.DiagnosticStage{
			State:     "passed",
			Code:      "runner_registered",
			Message:   "runner is registered in GitHub",
			Details:   map[string]any{"githubRunnerId": githubRunnerID, "runnerName": runnerRecord.RunnerName},
			UpdatedAt: now,
		}
	}
	if err := updateDiagnosticStage(ctx, deps, jobRecord.ID, admission.StageRunnerRegistration, registrationStage, func(diagnostic *store.JobDiagnostic) {
		runnerID := runnerRecord.ID
		diagnostic.RunnerID = &runnerID
		diagnostic.InstanceOCID = runnerRecord.InstanceOCID
		if githubRunnerID > 0 {
			diagnostic.SummaryCode = "runner_registered"
			diagnostic.BlockingStage = ""
		}
	}); err != nil {
		return "", err
	}
	attachmentCode := "runner_status_synced"
	attachmentMessage := fmt.Sprintf("runner %s updated to %s", runnerRecord.RunnerName, targetStatus)
	if strings.EqualFold(strings.TrimSpace(targetStatus), "in_progress") {
		attachmentCode = "runner_attached"
		attachmentMessage = "runner attached to workflow job"
	}
	if err := updateDiagnosticStage(ctx, deps, jobRecord.ID, admission.StageRunnerAttachment, store.DiagnosticStage{
		State:     "passed",
		Code:      attachmentCode,
		Message:   attachmentMessage,
		Details:   map[string]any{"runnerId": runnerRecord.ID, "runnerName": runnerRecord.RunnerName, "githubRunnerId": githubRunnerID, "status": targetStatus},
		UpdatedAt: now,
	}, func(diagnostic *store.JobDiagnostic) {
		runnerID := runnerRecord.ID
		diagnostic.RunnerID = &runnerID
		diagnostic.InstanceOCID = runnerRecord.InstanceOCID
		diagnostic.SummaryCode = attachmentCode
		diagnostic.BlockingStage = ""
	}); err != nil {
		return "", err
	}
	teardownMessage, teardownErr := maybeTerminateRunnerAfterWorkflowCompletion(ctx, deps, githubClient, deliveryID, runnerRecord, targetStatus)
	if teardownErr != nil {
		_ = updateDiagnosticStage(ctx, deps, jobRecord.ID, admission.StageCleanup, store.DiagnosticStage{
			State:     "blocked",
			Code:      "cleanup_failed",
			Message:   teardownErr.Error(),
			UpdatedAt: time.Now().UTC(),
		}, func(diagnostic *store.JobDiagnostic) {
			diagnostic.SummaryCode = "cleanup_failed"
			diagnostic.BlockingStage = admission.StageCleanup
		})
		_ = deps.Store.AddEventLog(ctx, store.EventLog{
			DeliveryID: deliveryID,
			Level:      "warn",
			Message:    teardownErr.Error(),
		})
	} else if teardownMessage != "" {
		cleanupState := "queued"
		cleanupCode := "cleanup_queued"
		if strings.Contains(strings.ToLower(teardownMessage), "executed") {
			cleanupState = "passed"
			cleanupCode = "cleanup_executed"
		}
		_ = updateDiagnosticStage(ctx, deps, jobRecord.ID, admission.StageCleanup, store.DiagnosticStage{
			State:     cleanupState,
			Code:      cleanupCode,
			Message:   teardownMessage,
			Details:   map[string]any{"runnerId": runnerRecord.ID, "runnerName": runnerRecord.RunnerName},
			UpdatedAt: time.Now().UTC(),
		}, func(diagnostic *store.JobDiagnostic) {
			diagnostic.SummaryCode = cleanupCode
			diagnostic.BlockingStage = ""
		})
	}
	if githubRunnerID > 0 {
		return joinRunnerMessages(
			fmt.Sprintf("runner %s synced as %d", runnerRecord.RunnerName, githubRunnerID),
			teardownMessage,
		), nil
	}
	return joinRunnerMessages(
		fmt.Sprintf("runner %s updated without GitHub ID", runnerRecord.RunnerName),
		teardownMessage,
	), nil
}

func resolveGitHubRunnerID(ctx context.Context, githubClient *githubapp.Client, runnerRecord store.Runner, payload workflowJobEvent) (int64, error) {
	if payload.WorkflowJob.RunnerID > 0 {
		return payload.WorkflowJob.RunnerID, nil
	}
	if runnerRecord.GitHubRunnerID > 0 {
		return runnerRecord.GitHubRunnerID, nil
	}
	runnerName := strings.TrimSpace(runnerRecord.RunnerName)
	if runnerName == "" {
		runnerName = strings.TrimSpace(payload.WorkflowJob.RunnerName)
	}
	match, err := githubClient.FindRepoRunnerByName(ctx, runnerRecord.InstallationID, runnerRecord.RepoOwner, runnerRecord.RepoName, runnerName)
	if err != nil {
		return 0, err
	}
	if match == nil {
		return 0, nil
	}
	return match.ID, nil
}

func workflowJobStatus(payload workflowJobEvent) string {
	switch strings.ToLower(strings.TrimSpace(payload.Action)) {
	case "queued":
		return "queued"
	case "in_progress":
		return "in_progress"
	case "completed":
		switch strings.ToLower(strings.TrimSpace(payload.WorkflowJob.Conclusion)) {
		case "failure", "timed_out", "action_required", "startup_failure":
			return "failed"
		case "cancelled":
			return "cancelled"
		default:
			return "completed"
		}
	default:
		status := strings.ToLower(strings.TrimSpace(payload.WorkflowJob.Status))
		if status != "" {
			return status
		}
		return strings.ToLower(strings.TrimSpace(payload.Action))
	}
}

func maybeTerminateRunnerAfterWorkflowCompletion(ctx context.Context, deps Dependencies, githubClient *githubapp.Client, deliveryID string, runnerRecord store.Runner, targetStatus string) (string, error) {
	if !isTerminalRunnerStatus(targetStatus) {
		return "", nil
	}
	if deps.Cleanup != nil {
		result, err := deps.Cleanup.RunOnce(ctx)
		if err != nil {
			return "cleanup queued after terminal job state", err
		}
		if result.Terminated > 0 {
			return "cleanup executed after terminal job state", nil
		}
	}
	return "cleanup queued after terminal job state", nil
}

func isTerminalRunnerStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func joinRunnerMessages(parts ...string) string {
	out := []string{}
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, ", ")
}

type workflowJobEvent struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	WorkflowJob struct {
		ID         int64    `json:"id"`
		RunID      int64    `json:"run_id"`
		RunAttempt int      `json:"run_attempt"`
		Status     string   `json:"status"`
		Conclusion string   `json:"conclusion"`
		RunnerID   int64    `json:"runner_id"`
		RunnerName string   `json:"runner_name"`
		Labels     []string `json:"labels"`
	} `json:"workflow_job"`
}

type installationWebhookEvent struct {
	Action       string `json:"action"`
	Installation struct {
		ID      int64 `json:"id"`
		Account struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
		RepositorySelection string `json:"repository_selection"`
	} `json:"installation"`
}

func parseInstallationWebhookEvent(body []byte) (installationWebhookEvent, error) {
	var payload installationWebhookEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		return installationWebhookEvent{}, err
	}
	return payload, nil
}

func processInstallationWebhook(ctx context.Context, deps Dependencies, resolution githubapp.WebhookResolution, eventType string, payload installationWebhookEvent) error {
	client := resolution.Client
	if client == nil || client.AuthMode() != store.GitHubAuthModeApp {
		return nil
	}
	accountLogin := strings.TrimSpace(payload.Installation.Account.Login)
	accountType := strings.TrimSpace(payload.Installation.Account.Type)
	selection := strings.ToLower(strings.TrimSpace(payload.Installation.RepositorySelection))
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "installation":
		switch strings.ToLower(strings.TrimSpace(payload.Action)) {
		case "deleted":
			return deps.GitHub.RecordInstallationStatus(ctx, resolution.Config, "deleted", accountLogin, accountType, selection, nil, "")
		case "suspend", "suspended":
			return deps.GitHub.RecordInstallationStatus(ctx, resolution.Config, "suspended", accountLogin, accountType, selection, nil, "")
		default:
			return deps.GitHub.RefreshInstallationSnapshot(ctx, resolution.Config, client, accountLogin, accountType, selection)
		}
	case "installation_repositories":
		return deps.GitHub.RefreshInstallationSnapshot(ctx, resolution.Config, client, accountLogin, accountType, selection)
	default:
		return nil
	}
}

func githubTraceSnapshot(record *store.GitHubConfig) (int64, string, []string) {
	if record == nil {
		return 0, "", nil
	}
	return record.ID, strings.TrimSpace(record.Name), append([]string(nil), record.Tags...)
}
