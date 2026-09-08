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
	"net/url"
	"os"
	"strings"
	"time"

	kcidentity "kc/identity"
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
	// When empty, gateway identity headers are not accepted.
	hmacSecret []byte

	// introspectionURL is the Taihu OAuth2 introspection endpoint.
	// When set, Authorization header is validated via token introspection.
	introspectionURL string

	// clientID / clientSecret for token introspection (if the introspection
	// endpoint requires client authentication).
	clientID     string
	clientSecret string

	client *http.Client
	issuer string
}

// taihuIdentity is the expected JSON structure of the x-tai-identity header.
type taihuIdentity struct {
	StaffID   string `json:"staff_id"`
	UserName  string `json:"user_name"`
	Username  string `json:"username"`
	NameCN    string `json:"name_cn"`
	DeptName  string `json:"dept_name"`
	StaffType string `json:"staff_type"`
	Exp       int64  `json:"exp"`
}

func (identity taihuIdentity) login() string {
	if name := identity.UserName; name != "" {
		return name
	}
	return identity.Username
}

// NewTaihuAuthenticator creates a Taihu authenticator.
//   - hmacSecretHex: hex-encoded HMAC secret for x-tai-identity verification.
//     Empty disables gateway identity headers; direct introspection may remain enabled.
//   - baseURL: trusted Taihu issuer base URL. Gateway identities also require
//     this declaration so switching trust domains cannot retain a username.
//     Empty disables introspection and cannot establish a gateway user binding.
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
		parsed, err := url.Parse(base)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "Taihu auth URL must declare a trusted HTTP(S) issuer without credentials, query or fragment")
		}
		a.introspectionURL = base + "/oauth2/introspect"
		a.issuer = base
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

	// A bare username has no signature or immutable subject, so it cannot
	// establish or recover a durable account binding.
	if user := headers.Get("X-Tai-User"); user != "" {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "x-tai-user is not a verified identity; use signed identity or token introspection")
	}

	// Strategy 3: Token introspection (direct Bearer token)
	if a.introspectionURL != "" {
		return a.authenticateFromToken(ctx, headers.Get("Authorization"))
	}

	return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated,
		"this Server is --auth taihu; missing verified identity (gateway x-tai-identity or --auth-url introspection)")
}

func (a *TaihuAuthenticator) authenticateFromIdentity(ctx context.Context, raw string) (HTTPIdentity, error) {
	// Only the HMAC-signed gateway format may create a username binding.
	parts := strings.SplitN(strings.TrimSpace(raw), ".", 2)
	if len(parts) != 2 || len(a.hmacSecret) == 0 {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: signed gateway identity and configured HMAC key are required")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: malformed x-tai-identity payload")
	}
	if err := verifyHMAC(payload, parts[1], a.hmacSecret); err != nil {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: invalid x-tai-identity signature")
	}
	if a.issuer == "" {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrPreconditionFailed, "signed Taihu gateway identities require a configured trusted auth URL")
	}

	var identity taihuIdentity
	if err := json.Unmarshal(payload, &identity); err != nil {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: invalid x-tai-identity JSON")
	}

	user, err := taihuVerifiedUser(identity.StaffID, identity.login(), a.issuer)
	if err != nil {
		return HTTPIdentity{}, err
	}

	// Check expiration
	if identity.Exp > 0 && time.Now().Unix() > identity.Exp {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: x-tai-identity expired")
	}

	return HTTPIdentity{
		Principal: user.Username,
		Provider:  "taihu",
		Subject:   user.Subject,
		Login:     user.Username,
		Admin:     false, // Admin status cannot be determined from x-tai-identity
		User:      user,
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
		Active   bool            `json:"active"`
		Sub      string          `json:"sub"`
		Subject  string          `json:"subject"`
		Username string          `json:"username"`
		ClientID string          `json:"client_id"`
		Act      json.RawMessage `json:"act"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "taihu introspection response: %v", err)
	}

	if !result.Active {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: token is not active")
	}

	return taihuIdentityFromIntrospection(result.Sub, result.Subject, result.ClientID, result.Username, result.Act, a.issuer)
}

func taihuVerifiedUser(subject, username, issuer string) (*kcidentity.VerifiedUser, error) {
	login, err := kcidentity.CanonicalUsername(username)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUnauthenticated, "taihu: invalid or missing username: %v", err)
	}
	if subject == "" || subject != strings.TrimSpace(subject) {
		return nil, kernel.Fail(kernel.ErrUnauthenticated, "taihu: user identity requires a stable subject")
	}
	return &kcidentity.VerifiedUser{Username: login, Provider: "taihu", Issuer: issuer, Subject: subject}, nil
}

func taihuIdentityFromIntrospection(sub, subject, clientID, username string, act json.RawMessage, issuer string) (HTTPIdentity, error) {
	staff := strings.TrimSpace(subject)
	if staff == "" {
		staff = strings.TrimSpace(sub)
	}
	login := username
	client := strings.TrimSpace(clientID)
	actor := taihuActor(act)
	switch {
	case staff != "" && actor != "":
		user, err := taihuVerifiedUser(staff, login, issuer)
		if err != nil {
			return HTTPIdentity{}, err
		}
		return HTTPIdentity{
			Principal:  prefixedPrincipal("agent", actor),
			OnBehalfOf: user.Username,
			Provider:   "taihu",
			Subject:    staff,
			Login:      login,
			User:       user,
		}, nil
	case staff != "" && staff != client:
		user, err := taihuVerifiedUser(staff, login, issuer)
		if err != nil {
			return HTTPIdentity{}, err
		}
		return HTTPIdentity{
			Principal: user.Username,
			Provider:  "taihu",
			Subject:   staff,
			Login:     login,
			User:      user,
		}, nil
	case client != "":
		return HTTPIdentity{
			Principal: prefixedPrincipal("service", client),
			Provider:  "taihu",
			Subject:   client,
			Login:     login,
		}, nil
	default:
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "taihu: token has no subject or client_id")
	}
}

func taihuActor(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var act struct {
		Sub string `json:"sub"`
	}
	if json.Unmarshal(raw, &act) != nil {
		return ""
	}
	return strings.TrimSpace(act.Sub)
}

func prefixedPrincipal(kind, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return kind
	}
	if strings.Contains(id, ":") {
		return id
	}
	return kind + ":" + id
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
		secret := strings.TrimSpace(FlagString(flags, "auth-hmac-secret"))
		if secret == "" {
			secret = strings.TrimSpace(os.Getenv("KC_TAIHU_HMAC_SECRET"))
		}
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
