#!/bin/sh
set -eu

fail() {
    printf 'kc uninstall: %s\n' "$*" >&2
    exit 1
}

case "$#:$*" in
    0:) ;;
    1:--help|1:-h)
        printf 'Usage: [PREFIX=/absolute/path] sh uninstall.sh\nDefault PREFIX: /usr/local\n'
        exit 0
        ;;
    *) fail 'unexpected arguments; use PREFIX=/absolute/path sh uninstall.sh' ;;
esac

prefix=${PREFIX-/usr/local}
case "$prefix" in
    /*) ;;
    *) fail 'PREFIX must be a nonempty absolute path' ;;
esac

for path in "$prefix/bin/kc" "$prefix/share/kc/build-info.json" "$prefix/share/kc/uninstall.sh"; do
    [ ! -L "$path" ] || fail "refusing symbolic link: $path"
    [ ! -e "$path" ] || [ -f "$path" ] || fail "refusing non-regular destination: $path"
done
for path in "$prefix/bin/kc" "$prefix/share/kc/build-info.json" "$prefix/share/kc/uninstall.sh"; do
    rm -f "$path" || fail "cannot remove $path; check ownership and permissions"
done
printf 'Removed kc from %s\n' "$prefix"
