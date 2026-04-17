package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ohoci/internal/store"
)

func TestNewThreadsCacheEnvDefaultsIntoRuntimeStatus(t *testing.T) {
	t.Setenv("OHOCI_SESSION_SECRET", "top-secret")
	t.Setenv("OHOCI_SQLITE_PATH", t.TempDir()+"/ohoci.db")
	t.Setenv("OHOCI_CACHE_COMPAT_ENABLED", "true")
	t.Setenv("OHOCI_CACHE_BUCKET_NAME", "env-cache-bucket")
	t.Setenv("OHOCI_CACHE_OBJECT_PREFIX", "env-prefix")
	t.Setenv("OHOCI_CACHE_RETENTION_DAYS", "9")

	instance, err := New()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = instance.Close() })

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte(`{"username":"admin","password":"admin"}`)))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	instance.Handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie after login")
	}

	changeReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", bytes.NewReader([]byte(`{"currentPassword":"admin","newPassword":"super-secret-password"}`)))
	changeReq.Header.Set("Content-Type", "application/json")
	changeReq.AddCookie(cookies[0])
	changeRec := httptest.NewRecorder()
	instance.Handler.ServeHTTP(changeRec, changeReq)
	if changeRec.Code != http.StatusOK {
		t.Fatalf("change password failed: %d %s", changeRec.Code, changeRec.Body.String())
	}

	runtimeReq := httptest.NewRequest(http.MethodGet, "/api/v1/oci/runtime", nil)
	runtimeReq.AddCookie(cookies[0])
	runtimeRec := httptest.NewRecorder()
	instance.Handler.ServeHTTP(runtimeRec, runtimeReq)
	if runtimeRec.Code != http.StatusOK {
		t.Fatalf("runtime status failed: %d %s", runtimeRec.Code, runtimeRec.Body.String())
	}

	var status struct {
		EffectiveSettings store.OCIRuntimeSettings `json:"effectiveSettings"`
	}
	if err := json.Unmarshal(runtimeRec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode runtime status: %v", err)
	}
	if !status.EffectiveSettings.CacheCompatEnabled {
		t.Fatalf("expected env cache compat to flow into runtime defaults, got %#v", status.EffectiveSettings)
	}
	if status.EffectiveSettings.CacheBucketName != "env-cache-bucket" || status.EffectiveSettings.CacheObjectPrefix != "env-prefix" || status.EffectiveSettings.CacheRetentionDays != 9 {
		t.Fatalf("unexpected cache defaults in runtime status: %#v", status.EffectiveSettings)
	}
}
