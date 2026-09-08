package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"kc/kernel"
)

// BrowserLoginConfig contains only public deployment-owned OAuth parameters.
// Its presence advertises a server token broker; no client application secret
// is distributed and callers cannot select the broker's upstream endpoint.
type BrowserLoginConfig struct {
	OAuth2Base string `json:"oauth2Base"`
	ClientID   string `json:"clientId"`
	Resource   string `json:"resource"`
	AppName    string `json:"appName,omitempty"`
	Scope      string `json:"scope,omitempty"`
}

// TokenRequest exchanges proof of browser authorization or refreshes an
// existing user credential. It creates no KC server session.
type TokenRequest struct {
	GrantType    string `json:"grantType"`
	Code         string `json:"code,omitempty"`
	CodeVerifier string `json:"codeVerifier,omitempty"`
	RedirectURI  string `json:"redirectURI,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

type TokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresIn    int    `json:"expiresIn"`
}

func (TokenRequest) String() string    { return "<redacted token request>" }
func (TokenRequest) GoString() string  { return "client.TokenRequest{<redacted>}" }
func (TokenResponse) String() string   { return "<redacted token response>" }
func (TokenResponse) GoString() string { return "client.TokenResponse{<redacted>}" }

// ExchangeToken uses only the fixed Identity endpoint and never sends current
// credentials as Authorization. Authorization is proven by the request body.
func (s IdentityService) ExchangeToken(ctx context.Context, input TokenRequest) (TokenResponse, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return TokenResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.client.baseURL+"/identity/v1/token", bytes.NewReader(raw))
	if err != nil {
		return TokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	// Tokens must not follow HTTP redirects, including same-host redirects
	// whose different path could target a different application.
	httpClient := *s.client.httpClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := httpClient.Do(request)
	if err != nil {
		return TokenResponse{}, err
	}
	defer response.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return TokenResponse{}, err
	}
	if len(raw) > maxResponseBytes {
		return TokenResponse{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "kc token response exceeds limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var envelope struct {
			Error *kernel.IngressError `json:"error"`
		}
		if json.Unmarshal(raw, &envelope) == nil && envelope.Error != nil {
			return TokenResponse{}, envelope.Error
		}
		return TokenResponse{}, kernel.Fail(kernel.ErrUnauthenticated, "kc token exchange returned HTTP %d; run kc login again", response.StatusCode)
	}
	var result TokenResponse
	if json.Unmarshal(raw, &result) != nil || result.AccessToken == "" || result.ExpiresIn <= 0 {
		return TokenResponse{}, kernel.Fail(kernel.ErrUnauthenticated, "kc token exchange returned an invalid credential")
	}
	return result, nil
}
