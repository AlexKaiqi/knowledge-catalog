package cli

import (
	_ "embed"
	"net/http"
)

//go:embed web/repository.html
var repositoryPageHTML string

//go:embed web/repository.js
var repositoryPageJS string

// The page is a public shell. All account and repository data is fetched from
// authenticated typed APIs; rendering a URL never grants access to its target.
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
