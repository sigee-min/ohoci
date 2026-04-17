package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func spaHandler(uiDir string) http.Handler {
	uiDir = strings.TrimSpace(uiDir)
	indexPath := filepath.Join(uiDir, "index.html")
	if uiDir == "" {
		return missingUIHandler(uiDir)
	}
	if _, err := os.Stat(indexPath); err != nil {
		return missingUIHandler(uiDir)
	}

	fileServer := http.FileServer(http.Dir(uiDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" || !strings.Contains(pathBase(r.URL.Path), ".") {
			http.ServeFile(w, r, indexPath)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func missingUIHandler(uiDir string) http.Handler {
	uiDir = strings.TrimSpace(uiDir)
	message := "UI assets are not available."
	if uiDir != "" {
		message = "UI assets are not available in " + uiDir + "."
	}
	message += " Run `cd web && npm install && npm run build` or set OHOCI_UI_DIR to a built UI directory."
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, message, http.StatusServiceUnavailable)
	})
}

func pathBase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	parts := strings.Split(value, "/")
	return parts[len(parts)-1]
}
