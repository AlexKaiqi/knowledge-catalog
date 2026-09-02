# Sourced by bash --login in the Compose HTTP CLI container.
if [[ -z "${KC_CLI_BANNER:-}" ]]; then
  export KC_CLI_BANNER=1
  cat <<EOF
Knowledge Catalog CLI (Compose / ttyd)
  KC_SERVER_URL=${KC_SERVER_URL:-}
  KC_CATALOG=${KC_CATALOG:-}
  KC_WORKSPACE=${KC_WORKSPACE:-}

This Server is --auth local (test pairing). Login first; do not export KC_AS.
Do not run kc local or kc serve here. Product commands go to KC Server.
kc help consumer|provider|governor

Consumer:
  kc login --mode local --as agent:dsh
  kc identity whoami
  kc catalog list
  kc catalog show
  kc knowledge schema browse --repo kr://dw/physical
  kc catalog workspace resolve > pin.json
  kc knowledge search --query lineitem
  kc knowledge read --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068
  kc knowledge relations --object kc://dw/physical/dw-mysql-tpch-table-c02fedc564bba85c8d5d1068
  kc resource access --object resource/mysql-tpch-sql --operation query \\
    --input '{"sql":"SELECT COUNT(*) FROM tpch.customer"}'
  kcfs plan --server "\$KC_SERVER_URL" --as agent:dsh --workspace warehouse-agent \\
    --view semantic --root /workspace

Provider / governor (same bootstrap identity in this fixture):
  kc login --mode local --as service:bootstrap
  kc writer head --repo kr://dw/physical
  kc writer ingest --repo kr://dw/semantic --dir /opt/data-warehouse/knowledge/semantic --out changeset.json
  kc knowledge read --repo kr://dw/physical --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068
  kc catalog show
  kc admin grant list
  kc catalog audit
  kc operations projection describe --repo kr://dw/physical
  kc operations access describe
EOF
fi
PS1='kc-cli:\w\$ '
