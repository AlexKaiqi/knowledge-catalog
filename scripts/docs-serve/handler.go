package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func newMux(root string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/docs/product.html", http.StatusFound)
			return
		}
		serveDoc(root, w, r)
	})
	return mux
}

func serveDoc(root string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	abs, rel, err := absUnderRoot(root, r.URL.Path)
	if err != nil {
		status := http.StatusNotFound
		if os.IsPermission(err) || err == os.ErrPermission {
			status = http.StatusForbidden
		}
		http.Error(w, "not found", status)
		return
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".html":
		serveRaw(w, r, abs, "text/html; charset=utf-8")
	case ".md":
		src, err := readFile(abs)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		page := wrapPage(firstHeading(src), rel, renderMarkdown(src))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = io.WriteString(w, page)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func serveRaw(w http.ResponseWriter, r *http.Request, abs, contentType string) {
	f, err := os.Open(abs)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}
