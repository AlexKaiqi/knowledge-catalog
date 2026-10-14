package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kc/identity"
	"kc/kernel"
)

// This retired direct OAuth exchange is legacy contract material. Current
// products use client.IdentityService.ExchangeToken through the Server broker;
// TestBrowserCodeCompletesThroughBrokerWithoutClientSecret verifies that path.
func legacyExchangeTaihuAccessToken(pending taihuPendingAuth, code, redirectURI, clientSecret string, httpClient *http.Client) (identity.TaihuTokenResult, error) {
	var zero identity.TaihuTokenResult
	clientSecret = strings.TrimSpace(clientSecret)
	if clientSecret == "" {
		return zero, kernel.Fail(kernel.ErrUsageInvalid, "Taihu token exchange requires KC_SERVICE_CLIENT_SECRET")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	tokenBody := fmt.Sprintf(
		"grant_type=authorization_code&code=%s&client_id=%s&client_secret=%s&redirect_uri=%s&code_verifier=%s",
		urlEncode(code), urlEncode(pending.ClientID), urlEncode(clientSecret),
		urlEncode(redirectURI), urlEncode(pending.CodeVerifier),
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
	var tokenResult identity.TaihuTokenResult
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
