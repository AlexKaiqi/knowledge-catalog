package main

import (
	"regexp"
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
	html := renderMarkdown("见 [权限](perm-doc.md)。\n\n```bash\necho 中文\n```\n")
	if !strings.Contains(html, `href="perm-doc.md"`) {
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

func TestRenderMarkdownImages(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"relative asset", `![Agent 环境](../assets/architecture.svg)`, `<img src="../assets/architecture.svg" alt="Agent 环境">`},
		{"empty alternative", `![](/doc/assets/architecture.png)`, `<img src="/doc/assets/architecture.png" alt="">`},
		{"code in alternative", "![文 `\" <x>`](../assets/architecture.svg)", `<img src="../assets/architecture.svg" alt="文 &#34; &lt;x&gt;">`},
		{"escape attributes", `![" onload="bad <b> & **说明**](../assets/architecture.svg?x="a"&y=<b>)`, `<img src="../assets/architecture.svg?x=&#34;a&#34;&amp;y=&lt;b&gt;" alt="&#34; onload=&#34;bad &lt;b&gt; &amp; **说明**">`},
		{"escape source quote", `![图](../assets/architecture.svg" onerror="bad)`, `<img src="../assets/architecture.svg&#34; onerror=&#34;bad" alt="图">`},
		{"reject script", `![图](javascript:bad)`, `<img src="#" alt="图">`},
		{"reject mixed case script", `![图](JaVaScRiPt:bad)`, `<img src="#" alt="图">`},
		{"reject escaped script", `![图](java&#115;cript:bad)`, `<img src="#" alt="图">`},
		{"reject data", `![图](data:image/png;base64,abcd)`, `<img src="#" alt="图">`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderMarkdown(tc.src)
			if got != "<p>"+tc.want+"</p>\n" {
				t.Fatalf("image rendering:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

func TestRenderMarkdownImagesRemainLiteralInCode(t *testing.T) {
	for _, src := range []string{
		"`![图](../assets/architecture.svg)`",
		"```markdown\n![图](../assets/architecture.svg)\n```",
	} {
		got := renderMarkdown(src)
		if strings.Contains(got, "<img") || !strings.Contains(got, "![图](../assets/architecture.svg)") {
			t.Fatalf("code must retain literal image syntax: %s", got)
		}
	}
}

func TestWrapPageImagesFitReadingWidth(t *testing.T) {
	page := wrapPage("设计书", "doc/architecture.md", "")
	rule := regexp.MustCompile(`main img\s*\{([^}]*)\}`).FindStringSubmatch(page)
	if rule == nil || !regexp.MustCompile(`max-width\s*:\s*100%\s*;`).MatchString(rule[1]) ||
		!regexp.MustCompile(`height\s*:\s*auto\s*;`).MatchString(rule[1]) {
		t.Fatal("document images must fit the reading width while preserving aspect ratio")
	}
}
