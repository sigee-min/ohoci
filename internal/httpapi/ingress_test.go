package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ohoci/internal/config"
)

func TestIngressControlsSplitAdminAndWebhookAllowlists(t *testing.T) {
	handler := withIngressControls(config.Config{
		TrustedProxyCIDRs: []string{"203.0.113.0/24"},
		AdminAllowCIDRs:   []string{"198.51.100.0/24"},
		WebhookAllowCIDRs: []string{"192.0.2.0/24"},
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(clientIP(r)))
	}))

	adminAllowed := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	adminAllowed.RemoteAddr = "203.0.113.10:12345"
	adminAllowed.Header.Set("X-Forwarded-For", "198.51.100.77, 203.0.113.20")
	adminAllowedRec := httptest.NewRecorder()
	handler.ServeHTTP(adminAllowedRec, adminAllowed)
	if adminAllowedRec.Code != http.StatusOK {
		t.Fatalf("expected admin request to pass, got %d: %s", adminAllowedRec.Code, adminAllowedRec.Body.String())
	}
	if got := strings.TrimSpace(adminAllowedRec.Body.String()); got != "198.51.100.77" {
		t.Fatalf("expected normalized admin client ip, got %q", got)
	}

	webhookDenied := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", nil)
	webhookDenied.RemoteAddr = "203.0.113.10:12345"
	webhookDenied.Header.Set("X-Forwarded-For", "198.51.100.77, 203.0.113.20")
	webhookDeniedRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookDeniedRec, webhookDenied)
	if webhookDeniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected webhook request to be rejected by webhook allowlist, got %d: %s", webhookDeniedRec.Code, webhookDeniedRec.Body.String())
	}

	webhookAllowed := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", nil)
	webhookAllowed.RemoteAddr = "203.0.113.10:12345"
	webhookAllowed.Header.Set("X-Forwarded-For", "192.0.2.44, 203.0.113.20")
	webhookAllowedRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookAllowedRec, webhookAllowed)
	if webhookAllowedRec.Code != http.StatusOK {
		t.Fatalf("expected webhook request to pass, got %d: %s", webhookAllowedRec.Code, webhookAllowedRec.Body.String())
	}
	if got := strings.TrimSpace(webhookAllowedRec.Body.String()); got != "192.0.2.44" {
		t.Fatalf("expected normalized webhook client ip, got %q", got)
	}

	remoteNotTrusted := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", nil)
	remoteNotTrusted.RemoteAddr = "198.51.100.50:12345"
	remoteNotTrusted.Header.Set("X-Forwarded-For", "192.0.2.44")
	remoteNotTrustedRec := httptest.NewRecorder()
	handler.ServeHTTP(remoteNotTrustedRec, remoteNotTrusted)
	if remoteNotTrustedRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-trusted remote to ignore forwarded headers and be rejected, got %d: %s", remoteNotTrustedRec.Code, remoteNotTrustedRec.Body.String())
	}
}

func TestIngressResolveClientIPFallsBackDeterministicallyWhenAllForwardedHopsAreTrusted(t *testing.T) {
	policy := newIngressPolicy(config.Config{
		TrustedProxyCIDRs: []string{"203.0.113.0/24"},
		AdminAllowCIDRs:   []string{"0.0.0.0/0"},
		WebhookAllowCIDRs: []string{"0.0.0.0/0"},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.20, 203.0.113.30")

	client, ok := policy.resolveClientIP(req)
	if !ok {
		t.Fatalf("expected client ip resolution to succeed")
	}
	if client != "203.0.113.20" {
		t.Fatalf("expected leftmost trusted forwarded ip as deterministic fallback, got %q", client)
	}
}

func TestIngressControlsAllowCacheCompatLaneOutsideAdminAllowlist(t *testing.T) {
	handler := withIngressControls(config.Config{
		TrustedProxyCIDRs: []string{"203.0.113.0/24"},
		AdminAllowCIDRs:   []string{"198.51.100.0/24"},
		WebhookAllowCIDRs: []string{"192.0.2.0/24"},
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(clientIP(r)))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/internal/cache/_apis/artifactcache/cache", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.9, 203.0.113.20")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected cache compat lane to bypass admin allowlist, got %d: %s", rec.Code, rec.Body.String())
	}
}
