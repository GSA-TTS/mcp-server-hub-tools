#!/bin/bash

set -euo pipefail

image="${1:?usage: $0 <providers-image>}"
registry="/obot-providers/gsa-tools"
container=""
work_dir="$(mktemp -d)"

cleanup() {
  if [[ -n "$container" ]]; then
    docker rm -f "$container" >/dev/null 2>&1 || true
  fi
  rm -rf "$work_dir"
}
trap cleanup EXIT

# Provider images are filesystem layers and intentionally have no CMD. Supply a
# placeholder command so Docker can create a stopped container for inspection;
# the command is never executed.
container="$(docker create --platform linux/amd64 "$image" /bin/true)"
docker cp "$container:/obot-providers/." "$work_dir"

test -f "$work_dir/.envrc.providers.gsa-tools"
test -x "$work_dir/gsa-tools/bin/login.gov-auth-provider"
test -x "$work_dir/gsa-tools/bin/login.gov-auth-provider.bin"
test -f "$work_dir/gsa-tools/auth-providers/login.gov-auth-provider.yaml"
test -f "$work_dir/gsa-tools/auth-providers-common/templates/error.html"
grep -F "OBOT_SERVER_PROVIDER_REGISTRIES=\"${registry}\"" "$work_dir/.envrc.providers.gsa-tools"
grep -F 'command: bin/login.gov-auth-provider' "$work_dir/gsa-tools/auth-providers/login.gov-auth-provider.yaml"

echo "login.gov provider image layout verified: $image"
