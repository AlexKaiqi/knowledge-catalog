package server

import (
	"context"
	"fmt"
	"io"
	inv "kc/invocation"
	"kc/invoke"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kc/identity"
	"kc/kernel"
)

// GiteaAuthenticator validates the request's Authorization header against
// Gitea's current-user endpoint. Incoming credentials are never reused as the
// server-side KC_GITEA_TOKEN used by repository adapters.
type GiteaAuthenticator struct {
	origin string
	client *http.Client
}

type giteaAuthUser struct {
	ID      int64  `json:"id"`
	Login   string `json:"login"`
	Active  bool   `json:"active"`
	IsAdmin bool   `json:"is_admin"`
}

func NewGiteaAuthenticator(origin string, client *http.Client) (*GiteaAuthenticator, error) {
	origin, err := verifierOrigin(origin, "Gitea")
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &GiteaAuthenticator{origin: origin, client: client}, nil
}

func (a *GiteaAuthenticator) Name() string { return "gitea" }

func (a *GiteaAuthenticator) Authenticate(ctx context.Context, headers http.Header) (inv.HTTPIdentity, error) {
	authorization := strings.TrimSpace(headers.Get("Authorization"))
	parts := strings.Fields(authorization)
	if len(parts) != 2 {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "missing or malformed Authorization header")
	}
	switch strings.ToLower(parts[0]) {
	case "bearer", "token", "basic":
	default:
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "unsupported Authorization scheme")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.origin+"/api/v1/user", nil)
	if err != nil {
		return inv.HTTPIdentity{}, err
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea authentication: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "Gitea rejected the credentials")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea authentication returned HTTP %d", resp.StatusCode)
	}
	var user giteaAuthUser
	if err := decodeVerifierUser(resp.Body, &user, false); err != nil {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea authentication response: %v", err)
	}
	if user.ID <= 0 || strings.TrimSpace(user.Login) == "" || !user.Active {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "Gitea account is missing or inactive")
	}
	subject := strconv.FormatInt(user.ID, 10)
	username, err := identity.CanonicalUsername(user.Login)
	if err != nil {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "Gitea username is not a valid KC username: %v", err)
	}
	return inv.HTTPIdentity{
		Principal: username,
		Provider:  "gitea",
		Subject:   subject,
		Login:     username,
		Admin:     user.IsAdmin,
		User:      &identity.VerifiedUser{Username: username, Provider: "gitea", Issuer: a.origin, Subject: subject},
	}, nil
}

func init() { RegisterGiteaAuthenticator() }

// RegisterGiteaAuthenticator registers the Gitea authenticator factory.
// Called from an init function or explicitly during server startup.
func RegisterGiteaAuthenticator() {
	inv.RegisterAuthenticator("gitea", func(flags map[string]FlagValue) (inv.HTTPAuthenticator, error) {
		url := strings.TrimSpace(invoke.FlagString(flags, "auth-url"))
		if url == "" {
			return nil, fmt.Errorf("--auth-url is required")
		}
		return NewGiteaAuthenticator(url, nil)
	})
}
