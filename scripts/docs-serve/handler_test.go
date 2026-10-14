package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeMarkdownCharsetAndNavigation(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module x\n")
	mustWrite(t, filepath.Join(root, "README.md"), "# 根说明\n")
	mustWrite(t, filepath.Join(root, "doc", "README.md"), "# 文档\n\n[概念](concepts.md)\n")
	mustWrite(t, filepath.Join(root, "doc", "concepts.md"), "# 核心概念\n\n谁访问过。\n")
	mustWrite(t, filepath.Join(root, "doc", "page.html"), "<p>raw</p>")
	mustWrite(t, filepath.Join(root, "secret.txt"), "nope")

	srv := httptest.NewServer(newMux(root))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /: %d", res.StatusCode)
	}
	if !strings.HasSuffix(res.Request.URL.Path, "/doc/README.md") {
		t.Fatalf("redirected to %s", res.Request.URL)
	}

	md := get(t, srv.URL+"/doc/concepts.md")
	if ct := md.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("md content-type: %s", ct)
	}
	body := readBody(t, md)
	if !strings.Contains(body, "<h1>核心概念</h1>") || !strings.Contains(body, "谁访问过。") {
		t.Fatalf("rendered: %s", body)
	}
	if !strings.Contains(body, `href="/doc/README.md"`) {
		t.Fatalf("nav: %s", body)
	}

	for _, path := range []string{"/secret.txt", "/go.mod", "/doc/page.html", "/doc/../../secret.txt"} {
		res := getRaw(t, srv.URL+path)
		if res.StatusCode == http.StatusOK {
			t.Fatalf("served %s", path)
		}
	}
}

func TestServeRejectsJavaScriptPath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "doc", "a.md"), "[x](javascript:alert(1))")
	srv := httptest.NewServer(newMux(root))
	t.Cleanup(srv.Close)
	body := readBody(t, get(t, srv.URL+"/doc/a.md"))
	if strings.Contains(strings.ToLower(body), "javascript:") {
		t.Fatalf("javascript href: %s", body)
	}
}

func TestServeRealDocumentation(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newMux(root))
	t.Cleanup(srv.Close)
	index := readBody(t, get(t, srv.URL+"/doc/README.md"))
	if !strings.Contains(index, "<h1>Knowledge Catalog 文档</h1>") || !strings.Contains(index, "<table>") {
		t.Fatalf("documentation index not rendered: %s", index[:min(500, len(index))])
	}
	if !strings.Contains(index, `href="design/object.md"`) {
		t.Fatalf("documentation index must link design chapters")
	}
	_ = readBody(t, get(t, srv.URL+"/doc/design/object.md"))
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("%s: %d %s", url, res.StatusCode, b)
	}
	return res
}

func getRaw(t *testing.T, url string) *http.Response {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
