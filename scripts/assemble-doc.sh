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
OUTPUT="${2:-docs/KNOWLEDGE_CATALOG_DESIGN.md}"

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
  echo "> 权威设计：从不可约事实推导最小协议，再落到契约与参考实现。"
  echo "> 第 1 章是第一性原理；其后各章是同一推导的领域展开。附录只留决策轨迹，不是第二套规范。"
  echo ""
  echo "---"
  echo ""
  # 前言：只取 Goal / Assumptions / Known Facts。
  awk '/^# Goal/{p=1} /^# Open Questions/{p=0} /^# Current Decisions/{p=0} /^# Deliverables/{p=0} /^# Acceptance Criteria/{p=0} p{print}' "$CHECKOUT/surface.md"
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
