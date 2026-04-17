package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Runner struct {
	ID               int64      `json:"id"`
	PolicyID         int64      `json:"policyId"`
	JobID            int64      `json:"jobId"`
	InstallationID   int64      `json:"installationId"`
	GitHubConfigID   int64      `json:"githubConfigId,omitempty"`
	GitHubConfigName string     `json:"githubConfigName,omitempty"`
	GitHubConfigTags []string   `json:"githubConfigTags,omitempty"`
	InstanceOCID     string     `json:"instanceOcid"`
	GitHubRunnerID   int64      `json:"githubRunnerId"`
	RepoOwner        string     `json:"repoOwner"`
	RepoName         string     `json:"repoName"`
	RunnerName       string     `json:"runnerName"`
	Status           string     `json:"status"`
	Labels           []string   `json:"labels"`
	Source           string     `json:"source"`
	WarmState        string     `json:"warmState"`
	WarmPolicyID     *int64     `json:"warmPolicyId,omitempty"`
	WarmRepoOwner    string     `json:"warmRepoOwner,omitempty"`
	WarmRepoName     string     `json:"warmRepoName,omitempty"`
	LaunchedAt       *time.Time `json:"launchedAt,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	TerminatedAt     *time.Time `json:"terminatedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

const runnerColumns = `id, policy_id, job_id, installation_id, github_config_id, github_config_name, github_config_tags_json, instance_ocid, github_runner_id, repo_owner, repo_name, runner_name, status, labels_json, source, warm_state, warm_policy_id, warm_repo_owner, warm_repo_name, launched_at, expires_at, terminated_at, created_at, updated_at`

func (s *Store) CreateRunner(ctx context.Context, runner Runner) (Runner, error) {
	now := s.now().UTC()
	traceTagsJSON, err := json.Marshal(normalizeStrings(runner.GitHubConfigTags))
	if err != nil {
		return Runner{}, err
	}
	labelsJSON, err := json.Marshal(normalizeLabels(runner.Labels))
	if err != nil {
		return Runner{}, err
	}
	if runner.LaunchedAt == nil {
		runner.LaunchedAt = &now
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO runners (policy_id, job_id, installation_id, github_config_id, github_config_name, github_config_tags_json, instance_ocid, github_runner_id, repo_owner, repo_name, runner_name, status, labels_json, source, warm_state, warm_policy_id, warm_repo_owner, warm_repo_name, launched_at, expires_at, terminated_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runner.PolicyID, runner.JobID, runner.InstallationID, runner.GitHubConfigID, strings.TrimSpace(runner.GitHubConfigName), string(traceTagsJSON), runner.InstanceOCID, runner.GitHubRunnerID, runner.RepoOwner, runner.RepoName, runner.RunnerName, runner.Status, string(labelsJSON), strings.TrimSpace(runner.Source), strings.TrimSpace(runner.WarmState), nullableInt64(runner.WarmPolicyID), strings.TrimSpace(runner.WarmRepoOwner), strings.TrimSpace(runner.WarmRepoName), runner.LaunchedAt, runner.ExpiresAt, runner.TerminatedAt, now, now)
	if err != nil {
		return Runner{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Runner{}, err
	}
	return s.FindRunnerByID(ctx, id)
}

func (s *Store) FindRunnerByID(ctx context.Context, id int64) (Runner, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+runnerColumns+` FROM runners WHERE id = ?`, id)
	return scanRunner(row)
}

func (s *Store) FindLatestRunnerByJobID(ctx context.Context, jobID int64) (Runner, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+runnerColumns+` FROM runners WHERE job_id = ? ORDER BY id DESC LIMIT 1`, jobID)
	return scanRunner(row)
}

func scanRunner(scanner interface{ Scan(dest ...any) error }) (Runner, error) {
	var runner Runner
	var traceTagsJSON string
	var labelsJSON string
	var launched, expires, terminated sql.NullTime
	var warmPolicyID sql.NullInt64
	if err := scanner.Scan(&runner.ID, &runner.PolicyID, &runner.JobID, &runner.InstallationID, &runner.GitHubConfigID, &runner.GitHubConfigName, &traceTagsJSON, &runner.InstanceOCID, &runner.GitHubRunnerID, &runner.RepoOwner, &runner.RepoName, &runner.RunnerName, &runner.Status, &labelsJSON, &runner.Source, &runner.WarmState, &warmPolicyID, &runner.WarmRepoOwner, &runner.WarmRepoName, &launched, &expires, &terminated, &runner.CreatedAt, &runner.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Runner{}, ErrNotFound
		}
		return Runner{}, err
	}
	_ = json.Unmarshal([]byte(traceTagsJSON), &runner.GitHubConfigTags)
	_ = json.Unmarshal([]byte(labelsJSON), &runner.Labels)
	if warmPolicyID.Valid {
		runner.WarmPolicyID = &warmPolicyID.Int64
	}
	runner.GitHubConfigName = strings.TrimSpace(runner.GitHubConfigName)
	runner.GitHubConfigTags = normalizeStrings(runner.GitHubConfigTags)
	if launched.Valid {
		value := launched.Time.UTC()
		runner.LaunchedAt = &value
	}
	if expires.Valid {
		value := expires.Time.UTC()
		runner.ExpiresAt = &value
	}
	if terminated.Valid {
		value := terminated.Time.UTC()
		runner.TerminatedAt = &value
	}
	return runner, nil
}

func (s *Store) ListRunners(ctx context.Context, limit int) ([]Runner, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+runnerColumns+` FROM runners ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runners := []Runner{}
	for rows.Next() {
		runner, err := scanRunner(rows)
		if err != nil {
			return nil, err
		}
		runners = append(runners, runner)
	}
	return runners, rows.Err()
}

func (s *Store) ListCleanupCandidates(ctx context.Context, now time.Time) ([]Runner, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+runnerColumns+` FROM runners WHERE terminated_at IS NULL ORDER BY updated_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Runner{}
	for rows.Next() {
		runner, err := scanRunner(rows)
		if err != nil {
			return nil, err
		}
		if runner.ExpiresAt != nil && !runner.ExpiresAt.After(now.UTC()) {
			out = append(out, runner)
			continue
		}
		switch strings.ToLower(strings.TrimSpace(runner.Status)) {
		case "completed", "failed", "cancelled", "stopped":
			out = append(out, runner)
		}
	}
	return out, rows.Err()
}

func (s *Store) ListBillingRunners(ctx context.Context, windowStart, windowEnd time.Time) ([]Runner, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT `+runnerColumns+`
		 FROM runners
		 WHERE launched_at IS NOT NULL
		   AND launched_at <= ?
		   AND (terminated_at IS NULL OR terminated_at >= ?)
		 ORDER BY launched_at ASC, id ASC`,
		windowEnd.UTC(),
		windowStart.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Runner{}
	for rows.Next() {
		runner, err := scanRunner(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, runner)
	}
	return items, rows.Err()
}

func (s *Store) CountActiveRunnersForPolicy(ctx context.Context, policyID int64) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runners WHERE policy_id = ? AND terminated_at IS NULL AND status NOT IN ('terminated')`, policyID)
	var count int
	err := row.Scan(&count)
	return count, err
}

func (s *Store) UpdateRunnerStatus(ctx context.Context, id int64, status string, githubRunnerID int64, terminatedAt *time.Time) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE runners
		 SET status = ?,
		     github_runner_id = CASE WHEN ? > 0 THEN ? ELSE github_runner_id END,
		     terminated_at = CASE WHEN ? IS NULL THEN terminated_at ELSE ? END,
		     updated_at = ?
		 WHERE id = ?`,
		strings.TrimSpace(status),
		githubRunnerID,
		githubRunnerID,
		terminatedAt,
		terminatedAt,
		s.now().UTC(),
		id,
	)
	return err
}

func (s *Store) UpdateRunnerWarmState(ctx context.Context, id int64, jobID int64, warmState string) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE runners
		 SET job_id = CASE WHEN ? > 0 THEN ? ELSE job_id END,
		     warm_state = ?,
		     updated_at = ?
		 WHERE id = ?`,
		jobID,
		jobID,
		strings.TrimSpace(warmState),
		s.now().UTC(),
		id,
	)
	return err
}

func (s *Store) FindWarmIdleRunner(ctx context.Context, policyID int64, repoOwner, repoName string) (Runner, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT `+runnerColumns+`
		 FROM runners
		 WHERE policy_id = ?
		   AND terminated_at IS NULL
		   AND source = 'warm'
		   AND warm_state = 'warm_idle'
		   AND lower(trim(repo_owner)) = lower(trim(?))
		   AND lower(trim(repo_name)) = lower(trim(?))
		 ORDER BY updated_at ASC, id ASC
		 LIMIT 1`,
		policyID,
		repoOwner,
		repoName,
	)
	return scanRunner(row)
}

func (s *Store) FindActiveWarmRunnerByTarget(ctx context.Context, policyID int64, repoOwner, repoName string) (Runner, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT `+runnerColumns+`
		 FROM runners
		 WHERE policy_id = ?
		   AND terminated_at IS NULL
		   AND source = 'warm'
		   AND lower(trim(repo_owner)) = lower(trim(?))
		   AND lower(trim(repo_name)) = lower(trim(?))
		 ORDER BY updated_at DESC, id DESC
		 LIMIT 1`,
		policyID,
		repoOwner,
		repoName,
	)
	return scanRunner(row)
}

func (s *Store) ListActiveWarmRunners(ctx context.Context) ([]Runner, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT `+runnerColumns+`
		 FROM runners
		 WHERE terminated_at IS NULL
		   AND source = 'warm'
		 ORDER BY updated_at ASC, id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Runner{}
	for rows.Next() {
		item, err := scanRunner(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) MarkRunnerTerminated(ctx context.Context, id int64) error {
	now := s.now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE runners SET status = 'terminated', terminated_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	return err
}
