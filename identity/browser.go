package identity

import (
	"mime"
	"net/http"

	"kc/kernel"
)

// BrowserRequestCheck returns the service's same-origin policy for browser
// requests. It is independent of the authentication method: a request carrying
// a Cookie, or a verified gateway identity that arrived from a browser, must
// prove same-origin intent with X-Kc-Console: 1 and acceptable Fetch Metadata,
// and its mutations must be JSON (multipart when uploads are allowed).
// Requests that carry Authorization are not browser-ambient and pass.
func BrowserRequestCheck(allowMultipart bool) func(*http.Request) error {
	return func(r *http.Request) error { return checkBrowserRequest(r, allowMultipart) }
}

func checkBrowserRequest(r *http.Request, allowMultipart bool) error {
	// A verified gateway header can also carry a machine request after the
	// gateway consumes its Bearer. Browser provenance requires CSRF protection;
	// the trusted header alone must not turn every existing CLI into a browser.
	browser := len(r.Header.Values("Cookie")) > 0 || r.Header.Get("X-Tai-Identity") != "" &&
		(r.Header.Get("Sec-Fetch-Site") != "" || r.Header.Get("Origin") != "" || r.Header.Get("X-Kc-Console") != "")
	if r.Header.Get("Authorization") != "" || !browser {
		return nil
	}
	markers := r.Header.Values("X-Kc-Console")
	sites := r.Header.Values("Sec-Fetch-Site")
	if len(markers) != 1 || markers[0] != "1" || len(sites) > 1 {
		return kernel.Fail(kernel.ErrForbidden, "browser session requires same-origin request proof")
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
	default:
		return kernel.Fail(kernel.ErrForbidden, "browser session rejects cross-origin requests")
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return nil
	}
	contentTypes := r.Header.Values("Content-Type")
	if len(contentTypes) == 1 {
		mediaType, _, err := mime.ParseMediaType(contentTypes[0])
		if err == nil && (mediaType == "application/json" || allowMultipart && mediaType == "multipart/form-data") {
			return nil
		}
	}
	return kernel.Fail(kernel.ErrForbidden, "browser session mutations require application/json")
}
