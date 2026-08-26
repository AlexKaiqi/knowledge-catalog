#!/usr/bin/env bash
# Deterministic acceptance for the current read-only host projection.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

go -C "$ROOT" test ./... -run '^$' -count=1
go -C "$ROOT" test ./workspacefs ./internal/arch -count=1
go -C "$ROOT" test ./catalog ./cli \
  -run 'Test(OneRepositoryCanProjectSeveralDisjointSubtrees|RepeatedRepositoryMustShareCoordinateAndDisjointSubPaths|Mount|Route|Virtual|RelativeMountPath|PrepareWorkspaceFS)' \
  -count=1
go -C "$ROOT" vet ./workspacefs ./cli ./cmd/kcfs
(cd "$ROOT/dsh-plugin" && npm run typecheck && npm test)
exec "$ROOT/scripts/e2e-kcfs-linux.sh"
