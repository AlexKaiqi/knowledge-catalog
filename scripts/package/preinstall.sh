#!/bin/sh
set -eu

fail() { printf 'kc installer: %s\n' "$*" >&2; exit 1; }

[ "$(uname -s)" = Darwin ] || fail 'this package requires macOS'
case "$(uname -m)" in
    arm64|aarch64) actual_arch=arm64 ;;
    x86_64|amd64) actual_arch=amd64 ;;
    *) fail 'unsupported CPU architecture' ;;
esac
# Installer may run under Rosetta; detect the physical Apple Silicon host.
if [ "$(/usr/sbin/sysctl -n hw.optional.arm64 2>/dev/null || true)" = 1 ]; then
    actual_arch=arm64
fi
[ "$actual_arch" = '@KC_ARCH@' ] || fail 'select the @KC_ARCH@ package only for a matching CPU'
[ "${3:-/}" = / ] || fail 'install on the current system volume'
