package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"ohoci/internal/store"
)

type Service struct {
	store      *store.Store
	secret     []byte
	sessionTTL time.Duration
	now        func() time.Time
}

func New(s *store.Store, secret string, ttl time.Duration) *Service {
	return &Service{
		store:      s,
		secret:     []byte(strings.TrimSpace(secret)),
		sessionTTL: ttl,
		now:        time.Now,
	}
}

func (s *Service) Create(ctx context.Context, userID int64) (string, time.Time, error) {
	token, err := s.newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.now().UTC().Add(s.sessionTTL)
	if err := s.store.CreateSession(ctx, userID, s.digest(token), expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (s *Service) Resolve(ctx context.Context, token string) (store.Session, error) {
	session, err := s.store.FindSessionByDigest(ctx, s.digest(token))
	if err != nil {
		return store.Session{}, err
	}
	if session.ExpiresAt.Before(s.now().UTC()) {
		_ = s.store.DeleteSessionByDigest(ctx, s.digest(token))
		return store.Session{}, store.ErrNotFound
	}
	_ = s.store.TouchSession(ctx, s.digest(token))
	return session, nil
}

func (s *Service) Delete(ctx context.Context, token string) error {
	return s.store.DeleteSessionByDigest(ctx, s.digest(token))
}

func (s *Service) GC(ctx context.Context) error {
	return s.store.DeleteExpiredSessions(ctx, s.now().UTC())
}

func (s *Service) digest(token string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(strings.TrimSpace(token)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
