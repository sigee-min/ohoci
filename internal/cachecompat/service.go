package cachecompat

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"ohoci/internal/ociruntime"
	"ohoci/internal/store"
)

var ErrUnavailable = errors.New("cache compatibility is unavailable")

type BlobStore interface {
	Put(ctx context.Context, bucketName, objectName string, body io.ReadSeeker, sizeBytes int64, contentType string) error
	Get(ctx context.Context, bucketName, objectName string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, bucketName, objectName string) error
}

type Reservation struct {
	ID         int64
	RepoOwner  string
	RepoName   string
	RunnerName string
	CacheKey   string
	Version    string
	TempPath   string
	Disabled   bool
	CreatedAt  time.Time
}

type Service struct {
	store        *store.Store
	runtime      *ociruntime.Service
	blobStore    BlobStore
	publicBase   string
	secretKey    string
	now          func() time.Time
	mu           sync.Mutex
	nextID       int64
	reservations map[int64]Reservation
}

func New(storeDB *store.Store, runtime *ociruntime.Service, blobStore BlobStore, publicBaseURL, secretKey string) *Service {
	return &Service{
		store:        storeDB,
		runtime:      runtime,
		blobStore:    blobStore,
		publicBase:   strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
		secretKey:    strings.TrimSpace(secretKey),
		now:          time.Now,
		nextID:       1,
		reservations: map[int64]Reservation{},
	}
}

func DeriveSharedSecret(secretKey, repoOwner, repoName, runnerName string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(secretKey),
		strings.ToLower(strings.TrimSpace(repoOwner)),
		strings.ToLower(strings.TrimSpace(repoName)),
		strings.TrimSpace(runnerName),
	}, "|")))
	return hex.EncodeToString(sum[:])
}

func (s *Service) ValidateSharedSecret(repoOwner, repoName, runnerName, provided string) bool {
	expected := DeriveSharedSecret(s.secretKey, repoOwner, repoName, runnerName)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(strings.TrimSpace(provided))) == 1
}

func (s *Service) Restore(ctx context.Context, repoOwner, repoName string, keys []string, version string) (*store.CacheEntry, error) {
	settings, enabled, err := s.enabledSettings(ctx)
	if err != nil || !enabled || s.blobStore == nil || strings.TrimSpace(settings.CacheBucketName) == "" {
		return nil, err
	}
	repoOwner = strings.TrimSpace(repoOwner)
	repoName = strings.TrimSpace(repoName)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		entry, err := s.store.FindCacheEntry(ctx, repoOwner, repoName, key, version)
		if err == nil {
			_ = s.store.TouchCacheEntry(ctx, entry.ID)
			return &entry, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	prefixMatches, err := s.store.FindCacheEntriesByPrefix(ctx, repoOwner, repoName, keys, version)
	if err != nil || len(prefixMatches) == 0 {
		return nil, err
	}
	_ = s.store.TouchCacheEntry(ctx, prefixMatches[0].ID)
	return &prefixMatches[0], nil
}

func (s *Service) OpenBlob(ctx context.Context, repoOwner, repoName string, entryID int64) (io.ReadCloser, int64, error) {
	settings, enabled, err := s.enabledSettings(ctx)
	if err != nil {
		return nil, 0, err
	}
	if !enabled || s.blobStore == nil || strings.TrimSpace(settings.CacheBucketName) == "" {
		return nil, 0, ErrUnavailable
	}
	entry, err := s.store.FindCacheEntryByID(ctx, entryID)
	if err != nil {
		return nil, 0, err
	}
	if !strings.EqualFold(strings.TrimSpace(entry.RepoOwner), strings.TrimSpace(repoOwner)) || !strings.EqualFold(strings.TrimSpace(entry.RepoName), strings.TrimSpace(repoName)) {
		return nil, 0, store.ErrNotFound
	}
	return s.blobStore.Get(ctx, settings.CacheBucketName, entry.ObjectName)
}

func (s *Service) Reserve(ctx context.Context, repoOwner, repoName, runnerName, cacheKey, version string) (Reservation, error) {
	now := s.now().UTC()
	reservation := Reservation{
		RepoOwner:  strings.TrimSpace(repoOwner),
		RepoName:   strings.TrimSpace(repoName),
		RunnerName: strings.TrimSpace(runnerName),
		CacheKey:   strings.TrimSpace(cacheKey),
		Version:    strings.TrimSpace(version),
		CreatedAt:  now,
	}
	if reservation.RepoOwner == "" || reservation.RepoName == "" || reservation.CacheKey == "" || reservation.Version == "" {
		return Reservation{}, fmt.Errorf("repo scope, cache key, and version are required")
	}
	settings, enabled, err := s.enabledSettings(ctx)
	if err != nil || !enabled || s.blobStore == nil || strings.TrimSpace(settings.CacheBucketName) == "" {
		reservation.Disabled = true
	} else {
		file, fileErr := os.CreateTemp("", "ohoci-cache-*")
		if fileErr != nil {
			return Reservation{}, fileErr
		}
		reservation.TempPath = file.Name()
		_ = file.Close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	reservation.ID = s.nextID
	s.nextID++
	s.reservations[reservation.ID] = reservation
	return reservation, nil
}

func (s *Service) UploadChunk(_ context.Context, repoOwner, repoName, runnerName string, reservationID int64, offset int64, body io.Reader) error {
	reservation, err := s.reservationForOwner(reservationID, repoOwner, repoName, runnerName)
	if err != nil || reservation.Disabled {
		return err
	}
	file, err := os.OpenFile(reservation.TempPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(file, body)
	return err
}

func (s *Service) Commit(ctx context.Context, repoOwner, repoName, runnerName string, reservationID int64, expectedSize int64) (store.CacheEntry, bool, error) {
	reservation, err := s.takeReservationForOwner(reservationID, repoOwner, repoName, runnerName)
	if err != nil {
		return store.CacheEntry{}, false, err
	}
	if reservation.Disabled {
		if reservation.TempPath != "" {
			_ = os.Remove(reservation.TempPath)
		}
		return store.CacheEntry{}, false, nil
	}

	settings, enabled, err := s.enabledSettings(ctx)
	if err != nil {
		return store.CacheEntry{}, false, err
	}
	if !enabled || s.blobStore == nil || strings.TrimSpace(settings.CacheBucketName) == "" {
		if reservation.TempPath != "" {
			_ = os.Remove(reservation.TempPath)
		}
		return store.CacheEntry{}, false, nil
	}

	file, err := os.Open(reservation.TempPath)
	if err != nil {
		return store.CacheEntry{}, false, err
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(reservation.TempPath)
	}()

	info, err := file.Stat()
	if err != nil {
		return store.CacheEntry{}, false, err
	}
	if expectedSize > 0 && info.Size() != expectedSize {
		return store.CacheEntry{}, false, fmt.Errorf("cache upload size mismatch: expected %d, got %d", expectedSize, info.Size())
	}
	objectName := s.objectName(settings, reservation)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return store.CacheEntry{}, false, err
	}
	if err := s.blobStore.Put(ctx, settings.CacheBucketName, objectName, file, info.Size(), "application/octet-stream"); err != nil {
		return store.CacheEntry{}, false, err
	}
	expiresAt := s.now().UTC().Add(time.Duration(maxInt(settings.CacheRetentionDays, 1)) * 24 * time.Hour)
	entry, err := s.store.UpsertCacheEntry(ctx, store.CacheEntry{
		RepoOwner:    reservation.RepoOwner,
		RepoName:     reservation.RepoName,
		CacheKey:     reservation.CacheKey,
		CacheVersion: reservation.Version,
		ObjectName:   objectName,
		SizeBytes:    info.Size(),
		ExpiresAt:    &expiresAt,
	})
	if err != nil {
		_ = s.blobStore.Delete(ctx, settings.CacheBucketName, objectName)
		return store.CacheEntry{}, false, err
	}
	return entry, true, nil
}

func (s *Service) ArchiveURL(entryID int64) string {
	return s.publicBase + "/api/internal/cache/_apis/artifactcache/artifacts/" + strconv.FormatInt(entryID, 10)
}

func (s *Service) enabledSettings(ctx context.Context) (store.OCIRuntimeSettings, bool, error) {
	if s.runtime == nil {
		return store.OCIRuntimeSettings{}, false, nil
	}
	status, err := s.runtime.CurrentStatus(ctx)
	if err != nil {
		return store.OCIRuntimeSettings{}, false, err
	}
	if !status.Ready {
		return status.EffectiveSettings, false, nil
	}
	settings := status.EffectiveSettings
	return settings, settings.CacheCompatEnabled, nil
}

func (s *Service) reservation(id int64) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reservation, ok := s.reservations[id]
	if !ok {
		return Reservation{}, store.ErrNotFound
	}
	return reservation, nil
}

func (s *Service) reservationForOwner(id int64, repoOwner, repoName, runnerName string) (Reservation, error) {
	reservation, err := s.reservation(id)
	if err != nil {
		return Reservation{}, err
	}
	if !reservationOwnedBy(reservation, repoOwner, repoName, runnerName) {
		return Reservation{}, store.ErrNotFound
	}
	return reservation, nil
}

func (s *Service) takeReservation(id int64) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reservation, ok := s.reservations[id]
	if !ok {
		return Reservation{}, store.ErrNotFound
	}
	delete(s.reservations, id)
	return reservation, nil
}

func (s *Service) takeReservationForOwner(id int64, repoOwner, repoName, runnerName string) (Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reservation, ok := s.reservations[id]
	if !ok {
		return Reservation{}, store.ErrNotFound
	}
	if !reservationOwnedBy(reservation, repoOwner, repoName, runnerName) {
		return Reservation{}, store.ErrNotFound
	}
	delete(s.reservations, id)
	return reservation, nil
}

func (s *Service) objectName(settings store.OCIRuntimeSettings, reservation Reservation) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		reservation.RepoOwner,
		reservation.RepoName,
		reservation.CacheKey,
		reservation.Version,
		s.now().UTC().Format(time.RFC3339Nano),
	}, "|")))
	hash := hex.EncodeToString(sum[:16])
	prefix := strings.Trim(strings.TrimSpace(settings.CacheObjectPrefix), "/")
	objectName := path.Join(
		prefix,
		strings.ToLower(reservation.RepoOwner),
		strings.ToLower(reservation.RepoName),
		hash+".tgz",
	)
	return strings.TrimLeft(objectName, "/")
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func reservationOwnedBy(reservation Reservation, repoOwner, repoName, runnerName string) bool {
	return strings.EqualFold(strings.TrimSpace(reservation.RepoOwner), strings.TrimSpace(repoOwner)) &&
		strings.EqualFold(strings.TrimSpace(reservation.RepoName), strings.TrimSpace(repoName)) &&
		strings.EqualFold(strings.TrimSpace(reservation.RunnerName), strings.TrimSpace(runnerName))
}
