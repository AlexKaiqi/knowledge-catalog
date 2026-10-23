#!/usr/bin/env bash
# Run a local payload on a remote host while streaming stdin through as data.
#
#   remote-run-stream.sh <user@host> <port> <payload-path> [args...]  < data
#
# Unlike remote-run.sh, stdin carries payload DATA (e.g. a tar stream). The
# script therefore cannot be fed on stdin: `bash -s` and a `tar xf -` inside the
# script would both read the same descriptor and corrupt the archive. The script
# is staged to a temp file first, so the payload's stdin stays exactly the data
# stream this command received.
set -euo pipefail

if [ $# -lt 3 ]; then
    printf 'usage: %s <user@host> <port> <payload> [args...]\n' "${0##*/}" >&2
    exit 2
fi

host=$1
port=$2
payload=$3
shift 3

[ -f "$payload" ] || { printf 'error: payload not found: %s\n' "$payload" >&2; exit 2; }

encoded=$(base64 < "$payload" | tr -d '\n')

exec ssh \
    -T \
    -o BatchMode=yes \
    -o ConnectTimeout=10 \
    -o LogLevel=ERROR \
    -o StrictHostKeyChecking=accept-new \
    -p "$port" \
    "$host" \
    "tmp=\$(mktemp /tmp/kc-payload.XXXXXX) || exit 1
trap 'rm -f \"\$tmp\"' EXIT
printf %s $encoded | base64 -d > \"\$tmp\" || exit 1
bash \"\$tmp\"$(printf ' %q' "$@")"
