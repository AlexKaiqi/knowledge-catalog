#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# HTTP ownership is split between the KC application and Snapshot Store module.
# Include each owner's production implementation in every HTTP surface guard.
HTTP_ROUTE_PATHS=(server/service*_routes.go)
for route_source in snapshot/service/*.go; do
  if [[ "$route_source" != *_test.go ]]; then
    HTTP_ROUTE_PATHS+=("$route_source")
  fi
done

fail=0
check_absent() {
  local label="$1"
	local pattern="$2"
	shift 2
	if rg -n --hidden --glob '!.git/**' --glob '!node_modules/**' -e "$pattern" "$@"; then
    echo "surface check failed: ${label}" >&2
    fail=1
  fi
}

check_absent "retired model tools" 'knowledge_(list|context|search|read|schema|relations|provenance)' \
  dsh-plugin/src dsh-plugin/README.md dsh-plugin/skills/knowledge-catalog
check_absent "Agent or CLI VFS verbs" 'vfs-(read|list|write)' \
	cli dsh-plugin/src dsh-plugin/README.md --glob '!**/*_test.go' --glob '!allow.go'
check_absent "legacy verb HTTP implementation" 'Handle(Func)?\("(GET |POST )?/v1/|doJSON\([^\n]*"/v1/|/v1/<verb>|POST /v1/' \
	cli client dsh-plugin/src dsh-plugin/README.md "${HTTP_ROUTE_PATHS[@]}" --glob '!**/*_test.go' --glob '!**/*REFACTOR.md'
check_absent "untyped client invocation" 'client\.Invoke\(|(^|[^[:alnum:]_])Invoke\(' client "${HTTP_ROUTE_PATHS[@]}" dsh-plugin/src
check_absent "model tool registration" 'registerTool\(' dsh-plugin/src
check_absent "public Knowledge list operation" '"list"[[:space:]]*:[[:space:]]*\{stage:|func[[:space:]]+verbList|type[[:space:]]+ListPage' \
	cli knowledge --glob '!knowledge/maintenance/**' --glob '!**/*_test.go'
DOC_CLI_PATHS=(README.md docs cli/README.md catalog/README.md snapshot/README.md snapshot/dataset/README.md snapshot/access/README.md snapshot/service/README.md snapshot/console/README.md knowledge/serving/README.md dsh-plugin/README.md dsh-plugin/skills/knowledge-catalog)
check_absent "stale flat CLI examples" $'kc (init|catalog-add|repo-add|store-set|store-ls|status|allow|revoke|allowed|define-workspace|retire-workspace|archive-catalog|archive-repo|put|remove|commit|ingest|receipt|propose|preview|validate|record-validation|merge|hook-|gate-|access-log|trace|hitmap|record-feedback|describe-index|index-sync|describe-access|describe-schema|inspect|checkout|resolve-binding)([[:space:]`]|$)' \
  "${DOC_CLI_PATHS[@]}"
check_absent "retired knowledge CLI prefix" $'kc knowledge[[:space:]]' \
  "${DOC_CLI_PATHS[@]}"

if rg -n '(commands|cliSurface|ParseArgs|dispatch|Run\()' "${HTTP_ROUTE_PATHS[@]}"; then
  echo "surface check failed: typed HTTP routes depend on CLI registries/parser" >&2
  fail=1
fi

for namespace in catalog knowledge dataset-files writer governance identity admin operations; do
  if ! rg -q "/${namespace}/v1" "${HTTP_ROUTE_PATHS[@]}"; then
    echo "surface check failed: missing /${namespace}/v1 route namespace" >&2
    fail=1
  fi
done

exit "$fail"
