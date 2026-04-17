package httpapi

import (
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"ohoci/internal/githubapp"
)

var githubManifestLaunchTemplate = template.Must(template.New("github-manifest-launch").Parse(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Redirecting to GitHub</title>
  </head>
  <body>
    <form id="github-manifest-launch" action="{{.PostURL}}" method="post">
      <input type="hidden" name="manifest" value="{{.ManifestJSON}}">
      <noscript>
        <p>Continue to GitHub App registration.</p>
        <button type="submit">Continue</button>
      </noscript>
    </form>
    <script>
      document.getElementById('github-manifest-launch').submit();
    </script>
  </body>
</html>
`))

func validatedSessionToken(r *http.Request, deps Dependencies, requirePasswordChanged bool) (string, bool) {
	token := readSessionCookie(r, deps.Config.SessionCookieName)
	sessionView, err := deps.Auth.SessionFromToken(r.Context(), token)
	if err != nil || !sessionView.Authenticated {
		return "", false
	}
	if requirePasswordChanged && sessionView.MustChangePassword {
		return "", false
	}
	return token, true
}

func githubManifestReturnURL(publicBaseURL, marker, installationID string) string {
	normalizedMarker := strings.TrimSpace(marker)
	if normalizedMarker == "" {
		normalizedMarker = "failed"
	}

	parsed, err := url.Parse(strings.TrimSpace(publicBaseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		parsed = &url.URL{Path: "/"}
	}
	values := parsed.Query()
	values.Set("github_manifest", normalizedMarker)
	if strings.TrimSpace(installationID) != "" {
		values.Set("github_installation_id", strings.TrimSpace(installationID))
	} else {
		values.Del("github_installation_id")
	}
	parsed.RawQuery = values.Encode()
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String()
}

func redirectGitHubManifestResult(w http.ResponseWriter, r *http.Request, deps Dependencies, marker, installationID string) {
	http.Redirect(w, r, githubManifestReturnURL(deps.Config.PublicBaseURL, marker, installationID), http.StatusFound)
}

func githubManifestLaunchHandler(deps Dependencies) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if deps.GitHub == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			_ = writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		token, ok := validatedSessionToken(r, deps, true)
		if !ok {
			redirectGitHubManifestResult(w, r, deps, "failed", "")
			return
		}

		launch, err := deps.GitHub.ManifestLaunch(token, r.URL.Query().Get("state"))
		if err != nil {
			redirectGitHubManifestResult(w, r, deps, "failed", "")
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := githubManifestLaunchTemplate.Execute(w, launch); err != nil {
			redirectGitHubManifestResult(w, r, deps, "failed", "")
			return
		}
	})
}

func githubManifestCallbackHandler(deps Dependencies) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if deps.GitHub == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			_ = writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		token, ok := validatedSessionToken(r, deps, true)
		if !ok {
			redirectGitHubManifestResult(w, r, deps, "failed", "")
			return
		}
		if strings.TrimSpace(r.URL.Query().Get("error")) != "" {
			redirectGitHubManifestResult(w, r, deps, "failed", "")
			return
		}

		state := r.URL.Query().Get("state")
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code != "" {
			pending, err := deps.GitHub.CompleteManifest(r.Context(), token, state, code)
			if err != nil {
				redirectGitHubManifestResult(w, r, deps, "failed", "")
				return
			}
			if strings.TrimSpace(pending.InstallURL) == "" {
				redirectGitHubManifestResult(w, r, deps, "failed", "")
				return
			}
			http.Redirect(w, r, pending.InstallURL, http.StatusFound)
			return
		}

		if strings.TrimSpace(r.URL.Query().Get("source")) != "install" {
			redirectGitHubManifestResult(w, r, deps, "failed", "")
			return
		}

		installationID := strings.TrimSpace(r.URL.Query().Get("installation_id"))
		if err := deps.GitHub.ValidateManifestInstallReturn(r.Context(), token, state, installationID); err != nil {
			redirectGitHubManifestResult(w, r, deps, "failed", "")
			return
		}

		redirectGitHubManifestResult(w, r, deps, "installed", installationID)
	})
}

func githubManifestPendingHandler(deps Dependencies) http.Handler {
	return requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.GitHub == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "GitHub config service is not configured"})
		}
		token := readSessionCookie(r, deps.Config.SessionCookieName)
		switch r.Method {
		case http.MethodGet:
			pending, err := deps.GitHub.PendingManifest(r.Context(), token)
			if err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusOK, map[string]any{"pending": pending})
		case http.MethodDelete:
			if err := deps.GitHub.ClearPendingManifest(r.Context(), token); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return writeJSON(w, http.StatusOK, map[string]any{"success": true})
		default:
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}))
}

func githubManifestStartHandler(deps Dependencies) http.Handler {
	return requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.GitHub == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "GitHub config service is not configured"})
		}
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}

		var payload githubapp.ManifestStartInput
		if r.ContentLength != 0 {
			if err := decodeJSON(r, &payload); err != nil {
				return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
		}

		result, err := deps.GitHub.StartManifestWithInput(readSessionCookie(r, deps.Config.SessionCookieName), payload)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, result)
	}))
}

func githubInstallationDiscoveryHandler(deps Dependencies) http.Handler {
	return requireUser(deps, true, jsonHandler(func(w http.ResponseWriter, r *http.Request) error {
		if deps.GitHub == nil {
			return writeStatus(w, http.StatusNotImplemented, map[string]string{"error": "GitHub config service is not configured"})
		}
		if r.Method != http.MethodPost {
			return writeStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}

		var payload githubapp.Input
		if err := decodeJSON(r, &payload); err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		result, err := deps.GitHub.DiscoverInstallations(r.Context(), payload)
		if err != nil {
			return writeStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return writeJSON(w, http.StatusOK, result)
	}))
}
