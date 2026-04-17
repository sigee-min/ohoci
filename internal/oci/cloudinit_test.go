package oci

import (
	"strings"
	"testing"
)

func TestBuildCloudInitWithoutCacheCompatBootstrap(t *testing.T) {
	script := BuildCloudInit(CloudInitInput{
		RepoOwner:          "ash",
		RepoName:           "repo",
		RunnerName:         "runner-1",
		RegistrationToken:  "token",
		Labels:             []string{"self-hosted", "linux"},
		RunnerDownloadBase: "https://github.com/actions/runner/releases/download",
		RunnerVersion:      "2.327.1",
		RunnerArch:         "x64",
		RunnerUser:         "runner",
		RunnerWorkDir:      "/home/runner/actions-runner",
	})
	disallowed := []string{
		".ohoci-cache-compat.env",
		"/opt/ohoci/cache-proxy.py",
		"ACTIONS_CACHE_URL",
		"python3 /opt/ohoci/cache-proxy.py",
	}
	for _, item := range disallowed {
		if strings.Contains(script, item) {
			t.Fatalf("did not expect cache bootstrap in default cloud-init for %q:\n%s", item, script)
		}
	}
	if !strings.Contains(script, "cd /home/runner/actions-runner && ./run.sh") {
		t.Fatalf("expected default runner startup command, got:\n%s", script)
	}
}

func TestBuildCloudInitWithCacheCompatBootstrap(t *testing.T) {
	script := BuildCloudInit(CloudInitInput{
		RepoOwner:          "ash",
		RepoName:           "repo",
		RunnerName:         "runner-1",
		RegistrationToken:  "token",
		Labels:             []string{"self-hosted", "linux"},
		RunnerDownloadBase: "https://github.com/actions/runner/releases/download",
		RunnerVersion:      "2.327.1",
		RunnerArch:         "x64",
		RunnerUser:         "runner",
		RunnerWorkDir:      "/home/runner/actions-runner",
		CacheCompat: &CloudInitCacheCompatInput{
			UpstreamBaseURL: "https://ohoci.example",
			SharedSecret:    "shared-secret",
		},
	})
	required := []string{
		"export ACTIONS_CACHE_URL='http://127.0.0.1:31888'",
		"export ACTIONS_RESULTS_URL='http://127.0.0.1:31888'",
		"export ACTIONS_RUNTIME_TOKEN='ohoci-cache'",
		"export ACTIONS_CACHE_SERVICE_V2='false'",
		"export OHOCI_CACHE_UPSTREAM='https://ohoci.example/api/internal/cache'",
		"export OHOCI_CACHE_SECRET='shared-secret'",
		"cat > /opt/ohoci/cache-proxy.py <<'PYEOF'",
		"connection.putheader(\"X-OhoCI-Cache-Secret\", SECRET)",
		"nohup python3 /opt/ohoci/cache-proxy.py >/tmp/ohoci-cache-proxy.log 2>&1 &",
		"set -a && . ./.ohoci-cache-compat.env && set +a && ./run.sh",
	}
	for _, item := range required {
		if !strings.Contains(script, item) {
			t.Fatalf("expected cloud-init to contain %q:\n%s", item, script)
		}
	}
}
