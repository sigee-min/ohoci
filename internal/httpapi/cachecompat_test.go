package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ohoci/internal/cachecompat"
	"ohoci/internal/config"
	"ohoci/internal/ociruntime"
	"ohoci/internal/store"
)

type testCacheBlobStore struct {
	objects map[string][]byte
}

func (s *testCacheBlobStore) Put(_ context.Context, bucketName, objectName string, body io.ReadSeeker, _ int64, _ string) error {
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.objects[bucketName+"/"+objectName] = data
	return nil
}

func (s *testCacheBlobStore) Get(_ context.Context, bucketName, objectName string) (io.ReadCloser, int64, error) {
	data, ok := s.objects[bucketName+"/"+objectName]
	if !ok {
		return nil, 0, store.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (s *testCacheBlobStore) Delete(_ context.Context, bucketName, objectName string) error {
	delete(s.objects, bucketName+"/"+objectName)
	return nil
}

func TestCacheCompatEndpointsUseSharedSecretAndBypassAdminAllowlist(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := config.Config{
		PublicBaseURL:     "https://ohoci.example",
		DataEncryptionKey: "top-secret",
		TrustedProxyCIDRs: []string{"203.0.113.0/24"},
		AdminAllowCIDRs:   []string{"198.51.100.0/24"},
		WebhookAllowCIDRs: []string{"192.0.2.0/24"},
	}
	runtimeService := ociruntime.New(db, ociruntime.Defaults{
		CompartmentID:      "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetID:           "ocid1.subnet.oc1..example",
		ImageID:            "ocid1.image.oc1..example",
		CacheCompatEnabled: true,
		CacheBucketName:    "cache-bucket",
		CacheObjectPrefix:  "actions-cache",
		CacheRetentionDays: 7,
	})
	handler := New(Dependencies{
		Config:      cfg,
		Store:       db,
		OCIRuntime:  runtimeService,
		CacheCompat: cachecompat.New(db, runtimeService, &testCacheBlobStore{}, cfg.PublicBaseURL, cfg.DataEncryptionKey),
	})

	setHeaders := func(req *http.Request) {
		req.RemoteAddr = "203.0.113.10:12345"
		req.Header.Set("X-Forwarded-For", "10.0.0.9, 203.0.113.20")
		req.Header.Set(cacheCompatHeaderRepoOwner, "ash")
		req.Header.Set(cacheCompatHeaderRepoName, "repo")
		req.Header.Set(cacheCompatHeaderRunner, "runner-1")
		req.Header.Set(cacheCompatHeaderSecret, cachecompat.DeriveSharedSecret(cfg.DataEncryptionKey, "ash", "repo", "runner-1"))
	}

	reserveReq := httptest.NewRequest(http.MethodPost, "/api/internal/cache/_apis/artifactcache/caches", strings.NewReader(`{"key":"linux-npm-123","version":"v1"}`))
	reserveReq.Header.Set("Content-Type", "application/json")
	setHeaders(reserveReq)
	reserveRec := httptest.NewRecorder()
	handler.ServeHTTP(reserveRec, reserveReq)
	if reserveRec.Code != http.StatusCreated {
		t.Fatalf("expected reserve success, got %d: %s", reserveRec.Code, reserveRec.Body.String())
	}
	var reservePayload struct {
		CacheID int64 `json:"cacheId"`
	}
	if err := json.Unmarshal(reserveRec.Body.Bytes(), &reservePayload); err != nil {
		t.Fatalf("decode reserve response: %v", err)
	}
	if reservePayload.CacheID <= 0 {
		t.Fatalf("expected cache id, got %#v", reservePayload)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/internal/cache/_apis/artifactcache/caches/"+jsonInt64(reservePayload.CacheID), bytes.NewReader([]byte("cache-bytes")))
	patchReq.Header.Set("Content-Range", "bytes 0-10/*")
	setHeaders(patchReq)
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusNoContent {
		t.Fatalf("expected upload success, got %d: %s", patchRec.Code, patchRec.Body.String())
	}

	commitReq := httptest.NewRequest(http.MethodPost, "/api/internal/cache/_apis/artifactcache/caches/"+jsonInt64(reservePayload.CacheID), strings.NewReader(`{"size":11}`))
	commitReq.Header.Set("Content-Type", "application/json")
	setHeaders(commitReq)
	commitRec := httptest.NewRecorder()
	handler.ServeHTTP(commitRec, commitReq)
	if commitRec.Code != http.StatusOK {
		t.Fatalf("expected commit success, got %d: %s", commitRec.Code, commitRec.Body.String())
	}

	restoreReq := httptest.NewRequest(http.MethodGet, "/api/internal/cache/_apis/artifactcache/cache?keys=linux-npm-123,linux-npm-&version=v1", nil)
	setHeaders(restoreReq)
	restoreRec := httptest.NewRecorder()
	handler.ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected restore success, got %d: %s", restoreRec.Code, restoreRec.Body.String())
	}
	var restorePayload struct {
		ArchiveLocation string `json:"archiveLocation"`
		CacheKey        string `json:"cacheKey"`
	}
	if err := json.Unmarshal(restoreRec.Body.Bytes(), &restorePayload); err != nil {
		t.Fatalf("decode restore response: %v", err)
	}
	if restorePayload.CacheKey != "linux-npm-123" {
		t.Fatalf("expected exact cache hit, got %#v", restorePayload)
	}

	artifactReq := httptest.NewRequest(http.MethodGet, strings.TrimPrefix(restorePayload.ArchiveLocation, cfg.PublicBaseURL), nil)
	setHeaders(artifactReq)
	artifactRec := httptest.NewRecorder()
	handler.ServeHTTP(artifactRec, artifactReq)
	if artifactRec.Code != http.StatusOK {
		t.Fatalf("expected artifact download success, got %d: %s", artifactRec.Code, artifactRec.Body.String())
	}
	if artifactRec.Body.String() != "cache-bytes" {
		t.Fatalf("unexpected artifact payload %q", artifactRec.Body.String())
	}
}

func TestCacheCompatEndpointsRejectInvalidSecret(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := config.Config{
		PublicBaseURL:     "https://ohoci.example",
		DataEncryptionKey: "top-secret",
		TrustedProxyCIDRs: []string{"0.0.0.0/0"},
		AdminAllowCIDRs:   []string{"0.0.0.0/0"},
		WebhookAllowCIDRs: []string{"0.0.0.0/0"},
	}
	runtimeService := ociruntime.New(db, ociruntime.Defaults{
		CompartmentID:      "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetID:           "ocid1.subnet.oc1..example",
		ImageID:            "ocid1.image.oc1..example",
		CacheCompatEnabled: true,
		CacheBucketName:    "cache-bucket",
		CacheObjectPrefix:  "actions-cache",
		CacheRetentionDays: 7,
	})
	handler := New(Dependencies{
		Config:      cfg,
		Store:       db,
		OCIRuntime:  runtimeService,
		CacheCompat: cachecompat.New(db, runtimeService, &testCacheBlobStore{}, cfg.PublicBaseURL, cfg.DataEncryptionKey),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/internal/cache/_apis/artifactcache/cache?keys=linux&version=v1", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set(cacheCompatHeaderRepoOwner, "ash")
	req.Header.Set(cacheCompatHeaderRepoName, "repo")
	req.Header.Set(cacheCompatHeaderRunner, "runner-1")
	req.Header.Set(cacheCompatHeaderSecret, "wrong-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid secret rejection, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCacheCompatEndpointsRejectCrossRepoReservationPoisoning(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := config.Config{
		PublicBaseURL:     "https://ohoci.example",
		DataEncryptionKey: "top-secret",
		TrustedProxyCIDRs: []string{"0.0.0.0/0"},
		AdminAllowCIDRs:   []string{"0.0.0.0/0"},
		WebhookAllowCIDRs: []string{"0.0.0.0/0"},
	}
	runtimeService := ociruntime.New(db, ociruntime.Defaults{
		CompartmentID:      "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetID:           "ocid1.subnet.oc1..example",
		ImageID:            "ocid1.image.oc1..example",
		CacheCompatEnabled: true,
		CacheBucketName:    "cache-bucket",
		CacheObjectPrefix:  "actions-cache",
		CacheRetentionDays: 7,
	})
	handler := New(Dependencies{
		Config:      cfg,
		Store:       db,
		OCIRuntime:  runtimeService,
		CacheCompat: cachecompat.New(db, runtimeService, &testCacheBlobStore{}, cfg.PublicBaseURL, cfg.DataEncryptionKey),
	})

	setHeaders := func(req *http.Request, owner, repo, runner string) {
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set(cacheCompatHeaderRepoOwner, owner)
		req.Header.Set(cacheCompatHeaderRepoName, repo)
		req.Header.Set(cacheCompatHeaderRunner, runner)
		req.Header.Set(cacheCompatHeaderSecret, cachecompat.DeriveSharedSecret(cfg.DataEncryptionKey, owner, repo, runner))
	}

	reserveReq := httptest.NewRequest(http.MethodPost, "/api/internal/cache/_apis/artifactcache/caches", strings.NewReader(`{"key":"linux-npm-123","version":"v1"}`))
	reserveReq.Header.Set("Content-Type", "application/json")
	setHeaders(reserveReq, "owner-a", "repo-a", "runner-a")
	reserveRec := httptest.NewRecorder()
	handler.ServeHTTP(reserveRec, reserveReq)
	if reserveRec.Code != http.StatusCreated {
		t.Fatalf("expected reserve success, got %d: %s", reserveRec.Code, reserveRec.Body.String())
	}
	var reservePayload struct {
		CacheID int64 `json:"cacheId"`
	}
	if err := json.Unmarshal(reserveRec.Body.Bytes(), &reservePayload); err != nil {
		t.Fatalf("decode reserve response: %v", err)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/internal/cache/_apis/artifactcache/caches/"+jsonInt64(reservePayload.CacheID), bytes.NewReader([]byte("poisoned")))
	patchReq.Header.Set("Content-Range", "bytes 0-7/*")
	setHeaders(patchReq, "owner-b", "repo-b", "runner-b")
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusNotFound {
		t.Fatalf("expected cross-repo patch to be rejected, got %d: %s", patchRec.Code, patchRec.Body.String())
	}

	commitReq := httptest.NewRequest(http.MethodPost, "/api/internal/cache/_apis/artifactcache/caches/"+jsonInt64(reservePayload.CacheID), strings.NewReader(`{"size":8}`))
	commitReq.Header.Set("Content-Type", "application/json")
	setHeaders(commitReq, "owner-b", "repo-b", "runner-b")
	commitRec := httptest.NewRecorder()
	handler.ServeHTTP(commitRec, commitReq)
	if commitRec.Code != http.StatusNotFound {
		t.Fatalf("expected cross-repo commit to be rejected, got %d: %s", commitRec.Code, commitRec.Body.String())
	}
}
