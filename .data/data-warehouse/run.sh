#!/usr/bin/env bash
set -euo pipefail

fixture_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${fixture_root}/../.." && pwd)"
venv_python="${repo_root}/.venv/bin/python"
run_root="${KC_DW_RUN_ROOT:-${fixture_root}/runs/current}"

[[ -x "${venv_python}" ]] || python3 -m venv "${repo_root}/.venv"
if ! "${venv_python}" -c 'import behave' >/dev/null 2>&1; then
  "${venv_python}" -m pip install --disable-pip-version-check -r "${fixture_root}/requirements.txt"
fi

mkdir -p "${run_root}/junit"
export KC_DW_RUN_ROOT="${run_root}"

tag_expression="not @agent"
if [[ $# -gt 0 ]]; then
  case "${1#@}" in
    DW-CLI-*) tag_expression="@${1#@} and not @agent" ;;
    *) echo "run.sh only accepts DW-CLI-* cases; Agent companions use run-agent.sh" >&2; exit 2 ;;
  esac
fi
args=("${fixture_root}/features" --tags "${tag_expression}" --format progress3 --outfile - --format json.pretty --outfile "${run_root}/behave.json" --junit --junit-directory "${run_root}/junit")

cd "${fixture_root}"
"${venv_python}" -m behave "${args[@]}"
echo "data-warehouse acceptance: PASS"
echo "${run_root}/behave.json"
