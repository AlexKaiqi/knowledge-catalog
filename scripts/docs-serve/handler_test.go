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

func TestServeMarkdownCharsetAndProductLinks(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "go.mod"), "module x\n")
	mustWrite(t, filepath.Join(root, "README.md"), "# 根说明\n")
	mustWrite(t, filepath.Join(root, "docs", "product.html"), "<!DOCTYPE html><meta charset=\"utf-8\"><p>产品</p>")
	mustWrite(t, filepath.Join(root, "docs", "TERMINOLOGY.md"), "# 术语表\n\n谁访问过。\n")
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
	if !strings.HasSuffix(res.Request.URL.Path, "/docs/product.html") {
		t.Fatalf("redirected to %s", res.Request.URL)
	}

	htmlRes := get(t, srv.URL+"/docs/product.html")
	if ct := htmlRes.Header.Get("Content-Type"); !strings.Contains(ct, "charset=utf-8") {
		htmlRes.Body.Close()
		t.Fatalf("html charset: %s", ct)
	}
	htmlRes.Body.Close()

	md := get(t, srv.URL+"/docs/TERMINOLOGY.md")
	if ct := md.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("md content-type: %s", ct)
	}
	body := readBody(t, md)
	if !strings.Contains(body, "<h1>术语表</h1>") || !strings.Contains(body, "谁访问过。") {
		t.Fatalf("rendered: %s", body)
	}
	if !strings.Contains(body, `href="/docs/product.html"`) {
		t.Fatalf("nav: %s", body)
	}

	deny := getRaw(t, srv.URL+"/secret.txt")
	if deny.StatusCode != http.StatusForbidden && deny.StatusCode != http.StatusNotFound {
		t.Fatalf("secret: %d", deny.StatusCode)
	}
	goSrc := getRaw(t, srv.URL+"/go.mod")
	if goSrc.StatusCode == http.StatusOK {
		t.Fatalf("served go.mod")
	}
	trav := getRaw(t, srv.URL+"/docs/../../secret.txt")
	if trav.StatusCode == http.StatusOK {
		t.Fatalf("traversal: %d", trav.StatusCode)
	}
}

func TestServeRejectsJavaScriptPath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "a.md"), "[x](javascript:alert(1))")
	srv := httptest.NewServer(newMux(root))
	t.Cleanup(srv.Close)
	body := readBody(t, get(t, srv.URL+"/docs/a.md"))
	if strings.Contains(strings.ToLower(body), "javascript:") {
		t.Fatalf("javascript href: %s", body)
	}
}

func TestServeRealProductAndMarkdown(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newMux(root))
	t.Cleanup(srv.Close)
	product := readBody(t, get(t, srv.URL+"/docs/product.html"))
	if !strings.Contains(product, "继续阅读") || !strings.Contains(product, "KNOWLEDGE_PRODUCT_AND_SCHEMA.md") {
		t.Fatalf("product.html missing 继续阅读 links")
	}
	if !strings.Contains(product, "LIVE_MATERIALIZATION.md") || !strings.Contains(product, "RETRIEVAL.md") {
		t.Fatalf("product.html must keep Binding and SEARCH algebra as separate 继续阅读 entries")
	}
	term := readBody(t, get(t, srv.URL+"/docs/TERMINOLOGY.md"))
	if !strings.Contains(term, "<h1>Knowledge Catalog 术语表</h1>") {
		t.Fatalf("heading missing: %s", term[:min(500, len(term))])
	}
	if !strings.Contains(term, "<table>") {
		t.Fatalf("table missing")
	}
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
