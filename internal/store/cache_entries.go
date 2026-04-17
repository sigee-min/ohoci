package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type CacheEntry struct {
	ID             int64      `json:"id"`
	RepoOwner      string     `json:"repoOwner"`
	RepoName       string     `json:"repoName"`
	CacheKey       string     `json:"cacheKey"`
	CacheVersion   string     `json:"cacheVersion"`
	ObjectName     string     `json:"objectName"`
	SizeBytes      int64      `json:"sizeBytes"`
	CreatedAt      time.Time  `json:"createdAt"`
	LastAccessedAt time.Time  `json:"lastAccessedAt"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
}

const cacheEntryColumns = `id, repo_owner, repo_name, cache_key, cache_version, object_name, size_bytes, created_at, last_accessed_at, expires_at`

func (s *Store) UpsertCacheEntry(ctx context.Context, entry CacheEntry) (CacheEntry, error) {
	now := s.now().UTC()
	if existing, err := s.FindCacheEntry(ctx, entry.RepoOwner, entry.RepoName, entry.CacheKey, entry.CacheVersion); err == nil {
		_, err = s.db.ExecContext(
			ctx,
			`UPDATE cache_entries
			 SET object_name = ?, size_bytes = ?, last_accessed_at = ?, expires_at = ?
			 WHERE id = ?`,
			entry.ObjectName,
			entry.SizeBytes,
			now,
			entry.ExpiresAt,
			existing.ID,
		)
		if err != nil {
			return CacheEntry{}, err
		}
		return s.FindCacheEntry(ctx, entry.RepoOwner, entry.RepoName, entry.CacheKey, entry.CacheVersion)
	} else if !errors.Is(err, ErrNotFound) {
		return CacheEntry{}, err
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO cache_entries (repo_owner, repo_name, cache_key, cache_version, object_name, size_bytes, created_at, last_accessed_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(entry.RepoOwner),
		strings.TrimSpace(entry.RepoName),
		strings.TrimSpace(entry.CacheKey),
		strings.TrimSpace(entry.CacheVersion),
		strings.TrimSpace(entry.ObjectName),
		entry.SizeBytes,
		now,
		now,
		entry.ExpiresAt,
	)
	if err != nil {
		return CacheEntry{}, err
	}
	return s.FindCacheEntry(ctx, entry.RepoOwner, entry.RepoName, entry.CacheKey, entry.CacheVersion)
}

func (s *Store) FindCacheEntry(ctx context.Context, repoOwner, repoName, cacheKey, cacheVersion string) (CacheEntry, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT `+cacheEntryColumns+`
		 FROM cache_entries
		 WHERE lower(trim(repo_owner)) = lower(trim(?))
		   AND lower(trim(repo_name)) = lower(trim(?))
		   AND cache_key = ?
		   AND cache_version = ?
		 LIMIT 1`,
		repoOwner,
		repoName,
		cacheKey,
		cacheVersion,
	)
	return scanCacheEntry(row)
}

func (s *Store) FindCacheEntryByID(ctx context.Context, id int64) (CacheEntry, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+cacheEntryColumns+` FROM cache_entries WHERE id = ?`, id)
	return scanCacheEntry(row)
}

func (s *Store) FindCacheEntriesByPrefix(ctx context.Context, repoOwner, repoName string, prefixes []string, cacheVersion string) ([]CacheEntry, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT `+cacheEntryColumns+`
		 FROM cache_entries
		 WHERE lower(trim(repo_owner)) = lower(trim(?))
		   AND lower(trim(repo_name)) = lower(trim(?))
		   AND cache_version = ?
		 ORDER BY last_accessed_at DESC, id DESC`,
		repoOwner,
		repoName,
		cacheVersion,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []CacheEntry{}
	for rows.Next() {
		item, err := scanCacheEntry(rows)
		if err != nil {
			return nil, err
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(item.CacheKey, prefix) {
				items = append(items, item)
				break
			}
		}
	}
	return items, rows.Err()
}

func (s *Store) TouchCacheEntry(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE cache_entries SET last_accessed_at = ? WHERE id = ?`, s.now().UTC(), id)
	return err
}

func scanCacheEntry(scanner interface{ Scan(dest ...any) error }) (CacheEntry, error) {
	var item CacheEntry
	var expiresAt sql.NullTime
	if err := scanner.Scan(&item.ID, &item.RepoOwner, &item.RepoName, &item.CacheKey, &item.CacheVersion, &item.ObjectName, &item.SizeBytes, &item.CreatedAt, &item.LastAccessedAt, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CacheEntry{}, ErrNotFound
		}
		return CacheEntry{}, err
	}
	if expiresAt.Valid {
		value := expiresAt.Time.UTC()
		item.ExpiresAt = &value
	}
	return item, nil
}
