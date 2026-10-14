package main

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestServeArchitectureImageAssets(t *testing.T) {
	root := t.TempDir()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><title>对象环境</title><rect width="10" height="10"/></svg>`
	var pngBody bytes.Buffer
	if err := png.Encode(&pngBody, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	type imageAsset struct {
		path        string
		contentType string
		body        string
	}
	assets := []imageAsset{
		{"/doc/assets/architecture.svg", "image/svg+xml", svg},
		{"/doc/assets/nested/architecture.png", "image/png", pngBody.String()},
	}
	for _, asset := range assets {
		mustWrite(t, filepath.Join(root, asset.path[1:]), asset.body)
	}
	if err := os.Symlink(filepath.Join(root, "doc/assets/architecture.svg"), filepath.Join(root, "doc/assets/alias.svg")); err != nil {
		t.Fatal(err)
	}
	assets = append(assets, imageAsset{"/doc/assets/alias.svg", "image/svg+xml", svg})
	mustWrite(t, filepath.Join(root, "doc/design/architecture.md"), "# 架构\n\n![对象环境](../assets/architecture.svg)\n\n![PNG](../assets/nested/architecture.png)\n")
	srv := httptest.NewServer(newMux(root))
	t.Cleanup(srv.Close)

	page := readBody(t, get(t, srv.URL+"/doc/design/architecture.md"))
	for _, ref := range []string{
		`<img src="../assets/architecture.svg" alt="对象环境">`,
		`<img src="../assets/nested/architecture.png" alt="PNG">`,
	} {
		if !strings.Contains(page, ref) {
			t.Errorf("Markdown did not render its asset reference: %s", ref)
		}
	}
	for _, asset := range assets {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+asset.path, func(t *testing.T) {
				res := requestAsset(t, srv.Client(), method, srv.URL+asset.path)
				body := readBody(t, res)
				if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != asset.contentType {
					t.Fatalf("asset response: status=%d type=%q", res.StatusCode, res.Header.Get("Content-Type"))
				}
				if length := res.Header.Get("Content-Length"); length != strconv.Itoa(len(asset.body)) {
					t.Fatalf("asset length %q, want %d", length, len(asset.body))
				}
				want := asset.body
				if method == http.MethodHead {
					want = ""
				}
				if body != want {
					t.Fatalf("asset bytes changed for %s", method)
				}
			})
		}
	}
}

func TestServeRejectsImagePaths(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"/architecture.svg",
		"/doc/design/architecture.svg",
		"/doc/architecture.png",
		"/doc/assets-other/architecture.svg",
		"/doc/assets/architecture.gif",
		"/doc/assets/architecture.webp",
		"/doc/assets/architecture.svg.js",
		"/doc/assets/architecture.okf",
	}
	for _, path := range paths {
		mustWrite(t, filepath.Join(root, path[1:]), "private")
	}
	outside := filepath.Join(t.TempDir(), "private.svg")
	mustWrite(t, outside, "outside")
	if err := os.Symlink(outside, filepath.Join(root, "doc/assets/escaped.svg")); err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "/doc/assets/escaped.svg", "/doc/assets/../design/architecture.svg")
	srv := httptest.NewServer(newMux(root))
	t.Cleanup(srv.Close)
	for _, path := range paths {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+path, func(t *testing.T) {
				res := requestAsset(t, srv.Client(), method, srv.URL+path)
				res.Body.Close()
				if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusNotFound {
					t.Fatalf("served forbidden asset: %d", res.StatusCode)
				}
			})
		}
	}
}

func requestAsset(t *testing.T, client *http.Client, method, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
