package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"ohoci/internal/session"
	"ohoci/internal/store"
)

const (
	defaultLockoutAttempts = 15
	defaultLockoutDuration = 5 * time.Minute
)

type SessionView struct {
	Authenticated      bool       `json:"authenticated"`
	UserID             int64      `json:"userId,omitempty"`
	Username           string     `json:"username,omitempty"`
	MustChangePassword bool       `json:"mustChangePassword,omitempty"`
	LockedUntil        *time.Time `json:"lockedUntil,omitempty"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
}

type Policy struct {
	LockoutAttempts int
	LockoutDuration time.Duration
}

type Service struct {
	store           *store.Store
	sessions        *session.Service
	lockoutAttempts int
	lockoutDuration time.Duration
	now             func() time.Time
}

func New(s *store.Store, sessions *session.Service) *Service {
	return NewWithPolicy(s, sessions, Policy{})
}

func NewWithPolicy(s *store.Store, sessions *session.Service, policy Policy) *Service {
	policy = normalizedPolicy(policy)
	return &Service{
		store:           s,
		sessions:        sessions,
		lockoutAttempts: policy.LockoutAttempts,
		lockoutDuration: policy.LockoutDuration,
		now:             time.Now,
	}
}

func (s *Service) Login(ctx context.Context, username, password, ip string) (string, SessionView, error) {
	username = strings.TrimSpace(strings.ToLower(username))
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return "", SessionView{}, fmt.Errorf("username and password are required")
	}

	user, err := s.store.FindUserByUsername(ctx, username)
	if err != nil {
		_ = s.store.UpdateLoginAttemptFailure(ctx, username, ip, s.lockoutAttempts, s.lockoutDuration)
		return "", SessionView{}, fmt.Errorf("invalid credentials")
	}
	if isLocked(user.LockedUntil, s.now()) {
		return "", SessionView{
			Authenticated: false,
			LockedUntil:   user.LockedUntil,
		}, fmt.Errorf("account is temporarily locked")
	}
	if attempt, err := s.store.GetLoginAttempt(ctx, username, ip); err == nil && isLocked(attempt.LockedUntil, s.now()) {
		return "", SessionView{
			Authenticated: false,
			LockedUntil:   attempt.LockedUntil,
		}, fmt.Errorf("ip address is temporarily locked")
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		_ = s.store.UpdateUserLoginFailure(ctx, user.ID, s.lockoutAttempts, s.lockoutDuration)
		_ = s.store.UpdateLoginAttemptFailure(ctx, username, ip, s.lockoutAttempts, s.lockoutDuration)
		return "", SessionView{}, fmt.Errorf("invalid credentials")
	}

	if err := s.store.UpdateUserLoginSuccess(ctx, user.ID); err != nil {
		return "", SessionView{}, err
	}
	_ = s.store.ResetLoginAttempt(ctx, username, ip)
	token, expiresAt, err := s.sessions.Create(ctx, user.ID)
	if err != nil {
		return "", SessionView{}, err
	}
	return token, SessionView{
		Authenticated:      true,
		UserID:             user.ID,
		Username:           user.Username,
		MustChangePassword: user.MustChangePassword,
		ExpiresAt:          &expiresAt,
	}, nil
}

func (s *Service) SessionFromToken(ctx context.Context, token string) (SessionView, error) {
	record, err := s.sessions.Resolve(ctx, token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return SessionView{Authenticated: false}, fmt.Errorf("authenticated session is required")
		}
		return SessionView{}, err
	}
	user, err := s.store.FindUserByID(ctx, record.UserID)
	if err != nil {
		return SessionView{}, err
	}
	expiresAt := record.ExpiresAt.UTC()
	return SessionView{
		Authenticated:      true,
		UserID:             user.ID,
		Username:           user.Username,
		MustChangePassword: user.MustChangePassword,
		ExpiresAt:          &expiresAt,
	}, nil
}

func (s *Service) ChangePassword(ctx context.Context, token, currentPassword, newPassword string) error {
	record, err := s.sessions.Resolve(ctx, token)
	if err != nil {
		return fmt.Errorf("authenticated session is required")
	}
	user, err := s.store.FindUserByID(ctx, record.UserID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(strings.TrimSpace(currentPassword))) != nil {
		return fmt.Errorf("current password is invalid")
	}
	if len(strings.TrimSpace(newPassword)) < 8 {
		return fmt.Errorf("new password must be at least 8 characters")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(strings.TrimSpace(newPassword)), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.store.UpdateUserPassword(ctx, user.ID, string(passwordHash))
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return s.sessions.Delete(ctx, token)
}

func isLocked(until *time.Time, now time.Time) bool {
	return until != nil && until.After(now.UTC())
}

func normalizedPolicy(policy Policy) Policy {
	if policy.LockoutAttempts <= 0 {
		policy.LockoutAttempts = defaultLockoutAttempts
	}
	if policy.LockoutDuration <= 0 {
		policy.LockoutDuration = defaultLockoutDuration
	}
	return policy
}
