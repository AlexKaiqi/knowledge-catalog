# Shared binding preflight for the explicit kcfs acceptance entry points.
kcfs_require_lakefs_binding() {
  if [[ -z "${KC_LAKEFS_URL:-}" || -z "${KC_LAKEFS_CREDENTIAL:-}" ]]; then
    echo "FAIL: kcfs acceptance requires KC_LAKEFS_URL and KC_LAKEFS_CREDENTIAL." >&2
    echo 'Prepare the stack with scripts/system-lakefs.sh up, then source "${KC_HOME:-/tmp/kc-system-lakefs}/system-lakefs.env".' >&2
    return 1
  fi
}
