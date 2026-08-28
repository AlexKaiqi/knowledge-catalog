#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image="${KCFS_DOCKER_IMAGE:-golang:1.25-bookworm}"

if ! command -v docker >/dev/null 2>&1; then
  echo "FAIL: docker is not installed" >&2
  exit 1
fi

exec docker run --rm \
  --device /dev/fuse \
  --cap-add SYS_ADMIN \
  --security-opt apparmor=unconfined \
  -v "$repo_root:/src:ro" \
  -w /src \
  -e GOCACHE=/tmp/go-cache \
  -e GOMODCACHE=/tmp/go-mod \
  "$image" \
  bash -c 'apt-get update -qq && apt-get install -y -qq curl fuse3 ripgrep python3 >/tmp/kcfs-apt.log && ./scripts/e2e-kcfs-linux.sh'
