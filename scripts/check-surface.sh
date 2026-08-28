#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

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
check_absent "legacy verb HTTP implementation" 'HandleFunc\("(GET |POST )?/v1/|doJSON\([^\n]*"/v1/|/v1/<verb>|POST /v1/' \
	cli client dsh-plugin/src dsh-plugin/README.md --glob '!**/*_test.go'
check_absent "untyped client invocation" 'client\.Invoke\(|(^|[^[:alnum:]_])Invoke\(' client cli/service_routes.go dsh-plugin/src
check_absent "model tool registration" 'registerTool\(' dsh-plugin/src
check_absent "public Knowledge list operation" '"list"[[:space:]]*:[[:space:]]*\{stage:|func[[:space:]]+verbList|type[[:space:]]+ListPage' \
	cli knowledge --glob '!knowledge/maintenance/**' --glob '!**/*_test.go'
check_absent "stale flat CLI examples" 'kc (init|catalog-add|repo-add|store-set|store-ls|status|whoami|allow|revoke|allowed|define-workspace|register|retire-workspace|archive-catalog|archive-repo|put|remove|commit|ingest|receipt|propose|preview|validate|record-validation|merge|hook-|gate-|access-log|trace|hitmap|record-feedback|describe-index|index-sync|describe-access|search|read|relations|provenance|describe-schema|log|diff|inspect|checkout|resolve-binding|resolve)([[:space:]`]|$)' \
  README.md docs cli/README.md catalog/README.md knowledge/serving/README.md dsh-plugin/README.md dsh-plugin/skills/knowledge-catalog .data/data-warehouse/README.md .data/data-warehouse/connector/README.md .data/data-warehouse/features

if rg -n '(commands|cliSurface|ParseArgs|dispatch|Run\()' cli/service*_routes.go; then
  echo "surface check failed: typed HTTP routes depend on CLI registries/parser" >&2
  fail=1
fi

for namespace in catalog knowledge workspace-files writer governance identity admin operations; do
  if ! rg -q "/${namespace}/v1" cli/service*_routes.go; then
    echo "surface check failed: missing /${namespace}/v1 route namespace" >&2
    fail=1
  fi
done

exit "$fail"
