#!/usr/bin/env bash
set -euo pipefail

fixture_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${fixture_root}/../.." && pwd)"
venv_python="${repo_root}/.venv/bin/python"
check_tmp="$(mktemp -d)"
trap 'rm -r "${check_tmp}"' EXIT
export PYTHONDONTWRITEBYTECODE=1

fail() {
  echo "data-warehouse acceptance: $*" >&2
  exit 1
}

[[ ! -e "${fixture_root}/.git" ]] || fail "must not be a nested git repository"
[[ ! -e "${fixture_root}/suite.json" && ! -e "${fixture_root}/run.py" && ! -d "${fixture_root}/cases" ]] \
  || fail "the retired custom JSON case runner must not return"
[[ -f "${fixture_root}/connector/connector.yaml" ]] || fail "Connector manifest is missing"
[[ -f "${fixture_root}/connector/collector.py" ]] || fail "Collector is missing"
[[ -f "${fixture_root}/connector/access.py" ]] || fail "Resource access provider is missing"
[[ -f "${fixture_root}/knowledge/physical/resources/mysql-tpch-sql.yaml" ]] || fail "SQL ResourceDescriptor is missing"
[[ $(find "${fixture_root}/knowledge/schemas/physical" -type f -name '*.aspect.yaml' | wc -l | tr -d ' ') == 9 ]] \
  || fail "expected 9 physical MVP Aspect YAML knowledge files"
[[ $(find "${fixture_root}/knowledge/schemas/semantic" -type f -name '*.aspect.yaml' | wc -l | tr -d ' ') == 7 ]] \
  || fail "expected 7 semantic MVP Aspect YAML knowledge files"
[[ $(find "${fixture_root}/knowledge/semantic" -type f -name '*.yaml' ! -name '*.aspect.yaml' | wc -l | tr -d ' ') == 8 ]] \
  || fail "expected 8 semantic MVP instance YAML knowledge files"
[[ ! -e "${fixture_root}/knowledge/model.json" && ! -e "${fixture_root}/knowledge/semantic.json" ]] \
  || fail "private model/semantic translation inputs must not return"
[[ -x "${venv_python}" ]] || python3 -m venv "${repo_root}/.venv"
if ! "${venv_python}" -c 'import behave' >/dev/null 2>&1; then
  "${venv_python}" -m pip install --disable-pip-version-check -r "${fixture_root}/requirements.txt"
fi

"${venv_python}" -m unittest discover -s "${fixture_root}/connector" -p 'test_*.py'
"${venv_python}" -m unittest discover -s "${fixture_root}/docker" -p 'test_*.py'
"${venv_python}" "${fixture_root}/features/verify_spec.py"
"${venv_python}" -c 'import pathlib,sys; [compile(path.read_text(encoding="utf-8"), str(path), "exec") for root in sys.argv[1:] for path in pathlib.Path(root).rglob("*.py")]' \
  "${fixture_root}/connector" "${fixture_root}/features"
(
  cd "${fixture_root}"
  "${venv_python}" -m behave features --dry-run --format null
  "${venv_python}" -m behave features/agent.feature --dry-run --tags @agent --format null
)
(
  cd "${repo_root}"
  ./scripts/check-surface.sh
  go build -o "${check_tmp}/connector-preview" ./\.data/data-warehouse/connector/preview
  go test ./internal/repofile ./knowledge/writer
  go run ./cmd/kc -- help >/dev/null
)

echo "data-warehouse acceptance: CHECK PASS"
