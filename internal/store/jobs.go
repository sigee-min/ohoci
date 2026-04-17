package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Job struct {
	ID               int64     `json:"id"`
	GitHubJobID      int64     `json:"githubJobId"`
	DeliveryID       string    `json:"deliveryId"`
	InstallationID   int64     `json:"installationId"`
	GitHubConfigID   int64     `json:"githubConfigId,omitempty"`
	GitHubConfigName string    `json:"githubConfigName,omitempty"`
	GitHubConfigTags []string  `json:"githubConfigTags,omitempty"`
	RepoOwner        string    `json:"repoOwner"`
	RepoName         string    `json:"repoName"`
	RunID            int64     `json:"runId"`
	RunAttempt       int       `json:"runAttempt"`
	Status           string    `json:"status"`
	Labels           []string  `json:"labels"`
	MatchedPolicyID  *int64    `json:"matchedPolicyId,omitempty"`
	ErrorMessage     string    `json:"errorMessage,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

const jobColumns = `id, github_job_id, delivery_id, installation_id, github_config_id, github_config_name, github_config_tags_json, repo_owner, repo_name, run_id, run_attempt, status, labels_json, matched_policy_id, error_message, created_at, updated_at`

func (s *Store) UpsertJob(ctx context.Context, job Job) (Job, error) {
	now := s.now().UTC()
	traceTagsJSON, err := json.Marshal(normalizeStrings(job.GitHubConfigTags))
	if err != nil {
		return Job{}, err
	}
	labelsJSON, err := json.Marshal(normalizeLabels(job.Labels))
	if err != nil {
		return Job{}, err
	}
	existing, err := s.FindJobByGitHubJobID(ctx, job.GitHubJobID)
	if err == nil {
		job = freezeJobGitHubTrace(existing, job)
		traceTagsJSON, err = json.Marshal(normalizeStrings(job.GitHubConfigTags))
		if err != nil {
			return Job{}, err
		}
		_, err = s.db.ExecContext(ctx, `UPDATE jobs SET delivery_id = ?, installation_id = ?, github_config_id = ?, github_config_name = ?, github_config_tags_json = ?, repo_owner = ?, repo_name = ?, run_id = ?, run_attempt = ?, status = ?, labels_json = ?, matched_policy_id = ?, error_message = ?, updated_at = ? WHERE id = ?`,
			job.DeliveryID, job.InstallationID, job.GitHubConfigID, strings.TrimSpace(job.GitHubConfigName), string(traceTagsJSON), job.RepoOwner, job.RepoName, job.RunID, job.RunAttempt, job.Status, string(labelsJSON), nullableInt64(job.MatchedPolicyID), strings.TrimSpace(job.ErrorMessage), now, existing.ID)
		if err != nil {
			return Job{}, err
		}
		return s.FindJobByGitHubJobID(ctx, job.GitHubJobID)
	}
	if !errors.Is(err, ErrNotFound) {
		return Job{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO jobs (github_job_id, delivery_id, installation_id, github_config_id, github_config_name, github_config_tags_json, repo_owner, repo_name, run_id, run_attempt, status, labels_json, matched_policy_id, error_message, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.GitHubJobID, job.DeliveryID, job.InstallationID, job.GitHubConfigID, strings.TrimSpace(job.GitHubConfigName), string(traceTagsJSON), job.RepoOwner, job.RepoName, job.RunID, job.RunAttempt, job.Status, string(labelsJSON), nullableInt64(job.MatchedPolicyID), strings.TrimSpace(job.ErrorMessage), now, now)
	if err != nil {
		return Job{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Job{}, err
	}
	return s.FindJobByID(ctx, id)
}

func (s *Store) FindJobByGitHubJobID(ctx context.Context, githubJobID int64) (Job, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE github_job_id = ?`, githubJobID)
	return scanJob(row)
}

func (s *Store) FindJobByID(ctx context.Context, id int64) (Job, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id)
	return scanJob(row)
}

func scanJob(scanner interface{ Scan(dest ...any) error }) (Job, error) {
	var job Job
	var traceTagsJSON string
	var labelsJSON string
	var matched sql.NullInt64
	if err := scanner.Scan(&job.ID, &job.GitHubJobID, &job.DeliveryID, &job.InstallationID, &job.GitHubConfigID, &job.GitHubConfigName, &traceTagsJSON, &job.RepoOwner, &job.RepoName, &job.RunID, &job.RunAttempt, &job.Status, &labelsJSON, &matched, &job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		return Job{}, err
	}
	if matched.Valid {
		job.MatchedPolicyID = &matched.Int64
	}
	_ = json.Unmarshal([]byte(traceTagsJSON), &job.GitHubConfigTags)
	_ = json.Unmarshal([]byte(labelsJSON), &job.Labels)
	job.GitHubConfigName = strings.TrimSpace(job.GitHubConfigName)
	job.GitHubConfigTags = normalizeStrings(job.GitHubConfigTags)
	return job, nil
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobColumns+` FROM jobs ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func freezeJobGitHubTrace(existing, incoming Job) Job {
	if existing.GitHubConfigID > 0 {
		incoming.GitHubConfigID = existing.GitHubConfigID
	}
	if strings.TrimSpace(existing.GitHubConfigName) != "" {
		incoming.GitHubConfigName = existing.GitHubConfigName
	}
	if len(existing.GitHubConfigTags) > 0 {
		incoming.GitHubConfigTags = append([]string(nil), existing.GitHubConfigTags...)
	}
	return incoming
}
