#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
go_bin="${GO:-go}"
max_cyclo="${KC_MAX_CYCLO:-50}"
max_file_lines="${KC_MAX_GO_FILE_LINES:-700}"
duplicate_tokens="${KC_MAX_DUPLICATE_TOKENS:-150}"
cd "$repo_root"

go_files=()
production_files=()
while IFS= read -r file; do
	go_files+=("$file")
	case "$file" in
		*_test.go|internal/testkit/*) ;;
		*) production_files+=("$file") ;;
	esac
done < <(rg --files -g '*.go' | sort)

formatted="$(gofmt -l "${go_files[@]}")"
if [[ -n "$formatted" ]]; then
	printf 'gofmt required:\n%s\n' "$formatted" >&2
	exit 1
fi

"$go_bin" mod tidy -diff
"$go_bin" vet ./...
"$go_bin" run honnef.co/go/tools/cmd/staticcheck@v0.6.1 ./...

if ! cyclo="$($go_bin run github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0 -over "$max_cyclo" "${production_files[@]}" 2>&1)"; then
	printf 'production function exceeds cyclomatic complexity %s:\n%s\n' "$max_cyclo" "$cyclo" >&2
	exit 1
fi

oversized=0
for file in "${production_files[@]}"; do
	lines="$(wc -l < "$file")"
	if (( lines > max_file_lines )); then
		printf '%s:%s exceeds %s lines\n' "$file" "$lines" "$max_file_lines" >&2
		oversized=1
	fi
done
if (( oversized != 0 )); then
	exit 1
fi

duplicates="$($go_bin run github.com/mibk/dupl@v1.0.0 -threshold "$duplicate_tokens" "${production_files[@]}")"
if ! grep -q 'Found total 0 clone groups.' <<<"$duplicates"; then
	printf 'production clone exceeds %s tokens:\n%s\n' "$duplicate_tokens" "$duplicates" >&2
	exit 1
fi

printf 'code quality passed (cyclo<=%s, file<=%s lines, duplicate<%s tokens)\n' \
	"$max_cyclo" "$max_file_lines" "$duplicate_tokens"
