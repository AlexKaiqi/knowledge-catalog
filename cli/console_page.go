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
	writeStaticHTMLPage(w, consolePageHTML)
}

func (f *httpFacade) consoleScript(w http.ResponseWriter, r *http.Request) {
	writeStaticScript(w, consolePageJS)
}
