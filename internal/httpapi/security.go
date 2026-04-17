package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"ohoci/internal/config"
	"ohoci/internal/store"
)

const (
	defaultLoginRateLimitRequests      = 5
	defaultAdminWriteRateLimitRequests = 30
	defaultRateLimitWindow             = time.Minute
)

const (
	rateLimitScopeLogin       = "login"
	rateLimitScopeAdminWrites = "admin_writes"
)

type fixedWindowRateLimiter struct {
	store  *store.Store
	scope  string
	limit  int
	window time.Duration
}

func newFixedWindowRateLimiter(db *store.Store, scope string, limit int, window time.Duration) *fixedWindowRateLimiter {
	if db == nil {
		return nil
	}
	return &fixedWindowRateLimiter{
		store:  db,
		scope:  scope,
		limit:  limit,
		window: window,
	}
}

func (l *fixedWindowRateLimiter) allow(r *http.Request, key string) (bool, time.Duration, error) {
	if l == nil {
		return true, 0, nil
	}
	return l.store.AllowHTTPRateLimit(r.Context(), l.scope, key, l.limit, l.window)
}

type apiRateLimiters struct {
	login       *fixedWindowRateLimiter
	adminWrites *fixedWindowRateLimiter
}

func newAPIRateLimiters(db *store.Store) apiRateLimiters {
	return apiRateLimiters{
		login:       newFixedWindowRateLimiter(db, rateLimitScopeLogin, defaultLoginRateLimitRequests, defaultRateLimitWindow),
		adminWrites: newFixedWindowRateLimiter(db, rateLimitScopeAdminWrites, defaultAdminWriteRateLimitRequests, defaultRateLimitWindow),
	}
}

func withRateLimits(limits apiRateLimiters, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isLoginRateLimitedRequest(r):
			if !allowRateLimitedRequest(w, r, limits.login) {
				return
			}
		case isSensitiveAdminWriteRequest(r):
			if !allowRateLimitedRequest(w, r, limits.adminWrites) {
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func allowRateLimitedRequest(w http.ResponseWriter, r *http.Request, limiter *fixedWindowRateLimiter) bool {
	if limiter == nil {
		return true
	}

	key := strings.TrimSpace(clientIP(r))
	if key == "" {
		key = "unknown"
	}

	allowed, retryAfter, err := limiter.allow(r, key)
	if err != nil {
		_ = writeStatus(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return false
	}
	if allowed {
		return true
	}

	w.Header().Set("Retry-After", strconvDurationSeconds(retryAfter))
	_ = writeStatus(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests"})
	return false
}

func isLoginRateLimitedRequest(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/login"
}

func isSensitiveAdminWriteRequest(r *http.Request) bool {
	if !isWriteMethod(r.Method) {
		return false
	}
	if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
		return false
	}
	switch r.URL.Path {
	case "/api/v1/auth/login", "/api/v1/github/webhook":
		return false
	default:
		return true
	}
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func withSecurityHeaders(cfg config.Config, next http.Handler) http.Handler {
	contentSecurityPolicy := strings.Join([]string{
		"default-src 'self'",
		"base-uri 'self'",
		"frame-ancestors 'none'",
		"form-action 'self' https://github.com https://*.github.com",
		"img-src 'self' data: https:",
		"script-src 'self' 'unsafe-inline'",
		"style-src 'self' 'unsafe-inline'",
		"font-src 'self' data:",
		"connect-src 'self' https:",
		"object-src 'none'",
	}, "; ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("Content-Security-Policy", contentSecurityPolicy)
		headers.Set("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), gyroscope=(), microphone=(), payment=(), usb=()")
		headers.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		if shouldSetHSTS(cfg, r) {
			headers.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func shouldSetHSTS(cfg config.Config, r *http.Request) bool {
	if r.TLS != nil {
		return true
	}

	forwardedProto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if strings.EqualFold(forwardedProto, "https") {
		return true
	}

	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.PublicBaseURL)), "https://")
}

func strconvDurationSeconds(value time.Duration) string {
	seconds := int(value / time.Second)
	if value%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
