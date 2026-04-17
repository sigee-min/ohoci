package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"path"
	"strconv"
	"strings"
	"time"

	"ohoci/internal/config"
	"ohoci/internal/store"
)

type clientIPContextKey struct{}

type ingressScope string

const (
	ingressScopeAdmin   ingressScope = "admin"
	ingressScopeCache   ingressScope = "cache"
	ingressScopeWebhook ingressScope = "webhook"
)

func requireUser(deps Dependencies, requirePasswordChanged bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionView, err := deps.Auth.SessionFromToken(r.Context(), readSessionCookie(r, deps.Config.SessionCookieName))
		if err != nil || !sessionView.Authenticated {
			_ = writeStatus(w, http.StatusUnauthorized, map[string]string{"error": "authenticated session is required"})
			return
		}
		if requirePasswordChanged && sessionView.MustChangePassword {
			_ = writeStatus(w, http.StatusForbidden, map[string]string{"error": "password change is required before accessing this resource"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, sessionView)))
	})
}

type ingressPolicy struct {
	trustedProxyCIDRs []netip.Prefix
	adminAllowCIDRs   []netip.Prefix
	webhookAllowCIDRs []netip.Prefix
	buildErr          error
}

func newIngressPolicy(cfg config.Config) ingressPolicy {
	trustedProxyCIDRs, err := parseCIDRPrefixes(cfg.TrustedProxyCIDRs)
	if err != nil {
		return ingressPolicy{buildErr: fmt.Errorf("trusted proxy CIDRs: %w", err)}
	}
	adminAllowCIDRs, err := parseCIDRPrefixes(cfg.AdminAllowCIDRs)
	if err != nil {
		return ingressPolicy{buildErr: fmt.Errorf("admin allow CIDRs: %w", err)}
	}
	webhookAllowCIDRs, err := parseCIDRPrefixes(cfg.WebhookAllowCIDRs)
	if err != nil {
		return ingressPolicy{buildErr: fmt.Errorf("webhook allow CIDRs: %w", err)}
	}
	return ingressPolicy{
		trustedProxyCIDRs: trustedProxyCIDRs,
		adminAllowCIDRs:   adminAllowCIDRs,
		webhookAllowCIDRs: webhookAllowCIDRs,
	}
}

func withIngressControls(cfg config.Config, next http.Handler) http.Handler {
	policy := newIngressPolicy(cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if policy.buildErr != nil {
			_ = writeStatus(w, http.StatusInternalServerError, map[string]string{"error": policy.buildErr.Error()})
			return
		}
		clientIP, ok := policy.resolveClientIP(r)
		if !ok {
			_ = writeStatus(w, http.StatusForbidden, map[string]string{"error": "client ip is not allowed"})
			return
		}
		scope := ingressScopeAdmin
		if strings.HasPrefix(r.URL.Path, "/api/internal/cache/") {
			scope = ingressScopeCache
		} else if r.URL.Path == "/api/v1/github/webhook" {
			scope = ingressScopeWebhook
		}
		if !policy.allowed(scope, clientIP) {
			_ = writeStatus(w, http.StatusForbidden, map[string]string{"error": "client ip is not allowed"})
			return
		}
		ctx := context.WithValue(r.Context(), clientIPContextKey{}, clientIP)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireSetupReady(deps Dependencies, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if deps.Setup == nil {
			next.ServeHTTP(w, r)
			return
		}
		status, err := deps.Setup.CurrentStatus(r.Context())
		if err != nil {
			_ = writeStatus(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if status.Ready {
			next.ServeHTTP(w, r)
			return
		}
		_ = writeStatus(w, http.StatusForbidden, map[string]any{
			"error": "setup_required",
			"setup": status,
		})
	})
}

type userContextKey struct{}

func jsonHandler(fn func(http.ResponseWriter, *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			_ = writeStatus(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
	})
}

func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				_ = writeStatus(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("%v", recovered)})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, payload any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

func writeStatus(w http.ResponseWriter, status int, payload any) error {
	return writeJSON(w, status, payload)
}

func buildCookie(cfg config.Config, token string, expiresAt *time.Time) *http.Cookie {
	cookie := &http.Cookie{
		Name:     cfg.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.HasPrefix(strings.ToLower(cfg.PublicBaseURL), "https://"),
	}
	if expiresAt != nil {
		cookie.Expires = expiresAt.UTC()
	}
	return cookie
}

func expiredCookie(name string) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func readSessionCookie(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func clientIP(r *http.Request) string {
	if clientIP, ok := r.Context().Value(clientIPContextKey{}).(string); ok && clientIP != "" {
		return clientIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return normalizeIPText(r.RemoteAddr)
}

func (p ingressPolicy) resolveClientIP(r *http.Request) (string, bool) {
	remoteAddr, ok := normalizeRequestIP(r.RemoteAddr)
	if !ok {
		return "", false
	}
	if !containsPrefix(remoteAddr, p.trustedProxyCIDRs) {
		return remoteAddr.String(), true
	}
	forwardedIPs := forwardedClientIPs(r.Header.Get("X-Forwarded-For"))
	if len(forwardedIPs) == 0 {
		return remoteAddr.String(), true
	}
	for i := len(forwardedIPs) - 1; i >= 0; i-- {
		if !containsPrefix(forwardedIPs[i], p.trustedProxyCIDRs) {
			return forwardedIPs[i].String(), true
		}
	}
	return forwardedIPs[0].String(), true
}

func (p ingressPolicy) allowed(scope ingressScope, clientIP string) bool {
	ip, err := netip.ParseAddr(strings.TrimSpace(clientIP))
	if err != nil {
		return false
	}
	switch scope {
	case ingressScopeCache:
		return true
	case ingressScopeWebhook:
		return containsPrefix(ip, p.webhookAllowCIDRs)
	default:
		return containsPrefix(ip, p.adminAllowCIDRs)
	}
}

func normalizeRequestIP(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err == nil {
		ip, parseErr := netip.ParseAddr(host)
		return ip, parseErr == nil
	}
	ip, parseErr := netip.ParseAddr(strings.TrimSpace(remoteAddr))
	return ip, parseErr == nil
}

func normalizeIPText(remoteAddr string) string {
	ip, ok := normalizeRequestIP(remoteAddr)
	if !ok {
		return strings.TrimSpace(remoteAddr)
	}
	return ip.String()
}

func forwardedClientIPs(raw string) []netip.Addr {
	parts := strings.Split(raw, ",")
	ips := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		ip, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		ips = append(ips, ip)
	}
	return ips
}

func parseCIDRPrefixes(values []string) ([]netip.Prefix, error) {
	items := values
	if len(items) == 0 {
		items = []string{"0.0.0.0/0"}
	}
	prefixes := make([]netip.Prefix, 0, len(items))
	for _, value := range items {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return nil, err
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func containsPrefix(ip netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func idFromPath(value string) (int64, error) {
	base := path.Base(strings.TrimSuffix(value, "/"))
	return strconv.ParseInt(base, 10, 64)
}

func mustJSON(value any) string {
	out, _ := json.Marshal(value)
	return string(out)
}

func effectivePolicySubnet(policy *store.Policy, defaultSubnetID string) string {
	if policy == nil {
		return strings.TrimSpace(defaultSubnetID)
	}
	if subnetID := strings.TrimSpace(policy.SubnetOCID); subnetID != "" {
		return subnetID
	}
	return strings.TrimSpace(defaultSubnetID)
}
