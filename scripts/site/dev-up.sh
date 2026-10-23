# Remote payload for `deploy.sh dev up`, executed on the dev machine via
# `bash -s -- <home> <dir> <port>`. Kept as a file rather than an inline
# heredoc because bash 3.2 (the macOS default) misparses heredocs nested in
# command substitution.
#
# Contract: the whole sequence runs in ONE ssh round trip. Every extra ssh
# call costs ~0.5s on this link, which made `dev up` look hung.
#
# Ownership rule: only a pidfile this script wrote, pointing at a process whose
# cmdline contains serve.py, may be stopped. A foreign listener on the port is
# reported and left alone.
set -u

home=${1:?usage: dev-up.sh <home> <dir> <port>}
dir=${2:?}
port=${3:?}
# Port-scoped state: a single site.pid would make two instances on different
# ports look like one, so `up` on port B would stop the instance on port A.
pidfile="$home/site-$port.pid"
logfile="$home/site-$port.log"

state=FREE
pid=
if [ -f "$pidfile" ]; then
    pid=$(cat "$pidfile" 2>/dev/null || true)
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        if grep -q 'serve.py' "/proc/$pid/cmdline" 2>/dev/null; then
            state=OURS
        else
            state=FOREIGNSAME
        fi
    else
        state=STALEPID
    fi
elif ss -tln 2>/dev/null | grep -q ":$port "; then
    state=FOREIGNPORT
fi

case $state in
    FOREIGNSAME)
        echo "pidfile points at a non-site process (pid $pid); refusing to touch it" >&2
        exit 1
        ;;
    FOREIGNPORT)
        echo "port $port is held by a process this script did not start; nothing was killed. Re-run with SITE_DEV_PORT=<other>" >&2
        exit 1
        ;;
    OURS)
        kill -TERM "$pid" 2>/dev/null || true
        for _ in 1 2 3 4 5 6 7 8 9 10; do
            kill -0 "$pid" 2>/dev/null || break
            sleep 0.2
        done
        rm -f "$pidfile"
        echo "stopped pid $pid"
        ;;
    STALEPID)
        rm -f "$pidfile"
        echo "removed stale pidfile"
        ;;
esac

cd "$home" || { echo "cannot enter $home" >&2; exit 1; }

# setsid detaches the service from this session so a disconnect cannot take it
# down; redirected descriptors keep it from holding the ssh channel open.
setsid nohup python3 serve.py --dir "$dir" --port "$port" \
    --bind 0.0.0.0 --pidfile "$pidfile" \
    < /dev/null > "$logfile" 2>&1 &

for _ in $(seq 100); do
    ss -tln 2>/dev/null | grep -q ":$port " && break
    sleep 0.1
done
if ! ss -tln 2>/dev/null | grep -q ":$port "; then
    echo "site service did not start listening on $port" >&2
    tail -20 "$logfile" >&2 || true
    exit 1
fi
echo "started pid $(cat "$pidfile" 2>/dev/null)"
