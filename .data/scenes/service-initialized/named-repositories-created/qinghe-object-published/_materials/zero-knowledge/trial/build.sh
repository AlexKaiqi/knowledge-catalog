#!/usr/bin/env bash
# Build current product and pinned trial agent; model credentials never enter the build.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(git -C "$HERE" rev-parse --show-toplevel)"
BUILD="$(mktemp -d "${TMPDIR:-/tmp}/kc-zk-build.XXXXXX")"
trap 'rm -rf "$BUILD"' EXIT
cd "$ROOT"
arch="$(docker image inspect --format '{{.Architecture}}' "${KC_ZK_RUNTIME_IMAGE:-kc-deploy-local:local}")"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -o "$BUILD/kc" ./cmd/kc
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -o "$BUILD/fixture-deployment" ./scripts/fixture-deployment
docker build -f "$HERE/Dockerfile.snapshot" --build-arg "KC_RUNTIME_IMAGE=${KC_ZK_RUNTIME_IMAGE:-kc-deploy-local:local}" -t "${KC_ZK_KC_IMAGE:-kc-zk-snapshot:local}" "$BUILD"
docker build -f "$HERE/Dockerfile.agent" --build-arg "KC_CLI_IMAGE=${KC_ZK_KC_IMAGE:-kc-zk-snapshot:local}" -t "${KC_ZK_AGENT_IMAGE:-kc-zk-agent:local}" "$HERE"
