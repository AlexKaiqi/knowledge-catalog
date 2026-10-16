package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeProductBookAndPublicationLinks(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "doc/product.html"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newMux(root))
	t.Cleanup(srv.Close)
	bookURL := srv.URL + "/doc/product.html"
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			res := requestAsset(t, srv.Client(), method, bookURL)
			body := readBody(t, res)
			if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/html; charset=utf-8" {
				t.Fatalf("product response: status=%d type=%q", res.StatusCode, res.Header.Get("Content-Type"))
			}
			want := string(raw)
			if method == http.MethodHead {
				want = ""
			}
			if body != want {
				t.Fatalf("product bytes changed for %s", method)
			}
		})
	}
	for _, guide := range []string{"first-publication", "share-a-dataset"} {
		ref := "../systemrepo/content/guides/" + guide + "/guide.md"
		if !strings.Contains(string(raw), `href="`+ref+`"`) {
			t.Fatalf("product does not link to %s", ref)
		}
		guideURL := resolveDocLink(t, bookURL, ref)
		guideHTML := readBody(t, get(t, guideURL))
		head := requestAsset(t, srv.Client(), http.MethodHead, guideURL)
		if body := readBody(t, head); head.StatusCode != http.StatusOK || body != "" || head.Header.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatalf("guide HEAD: status=%d type=%q body=%q", head.StatusCode, head.Header.Get("Content-Type"), body)
		}
		capabilityRef := "../../capabilities/read-object/definition.md"
		if !strings.Contains(guideHTML, `href="`+capabilityRef+`"`) {
			t.Fatalf("guide %s does not link to its reading capability", guide)
		}
		capability := readBody(t, get(t, resolveDocLink(t, guideURL, capabilityRef)))
		if !strings.Contains(capability, "<h1>精确读取与复核</h1>") {
			t.Fatalf("capability was not rendered as Markdown: %s", capability)
		}
	}
}

func resolveDocLink(t *testing.T, base, ref string) string {
	t.Helper()
	baseURL, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		t.Fatal(err)
	}
	return baseURL.ResolveReference(refURL).String()
}

func TestServeRejectsUnpublishedPaths(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"/doc/page.html", "/doc/product.html.js", "/systemrepo/content/page.html",
		"/systemrepo/content/config.yaml", "/systemrepo/content-other/guide.md",
		"/systemrepo/publication.go", "/systemrepo/README.md", "/.env",
	}
	for _, path := range paths {
		mustWrite(t, filepath.Join(root, path[1:]), "private")
	}
	paths = append(paths, "/systemrepo/content/../../.env", "/systemrepo/content/%2e%2e/publication.go")
	srv := httptest.NewServer(newMux(root))
	t.Cleanup(srv.Close)
	for _, path := range paths {
		assertPublicationDenied(t, srv, path)
	}
}

func TestServeRejectsPublicationSymlinkEscapes(t *testing.T) {
	for _, tc := range []struct {
		name, link, target string
		outside            bool
	}{
		{"product-outside-root", "doc/product.html", "private.html", true},
		{"product-unpublished-html", "doc/product.html", "doc/page.html", false},
		{"guide-outside-root", "systemrepo/content/guides/escape.md", "private.md", true},
		{"guide-private-source", "systemrepo/content/guides/escape.md", "systemrepo/publication.go", false},
		{"guide-private-markdown", "systemrepo/content/guides/escape.md", "tasks/private.md", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			targetRoot := root
			if tc.outside {
				targetRoot = t.TempDir()
			}
			target := filepath.Join(targetRoot, tc.target)
			mustWrite(t, target, "private")
			link := filepath.Join(root, tc.link)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(newMux(root))
			t.Cleanup(srv.Close)
			assertPublicationDenied(t, srv, "/"+tc.link)
		})
	}
}

func assertPublicationDenied(t *testing.T, srv *httptest.Server, path string) {
	t.Helper()
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		res := requestAsset(t, srv.Client(), method, srv.URL+path)
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s: status=%d, want forbidden", method, path, res.StatusCode)
		}
	}
}
