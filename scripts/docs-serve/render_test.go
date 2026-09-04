package main

import (
	"strings"
	"testing"
)

func TestRenderMarkdownUTF8AndTable(t *testing.T) {
	html := renderMarkdown("# 术语\n\n能查谁读过。\n\n| 规范名称 | 含义 |\n|---|---|\n| Catalog | 组合空间 |\n")
	for _, want := range []string{
		"<h1>术语</h1>",
		"能查谁读过。",
		"<table>",
		"<th>规范名称</th>",
		"<td>Catalog</td>",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in:\n%s", want, html)
		}
	}
}

func TestRenderMarkdownCodeFenceAndLink(t *testing.T) {
	html := renderMarkdown("见 [权限](PERMISSIONS.md)。\n\n```bash\necho 中文\n```\n")
	if !strings.Contains(html, `href="PERMISSIONS.md"`) {
		t.Fatalf("link: %s", html)
	}
	if !strings.Contains(html, "echo 中文") {
		t.Fatalf("fence: %s", html)
	}
	if strings.Contains(html, "<script") {
		t.Fatalf("raw script leaked")
	}
}

func TestRenderMarkdownEscapesHTML(t *testing.T) {
	html := renderMarkdown("a <b> & c")
	if strings.Contains(html, "<b>") {
		t.Fatalf("unescaped: %s", html)
	}
	if !strings.Contains(html, "&lt;b&gt;") {
		t.Fatalf("want escaped: %s", html)
	}
}
