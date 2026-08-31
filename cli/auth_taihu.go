package cli

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"kc/kernel"
)

// TaihuAuthenticator validates the x-tai-identity header injected by the
// Taihu API gateway. The gateway handles OAuth2 token validation; the backend
// service only needs to verify the HMAC-signed identity header.
//
// Two modes are supported:
//  1. x-tai-identity validation (default) — the gateway injects a signed
//     identity JSON header; the authenticator verifies the HMAC signature.
//  2. Token introspection — validates the Bearer token against Taihu's
//     OAuth2 introspection endpoint (used when the server isn't behind the
//     Taihu gateway).
type TaihuAuthenticator struct {
	// hmacSecret is the shared secret used to verify x-tai-identity HMAC.
	// When empty, HMAC verification is skipped (development mode).
	hmacSecret []byte

	// introspectionURL is the Taihu OAuth2 introspection endpoint.
	// When set, Authorization header is validated via token introspection.
	introspectionURL string

	// clientID / clientSecret for token introspection (if the introspection
	// endpoint requires client authentication).
	clientID     string
	clientSecret string

	client *http.Client
}

// taihuIdentity is the expected JSON structure of the x-tai-identity header.
type taihuIdentity struct {
	StaffID   string `json:"staff_id"`
	UserName  string `json:"user_name"`
	NameCN    string `json:"name_cn"`
	DeptName  string `json:"dept_name"`
	StaffType string `json:"staff_type"`
	Exp       int64  `json:"exp"`
}

// NewTaihuAuthenticator creates a Taihu authenticator.
//   - hmacSecretHex: hex-encoded HMAC secret for x-tai-identity verification.
//     Empty skips HMAC verification (development mode).
//   - baseURL: Taihu OAuth2 base URL (e.g. http://iam.it.woa.com). Empty disables
//     token introspection and relies solely on x-tai-identity.
//   - clientID / clientSecret: optional credentials for introspection endpoint
//     authentication (Basic Auth).
func NewTaihuAuthenticator(hmacSecretHex string, baseURL string, clientID string, clientSecret string, client *http.Client) (*TaihuAuthenticator, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	a := &TaihuAuthenticator{client: client}

	if hmacSecretHex != "" {
		secret, err := hex.DecodeString(strings.TrimSpace(hmacSecretHex))
		if err != nil {
			return nil, fmt.Errorf("taihu HMAC secret must be hex-encoded: %v", err)
		}
		a.hmacSecret = secret
	}

	if baseURL != "" {
		base := strings.TrimRight(baseURL, "/")
		// Strip trailing /oauth2 if the caller already provided it so the
		// introspection endpoint is appended exactly once.
		base = strings.TrimSuffix(base, "/oauth2")
		a.introspectionURL = base + "/oauth2/introspect"
	}
	a.clientID = clientID
	a.clientSecret = clientSecret

	return a, nil
}

func (a *TaihuAuthenticator) Name() string { return "taihu" }

func (a *TaihuAuthenticator) Authenticate(ctx context.Context, headers http.Header) (HTTPIdentity, error) {
	// Strategy 1: x-tai-identity header (gateway-injected, preferred)
	if identity := headers.Get("X-Tai-Identity"); identity != "" {
		return a.authenticateFromIdentity(ctx, identity)
	}

	// Strategy 2: x-tai-user header (simpler gateway-injected user ID)
	if user := headers.Get("X-Tai-User"); user != "" {
		user = strings.TrimSpace(user)
		if user == "" {
			return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "empty x-tai-user header")
		}
		return HTTPIdentity{
			Principal: "taihu:" + user,
			Provider:  "taihu",
			Subject:   user,
			Login:     user,
		}, nil
	}

	// Strategy 3: Token introspection (direct Bearer token)
	if a.introspectionURL != "" {
		return a.authenticateFromToken(ctx, headers.Get("Authorization"))
	}

	// Fallback: Authorization header as raw token (development mode)
	authorization := strings.TrimSpace(headers.Get("Authorization"))
	if authorization != "" {
		parts := strings.Fields(authorization)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			// In development mode, accept the token as-is and use it as
			// principal. Production deployments must use x-tai-identity or
			// token introspection.
			return HTTPIdentity{
				Principal: "taihu:dev:" + parts[1][:min(8, len(parts[1]))],
				Provider:  "taihu",
				Subject:   parts[1],
				Login:     "developer",
			}, nil
		}
	}

	return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "missing taihu identity; expected x-tai-identity, x-tai-user, or Authorization header")
}

func (a *TaihuAuthenticator) authenticateFromIdentity(ctx context.Context, raw string) (HTTPIdentity, error) {
	// x-tai-identity format: base64(json).hmac or just base64(json)
	parts := strings.SplitN(strings.TrimSpace(raw), ".", 2)

	var payload []byte
	var err error

	if len(parts) == 2 {
		// HMAC-signed format: base64(payload).hex(hmac)
		payload, err = base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil {
			return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: malformed x-tai-identity payload")
		}
		if a.hmacSecret != nil {
			if err := verifyHMAC(payload, parts[1], a.hmacSecret); err != nil {
				return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: invalid x-tai-identity signature")
			}
		}
	} else {
		// Plain base64 format
		payload, err = base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			payload = []byte(raw)
		}
	}

	var identity taihuIdentity
	if err := json.Unmarshal(payload, &identity); err != nil {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: invalid x-tai-identity JSON")
	}

	if identity.StaffID == "" {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: x-tai-identity missing staff_id")
	}

	// Check expiration
	if identity.Exp > 0 && time.Now().Unix() > identity.Exp {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: x-tai-identity expired")
	}

	return HTTPIdentity{
		Principal: "taihu:" + identity.StaffID,
		Provider:  "taihu",
		Subject:   identity.StaffID,
		Login:     identity.UserName,
		Admin:     false, // Admin status cannot be determined from x-tai-identity
	}, nil
}

func (a *TaihuAuthenticator) authenticateFromToken(ctx context.Context, authorization string) (HTTPIdentity, error) {
	parts := strings.Fields(strings.TrimSpace(authorization))
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: missing or malformed Authorization header")
	}

	token := parts[1]

	// Call Taihu token introspection endpoint
	body := fmt.Sprintf(`{"token":"%s","token_type_hint":"access_token"}`, token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.introspectionURL, strings.NewReader(body))
	if err != nil {
		return HTTPIdentity{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.clientID != "" && a.clientSecret != "" {
		req.SetBasicAuth(a.clientID, a.clientSecret)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "taihu introspection: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "taihu introspection returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Active   bool   `json:"active"`
		Sub      string `json:"sub"`
		Subject  string `json:"subject"`
		Username string `json:"username"`
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "taihu introspection response: %v", err)
	}

	if !result.Active {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: token is not active")
	}

	subject := result.Subject
	if subject == "" {
		subject = result.Sub
	}
	if subject == "" {
		subject = result.ClientID
	}

	return HTTPIdentity{
		Principal: "taihu:" + subject,
		Provider:  "taihu",
		Subject:   subject,
		Login:     result.Username,
	}, nil
}

func verifyHMAC(payload []byte, signatureHex string, secret []byte) error {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signatureHex)) {
		return fmt.Errorf("HMAC mismatch")
	}
	return nil
}

func init() { RegisterTaihuAuthenticator() }

// RegisterTaihuAuthenticator registers the Taihu authenticator factory.
func RegisterTaihuAuthenticator() {
	RegisterAuthenticator("taihu", func(flags map[string]FlagValue) (HTTPAuthenticator, error) {
		url := strings.TrimSpace(FlagString(flags, "auth-url"))
		secret := strings.TrimSpace(FlagString(flags, "auth-hmac-secret")) // HMAC secret hex
		clientID := strings.TrimSpace(FlagString(flags, "service-client-id"))
		clientSecret := strings.TrimSpace(FlagString(flags, "service-client-secret"))
		if clientSecret == "" {
			clientSecret = strings.TrimSpace(os.Getenv("KC_SERVICE_CLIENT_SECRET"))
		}

		if url == "" {
			// x-tai-identity only mode: behind Taihu gateway, no introspection
			return NewTaihuAuthenticator(secret, "", clientID, clientSecret, nil)
		}
		url = strings.TrimRight(url, "/")
		// Strip trailing /oauth2 if present so authURL works as base
		url = strings.TrimSuffix(url, "/oauth2")
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return nil, fmt.Errorf("--auth-url must be a Taihu http(s) origin")
		}
		return NewTaihuAuthenticator(secret, url, clientID, clientSecret, nil)
	})
}
