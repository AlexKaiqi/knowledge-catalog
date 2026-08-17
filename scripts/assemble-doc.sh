#!/usr/bin/env bash
# 模板组装脚本：把 WorkSurface 的 checkout（surface.md + blocks/）渲染成单文件设计文档。
#
# 用法：
#   assemble-doc.sh <checkout-dir> [output-file]
#
# <checkout-dir> 是 `ws checkout` 得到的目录（含 surface.md 与 blocks/）。
# 在 Orchestrator 内运行时（ws 在 PATH），典型用法：
#   TARGET="$WS_ATTEMPT_DIR/work/root"
#   ws checkout "$WS_ROOT_SURFACE" "$TARGET" >/dev/null
#   assemble-doc.sh "$TARGET" "$WS_ATTEMPT_DIR/assembled.md"

set -euo pipefail

CHECKOUT="${1:?usage: assemble-doc.sh <checkout-dir> [output-file]}"
OUTPUT="${2:-KNOWLEDGE_CATALOG_DESIGN.md}"

# 去掉 block 的 frontmatter（---...---）与 worksurface 注释。
strip_block() {
  awk 'BEGIN{n=0} /^---$/{n++; next} n<2{next} {print}' "$1" \
    | grep -vF '<!-- worksurface:block' \
    | grep -vF '<!-- /worksurface:block'
}

# 正文顺序（与 surface.md 的 Deliverables 引用一致）。
BLOCKS="architecture identity ingress repository catalog access maintenance decisions minimal-semantic-layer minimal-core-contracts ingestion-and-grounding gap-analysis refinements-p0 refinements-p1 refinements-p2"

{
  echo "# Knowledge Catalog 系统设计"
  echo ""
  echo "> 由 WorkSurface（surface.md + blocks）模板组装生成。"
  echo "> 权威文档是 WorkSurface 本身；本文件是可分发快照。"
  echo ""
  echo "---"
  echo ""
  # 前言：surface.md 的 Goal ~ Current Decisions（跳过 Deliverables 的引用列表）。
  awk '/^# Goal/{p=1} /^# Deliverables/{p=0} p{print}' "$CHECKOUT/surface.md"
  echo ""
  echo "---"
  echo ""
  echo "# 正文"
  for b in $BLOCKS; do
    echo ""
    strip_block "$CHECKOUT/blocks/$b.md"
    echo ""
  done
} > "$OUTPUT"

echo "assembled: $OUTPUT ($(wc -l < "$OUTPUT") lines)"
