package taihu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kc/identity"
	"kc/kernel"
)

// Application is the deployment's registered Taihu login application. Taihu
// IAM supports authorization_code and refresh_token grants only, so the
// service gets no machine token: the client_id names the service, and
// client_secret stays on the Server for introspection and this relay. Snapshot
// provisioning uses the Store's separate service credential; a user's KC token
// is never a downstream storage credential.
type Application struct {
	ClientID     string
	ClientSecret string
	// OAuth2Base is the deployment's Taihu OAuth2 base URL.
	OAuth2Base string
	Resource   string
	Scope      string
	AppName    string
}

func (Application) String() string { return "<redacted Taihu application>" }

// tokenResult is the Taihu OAuth token response.
type tokenResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

type broker struct {
	app    Application
	login  identity.BrowserLogin
	client *http.Client
}

// NewBroker returns the stateless login relay for one application, or nil
// when the application is incomplete. The relay never accepts an upstream,
// application identity or secret from its caller.
func NewBroker(app Application, client *http.Client) identity.LoginBroker {
	if app.ClientID == "" || app.ClientSecret == "" {
		return nil
	}
	u, err := url.Parse(app.OAuth2Base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	owned := *client
	owned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	login := identity.BrowserLogin{OAuth2Base: strings.TrimRight(app.OAuth2Base, "/"), ClientID: app.ClientID, Resource: app.Resource, AppName: app.AppName, Scope: app.Scope}
	if login.Resource == "" {
		login.Resource = app.ClientID
	}
	if login.Scope == "" {
		login.Scope = "openid profile"
	}
	return &broker{app: app, login: login, client: &owned}
}

func failure(status int, code kernel.ErrorCode, message string) error {
	return &identity.BrokerFailure{Status: status, Err: kernel.Fail(code, "%s", message)}
}

func (b *broker) BrowserLogin() identity.BrowserLogin { return b.login }

// Token exchanges a PKCE-protected code or refreshes a credential. The
// authorization server validates the proof; no KC session or grant is created.
func (b *broker) Token(ctx context.Context, grant identity.TokenGrant) (identity.Token, error) {
	form := url.Values{"grant_type": {grant.GrantType}, "client_id": {b.login.ClientID}, "client_secret": {b.app.ClientSecret}, "resource": {b.login.Resource}}
	switch grant.GrantType {
	case "authorization_code":
		if grant.Code == "" || len(grant.CodeVerifier) < 43 || len(grant.CodeVerifier) > 128 || grant.RedirectURI == "" || grant.RefreshToken != "" {
			return identity.Token{}, failure(http.StatusBadRequest, kernel.ErrUsageInvalid, "authorization_code requires code, PKCE verifier and redirectURI")
		}
		for _, c := range grant.CodeVerifier {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", c)) {
				return identity.Token{}, failure(http.StatusBadRequest, kernel.ErrUsageInvalid, "invalid PKCE verifier")
			}
		}
		form.Set("code", grant.Code)
		form.Set("code_verifier", grant.CodeVerifier)
		form.Set("redirect_uri", grant.RedirectURI)
	case "refresh_token":
		if grant.RefreshToken == "" || grant.Code != "" || grant.CodeVerifier != "" || grant.RedirectURI != "" {
			return identity.Token{}, failure(http.StatusBadRequest, kernel.ErrUsageInvalid, "refresh_token requires only a refresh credential")
		}
		form.Set("refresh_token", grant.RefreshToken)
	default:
		return identity.Token{}, failure(http.StatusBadRequest, kernel.ErrUsageInvalid, "unsupported token grant")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.login.OAuth2Base+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return identity.Token{}, failure(http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable, "invalid deployment authorization endpoint")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(b.login.ClientID, b.app.ClientSecret)
	response, err := b.client.Do(request)
	if err != nil {
		return identity.Token{}, failure(http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable, "authorization provider is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests {
			return identity.Token{}, failure(http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable, "authorization provider is temporarily unavailable")
		}
		return identity.Token{}, failure(http.StatusUnauthorized, kernel.ErrUnauthenticated, "authorization proof was rejected; run kc login again")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	var result tokenResult
	if err != nil || len(raw) > 1<<20 || json.Unmarshal(raw, &result) != nil || result.AccessToken == "" || result.ExpiresIn <= 0 || result.Error != "" {
		return identity.Token{}, failure(http.StatusBadGateway, kernel.ErrTemporaryUnavailable, "authorization provider returned an invalid credential")
	}
	return identity.Token{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken, ExpiresIn: result.ExpiresIn}, nil
}

// Authorize starts the same stateless PAR/PKCE flow the CLI uses. Relaying
// only deployment-owned endpoints avoids an IdP CORS dependency.
func (b *broker) Authorize(ctx context.Context, codeChallenge, state string) (identity.Authorization, error) {
	if len(codeChallenge) != 43 || !urlSafe(codeChallenge) || len(state) < 32 || len(state) > 128 || !urlSafe(state) {
		return identity.Authorization{}, failure(http.StatusBadRequest, kernel.ErrUsageInvalid, "browser authorization requires a SHA256 PKCE challenge and random state")
	}
	form := url.Values{"client_id": {b.login.ClientID}, "response_type": {"code"}, "code_challenge": {codeChallenge}, "code_challenge_method": {"S256"}, "state": {state}, "scope": {b.login.Scope}, "resource": {b.login.Resource}, "app_name": {b.login.AppName}}
	raw, err := b.relay(ctx, http.MethodPost, b.login.OAuth2Base+"/oauth2/par", form)
	if err != nil {
		return identity.Authorization{}, err
	}
	var result struct {
		RequestURI string `json:"request_uri"`
		ExpiresIn  int    `json:"expires_in"`
	}
	if json.Unmarshal(raw, &result) != nil || result.RequestURI == "" || result.ExpiresIn <= 0 {
		return identity.Authorization{}, failure(http.StatusBadGateway, kernel.ErrTemporaryUnavailable, "authorization provider returned an invalid browser request")
	}
	authURL := b.login.OAuth2Base + "/oauth2/authorize?" + url.Values{"client_id": {b.login.ClientID}, "request_uri": {result.RequestURI}}.Encode()
	return identity.Authorization{URL: authURL, RequestURI: result.RequestURI, ExpiresIn: result.ExpiresIn}, nil
}

// PollAuthorization reports one pending browser authorization.
func (b *broker) PollAuthorization(ctx context.Context, requestURI string) (identity.AuthorizationStatus, error) {
	if requestURI == "" || len(requestURI) > 2048 {
		return identity.AuthorizationStatus{}, failure(http.StatusBadRequest, kernel.ErrUsageInvalid, "requestURI is required and must be bounded")
	}
	endpoint := b.login.OAuth2Base + "/oauth2/par/poll?" + url.Values{"client_id": {b.login.ClientID}, "request_uri": {requestURI}}.Encode()
	raw, err := b.relay(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return identity.AuthorizationStatus{}, err
	}
	var result struct {
		Status      string `json:"status"`
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return identity.AuthorizationStatus{}, failure(http.StatusBadGateway, kernel.ErrTemporaryUnavailable, "authorization provider returned an invalid browser status")
	}
	switch result.Status {
	case "pending":
		return identity.AuthorizationStatus{Status: "pending"}, nil
	case "completed":
		if result.Code == "" || result.RedirectURI == "" {
			return identity.AuthorizationStatus{}, failure(http.StatusBadGateway, kernel.ErrTemporaryUnavailable, "authorization provider omitted browser proof")
		}
		return identity.AuthorizationStatus{Status: "completed", Code: result.Code, RedirectURI: result.RedirectURI}, nil
	default:
		return identity.AuthorizationStatus{}, failure(http.StatusUnauthorized, kernel.ErrUnauthenticated, "browser authorization expired or was rejected; sign in again")
	}
}

func urlSafe(s string) bool {
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (b *broker) relay(ctx context.Context, method, endpoint string, form url.Values) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, failure(http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable, "authorization endpoint is unavailable")
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, err := b.client.Do(request)
	if err != nil {
		return nil, failure(http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable, "authorization provider is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, failure(http.StatusBadGateway, kernel.ErrTemporaryUnavailable, "authorization provider rejected the browser request")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, failure(http.StatusBadGateway, kernel.ErrTemporaryUnavailable, "authorization provider response is unavailable")
	}
	return raw, nil
}
