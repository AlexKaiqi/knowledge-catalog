package server

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kc/identity"
	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
)

// LakeFSAuthenticator verifies caller proof with the deployment's native
// lakeFS service. Snapshot adapter credentials and native roles are not used.
type LakeFSAuthenticator struct {
	origin string
	client *http.Client
}

func NewLakeFSAuthenticator(origin string, client *http.Client) (*LakeFSAuthenticator, error) {
	origin, err := verifierOrigin(origin, "lakeFS")
	if err != nil {
		return nil, err
	}
	owned := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		owned = *client
	}
	if owned.Timeout == 0 {
		owned.Timeout = 10 * time.Second
	}
	owned.Jar = nil
	owned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &LakeFSAuthenticator{origin: origin, client: &owned}, nil
}

func (a *LakeFSAuthenticator) Name() string { return "lakefs" }

func (a *LakeFSAuthenticator) Authenticate(ctx context.Context, headers http.Header) (inv.HTTPIdentity, error) {
	proof, err := lakeFSCallerProof(headers)
	if err != nil {
		return inv.HTTPIdentity{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.origin+"/api/v1/user", nil)
	if err != nil {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakeFS authentication unavailable")
	}
	req.Header = proof
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakeFS authentication unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "lakeFS rejected the credentials")
	}
	if resp.StatusCode != http.StatusOK {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakeFS authentication returned HTTP %d", resp.StatusCode)
	}
	var response struct {
		User struct {
			ID           string `json:"id"`
			CreationDate int64  `json:"creation_date"`
		} `json:"user"`
	}
	if err := decodeVerifierUser(resp.Body, &response, true); err != nil {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid lakeFS authentication response")
	}
	if response.User.ID == "" || response.User.CreationDate <= 0 {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "lakeFS user has invalid binding coordinates")
	}
	username, err := identity.CanonicalUsername(response.User.ID)
	if err != nil {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrForbidden, "native lakeFS account ID is incompatible with the KC username contract; explicit identity association is required")
	}
	// The native API exposes creation time in seconds, not an immutable account
	// UUID. Deployments must explicitly manage same-name account recycling.
	subject := username + ":" + strconv.FormatInt(response.User.CreationDate, 10)
	return inv.HTTPIdentity{Principal: username, Login: username, Provider: "lakefs", Subject: subject,
		User: &identity.VerifiedUser{Username: username, Provider: "lakefs", Issuer: a.origin, Subject: subject}}, nil
}

func lakeFSCallerProof(headers http.Header) (http.Header, error) {
	unauthenticated := func(message string) (http.Header, error) {
		return nil, kernel.Fail(kernel.ErrUnauthenticated, "%s", message)
	}
	authorizations := headers.Values("Authorization")
	if len(authorizations) > 0 {
		if len(authorizations) != 1 || len(headers.Values("Cookie")) != 0 {
			return unauthenticated("ambiguous lakeFS caller credentials")
		}
		parts := strings.Fields(authorizations[0])
		if len(parts) != 2 || (!strings.EqualFold(parts[0], "Basic") && !strings.EqualFold(parts[0], "Bearer")) {
			return unauthenticated("missing or unsupported lakeFS Authorization proof")
		}
		return http.Header{"Authorization": {authorizations[0]}}, nil
	}
	var session *http.Cookie
	for _, cookie := range (&http.Request{Header: headers}).Cookies() {
		switch cookie.Name {
		case "internal_auth_session", "oidc_auth_session", "saml_auth_session":
			if session != nil || cookie.Value == "" {
				return unauthenticated("ambiguous or empty lakeFS session proof")
			}
			session = cookie
		}
	}
	if session == nil {
		return unauthenticated("missing lakeFS caller credentials")
	}
	if err := lakeFSConsoleHeaders(headers); err != nil {
		return nil, err
	}
	return http.Header{"Cookie": {session.String()}}, nil
}

func lakeFSConsoleHeaders(headers http.Header) error {
	markers := headers.Values("X-Kc-Console")
	sites := headers.Values("Sec-Fetch-Site")
	if len(markers) != 1 || markers[0] != "1" || len(sites) > 1 {
		return kernel.Fail(kernel.ErrForbidden, "lakeFS console session requires same-origin request proof")
	}
	switch headers.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return nil
	default:
		return kernel.Fail(kernel.ErrForbidden, "lakeFS console session rejects cross-origin requests")
	}
}

func validateLakeFSConsoleRequest(r *http.Request) error {
	if r.Header.Get("Authorization") != "" || len(r.Header.Values("Cookie")) == 0 {
		return nil
	}
	if err := lakeFSConsoleHeaders(r.Header); err != nil {
		return err
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return nil
	}
	contentTypes := r.Header.Values("Content-Type")
	if len(contentTypes) == 1 {
		mediaType, _, err := mime.ParseMediaType(contentTypes[0])
		if err == nil && mediaType == "application/json" {
			return nil
		}
	}
	return kernel.Fail(kernel.ErrForbidden, "lakeFS console session mutations require application/json")
}

func init() {
	inv.RegisterAuthenticator("lakefs", func(flags map[string]FlagValue) (inv.HTTPAuthenticator, error) {
		origin := strings.TrimSpace(invoke.FlagString(flags, "auth-url"))
		if origin == "" {
			return nil, fmt.Errorf("--auth-url is required")
		}
		return NewLakeFSAuthenticator(origin, nil)
	})
}
