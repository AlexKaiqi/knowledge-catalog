package cli

import (
	_ "embed"
	"net/http"
)

//go:embed web/console.html
var consolePageHTML string

//go:embed web/console.js
var consolePageJS string

// The console is the kc serve observation frontend: Server verdict, Snapshot
// and retrieval plane, Catalog map, then a repository or Dataset. Rendering
// /console never grants access and does not mutate Catalog state.
func (f *httpFacade) consolePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(consolePageHTML))
}

func (f *httpFacade) consoleScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(consolePageJS))
}
