#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "SKIP: kcfs host mount smoke test requires Linux"
  exit 0
fi
if [[ ! -e /dev/fuse ]]; then
  echo "SKIP: /dev/fuse is unavailable"
  exit 0
fi
if ! command -v fusermount3 >/dev/null 2>&1 && ! command -v fusermount >/dev/null 2>&1; then
  echo "SKIP: install fuse3 (fusermount3)"
  exit 0
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
run_root="$(mktemp -d)"
kc_pid=""
cleanup() {
  if [[ -n "$kc_pid" ]]; then
    kill -TERM "$kc_pid" >/dev/null 2>&1 || true
    wait "$kc_pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$run_root"
}
trap cleanup EXIT INT TERM

(cd "$repo_root" && go build -o "$run_root/kc" ./cmd/kc)
(cd "$repo_root" && go build -o "$run_root/kcfs" ./cmd/kcfs)

home_dir="$run_root/home"
project_dir="$run_root/project"
mkdir -p "$project_dir"
printf 'local\n' >"$project_dir/LOCAL.txt"

"$run_root/kc" --home "$home_dir" init --catalog kr://test/catalog >/dev/null
"$run_root/kc" --home "$home_dir" repo-add --repo kr://test/team >/dev/null
"$run_root/kc" --home "$home_dir" repo-add --repo kr://test/policy >/dev/null
"$run_root/kc" --home "$home_dir" define-workspace --workspace agent --revision 1 \
  --source 'kr://test/team=refs/heads/main@docs/team@team' \
  --source 'kr://test/team=refs/heads/main@docs/runbooks@runbooks' \
  --source 'kr://test/policy=refs/heads/main@knowledge/policy' >/dev/null
"$run_root/kc" --home "$home_dir" vfs-write --workspace agent --command-id seed-team \
  --path docs/team/README.md --content dGVhbQo= >/dev/null
"$run_root/kc" --home "$home_dir" vfs-write --workspace agent --command-id seed-policy \
  --path knowledge/policy/rules.md --content cG9saWN5Cg== >/dev/null
"$run_root/kc" --home "$home_dir" vfs-write --workspace agent --command-id seed-runbook \
  --path docs/runbooks/incident.md --content aW5jaWRlbnQK >/dev/null

"$run_root/kcfs" plan --home "$home_dir" --workspace agent --root "$project_dir" >"$run_root/plan.json"
python3 - "$run_root/plan.json" <<'PY'
import json, sys
plan = json.load(open(sys.argv[1]))
assert plan["pinId"]
assert {m["path"] for m in plan["mounts"]} == {"docs/team", "docs/runbooks", "knowledge/policy"}
assert all(m["files"] == 1 for m in plan["mounts"])
assert len({m["commit"] for m in plan["mounts"] if m["repository"] == "kr://test/team"}) == 1
PY

"$run_root/kcfs" mount --home "$home_dir" --workspace agent --root "$project_dir" >"$run_root/mount.json" 2>"$run_root/kcfs.log" &
kc_pid=$!
for _ in $(seq 1 100); do
  if [[ -f "$project_dir/docs/team/README.md" && -f "$project_dir/docs/runbooks/incident.md" && -f "$project_dir/knowledge/policy/rules.md" ]]; then
    break
  fi
  if ! kill -0 "$kc_pid" >/dev/null 2>&1; then
    cat "$run_root/kcfs.log" >&2
    exit 1
  fi
  sleep 0.05
done

[[ "$(cat "$project_dir/docs/team/README.md")" == "team" ]]
[[ "$(cat "$project_dir/docs/runbooks/incident.md")" == "incident" ]]
[[ "$(cat "$project_dir/knowledge/policy/rules.md")" == "policy" ]]
rg -q team "$project_dir/docs/team"
rg -q policy "$project_dir/knowledge/policy"
[[ "$(cat "$project_dir/LOCAL.txt")" == "local" ]]
if printf 'mutated\n' >"$project_dir/docs/team/README.md" 2>/dev/null; then
  echo "FAIL: kcfs mount accepted a write" >&2
  exit 1
fi

kill -TERM "$kc_pid"
wait "$kc_pid"
kc_pid=""
[[ ! -e "$project_dir/docs/team" ]]
[[ ! -e "$project_dir/docs/runbooks" ]]
[[ ! -e "$project_dir/knowledge/policy" ]]
[[ "$(cat "$project_dir/LOCAL.txt")" == "local" ]]
echo "PASS: kcfs multi-mount host filesystem smoke test"
