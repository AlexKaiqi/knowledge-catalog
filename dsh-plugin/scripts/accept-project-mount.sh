#!/usr/bin/env bash
# Release check for Linux host-level Workspace mounts.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

go -C "$ROOT" test ./workspacefs -count=1
go -C "$ROOT" test ./catalog ./cli \
  -run 'Test(OneRepositoryCanProjectSeveralDisjointSubtrees|RepeatedRepositoryMustShareCoordinateAndDisjointSubPaths|Mount|Route|Virtual|RelativeMountPath|PrepareWorkspaceFS)' \
  -count=1
(cd "$ROOT/dsh-plugin" && npm run typecheck && npm test)
exec "$ROOT/scripts/e2e-kcfs-linux.sh"
