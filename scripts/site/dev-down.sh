# Remote payload for `deploy.sh dev down`, executed on the dev machine via
# `bash -s -- <home>`. One ssh round trip, same reasoning as dev-up.sh.
#
# Ownership rule: stop only a pidfile process whose cmdline contains serve.py.
# Anything else is reported and left running.
set -u

home=${1:?usage: dev-down.sh <home> <port>}
port=${2:?}
# Port-scoped, matching dev-up.sh: stopping port B must not stop port A.
pidfile="$home/site-$port.pid"

if [ ! -f "$pidfile" ]; then
    echo "no pidfile; nothing started by this script is running"
    exit 0
fi

pid=$(cat "$pidfile" 2>/dev/null || true)
if [ -z "$pid" ]; then
    rm -f "$pidfile"
    echo "empty pidfile removed"
    exit 0
fi

if ! kill -0 "$pid" 2>/dev/null; then
    rm -f "$pidfile"
    echo "process already gone; pidfile removed"
    exit 0
fi

if ! grep -q 'serve.py' "/proc/$pid/cmdline" 2>/dev/null; then
    echo "pid $pid is not the site service; refusing to kill it" >&2
    exit 1
fi

kill -TERM "$pid" 2>/dev/null || true
for _ in 1 2 3 4 5 6 7 8 9 10; do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.2
done
if kill -0 "$pid" 2>/dev/null; then
    echo "pid $pid ignored SIGTERM; leaving it alone" >&2
    exit 1
fi
rm -f "$pidfile"
echo "stopped pid $pid"
