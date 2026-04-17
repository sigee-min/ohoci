package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	mysql "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

func resolveDatabase(databaseURL, sqlitePath string) (string, string, error) {
	trimmedURL := strings.TrimSpace(databaseURL)
	if trimmedURL == "" {
		path := filepath.Clean(strings.TrimSpace(sqlitePath))
		if path == "" {
			return "", "", fmt.Errorf("sqlite path is required")
		}
		return "sqlite", path, nil
	}
	parsed, err := url.Parse(trimmedURL)
	if err != nil {
		return "", "", fmt.Errorf("parse OHOCI_DATABASE_URL: %w", err)
	}
	switch parsed.Scheme {
	case "mysql":
		cfg := mysql.NewConfig()
		if parsed.User != nil {
			cfg.User = parsed.User.Username()
			cfg.Passwd, _ = parsed.User.Password()
		}
		cfg.Net = "tcp"
		cfg.Addr = parsed.Host
		cfg.DBName = strings.TrimPrefix(parsed.Path, "/")
		cfg.ParseTime = true
		cfg.MultiStatements = true
		cfg.Params = map[string]string{}
		for key, values := range parsed.Query() {
			if len(values) == 0 {
				continue
			}
			cfg.Params[key] = values[len(values)-1]
		}
		cfg.Params["parseTime"] = "true"
		return "mysql", cfg.FormatDSN(), nil
	default:
		return "", "", fmt.Errorf("unsupported database scheme %q", parsed.Scheme)
	}
}

func prepareDatabase(driver, dsn string) error {
	if driver != "sqlite" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
		return fmt.Errorf("create sqlite parent directory: %w", err)
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	var statements []string
	switch s.dialect {
	case "sqlite":
		statements = sqliteMigrations
	case "mysql":
		statements = mysqlMigrations
	default:
		return fmt.Errorf("unsupported dialect %s", s.dialect)
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			if isIgnorableMigrationError(err) {
				continue
			}
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

func (s *Store) ensureBootstrapAdmin(ctx context.Context) error {
	resetForLocal := bootstrapAdminResetEnabled()
	_, err := s.FindUserByUsername(ctx, "admin")
	adminExists := err == nil
	switch {
	case adminExists && !resetForLocal:
		return nil
	case err != nil && !errors.Is(err, ErrNotFound):
		return err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	if adminExists {
		if _, err := s.db.ExecContext(
			ctx,
			`UPDATE users
			 SET password_hash = ?, must_change_password = ?, failed_attempts = 0, locked_until = NULL, updated_at = ?
			 WHERE username = ?`,
			string(passwordHash),
			boolAsInt(true),
			now,
			"admin",
		); err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx, `DELETE FROM login_attempts WHERE username = ?`, "admin")
		return err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO users (username, password_hash, must_change_password, failed_attempts, locked_until, last_login_at, created_at, updated_at)
		 VALUES (?, ?, ?, 0, NULL, NULL, ?, ?)`,
		"admin",
		string(passwordHash),
		boolAsInt(true),
		now,
		now,
	)
	return err
}

func bootstrapAdminResetEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OHOCI_ENV"))) {
	case "", "local", "dev", "development":
		return true
	default:
		return false
	}
}
