#!/usr/bin/env python3
"""Render docs/reviewed/core-architecture.png.

Boxes = layers. Edge labels = protocol names (or a short phrase when the
full set will not fit; expanded in core-architecture.md). Every arrow
starts and ends on a box edge.
"""

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

OUT = Path(__file__).with_name("core-architecture.png")
FONT = "/usr/share/fonts/google-droid/DroidSansFallback.ttf"

W, H = 1680, 1240
BG = "#f7f4ef"
CARD = "#fffcf8"
SCENE = "#e8eef9"
KNOW = "#e7f1ea"
INK = "#1c1917"
MUTED = "#292524"
FAINT = "#44403c"
LINE = "#292524"
ACCENT = "#1e3a8a"
WORKER = "#f3efe8"
PAD = 16
TITLE = 30
BODY = 24
LABEL = 16
LINE_H = 34


def font(size):
    return ImageFont.truetype(FONT, size)


def rounded(draw, xy, r, fill, outline, width=2):
    draw.rounded_rectangle(xy, radius=r, fill=fill, outline=outline, width=width)


def text_w(draw, s, f):
    box = draw.textbbox((0, 0), s, font=f)
    return box[2] - box[0]


def needed_h(n_lines, title_size=TITLE, line_h=LINE_H):
    return 12 + title_size + 8 + n_lines * line_h + 12


def assert_fits(draw, lines, max_w, body_size=BODY, where=""):
    f = font(body_size)
    overflow = [s for s in lines if text_w(draw, s, f) > max_w]
    if overflow:
        raise SystemExit(f"text overflow in {where} (max {max_w}px): {overflow}")


def banner(draw, x, y, w, h, text):
    rounded(draw, (x, y, x + w, y + h), 8, SCENE, LINE, 2)
    f = font(18)
    tw = text_w(draw, text, f)
    if tw > w - 12:
        raise SystemExit(f"banner overflow: {text} {tw}>{w}")
    draw.text((x + (w - tw) // 2, y + (h - 22) // 2), text, font=f, fill=INK)


def card(draw, x, y, w, h, title, lines, fill=CARD, title_size=TITLE, body_size=BODY, line_h=LINE_H):
    rounded(draw, (x, y, x + w, y + h), 8, fill, outline=LINE, width=2)
    tf = font(title_size)
    if text_w(draw, title, tf) > w - PAD * 2:
        raise SystemExit(f"title overflow: {title} {text_w(draw, title, tf)}>{w - PAD * 2}")
    assert_fits(draw, lines, w - PAD * 2, body_size, title)
    draw.text((x + PAD, y + 10), title, font=tf, fill=INK)
    ty = y + 12 + title_size + 8
    bf = font(body_size)
    for line in lines:
        color = FAINT if line.startswith("不") or line.startswith("不是") else MUTED
        if ty + body_size > y + h - 8:
            raise SystemExit(f"vertical overflow in {title}: {line}")
        draw.text((x + PAD, ty), line, font=bf, fill=color)
        ty += line_h


def dash_line(draw, x1, y1, x2, y2, color, width=3, dash=7, gap=5):
    dx, dy = x2 - x1, y2 - y1
    length = max((dx * dx + dy * dy) ** 0.5, 1)
    ux, uy = dx / length, dy / length
    pos = 0.0
    while pos < length:
        a, b = pos, min(pos + dash, length)
        draw.line((x1 + ux * a, y1 + uy * a, x1 + ux * b, y1 + uy * b), fill=color, width=width)
        pos = b + gap


def arrowhead(draw, x, y, dx, dy, color):
    length = max((dx * dx + dy * dy) ** 0.5, 1)
    ux, uy = dx / length, dy / length
    px, py = -uy, ux
    draw.polygon(
        [
            (x, y),
            (x - ux * 12 + px * 6, y - uy * 12 + py * 6),
            (x - ux * 12 - px * 6, y - uy * 12 - py * 6),
        ],
        fill=color,
    )


def draw_label(draw, x, y, text, color):
    """Draw one or more label lines; '\\n' starts a new line."""
    f = font(LABEL)
    for i, line in enumerate(text.split("\n")):
        draw.text((x, y + i * 18), line, font=f, fill=color)


def vconnect(draw, x, y1, y2, label="", color=LINE, dashed=False, label_dx=8):
    stem = y2 - 12
    if dashed:
        dash_line(draw, x, y1, x, stem, color)
    else:
        draw.line((x, y1, x, stem), fill=color, width=3)
    arrowhead(draw, x, y2, 0, 1, color)
    if label:
        lines = label.split("\n")
        mid = (y1 + y2) // 2 - 9 - (len(lines) - 1) * 9
        draw_label(draw, x + label_dx, mid, label, color)


def hconnect(draw, x1, x2, y, label="", color=ACCENT, label_below=False):
    going_right = x2 > x1
    tip = x2 - 12 if going_right else x2 + 12
    draw.line((x1, y, tip, y), fill=color, width=3)
    arrowhead(draw, x2, y, 1 if going_right else -1, 0, color)
    if label:
        f = font(LABEL)
        lines = label.split("\n")
        tw = text_w(draw, max(lines, key=len), f)
        if label_below:
            ly = y + 6
        else:
            ly = y - 20 - (len(lines) - 1) * 18
        draw_label(draw, (x1 + x2) // 2 - tw // 2, ly, label, color)


def around_right_to_top(draw, x_leave, y_leave, rail_x, y_gap, x_land, y_land, label="", color=ACCENT):
    """Leave a box to the right, drop in the outer rail, come back, land on another box top."""
    draw.line((x_leave, y_leave, rail_x, y_leave), fill=color, width=3)
    draw.line((rail_x, y_leave, rail_x, y_gap), fill=color, width=3)
    draw.line((rail_x, y_gap, x_land, y_gap), fill=color, width=3)
    draw.line((x_land, y_gap, x_land, y_land - 12), fill=color, width=3)
    arrowhead(draw, x_land, y_land, 0, 1, color)
    if label:
        f = font(LABEL)
        # Sit to the right of the outer rail so the label does not enter Access.
        draw_label(draw, rail_x + 10, (y_leave + y_gap) // 2 - 9, label, color)
        right = rail_x + 10 + text_w(draw, max(label.split("\n"), key=len), f)
        if right > W - 8:
            raise SystemExit(f"rail label overflow: {label} ends at {right}>{W}")


def elbow_left_down(draw, x_from, y_from, x_to, y_to, label="", color=LINE):
    mid_y = (y_from + y_to) // 2
    draw.line((x_from, y_from, x_from, mid_y), fill=color, width=3)
    draw.line((x_from, mid_y, x_to, mid_y), fill=color, width=3)
    draw.line((x_to, mid_y, x_to, y_to - 12), fill=color, width=3)
    arrowhead(draw, x_to, y_to, 0, 1, color)
    if label:
        f = font(LABEL)
        tw = text_w(draw, label.split("\n")[0], f)
        draw_label(draw, (x_from + x_to) // 2 - tw // 2, mid_y - 20, label, color)


def main():
    img = Image.new("RGB", (W, H), BG)
    d = ImageDraw.Draw(img)

    X1, W1 = 14, 408
    X2, W2 = 478, 390
    X5, W5 = 998, 436

    writer_lines = [
        "唯一写面",
        "PUT / REMOVE",
        "COMMIT / PROPOSAL",
        "CAS 成功即结束",
        "Gate 只绑 merge",
        "COMMIT 不走 Gate",
        "不调用 3",
        "没有 APPEND",
        "PROPOSAL 不发 AfterSnapshot",
        "动态值先变成 ChangeSet",
    ]
    snap_lines = [
        "唯一权威",
        "版本图 + 字节",
        "1 写 · 3 追 · ② 读",
        "不解释知识",
    ]
    ingest_lines = [
        "2 的订阅者",
        "HEAD = READY",
        "不回写 2",
    ]
    access_head = [
        "冻结 pin",
        "只调 ② / ③",
    ]
    serving_lines = [
        "解释 Snapshot 声明",
        "观察编进同一值",
        "命中后必须 hydrate",
        "不把 tree 交给 Access",
        "观察不得冒充 Snapshot",
    ]
    a_lines = [
        "HEAD 动了才走",
        "Snapshot 投影",
    ]
    b_lines = [
        "HEAD 不动",
        "State 投影",
    ]
    m_lines = [
        "墙外物化",
        "不是权威",
    ]
    index_lines = [
        "发现面 · 可丢 · CandidateRef · 命中回 ② hydrate",
    ]

    Y_SCENE, H_SCENE = 12, 44
    Y_HUB = 128
    H_SNAP = needed_h(4)
    gap = 44
    H_ING = needed_h(3)
    H_WRITER = H_SNAP + gap + H_ING
    Y_ING = Y_HUB + H_SNAP + gap
    stack_bottom = Y_HUB + H_WRITER

    acc_header = 12 + TITLE + 8 + 2 * LINE_H + 8
    H_ACC = H_WRITER
    serve_h = H_ACC - acc_header - 8

    Y_AB = stack_bottom + gap
    H_AB = needed_h(2, title_size=26)
    Y_IDX = Y_AB + H_AB + gap
    H_IDX = needed_h(1, title_size=28)

    banner(d, X1, Y_SCENE, W1, H_SCENE, "知识维护")
    banner(d, X5, Y_SCENE, W5, H_SCENE, "知识消费")

    card(d, X1, Y_HUB, W1, H_WRITER, "1  Writer + Gate", writer_lines, fill=KNOW)
    card(d, X2, Y_HUB, W2, H_SNAP, "2  Snapshot Store", snap_lines)
    card(d, X2, Y_ING, W2, H_ING, "3  Ingestion", ingest_lines)

    assert_fits(d, access_head, W5 - PAD * 2, BODY, "5 Access")
    rounded(d, (X5, Y_HUB, X5 + W5, Y_HUB + H_ACC), 8, CARD, LINE, 2)
    d.text((X5 + PAD, Y_HUB + 10), "5  Access", font=font(TITLE), fill=INK)
    d.text((X5 + PAD, Y_HUB + 12 + TITLE + 8), access_head[0], font=font(BODY), fill=MUTED)
    d.text((X5 + PAD, Y_HUB + 12 + TITLE + 8 + LINE_H), access_head[1], font=font(BODY), fill=FAINT)
    serve_y = Y_HUB + acc_header
    serve_line_h = (serve_h - 12 - 26 - 8 - 12) // len(serving_lines)
    card(
        d, X5 + 8, serve_y, W5 - 16, serve_h,
        "② Knowledge", serving_lines, fill=KNOW, title_size=26, line_h=serve_line_h,
    )

    card(d, X1, Y_AB, W1, H_AB, "(A) Snapshot 变更", a_lines, fill=WORKER, title_size=26)
    card(d, X2, Y_AB, W2, H_AB, "(B) State 变更", b_lines, fill=WORKER, title_size=26)
    card(d, X5, Y_AB, W5, H_AB, "M  ·  非编号层", m_lines, fill=SCENE, title_size=26)
    card(d, X1, Y_IDX, X5 + W5 - X1, H_IDX, "4  Search Index", index_lines, title_size=28)

    vconnect(d, X1 + W1 // 2, Y_SCENE + H_SCENE, Y_HUB, "ChangeSet")
    vconnect(d, X5 + W5 // 2, Y_SCENE + H_SCENE, Y_HUB)
    consume = [
        "READ / RESOLVE / LOG",
        "GET_PROVENANCE / DESCRIBE_SCHEMA",
        "SEARCH / RELATIONS",
    ]
    cf = font(LABEL)
    consume_w = max(text_w(d, s, cf) for s in consume)
    consume_x = X5 + (W5 - consume_w) // 2
    consume_y = Y_SCENE + H_SCENE + 6
    draw_label(d, consume_x, consume_y, "\n".join(consume), LINE)

    cas_y = Y_HUB + H_SNAP // 2
    hconnect(d, X1 + W1, X2, cas_y, "CAS", color=LINE)

    two_r_y = max(serve_y + 24, Y_HUB + 12)
    two_r_y = min(two_r_y, Y_HUB + H_SNAP - 16)
    hconnect(d, X5, X2 + W2, two_r_y, "READ / RESOLVE\nLOG / DIFF", color=ACCENT, label_below=True)

    vconnect(d, X2 + W2 // 2, Y_HUB + H_SNAP, Y_ING, dashed=True, color=FAINT)
    as_name = "AfterSnapshot"
    as_w = text_w(d, as_name, font(LABEL))
    draw_label(d, X2 + W2 // 2 - as_w - 10, Y_HUB + H_SNAP + 8, as_name, FAINT)

    a_x = X1 + W1 // 2
    b_x = X2 + W2 // 2
    # 3 → (A)/(B) are lanes, not a third protocol. Labels live in the table.
    elbow_left_down(d, X2 + 28, Y_ING + H_ING, a_x, Y_AB)
    vconnect(d, b_x, Y_ING + H_ING, Y_AB)

    vconnect(d, X5 + W5 // 2, Y_HUB + H_ACC, Y_AB, "StateLookup", color=ACCENT)
    hconnect(d, X5, X2 + W2, Y_AB + 8, "ChangeNotice", color=ACCENT)

    vconnect(d, a_x, Y_AB + H_AB, Y_IDX, "Rebuild / Apply")
    vconnect(d, b_x, Y_AB + H_AB, Y_IDX, "Rebuild / Apply")

    # SEARCH / RELATIONS：从 5 右缘出去，外侧落下，再拐回接到 4 顶边。
    rail_x = X5 + W5 + 40
    gap_y = (Y_AB + H_AB + Y_IDX) // 2
    land_x = X5 + W5 - 48
    around_right_to_top(
        d, X5 + W5 + 2, serve_y + 28, rail_x, gap_y, land_x, Y_IDX,
        "SEARCH / RELATIONS", ACCENT,
    )

    cap_y = Y_IDX + H_IDX + 14
    cap = "边上是协议名。② 对 2 的箭头写不下 GET_PROVENANCE / DESCRIBE_SCHEMA，见图下。没画出的边 = 协议上不存在。虚线 = 可丢。"
    d.text((X1, cap_y), cap, font=font(16), fill=FAINT)
    if text_w(d, cap, font(16)) > W - X1 - 12:
        raise SystemExit("caption overflow")

    cropped = img.crop((0, 0, W, cap_y + 36))
    cropped.save(OUT, "PNG", optimize=True)
    print("wrote", OUT, cropped.size)


if __name__ == "__main__":
    main()
