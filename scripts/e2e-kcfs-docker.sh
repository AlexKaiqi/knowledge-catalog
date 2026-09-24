#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image="${KCFS_DOCKER_IMAGE:-golang:1.25-bookworm}"
node_version="${KCFS_NODE_VERSION:-24.20.0}"

if ! command -v docker >/dev/null 2>&1; then
  echo "FAIL: docker is not installed" >&2
  exit 1
fi

# Build the architecture-independent MountController module before mounting the
# repository read-only into Linux. The container then exercises that module
# against the real Linux kcfs binary and /dev/fuse.
if [[ ! -d "$repo_root/dsh-plugin/node_modules" ]]; then
  npm --prefix "$repo_root/dsh-plugin" ci --ignore-scripts --legacy-peer-deps >/dev/null
fi
npm --prefix "$repo_root/dsh-plugin" run build >/dev/null

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
  -e KCFS_NODE_VERSION="$node_version" \
  -e KC_DSH_PLUGIN_MOUNT_MODULE=/src/dsh-plugin/dist/mount.js \
  ${KC_LAKEFS_URL:+-e KC_LAKEFS_URL="${KC_LAKEFS_URL_CONTAINER:-$KC_LAKEFS_URL}"} \
  ${KC_LAKEFS_CREDENTIAL:+-e KC_LAKEFS_CREDENTIAL="$KC_LAKEFS_CREDENTIAL"} \
  ${KC_PRESIGN_TUNNEL:+-e KC_PRESIGN_TUNNEL="$KC_PRESIGN_TUNNEL"} \
  "$image" \
  bash -c '
    apt-get update -qq && apt-get install -y -qq curl fuse3 ripgrep python3 xz-utils socat >/tmp/kcfs-apt.log
    case "$(uname -m)" in
      aarch64|arm64) node_arch=arm64 ;;
      x86_64|amd64) node_arch=x64 ;;
      *) echo "FAIL: unsupported Node test architecture $(uname -m)" >&2; exit 1 ;;
    esac
    curl -fsSL "https://nodejs.org/dist/v${KCFS_NODE_VERSION}/node-v${KCFS_NODE_VERSION}-linux-${node_arch}.tar.xz" | tar -xJ -C /tmp
    export PATH="/tmp/node-v${KCFS_NODE_VERSION}-linux-${node_arch}/bin:$PATH"
    export GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false"
    ./scripts/e2e-kcfs-linux.sh
  '
