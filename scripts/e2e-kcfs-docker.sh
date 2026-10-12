#!/usr/bin/env bash
set -euo pipefail

kcfs_script_dir="${BASH_SOURCE[0]%/*}"
if [[ "$kcfs_script_dir" == "${BASH_SOURCE[0]}" ]]; then
  kcfs_script_dir="."
fi
source "$kcfs_script_dir/kcfs-env.sh"
kcfs_require_lakefs_binding

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image="${KCFS_DOCKER_IMAGE:-golang:1.25-bookworm}"

if ! command -v docker >/dev/null 2>&1; then
  echo "FAIL: docker is not installed" >&2
  exit 1
fi

# The stack env binds to the host loopback; inside the Linux container the
# loopback is the container itself, so retarget the host via its gateway alias.
if [[ -n "${KC_LAKEFS_URL:-}" ]]; then
  KC_LAKEFS_URL_CONTAINER="${KC_LAKEFS_URL/127.0.0.1/host.docker.internal}"
  KC_LAKEFS_URL_CONTAINER="${KC_LAKEFS_URL_CONTAINER/localhost/host.docker.internal}"
  export KC_LAKEFS_URL_CONTAINER
fi

exec docker run --rm \
  --device /dev/fuse \
  --cap-add SYS_ADMIN \
  --security-opt apparmor=unconfined \
  -v "$repo_root:/src:ro" \
  -w /src \
  -e GOCACHE=/tmp/go-cache \
  -e GOMODCACHE=/tmp/go-mod \
  ${KC_LAKEFS_URL:+-e KC_LAKEFS_URL="${KC_LAKEFS_URL_CONTAINER:-$KC_LAKEFS_URL}"} \
  ${KC_LAKEFS_CREDENTIAL:+-e KC_LAKEFS_CREDENTIAL="$KC_LAKEFS_CREDENTIAL"} \
  ${KC_PRESIGN_TUNNEL:+-e KC_PRESIGN_TUNNEL="$KC_PRESIGN_TUNNEL"} \
  "$image" \
  bash -c '
    set -euo pipefail
    apt-get update -qq
    apt-get install -y -qq curl fuse3 ripgrep python3 socat >/tmp/kcfs-apt.log
    export GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false"
    ./scripts/e2e-kcfs-linux.sh
  '
