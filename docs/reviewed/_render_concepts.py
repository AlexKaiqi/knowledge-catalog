#!/usr/bin/env python3
"""Render docs/reviewed/core-concepts.png — knowledge object graph, not the runtime layers."""

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

OUT = Path(__file__).with_name("core-concepts.png")
FONT = "/usr/share/fonts/google-droid/DroidSansFallback.ttf"

W, H = 1600, 780
BG = "#f7f4ef"
CARD = "#fffcf8"
ANCHOR = "#f3ebe0"
BOUND = "#eef4ea"
INK = "#1c1917"
MUTED = "#44403c"
FAINT = "#78716c"
LINE = "#44403c"


def font(size):
    return ImageFont.truetype(FONT, size)


def rounded(draw, xy, r, fill, outline, width=2):
    draw.rounded_rectangle(xy, radius=r, fill=fill, outline=outline, width=width)


def card(draw, x, y, w, h, title, lines, fill=CARD, title_size=20):
    rounded(draw, (x, y, x + w, y + h), 10, fill, LINE, width=2)
    draw.text((x + 16, y + 12), title, font=font(title_size), fill=INK)
    ty = y + 46
    for line in lines:
        color = FAINT if line.startswith("不") or line.startswith("不是") else MUTED
        draw.text((x + 18, ty), line, font=font(16), fill=color)
        ty += 26


def down_arrow(draw, x, y1, y2):
    draw.line((x, y1, x, y2 - 10), fill=LINE, width=2)
    draw.polygon([(x - 7, y2 - 12), (x + 7, y2 - 12), (x, y2)], fill=LINE)


def main():
    img = Image.new("RGB", (W, H), BG)
    d = ImageDraw.Draw(img)

    # Entity spans the top: identity only, not a runtime layer.
    card(
        d, 40, 28, 1520, 150,
        "Entity  （逻辑锚点）",
        [
            "身份是 object_id，不是路径，不是 URN。KnowledgeRef = (repository, object_id)。",
            "Aspect 不是另一套 Ref。默认 READ(object_id) 拼装切面；拼装是读策略，不是存储形状。",
            "写入与冲突按 Address。这里不存墙外当前值，也不等于 Snapshot Store / Catalog。",
        ],
        fill=ANCHOR, title_size=22,
    )

    # Three children. Left/mid are both Aspect; right is a sibling object kind.
    gap = 36
    col_w = 482
    y0 = 248
    x0, x1, x2 = 40, 40 + col_w + gap, 40 + 2 * (col_w + gap)
    cx = [x0 + col_w // 2, x1 + col_w // 2, x2 + col_w // 2]
    for x in cx:
        down_arrow(d, x, 178, y0)

    d.text((x0 + 18, 214), "同一 object_id 上的切面", font=font(14), fill=FAINT)
    d.text((x1 + 18, 214), "仍是 Aspect，差在取值来源", font=font(14), fill=FAINT)
    d.text((x2 + 18, 214), "独立对象，endpoints 指向锚点", font=font(14), fill=FAINT)

    card(
        d, x0, y0, col_w, 390,
        "Snapshot Aspect",
        [
            "Address：object_id + aspectName",
            "ValueSource = snapshot",
            "正文在该 commit 的 Canonical 里",
            "Schema 约束切面；Member 是切面内的键",
            "写入按 Address 一单元，不是 PATCH",
            "不把 GRANT 正文自动当成检索 text",
            "",
            "不是 Index 文档，不是文件路径",
        ],
    )
    card(
        d, x1, y0, col_w, 390,
        "Bound Aspect  （观察声明）",
        [
            "还是 Aspect，不是第三种 kind",
            "ValueSource = binding",
            "mode：state（当前值）或 stream（记录）",
            "Snapshot 只存 Binding 句柄",
            "当前值经 Serving 拉墙外 runtime",
            "声明 basis 与观察 basis 必须分开",
            "普通 READ 不得把 Stream 数组化",
            "",
            "不是 Recipe，不是宽表，不是 log 通道",
        ],
        fill=BOUND,
    )
    card(
        d, x2, y0, col_w, 390,
        "Relation",
        [
            "自己的 object_id，Kind = Relation",
            "不是 Entity 下的一个 Aspect 名",
            "公共信封：type / direction / endpoints",
            "endpoint 指向 KnowledgeRef",
            "领域事实放 schema 约束的 attributes",
            "单仓 COMMIT 不检查对端是否存在",
            "检索命中后仍回 Canonical hydrate",
            "",
            "DependsOn 等是领域类型，不是协议 kind",
        ],
    )

    d.text(
        (40, 660),
        "这是知识对象图，不是系统核心架构。左、中都是 Aspect（同一锚点上的切面）；右是独立对象，用 endpoints 指回 Entity。",
        font=font(15),
        fill=MUTED,
    )
    d.text(
        (40, 688),
        "静态/动态不分成两种实体。动态是切面上的 Binding。Schema 挂在 Address 上，不单独占一列。",
        font=font(15),
        fill=FAINT,
    )

    cropped = img.crop((0, 0, W, 730))
    cropped.save(OUT, "PNG", optimize=True)
    print("wrote", OUT, cropped.size)


if __name__ == "__main__":
    main()
