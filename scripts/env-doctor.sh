#!/usr/bin/env bash
# Read-only development environment diagnostics.
#
# Every finding names the command an owner runs to act on it. This script never
# starts, stops, removes, or repairs anything: containers, ports, leases,
# worktrees, and evidence directories are observed only. A read-only probe must
# keep reporting after one source is unavailable, so it does not use `set -e`.
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root" || exit 2

strict=0
case "${1:-}" in
  --strict) strict=1 ;;
  -h|--help)
    printf '%s\n' \
      'usage: ./scripts/env-doctor.sh [--strict]' \
      '' \
      'Read-only report of toolchain drift, container lifecycle, fixed ports,' \
      'the automatic OpenSearch lease, worktree validity, and evidence size.' \
      '--strict exits 1 when any warning is reported.'
    exit 0
    ;;
  '') ;;
  *) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
esac

warnings=0
section() { printf '\n== %s ==\n' "$1"; }
ok() { printf '[ok]   %s\n' "$*"; }
info() { printf '[info] %s\n' "$*"; }
warn() { warnings=$((warnings + 1)); printf '[warn] %s\n' "$*"; }

# Fixed host ports of the persistent dev profile (scripts/deploy/deploy.sh).
# They are deliberate product surface: bookmarks and KC_PUBLIC_URL stay stable.
dev_ports="server:7385 lakefs:18010 minio:19010 minio-console:19021 opensearch:19210 opensearch-dashboards:15611 grafana:7310 prometheus:19190 otlp:14328 ttyd:7692"

listener_of() {
  local port="$1" name=""
  if command -v lsof >/dev/null 2>&1; then
    name="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -Fc 2>/dev/null | sed -n 's/^c//p' | head -1)"
  elif command -v ss >/dev/null 2>&1; then
    name="$(ss -H -ltn "sport = :$port" 2>/dev/null | awk 'NR==1 {print $6}')"
  fi
  printf '%s' "$name"
}

section toolchain
if command -v go >/dev/null 2>&1; then
  go_local="$(go version 2>/dev/null | awk '{print $3}')"
else
  go_local=""
  warn "go is not on PATH; every Go entry point is unavailable"
fi
go_directive="$(awk '$1 == "go" {print "go" $2; exit}' go.mod)"
go_toolchain="$(awk '$1 == "toolchain" {print $2; exit}' go.mod)"
pin="${go_toolchain:-$go_directive}"
if [[ -z "$go_local" ]]; then
  :
elif [[ -n "$pin" && "$go_local" != "$pin" ]]; then
  warn "local $go_local but go.mod pins $pin; commands green here can fail in CI. Pin with: go env -w GOTOOLCHAIN=$pin"
else
  ok "go $go_local matches go.mod ($pin)"
fi
node_pin="$(tr -d '[:space:]' < .node-version 2>/dev/null)"
node_local="$(node --version 2>/dev/null)"
if [[ -z "$node_local" ]]; then
  info "node is not on PATH; snapshot/console/webui targets stay unavailable"
elif [[ -n "$node_pin" && "$node_local" != "v$node_pin" ]]; then
  warn "local node $node_local but .node-version pins v$node_pin; webui builds may drift"
else
  ok "node $node_local matches .node-version"
fi

docker_ok=0
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  docker_ok=1
else
  warn "docker is unavailable; container, port, and lease findings are skipped"
fi

section containers
if (( docker_ok == 1 )); then
  running="$(docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}' 2>/dev/null)"
  running_count="$(printf '%s' "$running" | grep -c . || true)"
  info "$running_count running container(s)"
  while IFS=$'\t' read -r name image status; do
    [[ -n "$name" ]] || continue
    case "$status" in
      Restarting*|*unhealthy*)
        warn "$name ($image) is $status; inspect with: docker logs --tail 100 $name"
        ;;
    esac
  done <<< "$running"
  while IFS= read -r name; do
    [[ -n "$name" ]] || continue
    info "$name is up 24h+ (long-lived by design for local|dev; stop with: make deploy-local-down / make deploy-dev-down)"
  done < <(docker ps --format '{{.Names}}\t{{.Status}}' 2>/dev/null | awk -F'\t' '$2 ~ / (day|days|week|weeks|month|months) / {print $1}')
  exited="$(docker ps -a --filter status=exited --format '{{.Names}}' 2>/dev/null | grep -c . || true)"
  if (( exited > 0 )); then
    warn "$exited exited container(s) still hold writable layers; inspect with: docker ps -a --filter status=exited"
  fi
  if [[ -n "$(docker ps --filter label=com.docker.compose.project=kc-deploy-dev --format '{{.Names}}' 2>/dev/null)" ]]; then
    info "dev stack is running and reserves every fixed port below"
  fi
else
  info "container lifecycle not observed"
fi

section ports
if (( docker_ok == 1 )); then
  held=0
  for entry in $dev_ports; do
    label="${entry%%:*}"
    port="${entry##*:}"
    holder="$(listener_of "$port")"
    if [[ -n "$holder" ]]; then
      held=$((held + 1))
      info "dev $label :$port held by $holder"
    fi
  done
  if (( held == 0 )); then
    ok "no dev fixed port is in use; the dev stack is not running"
  fi
  orphan=0
  for entry in $dev_ports; do
    port="${entry##*:}"
    holder="$(listener_of "$port")"
    if [[ -n "$holder" && -z "$(docker ps --filter label=com.docker.compose.project=kc-deploy-dev --format '{{.Names}}' 2>/dev/null)" ]]; then
      orphan=1
      label="${entry%%:*}"
      warn "dev $label :$port held by $holder while no kc-deploy-dev stack runs; free it or start: make deploy-dev-up"
    fi
  done
  (( orphan == 0 )) || true
fi

section opensearch-lease
# The lease file is keyed by sha256(Docker daemon ID), so a daemon whose identity
# changed leaves its file behind forever. Report the current daemon's lease in
# detail and aggregate the rest; never print one line per stale file.
shopt -s nullglob
lease_files=(/tmp/kc-opensearch-auto-*.lock)
shopt -u nullglob
lease_total=${#lease_files[@]}
current_lease=""
if (( docker_ok == 1 )); then
  daemon_key="$(docker info --format '{{.ID}}' 2>/dev/null | tr -d '[:space:]' | shasum -a 256 | awk '{print $1}')"
  [[ -n "$daemon_key" ]] && current_lease="/tmp/kc-opensearch-auto-${daemon_key}.lock"
fi
if (( lease_total == 0 )); then
  ok "no automatic OpenSearch lease on this host"
else
  lease_summary="$(KC_CURRENT_LEASE="$current_lease" python3 - "${lease_files[@]}" <<'PY'
import json, os, sys
paths = sys.argv[1:]
current = os.environ.get("KC_CURRENT_LEASE", "")
pending = 0
alive = 0
for path in paths:
    try:
        with open(path) as handle:
            value = json.load(handle)
    except Exception:
        continue
    if value.get("pendingCleanup"):
        pending += 1
    try:
        os.kill(int(value.get("pid")), 0)
        alive += 1
    except Exception:
        pass
detail = "-"
if current and os.path.exists(current):
    try:
        with open(current) as handle:
            value = json.load(handle)
        detail = "%s %s %s" % (value.get("pid", "-"), value.get("container", "-"), value.get("pendingCleanup", "-"))
    except Exception:
        detail = "unreadable - -"
print("total=%d pending=%d alive=%d current=%s" % (len(paths), pending, alive, detail))
PY
)"
  lease_total_v=0 pending_n=0 alive_n=0 cur_pid="-" cur_container="-" cur_pending="-"
  for pair in $lease_summary; do
    case "$pair" in
      total=*) lease_total_v="${pair#total=}" ;;
      pending=*) pending_n="${pair#pending=}" ;;
      alive=*) alive_n="${pair#alive=}" ;;
    esac
  done
  detail_part="${lease_summary##*current=}"
  if [[ "$detail_part" != "-" && -n "$detail_part" ]]; then
    read -r cur_pid cur_container cur_pending <<< "$detail_part"
  fi
  info "$lease_total_v lease file(s) under /tmp, $alive_n owned by a live process, $pending_n still flagged pendingCleanup"
  if [[ -n "$current_lease" && -f "$current_lease" ]]; then
    if [[ "$cur_pid" =~ ^[0-9]+$ ]] && kill -0 "$cur_pid" 2>/dev/null; then
      # pendingCleanup is the normal in-flight state between container creation
      # and its confirmed removal; only a dead owner makes the flag a fence.
      info "current daemon lease held by pid $cur_pid ($cur_container, pendingCleanup=$cur_pending); other runs on this host queue behind it"
    elif [[ "$cur_pending" == "True" ]]; then
      warn "current daemon lease fences automatic OpenSearch: owner pid $cur_pid is gone but $cur_container was never confirmed removed. Dispose of it explicitly, then clear the flag by hand (see doc/engineering/testing.md)"
    else
      info "current daemon lease exists with owner pid $cur_pid already gone; the next start overwrites it"
    fi
  else
    info "no lease file for the current Docker daemon identity yet"
  fi
  if (( pending_n > 0 && alive_n == 0 )); then
    warn "$pending_n stale pendingCleanup fence(s) belong to daemon identities that no longer exist; a daemon restart silently bypasses each fence instead of honouring it. Inspect with: ls /tmp/kc-opensearch-auto-*.lock | wc -l"
  fi
fi
if (( docker_ok == 1 )); then
  auto_count="$(docker ps --filter 'name=kc-local-opensearch-' --format '{{.Names}}' 2>/dev/null | grep -c . || true)"
  if (( auto_count > ${#lease_files[@]} )); then
    warn "$auto_count automatic OpenSearch container(s) but ${#lease_files[@]} lease file(s); a container outlived its lease. Inspect with: docker ps --filter name=kc-local-opensearch-"
  fi
fi

# The host fence, not the per-daemon lease, is what survives a daemon restart.
# It is reported separately because an entry there blocks every automatic start
# regardless of which daemon identity is current. An entry whose container is
# already gone does not block anything: the next start settles it, so reporting
# it as a blocker would be a false alarm.
fence_path="${KC_OPENSEARCH_FENCE:-/tmp/kc-opensearch-auto-fence.json}"
if [[ -f "$fence_path" ]]; then
  fence_list="$(python3 - "$fence_path" <<'PY'
import json, sys
try:
    with open(sys.argv[1]) as handle:
        value = json.load(handle)
except Exception:
    print("unreadable")
    raise SystemExit(0)
if not isinstance(value, dict):
    print("unreadable")
    raise SystemExit(0)
for name in sorted(value):
    print(name)
PY
)"
  if [[ "$fence_list" == "unreadable" ]]; then
    warn "automatic OpenSearch fence $fence_path is unreadable; automatic starts will refuse until it is disposed explicitly"
  elif [[ -z "$fence_list" ]]; then
    ok "automatic OpenSearch fence is clear"
  else
    blocking=()
    settled=0
    while IFS= read -r container; do
      [[ -n "$container" ]] || continue
      if (( docker_ok == 1 )) && docker inspect "$container" --format '{{.Id}}' >/dev/null 2>&1; then
        blocking+=("$container")
      else
        settled=$((settled + 1))
      fi
    done <<< "$fence_list"
    if (( ${#blocking[@]} > 0 )); then
      warn "automatic OpenSearch fence blocks starts: ${#blocking[@]} container(s) still exist with unproven cleanup (${blocking[*]}). Remove each container and clear its entry; inspect with: docker ps -a --filter name=kc-local-opensearch-"
    fi
    if (( settled > 0 )); then
      info "$settled fence entr(y/ies) whose container is already gone; the next automatic start settles them, no action needed"
    fi
  fi
else
  info "no automatic OpenSearch fence on this host"
fi

section worktrees
while IFS= read -r line; do
  case "$line" in
    "worktree "*)
      path="${line#worktree }"
      if [[ -d "$path" ]]; then
        info "$path"
      else
        warn "worktree entry $path no longer exists; run: git worktree prune"
      fi
      ;;
  esac
done < <(git worktree list --porcelain 2>/dev/null)
if [[ -n "$(git status --porcelain 2>/dev/null)" ]]; then
  changed="$(git status --porcelain 2>/dev/null | grep -c . || true)"
  info "$changed path(s) modified in this worktree"
fi

section evidence
size_kb() {
  if [[ -e "$1" ]]; then
    du -sk "$1" 2>/dev/null | awk '{print $1}'
  else
    printf '0'
  fi
}
human() {
  local kb="$1"
  if (( kb >= 1048576 )); then
    printf '%d.%dG' $((kb / 1048576)) $(( (kb % 1048576) * 10 / 1048576 ))
  elif (( kb >= 1024 )); then
    printf '%d.%dM' $((kb / 1024)) $(( (kb % 1024) * 10 / 1024 ))
  else
    printf '%dK' "$kb"
  fi
}
runs_kb="$(size_kb .validation/runs)"
runs_count="$(find .validation/runs -mindepth 1 -maxdepth 1 -type d 2>/dev/null | grep -c . || true)"
info "evidence .validation/runs: ${runs_count} run(s), $(human "$runs_kb")"
if (( runs_count > 0 )); then
  latest="$(find .validation/runs -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | tail -1)"
  latest="${latest##*/}"
  latest_status="$(python3 -c '
import json, sys
try:
    with open(sys.argv[1]) as handle:
        print(json.load(handle).get("status", "unknown"))
except Exception:
    print("no readable report.json")
' ".validation/runs/$latest/report.json" 2>/dev/null)"
  info "latest run $latest: ${latest_status:-no readable report.json}"
fi
if (( runs_kb > 1048576 )); then
  warn "evidence exceeds 1G and scripts/validation.py keeps every run; prune old runs explicitly, for example: du -sk .validation/runs/* | sort -rn | head"
fi
for path in .data/realcase .data/animation dist; do
  kb="$(size_kb "$path")"
  if (( kb > 524288 )); then
    warn "$path is $(human "$kb"); confirm it is a rebuildable local trial and keep it out of backups and Spotlight"
  elif (( kb > 0 )); then
    info "$path: $(human "$kb")"
  fi
done

section summary
if (( warnings == 0 )); then
  ok "no warnings"
else
  printf '%s warning(s)\n' "$warnings"
fi
if (( strict == 1 && warnings > 0 )); then
  exit 1
fi
exit 0
