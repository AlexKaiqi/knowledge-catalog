#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# Both owners participate in every typed HTTP surface guard.
HTTP_ROUTE_PATHS=(server/service*_routes.go)
HTTP_MODULE_ROUTE_PATHS=()
for route_source in snapshot/service/*.go; do
  if [[ "$route_source" != *_test.go ]]; then
    HTTP_ROUTE_PATHS+=("$route_source")
    HTTP_MODULE_ROUTE_PATHS+=("$route_source")
  fi
done

fail=0
check_absent() {
  local label="$1"
	local pattern="$2"
	shift 2
	local status=0
	rg -n --hidden --glob '!.git/**' --glob '!node_modules/**' -e "$pattern" "$@" || status=$?
	if (( status != 1 )); then
    echo "surface check failed: ${label} (rg exit ${status})" >&2
    fail=1
  fi
}

if [[ -e dsh-plugin ]]; then
  echo "surface check failed: retired DSH plugin directory" >&2
  fail=1
fi
check_absent "retired DSH plugin dependencies" 'dsh-plugin|KC_DSH_|DSH_EXECUTABLE|dsh_contract|test-plugin|test-agent-(e2e|metric-e2e|ux-e2e)' \
  Makefile .github .dockerignore scripts cli client snapshot/cli snapshot/client server cmd \
  --glob '!check-surface.sh' --glob '!maintenance_test.py' --glob '!**/*REFACTOR.md'
check_absent "retired private task context" 'inheritTaskContext|mountedTaskContext|selectMountedContext|_task-context' \
  cli snapshot/cli --glob '!**/*_test.go'
check_absent "retired model tools" 'knowledge_(list|context|search|read|schema|relations|provenance)' \
  cli client snapshot/cli snapshot/client server --glob '!**/*_test.go' --glob '!**/*REFACTOR.md'
check_absent "model tool registration" 'registerTool\(' \
  cli client snapshot/cli snapshot/client server --glob '!**/*_test.go' --glob '!**/*REFACTOR.md'
check_absent "Agent or CLI VFS verbs" 'vfs-(read|list|write)' \
	cli snapshot/cli --glob '!**/*_test.go' --glob '!allow.go'
check_absent "legacy verb HTTP implementation" 'Handle(Func)?\("(GET |POST )?/v1/|doJSON\([^\n]*"/v1/|/v1/<verb>|POST /v1/' \
	cli client snapshot/cli snapshot/client server snapshot/service --glob '!**/*_test.go' --glob '!**/*REFACTOR.md'
check_absent "untyped client invocation" 'client\.Invoke\(|(^|[^[:alnum:]_])Invoke\(' client snapshot/client "${HTTP_ROUTE_PATHS[@]}"
check_absent "public Knowledge list operation" '"list"[[:space:]]*:[[:space:]]*\{stage:|func[[:space:]]+verbList|type[[:space:]]+ListPage' \
	cli snapshot/cli object --glob '!object/maintenance/**' --glob '!**/*_test.go'
DOC_CLI_PATHS=(README.md doc cli/README.md snapshot/README.md snapshot/dataset/README.md snapshot/access/README.md snapshot/service/README.md snapshot/console/README.md snapshot/cli/README.md snapshot/client/README.md liveaccess/README.md)
check_absent "stale flat CLI examples" $'kc (init|catalog-add|repo-add|store-set|store-ls|status|allow|revoke|allowed|define-workspace|retire-workspace|archive-catalog|archive-repo|put|remove|commit|ingest|receipt|propose|preview|validate|record-validation|merge|hook-|gate-|access-log|trace|hitmap|record-feedback|describe-index|index-sync|describe-access|describe-schema|inspect|checkout|resolve-binding)([[:space:]`]|$)' \
  "${DOC_CLI_PATHS[@]}"
check_absent "retired knowledge CLI prefix" $'kc knowledge[[:space:]]' \
  "${DOC_CLI_PATHS[@]}"
check_absent "retired Catalog CLI examples" '(^|[^[:alnum:]_./-])(kc|go[[:space:]]+run[[:space:]]+\./cmd/kc[[:space:]]+--)[[:space:]]+catalog([[:space:]`]|$)' \
  "${DOC_CLI_PATHS[@]}"

if rg -n '(commands|cliSurface|ParseArgs|dispatch|Run\()' "${HTTP_ROUTE_PATHS[@]}"; then
  echo "surface check failed: typed HTTP routes depend on CLI registries/parser" >&2
  fail=1
fi

for namespace in snapshot knowledge writer governance identity admin operations; do
  if ! rg -q "/${namespace}/v1" "${HTTP_ROUTE_PATHS[@]}"; then
    echo "surface check failed: missing /${namespace}/v1 route namespace" >&2
    fail=1
  fi
done

if ! rg -q '/snapshot/v1/files/' "${HTTP_MODULE_ROUTE_PATHS[@]}"; then
  echo "surface check failed: missing /snapshot/v1/files route namespace in Snapshot owner" >&2
  fail=1
fi

exit "$fail"
