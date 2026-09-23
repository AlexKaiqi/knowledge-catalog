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
	writeStaticHTMLPage(w, repositoryPageHTML)
}

func (f *httpFacade) repositoryScript(w http.ResponseWriter, r *http.Request) {
	writeStaticScript(w, repositoryPageJS)
}
