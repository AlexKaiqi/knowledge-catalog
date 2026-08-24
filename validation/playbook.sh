#!/usr/bin/env bash
set -euo pipefail

validation_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
scene_root="$(cd "${validation_dir}/.." && pwd)"

cd "${scene_root}"
if [[ "$#" == "0" ]]; then
  set -- all
fi
exec go run ./validation/cmd/scenario-runner "$@"
