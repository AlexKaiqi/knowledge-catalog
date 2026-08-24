#!/usr/bin/env bash
# Resolve or build the kc executable used by dsh-loom. `path` prints the
# executable; every other invocation is passed through to kc unchanged.
set -euo pipefail

plugin_dir="$(cd "$(dirname "$0")/.." && pwd)"
source_root="${KC_SOURCE_ROOT:-}"
cache_root="${XDG_CACHE_HOME:-${HOME}/.cache}/dsh-loom"
checkout_root="${cache_root}/knowledge-catalog"
cached_bin="${cache_root}/bin/kc"

resolve_bin() {
  if [[ -n "${KC_BIN:-}" && -x "${KC_BIN}" ]]; then
    printf '%s\n' "${KC_BIN}"
    return
  fi
  if command -v kc >/dev/null 2>&1; then
    command -v kc
    return
  fi
  if [[ -z "$source_root" && -f "${plugin_dir}/../go.mod" && -d "${plugin_dir}/../cmd/kc" ]]; then
    source_root="$(cd "${plugin_dir}/.." && pwd)"
  fi
  if [[ -z "$source_root" ]]; then
    if [[ ! -d "${checkout_root}/.git" ]]; then
      mkdir -p "$(dirname "$checkout_root")"
      git clone --filter=blob:none --branch "${KC_SOURCE_REF:-main}" \
        https://github.com/AlexKaiqi/knowledge-catalog "$checkout_root"
    fi
    source_root="$checkout_root"
  fi
  if [[ ! -f "${source_root}/go.mod" || ! -d "${source_root}/cmd/kc" ]]; then
    printf 'kc source root is invalid: %s\n' "$source_root" >&2
    return 1
  fi
  mkdir -p "$(dirname "$cached_bin")"
  go -C "$source_root" build -o "$cached_bin" ./cmd/kc
  printf '%s\n' "$cached_bin"
}

kc_bin="$(resolve_bin)"
if [[ "${1:-}" == "path" ]]; then
  printf '%s\n' "$kc_bin"
  exit 0
fi
exec "$kc_bin" "$@"
