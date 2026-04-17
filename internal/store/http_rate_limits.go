package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Store) AllowHTTPRateLimit(ctx context.Context, scope, clientIP string, limit int, window time.Duration) (bool, time.Duration, error) {
	if s == nil {
		return true, 0, nil
	}
	if limit <= 0 {
		return false, 0, fmt.Errorf("rate limit must be positive")
	}
	if window <= 0 {
		return false, 0, fmt.Errorf("rate limit window must be positive")
	}

	now := s.now().UTC()
	windowStart := now.Truncate(window)
	expiresAt := windowStart.Add(window)

	if err := s.deleteExpiredHTTPRateLimits(ctx, now, window); err != nil {
		return false, 0, err
	}

	inserted, err := s.insertHTTPRateLimitWindow(ctx, scope, clientIP, windowStart, expiresAt, now)
	if err != nil {
		return false, 0, err
	}
	if inserted {
		return true, 0, nil
	}

	updated, err := s.incrementHTTPRateLimitWindow(ctx, scope, clientIP, windowStart, now, limit)
	if err != nil {
		return false, 0, err
	}
	if updated {
		return true, 0, nil
	}

	return false, rateLimitRetryAfter(now, expiresAt), nil
}

func (s *Store) deleteExpiredHTTPRateLimits(ctx context.Context, now time.Time, window time.Duration) error {
	cutoff := now.UTC().Add(-window)
	_, err := s.db.ExecContext(ctx, `DELETE FROM http_rate_limits WHERE expires_at <= ?`, cutoff)
	return err
}

func (s *Store) insertHTTPRateLimitWindow(ctx context.Context, scope, clientIP string, windowStart, expiresAt, now time.Time) (bool, error) {
	statement := `INSERT OR IGNORE INTO http_rate_limits (scope, client_ip, window_started, expires_at, request_count, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)`
	if s.dialect == "mysql" {
		statement = `INSERT IGNORE INTO http_rate_limits (scope, client_ip, window_started, expires_at, request_count, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)`
	}
	result, err := s.db.ExecContext(ctx, statement, normalizeRateLimitScope(scope), normalizeRateLimitIP(clientIP), windowStart.UTC(), expiresAt.UTC(), now.UTC(), now.UTC())
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Store) incrementHTTPRateLimitWindow(ctx context.Context, scope, clientIP string, windowStart, now time.Time, limit int) (bool, error) {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE http_rate_limits
		 SET request_count = request_count + 1, updated_at = ?
		 WHERE scope = ? AND client_ip = ? AND window_started = ? AND request_count < ?`,
		now.UTC(),
		normalizeRateLimitScope(scope),
		normalizeRateLimitIP(clientIP),
		windowStart.UTC(),
		limit,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func normalizeRateLimitScope(scope string) string {
	return strings.TrimSpace(scope)
}

func normalizeRateLimitIP(clientIP string) string {
	value := strings.TrimSpace(clientIP)
	if value == "" {
		return "unknown"
	}
	return value
}

func rateLimitRetryAfter(now, expiresAt time.Time) time.Duration {
	retryAfter := expiresAt.UTC().Sub(now.UTC())
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return retryAfter
}
