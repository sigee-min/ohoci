package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type DiagnosticStage struct {
	State     string         `json:"state"`
	Code      string         `json:"code,omitempty"`
	Message   string         `json:"message,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type JobDiagnostic struct {
	JobID           int64                      `json:"jobId"`
	DeliveryID      string                     `json:"deliveryId"`
	SummaryCode     string                     `json:"summaryCode"`
	BlockingStage   string                     `json:"blockingStage,omitempty"`
	MatchedPolicyID *int64                     `json:"matchedPolicyId,omitempty"`
	RunnerID        *int64                     `json:"runnerId,omitempty"`
	InstanceOCID    string                     `json:"instanceOcid,omitempty"`
	StageStatuses   map[string]DiagnosticStage `json:"stageStatuses"`
	CreatedAt       time.Time                  `json:"createdAt"`
	UpdatedAt       time.Time                  `json:"updatedAt"`
}

const jobDiagnosticColumns = `job_id, delivery_id, summary_code, blocking_stage, matched_policy_id, runner_id, instance_ocid, stage_statuses_json, created_at, updated_at`

func (s *Store) UpsertJobDiagnostic(ctx context.Context, diagnostic JobDiagnostic) (JobDiagnostic, error) {
	now := s.now().UTC()
	if diagnostic.JobID <= 0 {
		return JobDiagnostic{}, errors.New("job id is required")
	}
	if diagnostic.StageStatuses == nil {
		diagnostic.StageStatuses = map[string]DiagnosticStage{}
	}
	stageJSON, err := json.Marshal(diagnostic.StageStatuses)
	if err != nil {
		return JobDiagnostic{}, err
	}

	if _, err := s.FindJobDiagnosticByJobID(ctx, diagnostic.JobID); err == nil {
		_, err = s.db.ExecContext(
			ctx,
			`UPDATE job_diagnostics
			 SET delivery_id = ?, summary_code = ?, blocking_stage = ?, matched_policy_id = ?, runner_id = ?, instance_ocid = ?, stage_statuses_json = ?, updated_at = ?
			 WHERE job_id = ?`,
			diagnostic.DeliveryID,
			diagnostic.SummaryCode,
			diagnostic.BlockingStage,
			nullableInt64(diagnostic.MatchedPolicyID),
			nullableInt64(diagnostic.RunnerID),
			diagnostic.InstanceOCID,
			string(stageJSON),
			now,
			diagnostic.JobID,
		)
		if err != nil {
			return JobDiagnostic{}, err
		}
		return s.FindJobDiagnosticByJobID(ctx, diagnostic.JobID)
	} else if !errors.Is(err, ErrNotFound) {
		return JobDiagnostic{}, err
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO job_diagnostics (job_id, delivery_id, summary_code, blocking_stage, matched_policy_id, runner_id, instance_ocid, stage_statuses_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		diagnostic.JobID,
		diagnostic.DeliveryID,
		diagnostic.SummaryCode,
		diagnostic.BlockingStage,
		nullableInt64(diagnostic.MatchedPolicyID),
		nullableInt64(diagnostic.RunnerID),
		diagnostic.InstanceOCID,
		string(stageJSON),
		now,
		now,
	)
	if err != nil {
		return JobDiagnostic{}, err
	}
	return s.FindJobDiagnosticByJobID(ctx, diagnostic.JobID)
}

func (s *Store) FindJobDiagnosticByJobID(ctx context.Context, jobID int64) (JobDiagnostic, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+jobDiagnosticColumns+` FROM job_diagnostics WHERE job_id = ?`, jobID)
	return scanJobDiagnostic(row)
}

func (s *Store) ListJobDiagnosticsByJobIDs(ctx context.Context, jobIDs []int64) (map[int64]JobDiagnostic, error) {
	items := map[int64]JobDiagnostic{}
	for _, jobID := range jobIDs {
		if jobID <= 0 {
			continue
		}
		item, err := s.FindJobDiagnosticByJobID(ctx, jobID)
		switch {
		case err == nil:
			items[jobID] = item
		case errors.Is(err, ErrNotFound):
			continue
		default:
			return nil, err
		}
	}
	return items, nil
}

func scanJobDiagnostic(scanner interface{ Scan(dest ...any) error }) (JobDiagnostic, error) {
	var item JobDiagnostic
	var matchedPolicyID sql.NullInt64
	var runnerID sql.NullInt64
	var stageJSON string
	if err := scanner.Scan(&item.JobID, &item.DeliveryID, &item.SummaryCode, &item.BlockingStage, &matchedPolicyID, &runnerID, &item.InstanceOCID, &stageJSON, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JobDiagnostic{}, ErrNotFound
		}
		return JobDiagnostic{}, err
	}
	if matchedPolicyID.Valid {
		item.MatchedPolicyID = &matchedPolicyID.Int64
	}
	if runnerID.Valid {
		item.RunnerID = &runnerID.Int64
	}
	item.StageStatuses = map[string]DiagnosticStage{}
	_ = json.Unmarshal([]byte(stageJSON), &item.StageStatuses)
	return item, nil
}
