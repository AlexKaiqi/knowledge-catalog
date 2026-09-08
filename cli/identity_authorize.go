package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kc/kernel"
)

// Browser authorization uses the same stateless PAR/PKCE flow as the CLI.
// Relaying only the deployment-owned endpoints avoids an IdP CORS dependency;
// neither endpoint establishes a KC session nor accepts application secrets.
func (f *httpFacade) identityAuthorize(w http.ResponseWriter, r *http.Request) {
	config := f.browserLoginConfig()
	if config == nil {
		writeJSON(w, 503, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "browser login is not configured")))
		return
	}
	var input struct {
		CodeChallenge string `json:"codeChallenge"`
		State         string `json:"state"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if !decodeServiceRequest(w, r, &input) {
		return
	}
	if len(input.CodeChallenge) != 43 || !oauthURLSafe(input.CodeChallenge) || len(input.State) < 32 || len(input.State) > 128 || !oauthURLSafe(input.State) {
		writeJSON(w, 400, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "browser authorization requires a SHA256 PKCE challenge and random state")))
		return
	}
	form := url.Values{"client_id": {config.ClientID}, "response_type": {"code"}, "code_challenge": {input.CodeChallenge}, "code_challenge_method": {"S256"}, "state": {input.State}, "scope": {config.Scope}, "resource": {config.Resource}, "app_name": {config.AppName}}
	raw, ok := f.relayBrowserAuthorization(w, r, http.MethodPost, config.OAuth2Base+"/oauth2/par", form)
	if !ok {
		return
	}
	var result struct {
		RequestURI string `json:"request_uri"`
		ExpiresIn  int    `json:"expires_in"`
	}
	if json.Unmarshal(raw, &result) != nil || result.RequestURI == "" || result.ExpiresIn <= 0 {
		writeJSON(w, 502, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "authorization provider returned an invalid browser request")))
		return
	}
	authURL := config.OAuth2Base + "/oauth2/authorize?" + url.Values{"client_id": {config.ClientID}, "request_uri": {result.RequestURI}}.Encode()
	writeJSON(w, http.StatusOK, map[string]any{"authorizationURL": authURL, "requestURI": result.RequestURI, "expiresIn": result.ExpiresIn})
}

func (f *httpFacade) identityAuthorizePoll(w http.ResponseWriter, r *http.Request) {
	config := f.browserLoginConfig()
	if config == nil {
		writeJSON(w, 503, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "browser login is not configured")))
		return
	}
	var input struct {
		RequestURI string `json:"requestURI"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if !decodeServiceRequest(w, r, &input) {
		return
	}
	if input.RequestURI == "" || len(input.RequestURI) > 2048 {
		writeJSON(w, 400, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "requestURI is required and must be bounded")))
		return
	}
	endpoint := config.OAuth2Base + "/oauth2/par/poll?" + url.Values{"client_id": {config.ClientID}, "request_uri": {input.RequestURI}}.Encode()
	raw, ok := f.relayBrowserAuthorization(w, r, http.MethodGet, endpoint, nil)
	if !ok {
		return
	}
	var result struct {
		Status      string `json:"status"`
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if json.Unmarshal(raw, &result) != nil {
		writeJSON(w, 502, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "authorization provider returned an invalid browser status")))
		return
	}
	switch result.Status {
	case "pending":
		writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
	case "completed":
		if result.Code == "" || result.RedirectURI == "" {
			writeJSON(w, 502, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "authorization provider omitted browser proof")))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "completed", "code": result.Code, "redirectURI": result.RedirectURI})
	default:
		writeJSON(w, http.StatusUnauthorized, kernel.FaultJSON(kernel.Fail(kernel.ErrUnauthenticated, "browser authorization expired or was rejected; sign in again")))
	}
}

func oauthURLSafe(s string) bool {
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (f *httpFacade) relayBrowserAuthorization(w http.ResponseWriter, r *http.Request, method, endpoint string, form url.Values) ([]byte, bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if r.Header.Get("Authorization") != "" || r.Header.Get("X-Kc-As") != "" {
		writeJSON(w, 400, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "browser authorization does not accept caller identity headers")))
		return nil, false
	}
	request, err := http.NewRequestWithContext(r.Context(), method, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		writeJSON(w, 503, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "authorization endpoint is unavailable")))
		return nil, false
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	transport := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := transport.Do(request)
	if err != nil {
		writeJSON(w, 503, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "authorization provider is unavailable")))
		return nil, false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, 502, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "authorization provider rejected the browser request")))
		return nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		writeJSON(w, 502, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "authorization provider response is unavailable")))
		return nil, false
	}
	return raw, true
}
