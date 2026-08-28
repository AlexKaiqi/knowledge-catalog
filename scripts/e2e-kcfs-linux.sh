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
team_repo="$run_root/team-repo"
policy_repo="$run_root/policy-repo"
mkdir -p "$project_dir"
printf 'local\n' >"$project_dir/LOCAL.txt"

mkdir -p "$team_repo/team" "$team_repo/runbooks" "$policy_repo"
printf 'team\n' >"$team_repo/team/README.md"
printf 'incident\n' >"$team_repo/runbooks/incident.md"
printf 'policy\n' >"$policy_repo/rules.md"
git -C "$team_repo" init -q -b main
git -C "$team_repo" add team/README.md runbooks/incident.md
git -C "$team_repo" -c user.name=fixture -c user.email=fixture@local commit -q -m seed
git -C "$policy_repo" init -q -b main
git -C "$policy_repo" add rules.md
git -C "$policy_repo" -c user.name=fixture -c user.email=fixture@local commit -q -m seed

"$run_root/kc" --home "$home_dir" local init --catalog kr://test/catalog >/dev/null
"$run_root/kc" --home "$home_dir" local repository attach --repo kr://test/team --dir "$team_repo" >/dev/null
"$run_root/kc" --home "$home_dir" local repository attach --repo kr://test/policy --dir "$policy_repo" >/dev/null
"$run_root/kc" --home "$home_dir" catalog workspace define --workspace agent --revision 1 \
  --source 'kr://test/team=refs/heads/main@docs/team@team' \
  --source 'kr://test/team=refs/heads/main@docs/runbooks@runbooks' \
  --source 'kr://test/policy=refs/heads/main@knowledge/policy' >/dev/null

"$run_root/kcfs" plan --home "$home_dir" --workspace agent --root "$project_dir" >"$run_root/plan.json"
python3 - "$run_root/plan.json" <<'PY'
import json, sys
plan = json.load(open(sys.argv[1]))
assert plan["pinId"]
assert {m["path"] for m in plan["mounts"]} == {"docs/team", "docs/runbooks", "knowledge/policy"}
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
printf 'advanced\n' >"$team_repo/team/README.md"
git -C "$team_repo" add team/README.md
git -C "$team_repo" -c user.name=fixture -c user.email=fixture@local commit -q -m advance
[[ "$(cat "$project_dir/docs/team/README.md")" == "team" ]]
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
