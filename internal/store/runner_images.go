package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type RunnerImageRecipe struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	BaseImageOCID     string    `json:"baseImageOcid"`
	SubnetOCID        string    `json:"subnetOcid"`
	Shape             string    `json:"shape"`
	OCPU              int       `json:"ocpu"`
	MemoryGB          int       `json:"memoryGb"`
	ImageDisplayName  string    `json:"imageDisplayName"`
	SetupCommands     []string  `json:"setupCommands"`
	VerifyCommands    []string  `json:"verifyCommands"`
	PromotedBuildID   *int64    `json:"promotedBuildId,omitempty"`
	PromotedImageOCID string    `json:"promotedImageOcid"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type RunnerImageBuild struct {
	ID               int64      `json:"id"`
	RecipeID         int64      `json:"recipeId"`
	RecipeName       string     `json:"recipeName"`
	Status           string     `json:"status"`
	StatusMessage    string     `json:"statusMessage"`
	ErrorMessage     string     `json:"errorMessage"`
	BaseImageOCID    string     `json:"baseImageOcid"`
	SubnetOCID       string     `json:"subnetOcid"`
	Shape            string     `json:"shape"`
	OCPU             int        `json:"ocpu"`
	MemoryGB         int        `json:"memoryGb"`
	ImageDisplayName string     `json:"imageDisplayName"`
	SetupCommands    []string   `json:"setupCommands"`
	VerifyCommands   []string   `json:"verifyCommands"`
	InstanceOCID     string     `json:"instanceOcid"`
	ImageOCID        string     `json:"imageOcid"`
	LaunchedAt       *time.Time `json:"launchedAt,omitempty"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	PromotedAt       *time.Time `json:"promotedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

func (s *Store) ListRunnerImageRecipes(ctx context.Context) ([]RunnerImageRecipe, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, base_image_ocid, subnet_ocid, shape, ocpu, memory_gb, image_display_name, setup_commands_json, verify_commands_json, promoted_build_id, promoted_image_ocid, created_at, updated_at FROM runner_image_recipes ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []RunnerImageRecipe{}
	for rows.Next() {
		item, err := scanRunnerImageRecipe(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) FindRunnerImageRecipeByID(ctx context.Context, id int64) (RunnerImageRecipe, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, description, base_image_ocid, subnet_ocid, shape, ocpu, memory_gb, image_display_name, setup_commands_json, verify_commands_json, promoted_build_id, promoted_image_ocid, created_at, updated_at FROM runner_image_recipes WHERE id = ?`, id)
	return scanRunnerImageRecipe(row)
}

func (s *Store) CreateRunnerImageRecipe(ctx context.Context, recipe RunnerImageRecipe) (RunnerImageRecipe, error) {
	now := s.now().UTC()
	setupJSON, err := json.Marshal(normalizeCommands(recipe.SetupCommands))
	if err != nil {
		return RunnerImageRecipe{}, err
	}
	verifyJSON, err := json.Marshal(normalizeCommands(recipe.VerifyCommands))
	if err != nil {
		return RunnerImageRecipe{}, err
	}

	result, err := s.db.ExecContext(ctx, `INSERT INTO runner_image_recipes (name, description, base_image_ocid, subnet_ocid, shape, ocpu, memory_gb, image_display_name, setup_commands_json, verify_commands_json, promoted_build_id, promoted_image_ocid, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)`,
		recipe.Name,
		recipe.Description,
		recipe.BaseImageOCID,
		recipe.SubnetOCID,
		recipe.Shape,
		recipe.OCPU,
		recipe.MemoryGB,
		recipe.ImageDisplayName,
		string(setupJSON),
		string(verifyJSON),
		recipe.PromotedImageOCID,
		now,
		now,
	)
	if err != nil {
		return RunnerImageRecipe{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return RunnerImageRecipe{}, err
	}
	return s.FindRunnerImageRecipeByID(ctx, id)
}

func (s *Store) UpdateRunnerImageRecipe(ctx context.Context, id int64, recipe RunnerImageRecipe) (RunnerImageRecipe, error) {
	current, err := s.FindRunnerImageRecipeByID(ctx, id)
	if err != nil {
		return RunnerImageRecipe{}, err
	}
	setupJSON, err := json.Marshal(normalizeCommands(recipe.SetupCommands))
	if err != nil {
		return RunnerImageRecipe{}, err
	}
	verifyJSON, err := json.Marshal(normalizeCommands(recipe.VerifyCommands))
	if err != nil {
		return RunnerImageRecipe{}, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE runner_image_recipes SET name = ?, description = ?, base_image_ocid = ?, subnet_ocid = ?, shape = ?, ocpu = ?, memory_gb = ?, image_display_name = ?, setup_commands_json = ?, verify_commands_json = ?, promoted_build_id = ?, promoted_image_ocid = ?, updated_at = ? WHERE id = ?`,
		recipe.Name,
		recipe.Description,
		recipe.BaseImageOCID,
		recipe.SubnetOCID,
		recipe.Shape,
		recipe.OCPU,
		recipe.MemoryGB,
		recipe.ImageDisplayName,
		string(setupJSON),
		string(verifyJSON),
		current.PromotedBuildID,
		current.PromotedImageOCID,
		s.now().UTC(),
		id,
	)
	if err != nil {
		return RunnerImageRecipe{}, err
	}
	return s.FindRunnerImageRecipeByID(ctx, id)
}

func (s *Store) DeleteRunnerImageRecipe(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM runner_image_builds WHERE recipe_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM runner_image_recipes WHERE id = ?`, id)
	return err
}

func (s *Store) MarkRunnerImageRecipePromoted(ctx context.Context, recipeID, buildID int64, imageOCID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE runner_image_recipes SET promoted_build_id = ?, promoted_image_ocid = ?, updated_at = ? WHERE id = ?`, buildID, imageOCID, s.now().UTC(), recipeID)
	return err
}

func (s *Store) ListRunnerImageBuilds(ctx context.Context, limit int) ([]RunnerImageBuild, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, recipe_id, recipe_name, status, status_message, error_message, base_image_ocid, subnet_ocid, shape, ocpu, memory_gb, image_display_name, setup_commands_json, verify_commands_json, instance_ocid, image_ocid, launched_at, completed_at, promoted_at, created_at, updated_at FROM runner_image_builds ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []RunnerImageBuild{}
	for rows.Next() {
		item, err := scanRunnerImageBuild(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListPendingRunnerImageBuilds(ctx context.Context, limit int) ([]RunnerImageBuild, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, recipe_id, recipe_name, status, status_message, error_message, base_image_ocid, subnet_ocid, shape, ocpu, memory_gb, image_display_name, setup_commands_json, verify_commands_json, instance_ocid, image_ocid, launched_at, completed_at, promoted_at, created_at, updated_at FROM runner_image_builds WHERE status NOT IN ('available', 'failed', 'promoted') ORDER BY created_at ASC, id ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []RunnerImageBuild{}
	for rows.Next() {
		item, err := scanRunnerImageBuild(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) FindRunnerImageBuildByID(ctx context.Context, id int64) (RunnerImageBuild, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, recipe_id, recipe_name, status, status_message, error_message, base_image_ocid, subnet_ocid, shape, ocpu, memory_gb, image_display_name, setup_commands_json, verify_commands_json, instance_ocid, image_ocid, launched_at, completed_at, promoted_at, created_at, updated_at FROM runner_image_builds WHERE id = ?`, id)
	return scanRunnerImageBuild(row)
}

func (s *Store) CreateRunnerImageBuild(ctx context.Context, build RunnerImageBuild) (RunnerImageBuild, error) {
	now := s.now().UTC()
	setupJSON, err := json.Marshal(normalizeCommands(build.SetupCommands))
	if err != nil {
		return RunnerImageBuild{}, err
	}
	verifyJSON, err := json.Marshal(normalizeCommands(build.VerifyCommands))
	if err != nil {
		return RunnerImageBuild{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO runner_image_builds (recipe_id, recipe_name, status, status_message, error_message, base_image_ocid, subnet_ocid, shape, ocpu, memory_gb, image_display_name, setup_commands_json, verify_commands_json, instance_ocid, image_ocid, launched_at, completed_at, promoted_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		build.RecipeID,
		build.RecipeName,
		build.Status,
		build.StatusMessage,
		build.ErrorMessage,
		build.BaseImageOCID,
		build.SubnetOCID,
		build.Shape,
		build.OCPU,
		build.MemoryGB,
		build.ImageDisplayName,
		string(setupJSON),
		string(verifyJSON),
		build.InstanceOCID,
		build.ImageOCID,
		build.LaunchedAt,
		build.CompletedAt,
		build.PromotedAt,
		now,
		now,
	)
	if err != nil {
		return RunnerImageBuild{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return RunnerImageBuild{}, err
	}
	return s.FindRunnerImageBuildByID(ctx, id)
}

func (s *Store) UpdateRunnerImageBuild(ctx context.Context, build RunnerImageBuild) error {
	setupJSON, err := json.Marshal(normalizeCommands(build.SetupCommands))
	if err != nil {
		return err
	}
	verifyJSON, err := json.Marshal(normalizeCommands(build.VerifyCommands))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE runner_image_builds SET recipe_id = ?, recipe_name = ?, status = ?, status_message = ?, error_message = ?, base_image_ocid = ?, subnet_ocid = ?, shape = ?, ocpu = ?, memory_gb = ?, image_display_name = ?, setup_commands_json = ?, verify_commands_json = ?, instance_ocid = ?, image_ocid = ?, launched_at = ?, completed_at = ?, promoted_at = ?, updated_at = ? WHERE id = ?`,
		build.RecipeID,
		build.RecipeName,
		build.Status,
		build.StatusMessage,
		build.ErrorMessage,
		build.BaseImageOCID,
		build.SubnetOCID,
		build.Shape,
		build.OCPU,
		build.MemoryGB,
		build.ImageDisplayName,
		string(setupJSON),
		string(verifyJSON),
		build.InstanceOCID,
		build.ImageOCID,
		build.LaunchedAt,
		build.CompletedAt,
		build.PromotedAt,
		s.now().UTC(),
		build.ID,
	)
	return err
}

func scanRunnerImageRecipe(scanner interface{ Scan(dest ...any) error }) (RunnerImageRecipe, error) {
	var item RunnerImageRecipe
	var setupJSON string
	var verifyJSON string
	var promotedBuildID sql.NullInt64
	if err := scanner.Scan(&item.ID, &item.Name, &item.Description, &item.BaseImageOCID, &item.SubnetOCID, &item.Shape, &item.OCPU, &item.MemoryGB, &item.ImageDisplayName, &setupJSON, &verifyJSON, &promotedBuildID, &item.PromotedImageOCID, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RunnerImageRecipe{}, ErrNotFound
		}
		return RunnerImageRecipe{}, err
	}
	_ = json.Unmarshal([]byte(setupJSON), &item.SetupCommands)
	_ = json.Unmarshal([]byte(verifyJSON), &item.VerifyCommands)
	item.SetupCommands = normalizeCommands(item.SetupCommands)
	item.VerifyCommands = normalizeCommands(item.VerifyCommands)
	if promotedBuildID.Valid {
		value := promotedBuildID.Int64
		item.PromotedBuildID = &value
	}
	return item, nil
}

func scanRunnerImageBuild(scanner interface{ Scan(dest ...any) error }) (RunnerImageBuild, error) {
	var item RunnerImageBuild
	var setupJSON string
	var verifyJSON string
	var launchedAt sql.NullTime
	var completedAt sql.NullTime
	var promotedAt sql.NullTime
	if err := scanner.Scan(&item.ID, &item.RecipeID, &item.RecipeName, &item.Status, &item.StatusMessage, &item.ErrorMessage, &item.BaseImageOCID, &item.SubnetOCID, &item.Shape, &item.OCPU, &item.MemoryGB, &item.ImageDisplayName, &setupJSON, &verifyJSON, &item.InstanceOCID, &item.ImageOCID, &launchedAt, &completedAt, &promotedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RunnerImageBuild{}, ErrNotFound
		}
		return RunnerImageBuild{}, err
	}
	_ = json.Unmarshal([]byte(setupJSON), &item.SetupCommands)
	_ = json.Unmarshal([]byte(verifyJSON), &item.VerifyCommands)
	item.SetupCommands = normalizeCommands(item.SetupCommands)
	item.VerifyCommands = normalizeCommands(item.VerifyCommands)
	if launchedAt.Valid {
		value := launchedAt.Time.UTC()
		item.LaunchedAt = &value
	}
	if completedAt.Valid {
		value := completedAt.Time.UTC()
		item.CompletedAt = &value
	}
	if promotedAt.Valid {
		value := promotedAt.Time.UTC()
		item.PromotedAt = &value
	}
	return item, nil
}
