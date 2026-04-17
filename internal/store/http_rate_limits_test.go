package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestAllowHTTPRateLimitEnforcesFixedWindow(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "", filepath.Join(t.TempDir(), "ohoci.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, time.April, 11, 9, 30, 15, 0, time.UTC)
	db.now = func() time.Time { return now }

	for i := 0; i < 2; i++ {
		allowed, retryAfter, err := db.AllowHTTPRateLimit(ctx, "login", "203.0.113.10", 2, time.Minute)
		if err != nil {
			t.Fatalf("allow request %d: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed, retry after %s", i+1, retryAfter)
		}
	}

	allowed, retryAfter, err := db.AllowHTTPRateLimit(ctx, "login", "203.0.113.10", 2, time.Minute)
	if err != nil {
		t.Fatalf("limit request: %v", err)
	}
	if allowed {
		t.Fatalf("expected third request to be limited")
	}
	if retryAfter != 45*time.Second {
		t.Fatalf("expected retry-after 45s, got %s", retryAfter)
	}

	now = now.Add(45 * time.Second)
	allowed, retryAfter, err = db.AllowHTTPRateLimit(ctx, "login", "203.0.113.10", 2, time.Minute)
	if err != nil {
		t.Fatalf("allow next window: %v", err)
	}
	if !allowed {
		t.Fatalf("expected request in next window to be allowed, retry after %s", retryAfter)
	}
}

func TestAllowHTTPRateLimitPersistsAcrossStoreReopen(t *testing.T) {
	ctx := context.Background()
	sqlitePath := filepath.Join(t.TempDir(), "ohoci.db")
	now := time.Date(2026, time.April, 11, 9, 30, 15, 0, time.UTC)

	db, err := Open(ctx, "", sqlitePath)
	if err != nil {
		t.Fatalf("open first store: %v", err)
	}
	db.now = func() time.Time { return now }

	allowed, retryAfter, err := db.AllowHTTPRateLimit(ctx, "admin_writes", "198.51.100.5", 1, time.Minute)
	if err != nil {
		t.Fatalf("allow initial request: %v", err)
	}
	if !allowed {
		t.Fatalf("expected initial request to be allowed, retry after %s", retryAfter)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	reopened, err := Open(ctx, "", sqlitePath)
	if err != nil {
		t.Fatalf("open reopened store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	reopened.now = func() time.Time { return now }

	allowed, retryAfter, err = reopened.AllowHTTPRateLimit(ctx, "admin_writes", "198.51.100.5", 1, time.Minute)
	if err != nil {
		t.Fatalf("limit after reopen: %v", err)
	}
	if allowed {
		t.Fatalf("expected reopened store to preserve limit state")
	}
	if retryAfter != 45*time.Second {
		t.Fatalf("expected retry-after 45s after reopen, got %s", retryAfter)
	}
}

func TestAllowHTTPRateLimitDeniesAcrossWindowCleanupRace(t *testing.T) {
	ctx := context.Background()
	sqlitePath := filepath.Join(t.TempDir(), "ohoci.db")

	oldStore, err := Open(ctx, "", sqlitePath)
	if err != nil {
		t.Fatalf("open old store: %v", err)
	}
	t.Cleanup(func() { _ = oldStore.Close() })

	nextStore, err := Open(ctx, "", sqlitePath)
	if err != nil {
		t.Fatalf("open next store: %v", err)
	}
	t.Cleanup(func() { _ = nextStore.Close() })

	oldNow := time.Date(2026, time.April, 11, 9, 30, 59, 900_000_000, time.UTC)
	nextNow := oldNow.Add(200 * time.Millisecond)
	oldStore.now = func() time.Time { return oldNow }
	nextStore.now = func() time.Time { return nextNow }

	for i := 0; i < 2; i++ {
		allowed, retryAfter, err := oldStore.AllowHTTPRateLimit(ctx, "login", "203.0.113.10", 2, time.Minute)
		if err != nil {
			t.Fatalf("seed request %d: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("seed request %d should be allowed, retry after %s", i+1, retryAfter)
		}
	}

	allowed, retryAfter, err := nextStore.AllowHTTPRateLimit(ctx, "login", "203.0.113.10", 2, time.Minute)
	if err != nil {
		t.Fatalf("next-window request: %v", err)
	}
	if !allowed {
		t.Fatalf("expected next-window request to be allowed, retry after %s", retryAfter)
	}

	allowed, retryAfter, err = oldStore.AllowHTTPRateLimit(ctx, "login", "203.0.113.10", 2, time.Minute)
	if err != nil {
		t.Fatalf("old-window raced request: %v", err)
	}
	if allowed {
		t.Fatalf("expected old-window request to remain denied after next-window cleanup")
	}
	if retryAfter != time.Second {
		t.Fatalf("expected retry-after to clamp to 1s across boundary race, got %s", retryAfter)
	}

	var rows int
	oldWindowStart := oldNow.Truncate(time.Minute)
	if err := oldStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM http_rate_limits WHERE scope = ? AND client_ip = ? AND window_started = ?`, "login", "203.0.113.10", oldWindowStart).Scan(&rows); err != nil {
		t.Fatalf("count old-window buckets: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected old window bucket to remain during grace retention, got %d rows", rows)
	}
}
