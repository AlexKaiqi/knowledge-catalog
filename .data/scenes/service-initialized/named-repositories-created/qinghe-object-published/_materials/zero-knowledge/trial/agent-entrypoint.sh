#!/bin/sh
# Agent 容器入口：写 pi 的模型配置，以指定身份登录 kc，然后把人的原话交给 pi。
# 不设置 KC_AS（它会盖住登录会话），不放任何上下文文件，Agent 看到的只有 pi 自带提示与原话。
set -eu
: "${ZK_PRINCIPAL:?ZK_PRINCIPAL is required}"
: "${ZK_MODEL_BASE_URL:?ZK_MODEL_BASE_URL is required}"
: "${ZK_MODEL_ID:?ZK_MODEL_ID is required}"

mkdir -p "$PI_CODING_AGENT_DIR"
cat > "$PI_CODING_AGENT_DIR/models.json" <<EOF
{
  "providers": {
    "zk": {
      "baseUrl": "$ZK_MODEL_BASE_URL",
      "api": "openai-completions",
      "apiKey": "trial-token",
      "models": [{ "id": "$ZK_MODEL_ID", "reasoning": true, "contextWindow": 200000, "maxTokens": 16000 }]
    }
  }
}
EOF

kc login --mode local --as "$ZK_PRINCIPAL" >/dev/null

exec pi --provider zk --model "$ZK_MODEL_ID" --session-dir /runs/sessions "$@"
