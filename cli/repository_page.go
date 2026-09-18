package cli

import (
	_ "embed"
	"net/http"
)

//go:embed web/repository.html
var repositoryPageHTML string

//go:embed web/repository.js
var repositoryPageJS string

// The management URL is a public shell that opens the kc serve observation
// console on the same repository. Rendering it never grants access.
func (f *httpFacade) repositoryPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(repositoryPageHTML))
}

func (f *httpFacade) repositoryScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(repositoryPageJS))
}
