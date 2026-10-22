package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kc/kernel"
)

// legacyTaihuTokenResult is the provider's OAuth token response shape.
type legacyTaihuTokenResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// This retired direct OAuth exchange is legacy contract material. Current
// products use client.IdentityService.ExchangeToken through the Server broker;
// TestBrowserCodeCompletesThroughBrokerWithoutClientSecret verifies that path.
func legacyExchangeTaihuAccessToken(pending legacyPendingAuth, code, redirectURI, clientSecret string, httpClient *http.Client) (legacyTaihuTokenResult, error) {
	var zero legacyTaihuTokenResult
	clientSecret = strings.TrimSpace(clientSecret)
	if clientSecret == "" {
		return zero, kernel.Fail(kernel.ErrUsageInvalid, "Taihu token exchange requires KC_SERVICE_CLIENT_SECRET")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	tokenBody := fmt.Sprintf(
		"grant_type=authorization_code&code=%s&client_id=%s&client_secret=%s&redirect_uri=%s&code_verifier=%s",
		url.QueryEscape(code), url.QueryEscape(pending.ClientID), url.QueryEscape(clientSecret),
		url.QueryEscape(redirectURI), url.QueryEscape(pending.CodeVerifier),
	)
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(pending.OAuth2Base, "/")+"/oauth2/token", strings.NewReader(tokenBody))
	if err != nil {
		return zero, fmt.Errorf("token exchange with Taihu failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(pending.ClientID, clientSecret)
	resp, err := httpClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("token exchange with Taihu failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return zero, fmt.Errorf("read token response: %v", err)
	}
	var tokenResult legacyTaihuTokenResult
	if err := json.Unmarshal(body, &tokenResult); err != nil {
		return zero, fmt.Errorf("taihu token HTTP %d: unexpected response", resp.StatusCode)
	}
	if tokenResult.AccessToken == "" {
		if tokenResult.Error != "" {
			return zero, fmt.Errorf("taihu token HTTP %d: %s: %s", resp.StatusCode, tokenResult.Error, tokenResult.ErrorDesc)
		}
		return zero, fmt.Errorf("taihu token HTTP %d: empty access_token", resp.StatusCode)
	}
	return tokenResult, nil
}

type legacyPendingAuth struct{ ClientID, CodeVerifier, OAuth2Base string }
