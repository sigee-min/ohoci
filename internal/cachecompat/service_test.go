package cachecompat

import (
	"bytes"
	"context"
	"io"
	"testing"

	"ohoci/internal/ociruntime"
	"ohoci/internal/store"
)

type memoryBlobStore struct {
	objects map[string][]byte
}

func (m *memoryBlobStore) Put(_ context.Context, bucketName, objectName string, body io.ReadSeeker, _ int64, _ string) error {
	if m.objects == nil {
		m.objects = map[string][]byte{}
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	m.objects[bucketName+"/"+objectName] = data
	return nil
}

func (m *memoryBlobStore) Get(_ context.Context, bucketName, objectName string) (io.ReadCloser, int64, error) {
	data, ok := m.objects[bucketName+"/"+objectName]
	if !ok {
		return nil, 0, store.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

func (m *memoryBlobStore) Delete(_ context.Context, bucketName, objectName string) error {
	delete(m.objects, bucketName+"/"+objectName)
	return nil
}

func TestServiceReserveUploadCommitRestoreAndOpenBlob(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

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
	service := New(db, runtimeService, &memoryBlobStore{}, "https://ohoci.example", "top-secret")

	reservation, err := service.Reserve(ctx, "ash", "repo", "runner-1", "linux-npm-123", "v1")
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	payload := []byte("cache-bytes")
	if err := service.UploadChunk(ctx, "ash", "repo", "runner-1", reservation.ID, 0, bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload chunk: %v", err)
	}
	entry, stored, err := service.Commit(ctx, "ash", "repo", "runner-1", reservation.ID, int64(len(payload)))
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !stored || entry.ID <= 0 || entry.ObjectName == "" {
		t.Fatalf("expected stored cache entry, got stored=%v entry=%#v", stored, entry)
	}

	found, err := service.Restore(ctx, "ash", "repo", []string{"linux-npm-123", "linux-npm-"}, "v1")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if found == nil || found.ID != entry.ID {
		t.Fatalf("expected restored cache entry %d, got %#v", entry.ID, found)
	}

	reader, size, err := service.OpenBlob(ctx, "ash", "repo", entry.ID)
	if err != nil {
		t.Fatalf("open blob: %v", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read blob: %v", err)
	}
	if got := string(data); got != string(payload) || size != int64(len(payload)) {
		t.Fatalf("unexpected blob payload %q size=%d", got, size)
	}
	if _, _, err := service.OpenBlob(ctx, "other", "repo", entry.ID); err == nil {
		t.Fatalf("expected repo-scoped blob access to reject mismatched repo")
	}
}

func TestServiceDisabledModeDegradesWithoutFailing(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, "", t.TempDir()+"/ohoci.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runtimeService := ociruntime.New(db, ociruntime.Defaults{
		CompartmentID:      "ocid1.compartment.oc1..example",
		AvailabilityDomain: "AD-1",
		SubnetID:           "ocid1.subnet.oc1..example",
		ImageID:            "ocid1.image.oc1..example",
	})
	service := New(db, runtimeService, &memoryBlobStore{}, "https://ohoci.example", "top-secret")

	reservation, err := service.Reserve(ctx, "ash", "repo", "runner-1", "linux-go-123", "v1")
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if !reservation.Disabled {
		t.Fatalf("expected disabled reservation when cache compat is off")
	}
	if err := service.UploadChunk(ctx, "ash", "repo", "runner-1", reservation.ID, 0, bytes.NewReader([]byte("noop"))); err != nil {
		t.Fatalf("disabled upload should no-op, got %v", err)
	}
	if _, stored, err := service.Commit(ctx, "ash", "repo", "runner-1", reservation.ID, 4); err != nil || stored {
		t.Fatalf("disabled commit should no-op, got stored=%v err=%v", stored, err)
	}
	entry, err := service.Restore(ctx, "ash", "repo", []string{"linux-go-123"}, "v1")
	if err != nil {
		t.Fatalf("disabled restore should not fail, got %v", err)
	}
	if entry != nil {
		t.Fatalf("expected disabled restore to degrade to cache miss, got %#v", entry)
	}
	if _, _, err := service.OpenBlob(ctx, "ash", "repo", 1); err != ErrUnavailable {
		t.Fatalf("expected unavailable blob error, got %v", err)
	}
}
