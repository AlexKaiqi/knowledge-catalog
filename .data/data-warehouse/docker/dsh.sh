#!/usr/bin/env bash
set -euo pipefail

export AGENT_ENV_FILE=/run/user.env
export DSH_HOME=/run/dsh-home
export DSH_AGENT_EPHEMERAL_HOME="$DSH_HOME"
source /opt/dsh-loom/scripts/agent-env.sh
load_agent_api_env
require_agent_credential OPENAI_API_KEY
[[ -n "${OPENAI_BASE_URL:-}" ]] || { echo "OPENAI_BASE_URL is required" >&2; exit 1; }

profile=kc-compose-web
profile_dir="$DSH_HOME/profiles/$profile"
config="$DSH_HOME/composed.yml"
ready=/run/dsh-web-ready

# The verification Client never reuses a profile, pnpm store, Session, or
# settings document from a previous container. Registry or build failures are
# surfaced instead of silently falling back to stale generated state.
rm -rf \
  "$DSH_HOME" \
  /run-state/dsh-home \
  /run-state/.pnpm-store \
  /run/.pnpm-store \
  "$ready"
mkdir -p "$DSH_HOME" /run-state/kc-client /workspace
chmod 0700 "$DSH_HOME" /run-state/kc-client
cat /proc/sys/kernel/random/uuid >"$DSH_HOME/profile-build-id"

[[ "$(dsh --version)" == "$DSH_VERSION" ]] || {
  echo "unexpected DSH version: $(dsh --version)" >&2
  exit 1
}
mkdir -p "$profile_dir"
cp -a /opt/dsh-profile-seed/profiles/$profile/. "$profile_dir/"
pnpm --dir "$profile_dir" install --offline --frozen-lockfile --ignore-scripts
python3 /usr/local/lib/kc/dsh_profile.py prepare \
  --profile-dir "$profile_dir" \
  --source /opt/dsh-loom \
  --multi-version "$DSH_MULTI_MODEL_VERSION" \
  --lock-sha256 "$DSH_PROFILE_LOCK_SHA256"
dsh --profile "$profile" --patch /opt/dsh-config/gpt-web.patch.yml --dump-config >"$config"
python3 /usr/local/lib/kc/dsh_profile.py verify-config --config "$config"

socat TCP-LISTEN:7400,fork,reuseaddr TCP:127.0.0.1:7401 &

# HTTP readiness alone cannot see browser plugin activation. Render the real
# Web app once with Chromium and publish readiness only after every client
# entry has activated and the conversation shell is visible.
(
  for _attempt in $(seq 1 30); do
    if curl -fsS http://127.0.0.1:7401 >/dev/null 2>&1 \
      && /usr/local/bin/kc-compose-web-smoke http://127.0.0.1:7401 >/tmp/dsh-web-smoke.log 2>&1; then
      touch "$ready"
      exit 0
    fi
    sleep 2
  done
  echo "DSH browser readiness failed" >&2
  sed -n '1,160p' /tmp/dsh-web-smoke.log >&2 || true
  exit 1
) &

exec dsh \
  --profile "$profile" \
  --patch /opt/dsh-config/gpt-web.patch.yml \
  --host 127.0.0.1 \
  --port 7401 \
  --no-open
