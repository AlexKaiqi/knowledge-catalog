#!/usr/bin/env bash
# Run a local payload script on a remote host in a single ssh round trip.
#
#   remote-run.sh <user@host> <port> <payload-path> [args...]
#
# The payload is fed to `bash -s -- <args...>` on stdin. Two reasons it is a
# separate file instead of an inline heredoc in deploy.sh:
#   1. bash 3.2 (macOS default) misparses a heredoc nested inside command
#      substitution, which silently breaks multi-branch payloads.
#   2. The payload can be shell-checked and run standalone.
#
# Every ssh call to a DevCloud development machine costs ~0.5s of handshake.
# Anything that needs several remote steps should be one payload, not several
# calls, or the command looks hung while the work is already done.
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

exec ssh \
    -T \
    -o BatchMode=yes \
    -o ConnectTimeout=10 \
    -o LogLevel=ERROR \
    -o StrictHostKeyChecking=accept-new \
    -p "$port" \
    "$host" \
    "bash -s --$(printf ' %q' "$@")" \
    < "$payload"
