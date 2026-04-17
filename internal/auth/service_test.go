package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ohoci/internal/session"
	"ohoci/internal/store"
)

func TestChangePasswordClearsMustChangePassword(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sessions := session.New(db, "test-secret", time.Hour)
	service := New(db, sessions)
	token, sessionView, err := service.Login(ctx, "admin", "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !sessionView.MustChangePassword {
		t.Fatalf("expected bootstrap admin to require password change")
	}
	if err := service.ChangePassword(ctx, token, "admin", "new-password"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	view, err := service.SessionFromToken(ctx, token)
	if err != nil {
		t.Fatalf("session from token: %v", err)
	}
	if view.MustChangePassword {
		t.Fatalf("expected must change password to be cleared")
	}
}

func TestLoginUsesConfiguredLockoutPolicy(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sessions := session.New(db, "test-secret", time.Hour)
	service := NewWithPolicy(db, sessions, Policy{
		LockoutAttempts: 2,
		LockoutDuration: 10 * time.Minute,
	})

	if _, _, err := service.Login(ctx, "admin", "wrong-password", "127.0.0.1"); err == nil {
		t.Fatalf("expected first login attempt to fail")
	}
	user, err := db.FindUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("find user after first failure: %v", err)
	}
	if user.FailedAttempts != 1 || user.LockedUntil != nil {
		t.Fatalf("expected one failed attempt without lock, got %#v", user)
	}

	before := time.Now()
	if _, _, err := service.Login(ctx, "admin", "wrong-password", "127.0.0.1"); err == nil {
		t.Fatalf("expected second login attempt to fail")
	}
	after := time.Now()

	user, err = db.FindUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("find user after second failure: %v", err)
	}
	if user.LockedUntil == nil {
		t.Fatalf("expected user lock after configured threshold")
	}
	if user.FailedAttempts != 0 {
		t.Fatalf("expected failed attempts to reset after lock, got %d", user.FailedAttempts)
	}
	minLock := before.Add(9 * time.Minute)
	maxLock := after.Add(11 * time.Minute)
	if user.LockedUntil.Before(minLock) || user.LockedUntil.After(maxLock) {
		t.Fatalf("expected lock around 10m, got %s", user.LockedUntil)
	}

	attempt, err := db.GetLoginAttempt(ctx, "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("get login attempt: %v", err)
	}
	if attempt.LockedUntil == nil {
		t.Fatalf("expected ip lock after configured threshold")
	}
}

func TestLocalStartupResetsBootstrapAdminCredentialsAndLocks(t *testing.T) {
	t.Setenv("OHOCI_ENV", "local")

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "ohoci.db")
	db, err := store.Open(ctx, "", dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	sessions := session.New(db, "test-secret", time.Hour)
	service := New(db, sessions)
	token, _, err := service.Login(ctx, "admin", "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("bootstrap login: %v", err)
	}
	if err := service.ChangePassword(ctx, token, "admin", "new-password"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	user, err := db.FindUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("find user before restart: %v", err)
	}
	if err := db.UpdateUserLoginFailure(ctx, user.ID, 1, 30*time.Minute); err != nil {
		t.Fatalf("lock user before restart: %v", err)
	}
	if err := db.UpdateLoginAttemptFailure(ctx, "admin", "127.0.0.1", 1, 30*time.Minute); err != nil {
		t.Fatalf("lock ip before restart: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	db, err = store.Open(ctx, "", dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sessions = session.New(db, "test-secret", time.Hour)
	service = New(db, sessions)
	if _, sessionView, err := service.Login(ctx, "admin", "admin", "127.0.0.1"); err != nil {
		t.Fatalf("expected local restart to restore bootstrap login: %v", err)
	} else if !sessionView.MustChangePassword {
		t.Fatalf("expected reset bootstrap admin to require password change")
	}

	user, err = db.FindUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("find user after restart: %v", err)
	}
	if user.LockedUntil != nil || user.FailedAttempts != 0 {
		t.Fatalf("expected bootstrap reset to clear user lock state, got %#v", user)
	}
	if _, err := db.GetLoginAttempt(ctx, "admin", "127.0.0.1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected bootstrap reset to clear ip lock, got %v", err)
	}
}

func TestNonLocalStartupKeepsBootstrapAdminPassword(t *testing.T) {
	t.Setenv("OHOCI_ENV", "production")

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "ohoci.db")
	db, err := store.Open(ctx, "", dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	sessions := session.New(db, "test-secret", time.Hour)
	service := New(db, sessions)
	token, _, err := service.Login(ctx, "admin", "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("bootstrap login: %v", err)
	}
	if err := service.ChangePassword(ctx, token, "admin", "new-password"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	db, err = store.Open(ctx, "", dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sessions = session.New(db, "test-secret", time.Hour)
	service = New(db, sessions)
	if _, _, err := service.Login(ctx, "admin", "admin", "127.0.0.1"); err == nil {
		t.Fatalf("expected non-local restart to preserve changed password")
	}
	if _, _, err := service.Login(ctx, "admin", "new-password", "127.0.0.1"); err != nil {
		t.Fatalf("expected changed password to remain valid outside local/dev: %v", err)
	}
}
