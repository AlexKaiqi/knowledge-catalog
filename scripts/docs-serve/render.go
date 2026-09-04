package main

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	reHeading    = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	reUL         = regexp.MustCompile(`^(\s*)[-*]\s+(.*)$`)
	reOL         = regexp.MustCompile(`^(\s*)\d+\.\s+(.*)$`)
	reFence      = regexp.MustCompile("^```(.*)$")
	reSepRow     = regexp.MustCompile(`^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$`)
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reBold       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reItalic     = regexp.MustCompile(`\*([^*]+)\*`)
)

func renderMarkdown(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(src, "\n")
	var b strings.Builder
	i := 0
	for i < len(lines) {
		line := lines[i]
		if m := reFence.FindStringSubmatch(line); m != nil {
			body, next := consumeFence(lines, i+1)
			lang := strings.TrimSpace(m[1])
			b.WriteString("<pre><code")
			if lang != "" {
				fmt.Fprintf(&b, ` class="language-%s"`, html.EscapeString(lang))
			}
			b.WriteString(">")
			b.WriteString(html.EscapeString(strings.Join(body, "\n")))
			b.WriteString("</code></pre>\n")
			i = next
			continue
		}
		if tbl, next, ok := consumeTable(lines, i); ok {
			b.WriteString(tbl)
			i = next
			continue
		}
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		if line == "---" || line == "***" {
			b.WriteString("<hr>\n")
			i++
			continue
		}
		if m := reHeading.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			fmt.Fprintf(&b, "<h%d>%s</h%d>\n", level, renderInline(m[2]), level)
			i++
			continue
		}
		if strings.HasPrefix(line, "> ") || line == ">" {
			quote, next := consumeQuote(lines, i)
			b.WriteString("<blockquote>\n")
			b.WriteString(quote)
			b.WriteString("</blockquote>\n")
			i = next
			continue
		}
		if reUL.MatchString(line) || reOL.MatchString(line) {
			list, next := consumeList(lines, i)
			b.WriteString(list)
			i = next
			continue
		}
		para, next := consumeParagraph(lines, i)
		b.WriteString("<p>")
		b.WriteString(renderInline(para))
		b.WriteString("</p>\n")
		i = next
	}
	return b.String()
}

func consumeFence(lines []string, start int) ([]string, int) {
	var body []string
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "```") {
			return body, i + 1
		}
		body = append(body, lines[i])
	}
	return body, len(lines)
}

func consumeQuote(lines []string, start int) (string, int) {
	var parts []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if line == ">" {
			parts = append(parts, "")
			i++
			continue
		}
		if strings.HasPrefix(line, "> ") {
			parts = append(parts, strings.TrimPrefix(line, "> "))
			i++
			continue
		}
		break
	}
	return "<p>" + renderInline(strings.Join(parts, " ")) + "</p>\n", i
}

func consumeParagraph(lines []string, start int) (string, int) {
	var parts []string
	i := start
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" || reHeading.MatchString(line) || reFence.MatchString(line) ||
			line == "---" || strings.HasPrefix(line, "> ") || reUL.MatchString(line) || reOL.MatchString(line) {
			break
		}
		if i+1 < len(lines) && looksLikeTableHeader(lines, i) {
			break
		}
		parts = append(parts, strings.TrimSpace(line))
		i++
	}
	if i == start {
		return lines[start], start + 1
	}
	return strings.Join(parts, " "), i
}

func looksLikeTableHeader(lines []string, i int) bool {
	if !strings.Contains(lines[i], "|") {
		return false
	}
	if i+1 >= len(lines) {
		return false
	}
	return reSepRow.MatchString(lines[i+1])
}

func consumeTable(lines []string, i int) (string, int, bool) {
	if !looksLikeTableHeader(lines, i) {
		return "", i, false
	}
	header := splitRow(lines[i])
	i += 2
	var rows [][]string
	for i < len(lines) && strings.Contains(lines[i], "|") && strings.TrimSpace(lines[i]) != "" && !reFence.MatchString(lines[i]) {
		if reSepRow.MatchString(lines[i]) {
			i++
			continue
		}
		rows = append(rows, splitRow(lines[i]))
		i++
	}
	var b strings.Builder
	b.WriteString("<table>\n<thead><tr>")
	for _, cell := range header {
		b.WriteString("<th>")
		b.WriteString(renderInline(cell))
		b.WriteString("</th>")
	}
	b.WriteString("</tr></thead>\n<tbody>\n")
	for _, row := range rows {
		b.WriteString("<tr>")
		for _, cell := range row {
			b.WriteString("<td>")
			b.WriteString(renderInline(cell))
			b.WriteString("</td>")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody></table>\n")
	return b.String(), i, true
}

func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	raw := strings.Split(line, "|")
	out := make([]string, 0, len(raw))
	for _, cell := range raw {
		out = append(out, strings.TrimSpace(cell))
	}
	return out
}

func consumeList(lines []string, start int) (string, int) {
	ordered := reOL.MatchString(lines[start])
	tag := "ul"
	if ordered {
		tag = "ol"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<%s>\n", tag)
	i := start
	for i < len(lines) {
		line := lines[i]
		var item string
		if ordered {
			m := reOL.FindStringSubmatch(line)
			if m == nil {
				break
			}
			item = m[2]
		} else {
			m := reUL.FindStringSubmatch(line)
			if m == nil {
				break
			}
			item = m[2]
		}
		fmt.Fprintf(&b, "<li>%s</li>\n", renderInline(item))
		i++
	}
	fmt.Fprintf(&b, "</%s>\n", tag)
	return b.String(), i
}

func renderInline(s string) string {
	type piece struct {
		html string
	}
	var slots []string
	replaced := reInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := reInlineCode.FindStringSubmatch(m)[1]
		slots = append(slots, "<code>"+html.EscapeString(inner)+"</code>")
		return fmt.Sprintf("\x00%d\x00", len(slots)-1)
	})
	replaced = html.EscapeString(replaced)
	replaced = reLink.ReplaceAllStringFunc(replaced, func(m string) string {
		parts := reLink.FindStringSubmatch(m)
		return fmt.Sprintf(`<a href="%s">%s</a>`, safeHref(htmlUnescapeKeep(parts[2])), parts[1])
	})
	replaced = reBold.ReplaceAllString(replaced, "<strong>$1</strong>")
	replaced = reItalic.ReplaceAllString(replaced, "<em>$1</em>")
	for i, slot := range slots {
		replaced = strings.ReplaceAll(replaced, html.EscapeString(fmt.Sprintf("\x00%d\x00", i)), slot)
		replaced = strings.ReplaceAll(replaced, fmt.Sprintf("\x00%d\x00", i), slot)
	}
	return replaced
}

func htmlUnescapeKeep(s string) string {
	return html.UnescapeString(s)
}

func safeHref(raw string) string {
	u := strings.TrimSpace(html.UnescapeString(raw))
	lower := strings.ToLower(u)
	if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "data:") {
		return "#"
	}
	return html.EscapeString(u)
}
