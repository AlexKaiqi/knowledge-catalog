#!/usr/bin/env bash
set -euo pipefail

fixture_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${fixture_root}/../.." && pwd)"
venv_python="${repo_root}/.venv/bin/python"
run_root="${KC_DW_RUN_ROOT:-${fixture_root}/runs/current-agent}"
cli_run_root="${run_root}/cli"
agent_run_root="${run_root}/agent"
profile_name="${DSH_PROFILE:-loom-data-warehouse}"

[[ -x "${venv_python}" ]] || python3 -m venv "${repo_root}/.venv"
if ! "${venv_python}" -c 'import behave' >/dev/null 2>&1; then
  "${venv_python}" -m pip install --disable-pip-version-check -r "${fixture_root}/requirements.txt"
fi

# Agent acceptance is supplemental. Build and execute the complete normative
# CLI suite first; a failure stops here before credentials or model calls.
KC_DW_RUN_ROOT="${cli_run_root}" "${fixture_root}/run.sh"

source "${repo_root}/dsh-plugin/scripts/agent-env.sh"
load_agent_api_env
model_patch="$(select_agent_model_patch "${repo_root}/dsh-plugin")"
require_agent_api_key_for_patch "${model_patch}"
prepare_agent_profile "${repo_root}/dsh-plugin" "${DSH_EXECUTABLE:-dsh}" "${profile_name}"

mkdir -p "${agent_run_root}/junit"
export KC_DW_RUN_ROOT="${agent_run_root}"
export KC_BIN="${cli_run_root}/bin/kc"
export KC_CONNECTOR_PREVIEW_BIN="${cli_run_root}/bin/connector-preview"
export KC_DW_AGENT_HOME="${cli_run_root}/scenarios/DW-CLI-03/kc-home"
export KC_DW_CHECKPOINT="${cli_run_root}/scenarios/DW-CLI-03/mysql.observation.json"
export DSH_PROFILE="${profile_name}"
export DSH_MODEL_PATCH="${model_patch}"

cd "${fixture_root}"
"${venv_python}" -m behave "${fixture_root}/features/agent.feature" \
  --tags @agent --format progress3 --outfile - --format json.pretty \
  --outfile "${agent_run_root}/behave.json" --junit --junit-directory "${agent_run_root}/junit"
echo "data-warehouse Agent acceptance: PASS"
echo "CLI evidence: ${cli_run_root}/behave.json"
echo "Agent evidence: ${agent_run_root}/behave.json"
