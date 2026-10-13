package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kc/kernel"
)

type nativeResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *nativeResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *nativeResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}
func (w *nativeResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func nativeUploadRequest(r *http.Request) bool {
	route, _, ok := matchNativeRoute(r)
	return ok && route.path == "/repositories/{repository}/branches/{branch}/objects" && r.Method == http.MethodPost
}

func nativeNavigationRequest(r *http.Request) *http.Request {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return r
	}
	if r.Header.Get("X-Kc-Console") != "" {
		return r
	}
	site := r.Header.Get("Sec-Fetch-Site")
	if site != "same-origin" && !(site == "none" && r.Header.Get("Sec-Fetch-Mode") == "navigate") {
		return r
	}
	r = r.Clone(r.Context())
	r.Header.Set("X-Kc-Console", "1")
	return r
}

func fetchNativeJSON(ctx context.Context, target *url.URL, path string, query url.Values, proof http.Header, result any) error {
	endpoint := *target
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api/v1" + path
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	request.Header = proof.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.Header.Set("Accept", "application/json")
	client := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "native metadata request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "native metadata returned HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxRequestBytes+1))
	if err != nil || len(raw) > MaxRequestBytes {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid native metadata response")
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid native metadata JSON")
	}
	return nil
}
