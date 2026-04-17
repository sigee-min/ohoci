package httpapi

import (
	"context"

	"ohoci/internal/admission"
	"ohoci/internal/store"
)

func upsertJobDiagnosticFromDecision(ctx context.Context, deps Dependencies, job store.Job, decision admission.Decision) error {
	if deps.Store == nil || job.ID <= 0 {
		return nil
	}
	var matchedPolicyID *int64
	if decision.MatchedPolicy != nil {
		matchedPolicyID = &decision.MatchedPolicy.ID
	}
	_, err := deps.Store.UpsertJobDiagnostic(ctx, store.JobDiagnostic{
		JobID:           job.ID,
		DeliveryID:      job.DeliveryID,
		SummaryCode:     decision.SummaryCode,
		BlockingStage:   decision.BlockingStage,
		MatchedPolicyID: matchedPolicyID,
		StageStatuses:   cloneStageStatuses(decision.StageStatuses),
	})
	return err
}

func updateDiagnosticStage(ctx context.Context, deps Dependencies, jobID int64, stageName string, stageStatus store.DiagnosticStage, mutate func(*store.JobDiagnostic)) error {
	if deps.Store == nil || jobID <= 0 {
		return nil
	}
	diagnostic, err := deps.Store.FindJobDiagnosticByJobID(ctx, jobID)
	if err != nil {
		if err == store.ErrNotFound {
			diagnostic = store.JobDiagnostic{JobID: jobID, StageStatuses: map[string]store.DiagnosticStage{}}
		} else {
			return err
		}
	}
	if diagnostic.StageStatuses == nil {
		diagnostic.StageStatuses = map[string]store.DiagnosticStage{}
	}
	diagnostic.StageStatuses[stageName] = stageStatus
	if mutate != nil {
		mutate(&diagnostic)
	}
	_, err = deps.Store.UpsertJobDiagnostic(ctx, diagnostic)
	return err
}

func cloneStageStatuses(source map[string]store.DiagnosticStage) map[string]store.DiagnosticStage {
	if len(source) == 0 {
		return map[string]store.DiagnosticStage{}
	}
	out := make(map[string]store.DiagnosticStage, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
