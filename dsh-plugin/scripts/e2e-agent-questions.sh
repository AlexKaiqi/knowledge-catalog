#!/usr/bin/env bash
# Real-model semantic acceptance: can an Agent answer ordinary user questions
# correctly from the bundled Skill without filesystem/source inspection?
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/../.." && pwd)"
plugin_dir="${root_dir}/dsh-plugin"
source "${plugin_dir}/scripts/agent-env.sh"
load_agent_api_env
require_agent_api_key_for_patch "${DSH_MODEL_PATCH:-$plugin_dir/scripts/deepseek-official.patch.yml}"
profile_name="${DSH_PROFILE:-loom-agent-questions}"
dsh_executable="${DSH_EXECUTABLE:-$(command -v dsh || true)}"
if [[ -z "$dsh_executable" || ! -x "$dsh_executable" ]]; then
  echo "dsh executable not found; put dsh on PATH or set DSH_EXECUTABLE=/absolute/path/to/dsh" >&2
  exit 1
fi

prepare_agent_profile "$plugin_dir" "$dsh_executable" "$profile_name"
export DSH_EXECUTABLE="$dsh_executable"
export DSH_PROFILE="$profile_name"
export KC_QUESTION_ARTIFACTS="${KC_QUESTION_ARTIFACTS:-$(mktemp -d /tmp/kc-agent-question-evidence.XXXXXX)}"
python3 "$plugin_dir/scripts/e2e_agent_questions.py"
printf 'evidence: %s\n' "$KC_QUESTION_ARTIFACTS"
