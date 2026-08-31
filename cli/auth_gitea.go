package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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
	origin = strings.TrimSpace(origin)
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("--auth-url must be a Gitea http(s) origin")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("--auth-url must be a Gitea http(s) origin")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("--auth-url must not contain credentials, query, or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(u.Path, "/api/v1") {
		return nil, fmt.Errorf("--auth-url is the Gitea origin, without /api/v1")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &GiteaAuthenticator{origin: strings.TrimRight(u.String(), "/"), client: client}, nil
}

func (a *GiteaAuthenticator) Name() string { return "gitea" }

func (a *GiteaAuthenticator) Authenticate(ctx context.Context, headers http.Header) (HTTPIdentity, error) {
	authorization := strings.TrimSpace(headers.Get("Authorization"))
	parts := strings.Fields(authorization)
	if len(parts) != 2 {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "missing or malformed Authorization header")
	}
	switch strings.ToLower(parts[0]) {
	case "bearer", "token", "basic":
	default:
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "unsupported Authorization scheme")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.origin+"/api/v1/user", nil)
	if err != nil {
		return HTTPIdentity{}, err
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea authentication: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "Gitea rejected the credentials")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea authentication returned HTTP %d", resp.StatusCode)
	}
	var user giteaAuthUser
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&user); err != nil {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea authentication response: %v", err)
	}
	if user.ID <= 0 || strings.TrimSpace(user.Login) == "" || !user.Active {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "Gitea account is missing or inactive")
	}
	subject := strconv.FormatInt(user.ID, 10)
	return HTTPIdentity{
		Principal: "gitea:" + subject,
		Provider:  "gitea",
		Subject:   subject,
		Login:     user.Login,
		Admin:     user.IsAdmin,
	}, nil
}

func init() { RegisterGiteaAuthenticator() }

// RegisterGiteaAuthenticator registers the Gitea authenticator factory.
// Called from an init function or explicitly during server startup.
func RegisterGiteaAuthenticator() {
	RegisterAuthenticator("gitea", func(flags map[string]FlagValue) (HTTPAuthenticator, error) {
		url := strings.TrimSpace(FlagString(flags, "auth-url"))
		if url == "" {
			return nil, fmt.Errorf("--auth-url is required")
		}
		return NewGiteaAuthenticator(url, nil)
	})
}
