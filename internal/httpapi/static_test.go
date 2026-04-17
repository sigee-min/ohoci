package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSPAHandlerServesIndexAndStaticAssets(t *testing.T) {
	uiDir := t.TempDir()
	assetsDir := filepath.Join(uiDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	writeStaticFile(t, filepath.Join(uiDir, "index.html"), "<!doctype html><html><body>ohoci</body></html>")
	writeStaticFile(t, filepath.Join(assetsDir, "app.js"), "console.log('ok');")

	handler := spaHandler(uiDir)

	rootResponse := httptest.NewRecorder()
	handler.ServeHTTP(rootResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if rootResponse.Code != http.StatusOK {
		t.Fatalf("expected root route success, got %d", rootResponse.Code)
	}
	if !strings.Contains(rootResponse.Body.String(), "ohoci") {
		t.Fatalf("expected index payload, got %q", rootResponse.Body.String())
	}

	routeResponse := httptest.NewRecorder()
	handler.ServeHTTP(routeResponse, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if routeResponse.Code != http.StatusOK {
		t.Fatalf("expected SPA route success, got %d", routeResponse.Code)
	}
	if !strings.Contains(routeResponse.Body.String(), "ohoci") {
		t.Fatalf("expected SPA route to serve index, got %q", routeResponse.Body.String())
	}

	assetResponse := httptest.NewRecorder()
	handler.ServeHTTP(assetResponse, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if assetResponse.Code != http.StatusOK {
		t.Fatalf("expected asset success, got %d", assetResponse.Code)
	}
	if !strings.Contains(assetResponse.Body.String(), "console.log") {
		t.Fatalf("expected asset payload, got %q", assetResponse.Body.String())
	}
}

func TestSPAHandlerReturnsServiceUnavailableWhenAssetsAreMissing(t *testing.T) {
	handler := spaHandler(filepath.Join(t.TempDir(), "missing-ui"))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when UI assets are missing, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "OHOCI_UI_DIR") {
		t.Fatalf("expected operator guidance in response, got %q", response.Body.String())
	}

	apiResponse := httptest.NewRecorder()
	handler.ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "/api/v1/setup", nil))
	if apiResponse.Code != http.StatusNotFound {
		t.Fatalf("expected API routes to stay outside SPA handler, got %d", apiResponse.Code)
	}
}

func writeStaticFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
