package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type User struct {
	ID                 int64      `json:"id"`
	Username           string     `json:"username"`
	PasswordHash       string     `json:"-"`
	MustChangePassword bool       `json:"mustChangePassword"`
	FailedAttempts     int        `json:"failedAttempts"`
	LockedUntil        *time.Time `json:"lockedUntil,omitempty"`
	LastLoginAt        *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type Session struct {
	ID          int64
	UserID      int64
	TokenDigest string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	LastSeenAt  time.Time
}

type LoginAttempt struct {
	Username       string
	IPAddress      string
	FailedAttempts int
	LockedUntil    *time.Time
}

func (s *Store) FindUserByUsername(ctx context.Context, username string) (User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, must_change_password, failed_attempts, locked_until, last_login_at, created_at, updated_at FROM users WHERE username = ?`, strings.TrimSpace(username))
	return scanUser(row)
}

func (s *Store) FindUserByID(ctx context.Context, id int64) (User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, must_change_password, failed_attempts, locked_until, last_login_at, created_at, updated_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func scanUser(scanner interface{ Scan(dest ...any) error }) (User, error) {
	var user User
	var lockedUntil sql.NullTime
	var lastLogin sql.NullTime
	var mustChange int
	err := scanner.Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&mustChange,
		&user.FailedAttempts,
		&lockedUntil,
		&lastLogin,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	user.MustChangePassword = mustChange == 1
	if lockedUntil.Valid {
		value := lockedUntil.Time.UTC()
		user.LockedUntil = &value
	}
	if lastLogin.Valid {
		value := lastLogin.Time.UTC()
		user.LastLoginAt = &value
	}
	user.CreatedAt = user.CreatedAt.UTC()
	user.UpdatedAt = user.UpdatedAt.UTC()
	return user, nil
}

func (s *Store) UpdateUserLoginSuccess(ctx context.Context, userID int64) error {
	now := s.now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE users SET failed_attempts = 0, locked_until = NULL, last_login_at = ?, updated_at = ? WHERE id = ?`, now, now, userID)
	return err
}

func (s *Store) UpdateUserLoginFailure(ctx context.Context, userID int64, lockAfter int, lockDuration time.Duration) error {
	user, err := s.FindUserByID(ctx, userID)
	if err != nil {
		return err
	}
	failed := user.FailedAttempts + 1
	var locked any
	if failed >= lockAfter {
		locked = s.now().UTC().Add(lockDuration)
		failed = 0
	} else {
		locked = nil
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET failed_attempts = ?, locked_until = ?, updated_at = ? WHERE id = ?`, failed, locked, s.now().UTC(), userID)
	return err
}

func (s *Store) UpdateUserPassword(ctx context.Context, userID int64, passwordHash string) error {
	now := s.now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, must_change_password = 0, failed_attempts = 0, locked_until = NULL, updated_at = ? WHERE id = ?`, passwordHash, now, userID)
	return err
}

func (s *Store) GetLoginAttempt(ctx context.Context, username, ip string) (LoginAttempt, error) {
	row := s.db.QueryRowContext(ctx, `SELECT username, ip_address, failed_attempts, locked_until FROM login_attempts WHERE username = ? AND ip_address = ?`, username, ip)
	var attempt LoginAttempt
	var locked sql.NullTime
	if err := row.Scan(&attempt.Username, &attempt.IPAddress, &attempt.FailedAttempts, &locked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LoginAttempt{}, ErrNotFound
		}
		return LoginAttempt{}, err
	}
	if locked.Valid {
		value := locked.Time.UTC()
		attempt.LockedUntil = &value
	}
	return attempt, nil
}

func (s *Store) UpdateLoginAttemptFailure(ctx context.Context, username, ip string, lockAfter int, lockDuration time.Duration) error {
	attempt, err := s.GetLoginAttempt(ctx, username, ip)
	shouldInsert := errors.Is(err, ErrNotFound)
	if err != nil && !shouldInsert {
		return err
	}
	failed := 1
	var locked any
	if !shouldInsert {
		failed = attempt.FailedAttempts + 1
	}
	if failed >= lockAfter {
		failed = 0
		locked = s.now().UTC().Add(lockDuration)
	}
	now := s.now().UTC()
	if shouldInsert {
		_, err = s.db.ExecContext(ctx, `INSERT INTO login_attempts (username, ip_address, failed_attempts, locked_until, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, username, ip, failed, locked, now, now)
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE login_attempts SET failed_attempts = ?, locked_until = ?, updated_at = ? WHERE username = ? AND ip_address = ?`, failed, locked, now, username, ip)
	return err
}

func (s *Store) ResetLoginAttempt(ctx context.Context, username, ip string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_attempts WHERE username = ? AND ip_address = ?`, username, ip)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID int64, tokenDigest string, expiresAt time.Time) error {
	now := s.now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (user_id, token_digest, expires_at, created_at, last_seen_at) VALUES (?, ?, ?, ?, ?)`, userID, tokenDigest, expiresAt.UTC(), now, now)
	return err
}

func (s *Store) FindSessionByDigest(ctx context.Context, tokenDigest string) (Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, user_id, token_digest, expires_at, created_at, last_seen_at FROM sessions WHERE token_digest = ?`, tokenDigest)
	var session Session
	if err := row.Scan(&session.ID, &session.UserID, &session.TokenDigest, &session.ExpiresAt, &session.CreatedAt, &session.LastSeenAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrNotFound
		}
		return Session{}, err
	}
	return session, nil
}

func (s *Store) TouchSession(ctx context.Context, tokenDigest string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE token_digest = ?`, s.now().UTC(), tokenDigest)
	return err
}

func (s *Store) DeleteSessionByDigest(ctx context.Context, tokenDigest string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_digest = ?`, tokenDigest)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now.UTC())
	return err
}
