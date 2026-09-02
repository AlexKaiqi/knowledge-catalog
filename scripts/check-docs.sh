#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
graph="$repo_root/docs/DOCUMENT_GRAPH.yaml"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

awk 'BEGIN { separators=0 } /^---[[:space:]]*$/ { separators++; next } separators >= 2 { print }' \
  "$graph" >"$tmp_dir/graph.json"

jq -e '
  .entityType == "DocumentGraph" and
  (.version | type == "number") and
  (.documents | length > 0) and
  ([.documents[].id] | length) == ([.documents[].id] | unique | length) and
  ([.documents[].path] | length) == ([.documents[].path] | unique | length) and
  ([.documents[].ownerTopics[]] | length) == ([.documents[].ownerTopics[]] | unique | length) and
  ([.documents[].id] as $ids |
    all(.relations[]; . as $relation |
      ($ids | index($relation.from)) != null and ($ids | index($relation.to)) != null)) and
  all(.documents[];
    (.class | IN("entrypoint", "foundation", "decision", "runtime", "evolution", "validation", "guide")) and
    (.lifecycle | IN("normative", "active", "draft", "planned")) and
    (.ownerTopics | length > 0)) and
  all(.relations[];
    .type | IN("depends_on", "refines", "verifies", "operationalizes", "measures", "catalogs"))
' "$tmp_dir/graph.json" >/dev/null

{
  printf '%s\n' README.md
  find "$repo_root/docs" -maxdepth 1 -type f -name '*.md' -print \
    | sed "s#^$repo_root/##"
} | sort >"$tmp_dir/actual-paths"
jq -r '.documents[].path' "$tmp_dir/graph.json" | sort >"$tmp_dir/catalog-paths"
if ! diff -u "$tmp_dir/catalog-paths" "$tmp_dir/actual-paths"; then
  echo "top-level Markdown inventory differs from docs/DOCUMENT_GRAPH.yaml" >&2
  exit 1
fi

while IFS= read -r path; do
  [[ -f "$repo_root/$path" ]] || { echo "document does not exist: $path" >&2; exit 1; }
done <"$tmp_dir/catalog-paths"

broken_links=0
while IFS=: read -r source raw; do
  target="${raw#']('}"
  case "$target" in
    http://*|https://*|mailto:*|'') continue ;;
  esac
  target="${target%%#*}"
  if [[ "$target" = /* ]]; then
    resolved="$target"
  else
    resolved="$(dirname "$repo_root/$source")/$target"
  fi
  if [[ ! -e "$resolved" ]]; then
    printf 'broken local Markdown link: %s -> %s\n' "$source" "$target" >&2
    broken_links=1
  fi
done < <(cd "$repo_root" && rg -N -o '\]\([^\)#]+' README.md docs/*.md)
(( broken_links == 0 ))

jq -r '.relations[] | select(.type == "depends_on") | "\(.from) \(.to)"' \
  "$tmp_dir/graph.json" | tsort >/dev/null

printf 'documentation graph: PASS (%s documents, %s relations, unique topics, valid links, acyclic dependencies)\n' \
  "$(jq '.documents | length' "$tmp_dir/graph.json")" \
  "$(jq '.relations | length' "$tmp_dir/graph.json")"
