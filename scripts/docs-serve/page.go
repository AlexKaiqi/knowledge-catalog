package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

func wrapPage(title, rel, body string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = rel
	}
	escTitle := html.EscapeString(title)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    :root {
      --ink: #1c1917;
      --muted: #57534e;
      --faint: #78716c;
      --paper: #f7f4ef;
      --card: #fffcf8;
      --line: #e4ddd4;
      --accent: #1e40af;
      --accent-hover: #1e3a8a;
      --accent-soft: #e8eef9;
      --sans: "Avenir Next", "PingFang SC", "Hiragino Sans GB", "Noto Sans SC", "Source Han Sans SC", "Microsoft YaHei", system-ui, sans-serif;
      --serif: "Iowan Old Style", "Palatino Linotype", "Songti SC", "Noto Serif SC", "Source Han Serif SC", Georgia, serif;
      --mono: ui-monospace, "SF Mono", "Cascadia Mono", "Noto Sans Mono", Menlo, Consolas, monospace;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      color: var(--ink);
      background: var(--paper);
      font-family: var(--sans);
      font-size: 16px;
      line-height: 1.65;
    }
    a { color: var(--accent); }
    a:hover { color: var(--accent-hover); }
    header.bar {
      position: sticky;
      top: 0;
      display: flex;
      gap: 1rem;
      flex-wrap: wrap;
      align-items: center;
      padding: 0.7rem 1.25rem;
      background: var(--card);
      border-bottom: 1px solid var(--line);
    }
    header.bar a { color: var(--muted); text-decoration: none; }
    header.bar a:hover { color: var(--ink); }
    header.bar .here { color: var(--faint); font-size: 0.85rem; }
    main {
      max-width: 48rem;
      margin: 0 auto;
      padding: 1.5rem 1.25rem 3rem;
    }
    h1, h2, h3, h4 {
      font-family: var(--serif);
      letter-spacing: -0.02em;
      line-height: 1.25;
    }
    h1 { font-size: 1.85rem; }
    p { color: var(--muted); }
    code {
      font-family: var(--mono);
      font-size: 0.86em;
      background: var(--accent-soft);
      padding: 0.08em 0.38em;
      border-radius: 0.25rem;
    }
    pre {
      padding: 1rem 1.1rem;
      overflow: auto;
      background: #1c1917;
      color: #f5f0e8;
      border-radius: 0.6rem;
      font-family: var(--mono);
      font-size: 0.8rem;
      line-height: 1.55;
    }
    pre code { background: transparent; color: inherit; padding: 0; }
    table {
      width: 100%%;
      border-collapse: collapse;
      font-size: 0.92rem;
      margin: 1rem 0;
    }
    th, td {
      text-align: left;
      vertical-align: top;
      padding: 0.4rem 0.55rem 0.4rem 0;
      border-bottom: 1px solid var(--line);
    }
    th { color: var(--faint); font-size: 0.78rem; }
    blockquote {
      margin: 1rem 0;
      padding: 0.2rem 0 0.2rem 0.9rem;
      border-left: 3px solid var(--accent);
      color: var(--muted);
    }
    hr { border: 0; border-top: 1px solid var(--line); }
  </style>
</head>
<body>
  <header class="bar">
    <a href="/docs/product.html">产品说明</a>
    <a href="/docs/README.md">文档地图</a>
    <span class="here">%s</span>
  </header>
  <main>
%s  </main>
</body>
</html>
`, escTitle, html.EscapeString(rel), body)
}

func firstHeading(src string) string {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

func readFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func absUnderRoot(root, urlPath string) (string, string, error) {
	rel := strings.TrimPrefix(urlPath, "/")
	rel = filepath.Clean(rel)
	relSlash := filepath.ToSlash(rel)
	if !allowedRel(relSlash) {
		return "", "", os.ErrPermission
	}
	abs := filepath.Join(root, filepath.FromSlash(relSlash))
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", err
		}
		// dangling or not a symlink
		resolved = abs
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		rootResolved = root
	}
	if !underRoot(rootResolved, resolved) && !underRoot(root, abs) {
		return "", "", os.ErrPermission
	}
	return abs, relSlash, nil
}
