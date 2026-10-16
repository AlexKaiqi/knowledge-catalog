#!/bin/sh
# Package target placeholders are rendered by build.py.
set -eu

fail() {
    printf 'kc install: %s\n' "$*" >&2
    exit 1
}

case "$#:$*" in
    0:) ;;
    1:--help|1:-h)
        printf 'Usage: [PREFIX=/absolute/path] sh install.sh\nDefault PREFIX: /usr/local\n'
        exit 0
        ;;
    *) fail 'unexpected arguments; use PREFIX=/absolute/path sh install.sh' ;;
esac

prefix=${PREFIX-/usr/local}
case "$prefix" in
    /*) ;;
    *) fail 'PREFIX must be a nonempty absolute path' ;;
esac

target_os='@KC_OS@'
target_arch='@KC_ARCH@'
case "$(uname -s)" in
    Linux) host_os=linux ;;
    Darwin) host_os=darwin ;;
    *) fail 'unsupported operating system' ;;
esac
case "$(uname -m)" in
    x86_64|amd64) host_arch=amd64 ;;
    aarch64|arm64) host_arch=arm64 ;;
    *) fail 'unsupported CPU architecture' ;;
esac
[ "$host_os/$host_arch" = "$target_os/$target_arch" ] ||
    fail "package targets $target_os/$target_arch; this machine is $host_os/$host_arch"

payload=$(CDPATH= cd -P "$(dirname "$0")" && pwd) || fail 'cannot locate package contents'
for name in kc build-info.json uninstall.sh; do
    [ -f "$payload/$name" ] && [ ! -L "$payload/$name" ] || fail "missing regular package file: $name"
done

bin_dir=$prefix/bin
share_dir=$prefix/share/kc
check_target() {
    [ ! -L "$1" ] || fail "refusing symbolic link: $1"
    [ ! -e "$1" ] || [ -f "$1" ] || fail "refusing non-regular destination: $1"
}
check_targets() {
    check_target "$bin_dir/kc"
    check_target "$share_dir/build-info.json"
    check_target "$share_dir/uninstall.sh"
}
check_targets
mkdir -p "$bin_dir" "$share_dir" || fail "cannot create installation directories under $prefix; check ownership and permissions"

binary_tmp=
metadata_tmp=
uninstall_tmp=
cleanup() {
    [ -z "$binary_tmp" ] || rm -f "$binary_tmp"
    [ -z "$metadata_tmp" ] || rm -f "$metadata_tmp"
    [ -z "$uninstall_tmp" ] || rm -f "$uninstall_tmp"
    :
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

binary_tmp=$(mktemp "$bin_dir/.kc.XXXXXX") || fail "cannot stage kc in $bin_dir; check ownership and permissions"
metadata_tmp=$(mktemp "$share_dir/.build-info.XXXXXX") || fail "cannot stage metadata in $share_dir; check ownership and permissions"
uninstall_tmp=$(mktemp "$share_dir/.uninstall.XXXXXX") || fail "cannot stage uninstaller in $share_dir; check ownership and permissions"
cp "$payload/kc" "$binary_tmp" && chmod 0755 "$binary_tmp" || fail 'cannot stage kc; check available space and permissions'
cp "$payload/build-info.json" "$metadata_tmp" && chmod 0644 "$metadata_tmp" || fail 'cannot stage metadata; check available space and permissions'
cp "$payload/uninstall.sh" "$uninstall_tmp" && chmod 0755 "$uninstall_tmp" || fail 'cannot stage uninstaller; check available space and permissions'
check_targets
mv -f "$binary_tmp" "$bin_dir/kc" || fail "cannot install kc in $bin_dir; check ownership and permissions"
binary_tmp=
mv -f "$metadata_tmp" "$share_dir/build-info.json" || fail "cannot install metadata in $share_dir; check ownership and permissions"
metadata_tmp=
mv -f "$uninstall_tmp" "$share_dir/uninstall.sh" || fail "cannot install uninstaller in $share_dir; check ownership and permissions"
uninstall_tmp=
printf 'Installed kc at %s\nUninstaller: %s/uninstall.sh (use the same PREFIX)\n' "$bin_dir/kc" "$share_dir"
