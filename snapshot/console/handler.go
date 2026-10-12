// Package console serves the Snapshot Store web console, including the complete
// vendored lakeFS interface and the Dataset and access management extensions.
package console

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// The console is built from snapshot/console/webui/ (vendored lakeFS webui,
// Apache-2.0; see webui/KC-VENDOR.md) with Vite emitting into this package
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
	entry, statErr := fs.Stat(sub, "index.html")
	return sub, statErr == nil && !entry.IsDir()
}

// Handler serves the console at /ui/. The caller mounts it at that path; all
// authentication and data requests remain the responsibility of their APIs.
func newHandler() http.Handler {
	root, built := webappRoot()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWebapp(w, r, root, built)
	})
}

func serveWebapp(w http.ResponseWriter, r *http.Request, root fs.FS, built bool) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	// The original console supports configured SSO, presigned storage URLs,
	// Markdown images and PDF objects. A fixed CSP here would narrow those
	// capabilities; deployments can set a policy for their configured origins.
	path := strings.TrimPrefix(r.URL.Path, "/ui/")
	if path == "" {
		path = "index.html"
	}
	exists := false
	if built {
		entry, err := fs.Stat(root, path)
		exists = err == nil && !entry.IsDir()
	}
	if !exists && !webappPagePath(path) {
		http.NotFound(w, r)
		return
	}
	if !built {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(datasetUIBuildHint))
		return
	}
	// Only page namespaces fall back to the shell. Repository and Dataset IDs
	// may contain dots, so file extensions cannot distinguish pages from assets.
	if !exists {
		r = r.Clone(r.Context())
		r.URL.Path = "/ui/"
	}
	http.StripPrefix("/ui/", http.FileServer(http.FS(root))).ServeHTTP(w, r)
}

func webappPagePath(path string) bool {
	page, _, _ := strings.Cut(path, "/")
	switch page {
	case "index.html", "repositories", "auth", "setup", "datasets":
		return true
	default:
		return false
	}
}

const datasetUIBuildHint = `<!doctype html>
<html lang="zh">
<head><meta charset="utf-8"><title>Snapshot Store 控制台</title></head>
<body style="font-family: sans-serif; max-width: 40rem; margin: 4rem auto;">
<h1>Snapshot Store 控制台尚未构建</h1>
<p>前端源代码在仓库的 <code>snapshot/console/webui/</code>（vendor 的 lakeFS webui，Apache-2.0）。
构建一次即可：</p>
<pre><code>npm --prefix snapshot/console/webui run build</code></pre>
<p>构建产物会输出到 <code>snapshot/console/webapp/</code>，可由调用方挂载。详见 <code>snapshot/console/webui/KC-VENDOR.md</code>。</p>
</body>
</html>`
