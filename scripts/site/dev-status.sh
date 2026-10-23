# Remote payload for `deploy.sh dev status`, executed on the dev machine via
# `bash -s -- <home> <dir> <port>`. One ssh round trip.
#
# Read-only: reports the pidfile, the listener and the tree size so a caller can
# tell "our service is down" from "someone else owns the port".
set -u

home=${1:?usage: dev-status.sh <home> <dir> <port>}
dir=${2:?}
port=${3:?}
# Port-scoped, matching dev-up.sh.
pidfile="$home/site-$port.pid"

echo '-- pidfile --'
if [ -f "$pidfile" ]; then
    pid=$(cat "$pidfile" 2>/dev/null || true)
    echo "pid=${pid:-<empty>}"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        echo alive
    else
        echo dead
    fi
else
    echo absent
fi

echo '-- listener --'
if ss -tlnp 2>/dev/null | grep -q ":$port "; then
    ss -tlnp 2>/dev/null | grep ":$port "
else
    echo 'port not listening'
fi

echo '-- tree --'
if [ -d "$dir" ]; then
    du -sh "$dir" 2>/dev/null | awk '{print $1}'
    find "$dir" -type f 2>/dev/null | wc -l | sed 's/^ *//; s/^/files=/'
else
    echo 'no tree'
fi
