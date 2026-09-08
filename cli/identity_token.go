package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	kcclient "kc/client"
	"kc/kernel"
)

func (f *httpFacade) browserLoginConfig() *kcclient.BrowserLoginConfig {
	s := f.options.ServiceIdentity
	if f.options.authMode() != "taihu" || s == nil || s.ClientID == "" || s.ClientSecret == "" {
		return nil
	}
	u, err := url.Parse(s.OAuth2Base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil
	}
	resource := s.Resource
	if resource == "" {
		resource = s.ClientID
	}
	scope := s.Scope
	if scope == "" {
		scope = "openid profile"
	}
	return &kcclient.BrowserLoginConfig{OAuth2Base: strings.TrimRight(s.OAuth2Base, "/"), ClientID: s.ClientID, Resource: resource, AppName: s.AppName, Scope: scope}
}

// identityToken is a stateless broker: the authorization server validates the
// code/PKCE proof or refresh credential. No upstream URL, secret or application
// identity comes from the caller, and no KC session or grant is created here.
func (f *httpFacade) identityToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	fail := func(status int, code kernel.ErrorCode, message string) {
		writeJSON(w, status, kernel.FaultJSON(kernel.Fail(code, message)))
	}
	config := f.browserLoginConfig()
	if config == nil {
		fail(http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable, "browser login is not configured for this deployment")
		return
	}
	if r.Header.Get("Authorization") != "" || r.Header.Get("X-Kc-As") != "" {
		fail(http.StatusBadRequest, kernel.ErrUsageInvalid, "token exchange accepts authorization proof in its request body only")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input kcclient.TokenRequest
	if !decodeServiceRequest(w, r, &input) {
		return
	}
	form := url.Values{"grant_type": {input.GrantType}, "client_id": {config.ClientID}, "client_secret": {f.options.ServiceIdentity.ClientSecret}, "resource": {config.Resource}}
	switch input.GrantType {
	case "authorization_code":
		if input.Code == "" || len(input.CodeVerifier) < 43 || len(input.CodeVerifier) > 128 || input.RedirectURI == "" || input.RefreshToken != "" {
			fail(http.StatusBadRequest, kernel.ErrUsageInvalid, "authorization_code requires code, PKCE verifier and redirectURI")
			return
		}
		for _, c := range input.CodeVerifier {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", c)) {
				fail(http.StatusBadRequest, kernel.ErrUsageInvalid, "invalid PKCE verifier")
				return
			}
		}
		form.Set("code", input.Code)
		form.Set("code_verifier", input.CodeVerifier)
		form.Set("redirect_uri", input.RedirectURI)
	case "refresh_token":
		if input.RefreshToken == "" || input.Code != "" || input.CodeVerifier != "" || input.RedirectURI != "" {
			fail(http.StatusBadRequest, kernel.ErrUsageInvalid, "refresh_token requires only a refresh credential")
			return
		}
		form.Set("refresh_token", input.RefreshToken)
	default:
		fail(http.StatusBadRequest, kernel.ErrUsageInvalid, "unsupported token grant")
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, config.OAuth2Base+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		fail(http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable, "invalid deployment authorization endpoint")
		return
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(config.ClientID, f.options.ServiceIdentity.ClientSecret)
	httpClient := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := httpClient.Do(request)
	if err != nil {
		fail(http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable, "authorization provider is unavailable")
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests {
			fail(http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable, "authorization provider is temporarily unavailable")
		} else {
			fail(http.StatusUnauthorized, kernel.ErrUnauthenticated, "authorization proof was rejected; run kc login again")
		}
		return
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	var result taihuTokenResult
	if err != nil || len(raw) > 1<<20 || json.Unmarshal(raw, &result) != nil || result.AccessToken == "" || result.ExpiresIn <= 0 || result.Error != "" {
		fail(http.StatusBadGateway, kernel.ErrTemporaryUnavailable, "authorization provider returned an invalid credential")
		return
	}
	writeJSON(w, http.StatusOK, kcclient.TokenResponse{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken, ExpiresIn: result.ExpiresIn})
}
