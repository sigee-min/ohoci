package httpapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ohoci/internal/config"
)

func TestLoginRateLimitReturnsTooManyRequests(t *testing.T) {
	handler, _, _, _ := newBackendTestHandler(t, backendTestOptions{})

	for i := 0; i < defaultLoginRateLimitRequests; i++ {
		response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": "admin",
			"password": "wrong-password",
		}, "", "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: expected unauthorized before rate limit, got %d: %s", i+1, response.Code, response.Body.String())
		}
	}

	limited := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "admin",
		"password": "wrong-password",
	}, "", "")
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("expected login rate limit, got %d: %s", limited.Code, limited.Body.String())
	}
	if limited.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header on rate-limited login response")
	}
}

func TestAuthenticatedAdminWriteRateLimitReturnsTooManyRequests(t *testing.T) {
	handler, cfg, _, _ := newBackendTestHandler(t, backendTestOptions{})
	token := authenticatedToken(t, handler.auth)

	for i := 0; i < defaultAdminWriteRateLimitRequests; i++ {
		response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/system/cleanup", nil, cfg.SessionCookieName, token)
		if response.Code != http.StatusForbidden {
			t.Fatalf("request %d: expected protected write to reach normal handler before rate limit, got %d: %s", i+1, response.Code, response.Body.String())
		}
	}

	limited := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/system/cleanup", nil, cfg.SessionCookieName, token)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("expected admin write rate limit, got %d: %s", limited.Code, limited.Body.String())
	}
	if limited.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header on rate-limited admin write response")
	}
}

func TestLoginRateLimitPersistsAcrossHandlerRecreation(t *testing.T) {
	sqlitePath := filepath.Join(t.TempDir(), "ohoci.db")
	first, _, _, _ := newBackendTestHandlerWithSQLitePath(t, sqlitePath, backendTestOptions{})

	for i := 0; i < defaultLoginRateLimitRequests; i++ {
		response := performJSONRequest(t, first.handler, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": "admin",
			"password": "wrong-password",
		}, "", "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: expected unauthorized before rate limit, got %d: %s", i+1, response.Code, response.Body.String())
		}
	}

	second, _, _, _ := newBackendTestHandlerWithSQLitePath(t, sqlitePath, backendTestOptions{})
	limited := performJSONRequest(t, second.handler, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "admin",
		"password": "wrong-password",
	}, "", "")
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("expected recreated handler to preserve login rate limit, got %d: %s", limited.Code, limited.Body.String())
	}
	if limited.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header on recreated handler rate-limited response")
	}
}

func TestRateLimitedResponsesIncludeSecurityHeaders(t *testing.T) {
	handler, _, _, _ := newBackendTestHandler(t, backendTestOptions{})

	for i := 0; i < defaultLoginRateLimitRequests; i++ {
		response := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": "admin",
			"password": "wrong-password",
		}, "", "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: expected unauthorized before rate limit, got %d: %s", i+1, response.Code, response.Body.String())
		}
	}

	limited := performJSONRequest(t, handler.handler, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "admin",
		"password": "wrong-password",
	}, "", "")
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("expected login rate limit, got %d: %s", limited.Code, limited.Body.String())
	}
	if got := limited.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY on 429, got %q", got)
	}
	if got := limited.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options nosniff on 429, got %q", got)
	}
	if got := limited.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("expected Referrer-Policy on 429, got %q", got)
	}
	if got := limited.Header().Get("Permissions-Policy"); got == "" {
		t.Fatalf("expected Permissions-Policy on 429")
	}
	if got := limited.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatalf("expected Content-Security-Policy on 429")
	}
}

func TestSecurityHeadersAreAppliedToNormalResponses(t *testing.T) {
	handler := withSecurityHeaders(config.Config{
		PublicBaseURL: "https://ohoci.example.test",
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if got := response.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY, got %q", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options nosniff, got %q", got)
	}
	if got := response.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("expected Referrer-Policy header, got %q", got)
	}
	if got := response.Header().Get("Permissions-Policy"); got == "" {
		t.Fatalf("expected Permissions-Policy header")
	}
	if got := response.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatalf("expected Content-Security-Policy header")
	}
	if got := response.Header().Get("Strict-Transport-Security"); got == "" {
		t.Fatalf("expected Strict-Transport-Security header when public base URL is https")
	}
}
