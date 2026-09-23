package cli

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// The Dataset web console is built from webui/ (vendored lakeFS webui, Apache-
// 2.0; see webui/KC-VENDOR.md) with Vite emitting directly into this package
// directory, because go:embed cannot reference paths above it. When the bundle
// has not been built yet the directory holds only a placeholder and /ui/
// degrades to a build hint instead of a broken page.
//
//go:embed all:webapp
var webappFS embed.FS

func webappRoot() (fs.FS, bool) {
	sub, err := fs.Sub(webappFS, "webapp")
	if err != nil {
		return nil, false
	}
	built := false
	if entries, listErr := fs.ReadDir(sub, "."); listErr == nil {
		for _, entry := range entries {
			if entry.Name() != ".gitkeep" && entry.Name() != "README.md" {
				built = true
			}
		}
	}
	return sub, built
}

func (f *httpFacade) datasetUI(w http.ResponseWriter, r *http.Request) {
	root, built := webappRoot()
	if !built {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(datasetUIBuildHint))
		return
	}
	// Single-page app: unknown /ui/* paths fall back to index.html so
	// client-side routing works from a deep link. Assets are addressed
	// absolutely from /ui/ (Vite base), so the rewrite never breaks them.
	path := strings.Trim(r.URL.Path, "/")
	path = strings.TrimPrefix(path, "ui/")
	if path == "" {
		path = "index.html"
	}
	if _, statErr := fs.Stat(root, path); statErr != nil {
		r.URL.Path = "/ui/"
	}
	http.StripPrefix("/ui/", http.FileServer(http.FS(root))).ServeHTTP(w, r)
}

const datasetUIBuildHint = `<!doctype html>
<html lang="zh">
<head><meta charset="utf-8"><title>KC Dataset 控制台</title></head>
<body style="font-family: sans-serif; max-width: 40rem; margin: 4rem auto;">
<h1>KC Dataset 控制台尚未构建</h1>
<p>前端源代码在仓库的 <code>webui/</code>（vendor 的 lakeFS webui，Apache-2.0）。
构建一次即可：</p>
<pre><code>cd webui
npm install
npm run build</code></pre>
<p>构建产物会输出到 <code>cli/webapp/</code> 并随 <code>kc serve</code> 一起发布。详见 <code>webui/KC-VENDOR.md</code>。</p>
</body>
</html>`
