package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"ohoci/internal/cachecompat"
	"ohoci/internal/store"
)

const (
	cacheCompatHeaderRepoOwner = "X-OhoCI-Repo-Owner"
	cacheCompatHeaderRepoName  = "X-OhoCI-Repo-Name"
	cacheCompatHeaderRunner    = "X-OhoCI-Runner-Name"
	cacheCompatHeaderSecret    = "X-OhoCI-Cache-Secret"
)

func cacheCompatHandler(deps Dependencies, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if deps.CacheCompat == nil {
			_ = writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "cache compatibility is not configured"})
			return
		}
		repoOwner := strings.TrimSpace(r.Header.Get(cacheCompatHeaderRepoOwner))
		repoName := strings.TrimSpace(r.Header.Get(cacheCompatHeaderRepoName))
		runnerName := strings.TrimSpace(r.Header.Get(cacheCompatHeaderRunner))
		secret := strings.TrimSpace(r.Header.Get(cacheCompatHeaderSecret))
		if repoOwner == "" || repoName == "" || runnerName == "" || secret == "" {
			_ = writeStatus(w, http.StatusUnauthorized, map[string]string{"error": "cache compatibility authentication headers are required"})
			return
		}
		if !deps.CacheCompat.ValidateSharedSecret(repoOwner, repoName, runnerName, secret) {
			_ = writeStatus(w, http.StatusUnauthorized, map[string]string{"error": "invalid cache compatibility secret"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func cacheCompatRestoreHandler(deps Dependencies) http.Handler {
	return cacheCompatHandler(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		keys := ensureCSV(r.URL.Query().Get("keys"))
		version := strings.TrimSpace(r.URL.Query().Get("version"))
		if len(keys) == 0 || version == "" {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "keys and version are required"})
		}
		entry, err := deps.CacheCompat.Restore(
			r.Context(),
			r.Header.Get(cacheCompatHeaderRepoOwner),
			r.Header.Get(cacheCompatHeaderRepoName),
			keys,
			version,
		)
		if err != nil {
			if errors.Is(err, cachecompat.ErrUnavailable) {
				w.WriteHeader(http.StatusNoContent)
				return nil
			}
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		if entry == nil {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		return writeJSON(w, http.StatusOK, map[string]any{
			"archiveLocation": deps.CacheCompat.ArchiveURL(entry.ID),
			"cacheKey":        entry.CacheKey,
			"scope":           entry.RepoOwner + "/" + entry.RepoName,
		})
	}))
}

func cacheCompatReserveHandler(deps Dependencies) http.Handler {
	return cacheCompatHandler(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		var payload struct {
			Key     string `json:"key"`
			Version string `json:"version"`
		}
		if err := decodeJSON(r, &payload); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		reservation, err := deps.CacheCompat.Reserve(
			r.Context(),
			r.Header.Get(cacheCompatHeaderRepoOwner),
			r.Header.Get(cacheCompatHeaderRepoName),
			r.Header.Get(cacheCompatHeaderRunner),
			payload.Key,
			payload.Version,
		)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusCreated, map[string]any{"cacheId": reservation.ID})
	}))
}

func cacheCompatUploadHandler(deps Dependencies) http.Handler {
	return cacheCompatHandler(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		reservationID, err := pathIDBetween(r.URL.Path, "/api/internal/cache/_apis/artifactcache/caches/", "")
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid cache reservation id"})
		}
		switch r.Method {
		case http.MethodPatch:
			startOffset, err := contentRangeStart(r.Header.Get("Content-Range"))
			if err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			if err := deps.CacheCompat.UploadChunk(
				r.Context(),
				r.Header.Get(cacheCompatHeaderRepoOwner),
				r.Header.Get(cacheCompatHeaderRepoName),
				r.Header.Get(cacheCompatHeaderRunner),
				reservationID,
				startOffset,
				r.Body,
			); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return writeStatus(w, http.StatusNotFound, map[string]string{"error": "cache reservation not found"})
				}
				return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			w.WriteHeader(http.StatusNoContent)
			return nil
		case http.MethodPost:
			var payload struct {
				Size int64 `json:"size"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			entry, stored, err := deps.CacheCompat.Commit(
				r.Context(),
				r.Header.Get(cacheCompatHeaderRepoOwner),
				r.Header.Get(cacheCompatHeaderRepoName),
				r.Header.Get(cacheCompatHeaderRunner),
				reservationID,
				payload.Size,
			)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return writeStatus(w, http.StatusNotFound, map[string]string{"error": "cache reservation not found"})
				}
				return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			}
			if !stored {
				w.WriteHeader(http.StatusNoContent)
				return nil
			}
			return writeJSON(w, http.StatusOK, map[string]any{"cacheId": entry.ID})
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}))
}

func cacheCompatArtifactHandler(deps Dependencies) http.Handler {
	return cacheCompatHandler(deps, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != http.MethodGet {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		entryID, err := pathIDBetween(r.URL.Path, "/api/internal/cache/_apis/artifactcache/artifacts/", "")
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid cache artifact id"})
		}
		reader, size, err := deps.CacheCompat.OpenBlob(
			r.Context(),
			r.Header.Get(cacheCompatHeaderRepoOwner),
			r.Header.Get(cacheCompatHeaderRepoName),
			entryID,
		)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return writeStatus(w, http.StatusNotFound, map[string]string{"error": "cache artifact not found"})
			}
			if errors.Is(err, cachecompat.ErrUnavailable) {
				return writeStatus(w, http.StatusNotFound, map[string]string{"error": "cache compatibility is unavailable"})
			}
			return writeStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		}
		defer reader.Close()
		if size > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, copyErr := io.Copy(w, reader)
		return copyErr
	}))
}

func ensureCSV(value string) []string {
	items := strings.Split(value, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func contentRangeStart(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if !strings.HasPrefix(strings.ToLower(value), "bytes ") {
		return 0, errors.New("content-range must use bytes")
	}
	rangeValue := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(value, "bytes"), " "))
	parts := strings.SplitN(rangeValue, "-", 2)
	if len(parts) != 2 {
		return 0, errors.New("content-range start is invalid")
	}
	return strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
}
