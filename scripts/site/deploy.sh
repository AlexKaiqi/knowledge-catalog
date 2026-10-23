#!/usr/bin/env bash
# Deploy the kc landing site to its two homes.
#
#   with  The With preview serves this workspace's .data/site directly, so
#         "deploying to with" is a verification step, not an upload. It checks
#         the tree is complete, the packages match SHA256SUMS, every local
#         reference resolves, and install.sh advertises the expected URL.
#   dev   A DevCloud development machine. The site is copied there and served
#         over plain HTTP so other containers in the same network region can
#         read it without an iGate policy (the preview host sits in IDC and is
#         refused by ACL from DevCloud).
#
# Never kills a process it did not start: ownership is the pidfile written by
# scripts/site/serve.py, and a foreign listener on the target port is reported
# instead of being replaced. Pidfiles and logs are port-scoped
# (site-<port>.pid), so instances on different ports do not interfere.
#
# Each dev subcommand is ONE ssh round trip: a payload script under
# scripts/site/ is fed to `bash -s` on the remote host. An ssh call to a
# DevCloud machine costs ~0.5s of handshake, so a sequence built from several
# calls makes a 0.2s action look like a hang.
#
# Usage:
#   ./scripts/site/deploy.sh with check
#   ./scripts/site/deploy.sh with serve [--port N]
#   ./scripts/site/deploy.sh with verify
#   ./scripts/site/deploy.sh dev sync
#   ./scripts/site/deploy.sh dev up | down | status | verify
#   ./scripts/site/deploy.sh dev deploy      # sync + up + verify
#
# Environment:
#   SITE_DIR           local site root        (default <repo>/.data/site)
#   SITE_PUBLIC_URL    expected canonical URL (default the current preview URL)
#   SITE_DEV_SSH       user@host              (default kaiqidong@kaiqidong-any1.devcloud.woa.com)
#   SITE_DEV_SSH_PORT  ssh port               (default 36000)
#   SITE_DEV_DIR       remote public root     (default /data/home/<user>/site)
#   SITE_DEV_HOME      remote runtime dir     (default /data/home/<user>/.site-deploy)
#   SITE_DEV_PORT      remote http port       (default 18888)
#   SITE_LOCAL_PORT    local http port        (default 18889)

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/../.." && pwd)

SITE_DIR=${SITE_DIR:-$REPO_ROOT/.data/site}
SITE_PUBLIC_URL=${SITE_PUBLIC_URL:-https://d65b3089-nomttpckp-devc.preview.with.woa.com}
SITE_DEV_SSH=${SITE_DEV_SSH:-kaiqidong@kaiqidong-any1.devcloud.woa.com}
SITE_DEV_SSH_PORT=${SITE_DEV_SSH_PORT:-36000}
SITE_DEV_USER=${SITE_DEV_SSH%@*}
SITE_DEV_DIR=${SITE_DEV_DIR:-/data/home/$SITE_DEV_USER/site}
SITE_DEV_HOME=${SITE_DEV_HOME:-/data/home/$SITE_DEV_USER/.site-deploy}
SITE_DEV_PORT=${SITE_DEV_PORT:-18888}
SITE_LOCAL_PORT=${SITE_LOCAL_PORT:-18889}

SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o LogLevel=ERROR
    -o StrictHostKeyChecking=accept-new -p "$SITE_DEV_SSH_PORT")
SCP_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o LogLevel=ERROR
    -o StrictHostKeyChecking=accept-new -P "$SITE_DEV_SSH_PORT")

log() { printf '==> %s\n' "$*"; }
warn() { printf 'warn: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

remote() { ssh "${SSH_OPTS[@]}" "$SITE_DEV_SSH" "$@"; }

require_site() {
    [ -d "$SITE_DIR" ] || die "site root not found: $SITE_DIR"
    [ -f "$SITE_DIR/index.html" ] || die "no index.html in $SITE_DIR"
}

# Deterministic fingerprint of a tree: relative path, size and content hash of
# every regular file, sorted. Used to prove the copy on the dev machine is
# byte-identical to the local tree without transferring it twice.
tree_hash_of() {
    python3 - "$1" <<'PY'
import hashlib, os, sys
root = sys.argv[1]
rows = []
for base, dirs, files in os.walk(root):
    dirs[:] = sorted(d for d in dirs if d != '.git')
    for name in sorted(files):
        if name == '.DS_Store' or name.startswith('._'):
            continue
        path = os.path.join(base, name)
        if not os.path.isfile(path) or os.path.islink(path):
            continue
        rel = os.path.relpath(path, root)
        digest = hashlib.sha256()
        with open(path, 'rb') as handle:
            for chunk in iter(lambda: handle.read(1 << 20), b''):
                digest.update(chunk)
        rows.append('%s\t%d\t%s' % (rel, os.path.getsize(path), digest.hexdigest()))
print(hashlib.sha256('\n'.join(rows).encode()).hexdigest())
PY
}

# ---------------------------------------------------------------- with checks

check_required() {
    local missing=0
    for rel in index.html install.sh assets/style.css assets/site.js dl/SHA256SUMS; do
        if [ -e "$SITE_DIR/$rel" ]; then
            printf '    ok   %s\n' "$rel"
        else
            printf '    MISS %s\n' "$rel"
            missing=$((missing + 1))
        fi
    done
    [ "$missing" -eq 0 ] || die "$missing required file(s) missing"
}

check_packages() {
    local dir="$SITE_DIR/dl" bad=0 name expected actual
    [ -d "$dir" ] || die "no dl/ directory"
    while read -r expected name; do
        [ -n "${name:-}" ] || continue
        case "$expected" in \#*) continue ;; esac
        if [ ! -f "$dir/$name" ]; then
            printf '    MISS %s\n' "$name"
            bad=$((bad + 1))
            continue
        fi
        actual=$(sha256_of "$dir/$name")
        if [ "$actual" = "$expected" ]; then
            printf '    ok   %s\n' "$name"
        else
            printf '    BAD  %s (checksum mismatch)\n' "$name"
            bad=$((bad + 1))
        fi
    done < "$dir/SHA256SUMS"
    [ "$bad" -eq 0 ] || die "$bad package(s) failed verification"
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

check_references() {
    python3 - "$SITE_DIR" <<'PY'
import os, re, sys
root = sys.argv[1]
pattern = re.compile(r'(?:href|src)\s*=\s*["\']([^"\']+)["\']', re.I)
bad = []
checked = 0
for base, dirs, files in os.walk(root):
    dirs[:] = sorted(d for d in dirs if d != '.git')
    for name in sorted(files):
        if not name.endswith('.html'):
            continue
        page = os.path.join(base, name)
        with open(page, encoding='utf-8', errors='replace') as handle:
            text = handle.read()
        for target in pattern.findall(text):
            if not target or target.startswith(('#', 'http://', 'https://', '//', 'mailto:', 'data:', 'javascript:')):
                continue
            target = target.split('?')[0].split('#')[0]
            if not target:
                continue
            resolved = os.path.normpath(os.path.join(base, target.lstrip('/') if target.startswith('/') else target))
            checked += 1
            if not os.path.exists(resolved):
                bad.append('%s -> %s' % (os.path.relpath(page, root), target))
print('    checked %d local reference(s)' % checked)
if bad:
    for row in bad:
        print('    BROKEN %s' % row)
    sys.exit(1)
PY
}

check_public_url() {
    local line
    line=$(grep -m1 "^base=" "$SITE_DIR/install.sh" || true)
    [ -n "$line" ] || die "install.sh has no base= line"
    case "$line" in
        *"$SITE_PUBLIC_URL"*)
            printf '    ok   install.sh base matches %s\n' "$SITE_PUBLIC_URL"
            ;;
        *)
            warn "install.sh base does not match SITE_PUBLIC_URL"
            printf '    install.sh: %s\n' "$line"
            printf '    expected  : %s\n' "$SITE_PUBLIC_URL"
            return 1
            ;;
    esac
}

cmd_with_check() {
    require_site
    log "checking site tree: $SITE_DIR"
    local hash files
    hash=$(tree_hash_of "$SITE_DIR" 2>/dev/null)
    log "required files"
    check_required
    log "package checksums"
    check_packages
    log "local references"
    check_references
    log "advertised url"
    check_public_url || warn "pass SITE_PUBLIC_URL=<url> to silence this after a domain change"
    files=$(find "$SITE_DIR" -type f ! -name '.DS_Store' | wc -l | tr -d ' ')
    log "tree hash $hash ($files files, $(du -sh "$SITE_DIR" | cut -f1))"
    log "with: tree is deployable as-is (preview serves this directory)"
}

cmd_with_serve() {
    require_site
    local port=$SITE_LOCAL_PORT
    while [ $# -gt 0 ]; do
        case "$1" in
            --port) port=$2; shift 2 ;;
            *) die "unknown argument: $1" ;;
        esac
    done
    exec python3 "$SCRIPT_DIR/serve.py" \
        --dir "$SITE_DIR" --port "$port" --bind 127.0.0.1 \
        --pidfile "/tmp/kc-site-$port.pid"
}

cmd_with_verify() {
    local port=$SITE_LOCAL_PORT base status
    base="http://127.0.0.1:$port"
    log "verifying $base"
    status=$(curl -s -o /dev/null -m 8 -w '%{http_code}' "$base/index.html" || true)
    [ "$status" = 200 ] || die "index.html returned $status (is 'with serve' running?)"
    local code
    for rel in index.html http.html install.sh dl/SHA256SUMS; do
        code=$(curl -s -o /dev/null -m 8 -w '%{http_code}' "$base/$rel")
        printf '    %-16s %s\n' "$rel" "$code"
        [ "$code" = 200 ] || die "$rel returned $code"
    done
    log "local preview serves the whole tree"
}

# ----------------------------------------------------------------- dev remote

cmd_dev_sync() {
    require_site
    log "syncing $SITE_DIR -> $SITE_DEV_SSH:$SITE_DEV_DIR"
    local local_hash remote_hash
    local_hash=$(tree_hash_of "$SITE_DIR" 2>/dev/null)
    # The archive streams in while the payload prepares, extracts, cleans and
    # hashes: one round trip instead of mkdir + extract + find + hash. Each ssh
    # call to this host costs ~0.5s, so folding them matters.
    remote_hash=$(tar cf - --no-xattrs -C "$SITE_DIR" --exclude '.DS_Store' --exclude '._*' --exclude '__MACOSX' . \
        | "$SCRIPT_DIR/remote-run-stream.sh" "$SITE_DEV_SSH" "$SITE_DEV_SSH_PORT" \
            "$SCRIPT_DIR/dev-sync.sh" "$SITE_DEV_DIR" "$SITE_DEV_HOME" | tail -1)
    if [ "$local_hash" != "$remote_hash" ]; then
        die "remote tree differs after sync (local $local_hash remote $remote_hash)"
    fi
    log "remote tree matches local ($remote_hash)"
    scp "${SCP_OPTS[@]}" "$SCRIPT_DIR/serve.py" "$SITE_DEV_SSH:$SITE_DEV_HOME/serve.py" >/dev/null
    log "runtime helper installed at $SITE_DEV_HOME/serve.py"
}

cmd_dev_up() {
    log "starting site service on port $SITE_DEV_PORT on $SITE_DEV_SSH"
    # One round trip performs the whole sequence: ownership check, stop our own
    # instance, launch, wait for the listener, report the pid. The earlier
    # version spent six separate ssh calls on that, ~0.5s each on this link,
    # which made `dev up` look hung while the service was already up.
    local out
    if ! out=$("$SCRIPT_DIR/remote-run.sh" "$SITE_DEV_SSH" "$SITE_DEV_SSH_PORT" \
        "$SCRIPT_DIR/dev-up.sh" "$SITE_DEV_HOME" "$SITE_DEV_DIR" "$SITE_DEV_PORT"); then
        printf '%s\n' "$out" | sed 's/^/    /' >&2
        die "failed to start site service on $SITE_DEV_PORT"
    fi
    printf '%s\n' "$out" | sed 's/^/==> /'
}

cmd_dev_down() {
    # Same one-round-trip rule as dev up: the ownership check, the signal and
    # the wait all belong to a single payload, not four ssh calls.
    local out
    if ! out=$("$SCRIPT_DIR/remote-run.sh" "$SITE_DEV_SSH" "$SITE_DEV_SSH_PORT" \
        "$SCRIPT_DIR/dev-down.sh" "$SITE_DEV_HOME" "$SITE_DEV_PORT"); then
        printf '%s\n' "$out" | sed 's/^/    /' >&2
        die "failed to stop site service"
    fi
    printf '%s\n' "$out" | sed 's/^/==> /'
}

cmd_dev_status() {
    log "site service on $SITE_DEV_SSH"
    "$SCRIPT_DIR/remote-run.sh" "$SITE_DEV_SSH" "$SITE_DEV_SSH_PORT" \
        "$SCRIPT_DIR/dev-status.sh" "$SITE_DEV_HOME" "$SITE_DEV_DIR" "$SITE_DEV_PORT" \
        | sed 's/^/    /'
    local code
    code=$(curl -s -o /dev/null -m 8 -w '%{http_code}' "http://${SITE_DEV_SSH#*@}:$SITE_DEV_PORT/index.html" || true)
    printf '==> http probe: %s\n' "$code"
}

cmd_dev_verify() {
    local host=${SITE_DEV_SSH#*@} base="http://${SITE_DEV_SSH#*@}:$SITE_DEV_PORT"
    log "verifying $base"
    # One request per file: asking for the status and the size separately would
    # download the 28MB package twice, which dominated this command's runtime.
    local metrics code size
    for rel in index.html http.html install.sh dl/SHA256SUMS dl/kc_client_darwin_arm64.tar.gz; do
        metrics=$(curl -s -o /dev/null -m 90 -w '%{http_code} %{size_download}' "$base/$rel" || echo '000 0')
        code=${metrics%% *}
        size=${metrics##* }
        printf '    %-40s %s (%s bytes)\n' "$rel" "$code" "$size"
        [ "$code" = 200 ] || die "$rel returned $code"
        [ "$size" -gt 0 ] || die "$rel returned no body"
    done
    log "reachable from this host; containers in the same region reach ${SITE_DEV_SSH#*@}:$SITE_DEV_PORT"
}

cmd_dev_deploy() {
    cmd_dev_sync
    cmd_dev_up
    cmd_dev_verify
    log "dev deploy complete: http://${SITE_DEV_SSH#*@}:$SITE_DEV_PORT/"
}

usage() {
    sed -n '2,32p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
    exit "${1:-0}"
}

main() {
    [ $# -ge 1 ] || usage 2
    local target=$1; shift
    case "$target" in
        with)
            local action=${1:-check}; shift || true
            case "$action" in
                check) cmd_with_check ;;
                serve) cmd_with_serve "$@" ;;
                verify) cmd_with_verify "$@" ;;
                *) die "unknown with action: $action (check|serve|verify)" ;;
            esac
            ;;
        dev)
            local action=${1:-status}; shift || true
            case "$action" in
                sync) cmd_dev_sync ;;
                up) cmd_dev_up ;;
                down) cmd_dev_down ;;
                status) cmd_dev_status ;;
                verify) cmd_dev_verify ;;
                deploy) cmd_dev_deploy ;;
                *) die "unknown dev action: $action (sync|up|down|status|verify|deploy)" ;;
            esac
            ;;
        -h|--help|help) usage 0 ;;
        *) die "unknown target: $target (with|dev)" ;;
    esac
}

main "$@"
