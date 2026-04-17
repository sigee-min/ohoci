package oci

import (
	"fmt"
	"strings"
)

type RunnerImageBakeCloudInitInput struct {
	SetupCommands  []string
	VerifyCommands []string
}

func BuildRunnerImageBakeCloudInit(input RunnerImageBakeCloudInitInput) string {
	script := strings.TrimSpace(fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

mkdir -p /var/lib/ohoci

setup_commands=$(cat <<'__OHOCI_SETUP__'
%s
__OHOCI_SETUP__
)

verify_commands=$(cat <<'__OHOCI_VERIFY__'
%s
__OHOCI_VERIFY__
)

echo "OHOCI_IMAGE_BAKE_PHASE:provisioning"
set +e
bash -lc "$setup_commands"
setup_exit=$?
set -e

verify_exit=0
summary="setup and verify commands passed"
success=true

if [ "$setup_exit" -eq 0 ]; then
  echo "OHOCI_IMAGE_BAKE_PHASE:verifying"
  set +e
  bash -lc "$verify_commands"
  verify_exit=$?
  set -e
else
  success=false
  summary="setup commands failed"
fi

if [ "$verify_exit" -ne 0 ]; then
  success=false
  summary="verify commands failed"
fi

cat >/var/lib/ohoci/bake-result.json <<EOF
{"success":$success,"summary":"$summary","setupExitCode":$setup_exit,"verifyExitCode":$verify_exit}
EOF

echo "OHOCI_IMAGE_BAKE_RESULT_BEGIN"
cat /var/lib/ohoci/bake-result.json
echo "OHOCI_IMAGE_BAKE_RESULT_END"

shutdown -h now
`, strings.Join(input.SetupCommands, "\n"), strings.Join(input.VerifyCommands, "\n")))

	return fmt.Sprintf(`#cloud-config
package_update: true
package_upgrade: false
write_files:
  - path: /usr/local/bin/ohoci-runner-image-bake.sh
    permissions: '0755'
    content: |
%s
runcmd:
  - bash -lc '/usr/local/bin/ohoci-runner-image-bake.sh > >(tee /var/log/ohoci-runner-image-bake.log /dev/console) 2>&1'
`, indentForCloudInit(script))
}

func indentForCloudInit(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for index := range lines {
		lines[index] = "      " + lines[index]
	}
	return strings.Join(lines, "\n")
}
