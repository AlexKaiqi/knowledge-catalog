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
npm --prefix "$repo_root/dsh-plugin" ci --ignore-scripts --legacy-peer-deps >/dev/null
npm --prefix "$repo_root/dsh-plugin" run build >/dev/null

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
  "$image" \
  bash -c '
    apt-get update -qq && apt-get install -y -qq curl fuse3 ripgrep python3 xz-utils >/tmp/kcfs-apt.log
    case "$(uname -m)" in
      aarch64|arm64) node_arch=arm64 ;;
      x86_64|amd64) node_arch=x64 ;;
      *) echo "FAIL: unsupported Node test architecture $(uname -m)" >&2; exit 1 ;;
    esac
    curl -fsSL "https://nodejs.org/dist/v${KCFS_NODE_VERSION}/node-v${KCFS_NODE_VERSION}-linux-${node_arch}.tar.xz" | tar -xJ -C /tmp
    export PATH="/tmp/node-v${KCFS_NODE_VERSION}-linux-${node_arch}/bin:$PATH"
    ./scripts/e2e-kcfs-linux.sh
  '
