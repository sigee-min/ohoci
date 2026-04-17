package oci

import (
	"fmt"
	"strings"
)

type CloudInitInput struct {
	RepoOwner          string
	RepoName           string
	RunnerName         string
	RegistrationToken  string
	Labels             []string
	RunnerDownloadBase string
	RunnerVersion      string
	RunnerArch         string
	RunnerUser         string
	RunnerWorkDir      string
	CacheCompat        *CloudInitCacheCompatInput
}

type CloudInitCacheCompatInput struct {
	UpstreamBaseURL string
	SharedSecret    string
}

func BuildCloudInit(input CloudInitInput) string {
	labels := strings.Join(input.Labels, ",")
	downloadURL := fmt.Sprintf("%s/v%s/actions-runner-linux-%s-%s.tar.gz", strings.TrimRight(input.RunnerDownloadBase, "/"), input.RunnerVersion, input.RunnerArch, input.RunnerVersion)
	cacheCompatSetup := buildCloudInitCacheCompatSetup(input)
	runnerStart := buildCloudInitRunnerStart(input)
	return fmt.Sprintf(`#cloud-config
package_update: true
package_upgrade: false
runcmd:
  - useradd -m -s /bin/bash %[1]s || true
  - mkdir -p %[2]s
  - chown -R %[1]s:%[1]s %[2]s
  - su - %[1]s -c 'cd %[2]s && curl -fsSL -o actions-runner.tar.gz %[3]s && tar xzf actions-runner.tar.gz'
%[9]s  - su - %[1]s -c 'cd %[2]s && ./config.sh --url https://github.com/%[4]s/%[5]s --token %[6]s --name %[7]s --labels %[8]q --ephemeral --unattended --replace'
%[10]s  - shutdown -h now
`, input.RunnerUser, input.RunnerWorkDir, downloadURL, input.RepoOwner, input.RepoName, input.RegistrationToken, input.RunnerName, labels, cacheCompatSetup, runnerStart)
}

func buildCloudInitCacheCompatSetup(input CloudInitInput) string {
	if input.CacheCompat == nil {
		return ""
	}
	upstreamBase := strings.TrimRight(strings.TrimSpace(input.CacheCompat.UpstreamBaseURL), "/")
	sharedSecret := strings.TrimSpace(input.CacheCompat.SharedSecret)
	if upstreamBase == "" || sharedSecret == "" {
		return ""
	}
	localBaseURL := cacheCompatLocalBaseURL()
	return fmt.Sprintf(`  - |
      cat > %[1]s/.ohoci-cache-compat.env <<'EOF'
      export ACTIONS_CACHE_URL=%[2]s
      export ACTIONS_RESULTS_URL=%[2]s
      export ACTIONS_RUNTIME_TOKEN='ohoci-cache'
      export ACTIONS_CACHE_SERVICE_V2='false'
      export OHOCI_CACHE_UPSTREAM=%[3]s
      export OHOCI_CACHE_REPO_OWNER=%[4]s
      export OHOCI_CACHE_REPO_NAME=%[5]s
      export OHOCI_CACHE_RUNNER_NAME=%[6]s
      export OHOCI_CACHE_SECRET=%[7]s
      EOF
  - chown %[8]s:%[8]s %[1]s/.ohoci-cache-compat.env
  - mkdir -p /opt/ohoci
  - |
      cat > /opt/ohoci/cache-proxy.py <<'PYEOF'
      #!/usr/bin/env python3
      import http.client
      import http.server
      import os
      import socketserver
      import urllib.parse

      UPSTREAM = os.environ["OHOCI_CACHE_UPSTREAM"]
      REPO_OWNER = os.environ["OHOCI_CACHE_REPO_OWNER"]
      REPO_NAME = os.environ["OHOCI_CACHE_REPO_NAME"]
      RUNNER_NAME = os.environ["OHOCI_CACHE_RUNNER_NAME"]
      SECRET = os.environ["OHOCI_CACHE_SECRET"]
      LISTEN_HOST = os.environ.get("OHOCI_CACHE_LISTEN_HOST", "127.0.0.1")
      LISTEN_PORT = int(os.environ.get("OHOCI_CACHE_LISTEN_PORT", "31888"))
      PARSED = urllib.parse.urlsplit(UPSTREAM)

      class Handler(http.server.BaseHTTPRequestHandler):
          protocol_version = "HTTP/1.1"

          def do_GET(self):
              self._forward()

          def do_POST(self):
              self._forward()

          def do_PATCH(self):
              self._forward()

          def do_HEAD(self):
              self._forward()

          def log_message(self, _format, *_args):
              return

          def _forward(self):
              target_path = (PARSED.path.rstrip("/") or "") + self.path
              connection_class = http.client.HTTPSConnection if PARSED.scheme == "https" else http.client.HTTPConnection
              connection = connection_class(PARSED.netloc, timeout=900)
              try:
                  connection.putrequest(self.command, target_path, skip_host=True, skip_accept_encoding=True)
                  content_length = self.headers.get("Content-Length")
                  for key, value in self.headers.items():
                      lowered = key.lower()
                      if lowered in {"host", "connection", "accept-encoding", "x-ohoci-repo-owner", "x-ohoci-repo-name", "x-ohoci-runner-name", "x-ohoci-cache-secret"}:
                          continue
                      connection.putheader(key, value)
                  connection.putheader("Host", PARSED.netloc)
                  connection.putheader("X-OhoCI-Repo-Owner", REPO_OWNER)
                  connection.putheader("X-OhoCI-Repo-Name", REPO_NAME)
                  connection.putheader("X-OhoCI-Runner-Name", RUNNER_NAME)
                  connection.putheader("X-OhoCI-Cache-Secret", SECRET)
                  if content_length:
                      connection.putheader("Content-Length", content_length)
                  connection.endheaders()
                  remaining = int(content_length or "0")
                  while remaining > 0:
                      chunk = self.rfile.read(min(65536, remaining))
                      if not chunk:
                          break
                      connection.send(chunk)
                      remaining -= len(chunk)
                  response = connection.getresponse()
                  self.send_response(response.status)
                  for key, value in response.getheaders():
                      if key.lower() in {"transfer-encoding", "connection"}:
                          continue
                      self.send_header(key, value)
                  self.end_headers()
                  while True:
                      chunk = response.read(65536)
                      if not chunk:
                          break
                      self.wfile.write(chunk)
              except Exception as exc:
                  self.send_error(502, str(exc))
              finally:
                  connection.close()

      class ThreadingHTTPServer(socketserver.ThreadingMixIn, http.server.HTTPServer):
          daemon_threads = True

      if __name__ == "__main__":
          server = ThreadingHTTPServer((LISTEN_HOST, LISTEN_PORT), Handler)
          server.serve_forever()
      PYEOF
  - chmod 0755 /opt/ohoci/cache-proxy.py
`, input.RunnerWorkDir, shellEnvValue(localBaseURL), shellEnvValue(upstreamBase+"/api/internal/cache"), shellEnvValue(input.RepoOwner), shellEnvValue(input.RepoName), shellEnvValue(input.RunnerName), shellEnvValue(sharedSecret), input.RunnerUser)
}

func buildCloudInitRunnerStart(input CloudInitInput) string {
	if input.CacheCompat == nil || strings.TrimSpace(input.CacheCompat.UpstreamBaseURL) == "" || strings.TrimSpace(input.CacheCompat.SharedSecret) == "" {
		return fmt.Sprintf("  - su - %s -c 'cd %s && ./run.sh'\n", input.RunnerUser, input.RunnerWorkDir)
	}
	return fmt.Sprintf(`  - command -v python3 >/dev/null 2>&1 || (apt-get update && apt-get install -y python3)
  - su - %[1]s -c 'cd %[2]s && set -a && . ./.ohoci-cache-compat.env && set +a && nohup python3 /opt/ohoci/cache-proxy.py >/tmp/ohoci-cache-proxy.log 2>&1 &'
  - su - %[1]s -c 'cd %[2]s && set -a && . ./.ohoci-cache-compat.env && set +a && ./run.sh'
`, input.RunnerUser, input.RunnerWorkDir)
}

func cacheCompatLocalBaseURL() string {
	return "http://127.0.0.1:31888"
}

func shellEnvValue(value string) string {
	cleaned := strings.ReplaceAll(strings.TrimSpace(value), "\n", "")
	return "'" + strings.ReplaceAll(cleaned, "'", `'\''`) + "'"
}
