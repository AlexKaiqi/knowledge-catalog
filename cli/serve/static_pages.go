package cli

import "net/http"

// writeStaticHTMLPage serves one embedded observation page. Rendering is a
// fixed byte stream: no access decision, no Catalog mutation, and caching is
// disabled so a stale page cannot outlive the server process.
func writeStaticHTMLPage(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(body))
}

// writeStaticScript serves one embedded page script with cacheable-but-stale
// semantics: scripts may live across requests, pages may not.
func writeStaticScript(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(body))
}
