#!/usr/bin/env bash
# Release-level acceptance for project-local read-only knowledge mounts.
# Three consecutive independent real-model runs are required; one failure
# aborts the sequence and the count must restart after a fix.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
export PATH="$HOME/.local/go/bin:$HOME/.local/bin:$PATH"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-$$"
ARTIFACT_ROOT="${KC_PROJECT_MOUNT_ACCEPT_ARTIFACTS:-$ROOT/.data/project-mount-acceptance/$RUN_ID}"
mkdir -p "$ARTIFACT_ROOT"

echo "acceptance artifacts: $ARTIFACT_ROOT"
bash -n "$ROOT/dsh-plugin/scripts/e2e-project-mount.sh" "$ROOT/dsh-plugin/scripts/accept-project-mount.sh"
python3 -m py_compile "$ROOT/dsh-plugin/scripts/e2e-project-mount.py"

for run in R1 R2 R3; do
  echo "==> $run clean-room: real Gitea + real DSH Agents"
  KC_PROJECT_MOUNT_ARTIFACTS="$ARTIFACT_ROOT/$run" \
  DSH_PROFILE="project-mount-${RUN_ID}-${run}" \
  "$ROOT/dsh-plugin/scripts/e2e-project-mount.sh" 2>&1 | tee "$ARTIFACT_ROOT/$run.log"
done

python3 - "$ARTIFACT_ROOT" <<'PY'
import json
import sys
from pathlib import Path

root = Path(sys.argv[1])
rows = []
seen = set()
for name in ("R1", "R2", "R3"):
    oracle = json.loads((root / name / "oracle.json").read_text())
    required = {
        "gitProject": "clean",
        "nonGitProject": "unchanged",
        "mountMaterialized": False,
        "oldTask": "V1->V1",
        "newTask": "V2",
        "remoteWrite": "DENIED_WITHOUT_HEAD_CHANGE",
        "repositoryIdentityVisibleToAgent": False,
    }
    for key, expected in required.items():
        if oracle.get(key) != expected:
            raise SystemExit(f"FAIL {name}: {key}={oracle.get(key)!r}, want {expected!r}")
    pair = (oracle["v1Commit"], oracle["v2Commit"])
    if pair[0] == pair[1] or pair in seen:
        raise SystemExit(f"FAIL {name}: commits are not an independent advance: {pair}")
    seen.add(pair)
    for evidence in (
        "pin.session.jsonl.zstd", "pin.tool-calls.json",
        "live.session.jsonl.zstd", "live.tool-calls.json",
        "readonly.session.jsonl.zstd", "readonly.tool-calls.json",
        "gitea-commits.json", "kc-serve.log",
    ):
        if not (root / name / evidence).is_file():
            raise SystemExit(f"FAIL {name}: missing evidence {evidence}")
    rows.append({"run": name, **oracle})
(root / "PASS.json").write_text(json.dumps({"runs": rows}, indent=2, sort_keys=True) + "\n")
print("PASS: three independent project-mount runs verified")
PY

echo "==> deterministic plugin and protocol regression"
CHECK_BIN="$(mktemp /tmp/kc-project-mount-accept.XXXXXX)"
cleanup() { rm -f "$CHECK_BIN"; }
trap cleanup EXIT
go -C "$ROOT" build -o "$CHECK_BIN" ./cmd/kc
(cd "$ROOT/dsh-plugin" && npm run build && npm run typecheck && KC_BIN="$CHECK_BIN" npm test) \
  2>&1 | tee "$ARTIFACT_ROOT/dsh-plugin-test.log"
go -C "$ROOT" test ./cli ./catalog ./snapshot/gitea -timeout 20m \
  2>&1 | tee "$ARTIFACT_ROOT/protocol-test.log"

echo "PASS artifacts=$ARTIFACT_ROOT"
