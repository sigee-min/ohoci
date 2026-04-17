package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db      *sql.DB
	dialect string
	now     func() time.Time
}

func Open(ctx context.Context, databaseURL, sqlitePath string) (*Store, error) {
	driver, dsn, err := resolveDatabase(databaseURL, sqlitePath)
	if err != nil {
		return nil, err
	}
	if err := prepareDatabase(driver, dsn); err != nil {
		return nil, err
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxIdleTime(2 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)
	if driver == "sqlite" {
		db.SetMaxOpenConns(8)
		db.SetMaxIdleConns(4)
	} else {
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &Store{db: db, dialect: driver, now: time.Now}
	if err := store.configure(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.ensureBootstrapAdmin(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) configure(ctx context.Context) error {
	if s.dialect != "sqlite" {
		return nil
	}

	for _, statement := range []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`PRAGMA foreign_keys = ON;`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}

	return nil
}
