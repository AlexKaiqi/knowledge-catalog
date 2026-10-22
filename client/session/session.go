package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	kcclient "kc/client/auth"
	"kc/internal/clientconfig"
	"kc/internal/httpjson"
	"kc/internal/jsonfile"
	"kc/kernel"
)

// clientEndpoint is non-secret packaging/login configuration. Sessions remain
// separate and each filename is scoped by the complete service base URL.
type clientEndpoint = clientconfig.Endpoint

// Selection contains only explicit caller identity selection. Environment and
// saved state are re-read on every request.
type Selection struct {
	Principal  string
	OnBehalfOf string
}

var tokenRefreshMu sync.Mutex

// Configure resolves current login state and supplies the authenticated transport
// configuration used by service SDKs. Its authenticator reloads state per request.
func Configure(ctx context.Context, server string, selection Selection) (kcclient.Config, error) {
	server = clientconfig.ServerURL(server)
	if selection.OnBehalfOf != "" {
		return kcclient.Config{}, kernel.Fail(kernel.ErrUsageInvalid, "remote delegation comes from the authenticator")
	}
	session, err := resolveRemoteSession(ctx, server, selection)
	if err != nil {
		return kcclient.Config{}, err
	}
	store := &kcclient.MemorySessionStore{}
	if err := store.Save(ctx, session); err != nil {
		return kcclient.Config{}, err
	}
	return kcclient.Config{BaseURL: server, HTTPClient: credentialHTTPClient(), Authenticator: remoteSessionAuthenticator{server: server, selection: selection}, Sessions: store}, nil
}

func newClient(ctx context.Context, server string, selection Selection) (*kcclient.Client, error) {
	config, err := Configure(ctx, server, selection)
	if err != nil {
		return nil, err
	}
	return kcclient.New(config)
}

func resolveRemoteSession(ctx context.Context, server string, selection Selection) (kcclient.Session, error) {
	principal := strings.TrimSpace(selection.Principal)
	if principal == "" {
		principal = strings.TrimSpace(os.Getenv("KC_AS"))
	}
	authentication := strings.TrimSpace(os.Getenv("KC_AUTH_TOKEN"))
	var saved taihuSession
	if authentication == "" {
		var ok bool
		var err error
		saved, ok, err = currentServerTokenSession(ctx, server)
		if err != nil {
			return kcclient.Session{}, err
		}
		if ok {
			authentication = "Bearer " + saved.AccessToken
		}
	}
	if authentication != "" {
		if principal != "" {
			return kcclient.Session{}, kernel.Fail(kernel.ErrUsageInvalid, "token pairing sends Authorization only; do not also set --as or KC_AS (pairing mismatch)")
		}
		principal = saved.Principal
		if principal == "" {
			principal = "token-user"
		}
		authentication, _ = bearerParts(authentication)
	} else if principal == "" {
		if saved, ok := loadLocalSession(server); ok {
			principal = saved.Principal
		}
	}
	if principal == "" {
		return kcclient.Session{}, kernel.Fail(kernel.ErrUnauthenticated, "remote kc requires an explicit principal or authenticated client session; run kc login")
	}
	return kcclient.Session{Identity: kcclient.Identity{Principal: principal}, Authentication: kcclient.Authentication{Authorization: authentication}}, nil
}

type remoteSessionAuthenticator struct {
	server    string
	selection Selection
}

func (remoteSessionAuthenticator) Login(_ context.Context, request kcclient.LoginRequest) (kcclient.Session, error) {
	return kcclient.Session(request), nil
}
func (remoteSessionAuthenticator) Logout(context.Context, kcclient.Session) error { return nil }
func (a remoteSessionAuthenticator) AuthenticateRequest(ctx context.Context, _ kcclient.Session, _ string, request *http.Request) error {
	base, err := url.Parse(a.server)
	if err != nil || request.URL == nil || base.Scheme != request.URL.Scheme || base.Host != request.URL.Host || (strings.TrimRight(base.Path, "/") != "" && request.URL.Path != strings.TrimRight(base.Path, "/") && !strings.HasPrefix(request.URL.Path, strings.TrimRight(base.Path, "/")+"/")) {
		return kernel.Fail(kernel.ErrUnauthenticated, "saved login is bound to the selected service")
	}
	session, err := resolveRemoteSession(ctx, a.server, a.selection)
	if err != nil {
		return err
	}
	request.Header.Del("Authorization")
	request.Header.Del("X-Kc-As")
	request.Header.Del("X-Kc-On-Behalf-Of")
	if session.Authentication.Authorization != "" {
		request.Header.Set("Authorization", session.Authentication.Authorization)
	} else {
		request.Header.Set("X-Kc-As", session.Identity.Principal)
	}
	return nil
}

func persistClientServer(server string) error {
	return jsonfile.WritePrivate(filepath.Join(clientconfig.ConfigDir(), "client.json"), clientEndpoint{Server: clientconfig.NormalizeServer(server)})
}

func persistTokenLogin(s taihuSession) error {
	if err := jsonfile.WritePrivate(clientconfig.SessionPath(s.Server, "session-taihu.json"), s); err != nil {
		return err
	}
	if err := persistClientServer(s.Server); err != nil {
		return err
	}
	return removeMatchingSession(s.Server, "session-local.json")
}

func removeMatchingSession(server, name string) error {
	if err := os.Remove(clientconfig.SessionPath(server, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	legacy := filepath.Join(clientconfig.ConfigDir(), name)
	raw, err := os.ReadFile(legacy)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var endpoint clientEndpoint
	if json.Unmarshal(raw, &endpoint) == nil && clientconfig.NormalizeServer(endpoint.Server) == clientconfig.NormalizeServer(server) {
		return os.Remove(legacy)
	}
	return nil
}

func clearServerSessions(server string) error {
	return errors.Join(removeMatchingSession(server, "session-taihu.json"), removeMatchingSession(server, "session-local.json"))
}

func readServerTokenSession(server string) (taihuSession, bool, error) {
	for _, path := range []string{clientconfig.SessionPath(server, "session-taihu.json"), filepath.Join(clientconfig.ConfigDir(), "session-taihu.json")} {
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return taihuSession{}, false, err
		}
		var session taihuSession
		if json.Unmarshal(raw, &session) != nil {
			return taihuSession{}, false, kernel.Fail(kernel.ErrUnauthenticated, "saved login is unreadable; run kc login again")
		}
		if strings.TrimSpace(session.Server) == "" || clientconfig.NormalizeServer(session.Server) != clientconfig.NormalizeServer(server) {
			continue
		}
		if session.AccessToken == "" {
			return taihuSession{}, false, kernel.Fail(kernel.ErrUnauthenticated, "saved login has no credential; run kc login again")
		}
		return session, true, nil
	}
	return taihuSession{}, false, nil
}

func currentServerTokenSession(ctx context.Context, server string) (taihuSession, bool, error) {
	tokenRefreshMu.Lock()
	defer tokenRefreshMu.Unlock()
	session, ok, err := readServerTokenSession(server)
	if err != nil || !ok || session.ExpiresAt.IsZero() || time.Now().Before(session.ExpiresAt) {
		return session, ok, err
	}
	if session.RefreshToken == "" {
		return taihuSession{}, false, kernel.Fail(kernel.ErrUnauthenticated, "saved login expired; run kc login again for %s", clientconfig.NormalizeServer(server))
	}
	client, err := kcclient.New(kcclient.Config{BaseURL: server})
	if err != nil {
		return taihuSession{}, false, err
	}
	result, err := client.IdentityService().ExchangeToken(ctx, kcclient.TokenRequest{GrantType: "refresh_token", RefreshToken: session.RefreshToken})
	if err != nil {
		return taihuSession{}, false, err
	}
	principal, err := verifyTokenPrincipal(ctx, server, result.AccessToken)
	if err != nil {
		return taihuSession{}, false, err
	}
	if session.Principal != "" && principal != session.Principal {
		return taihuSession{}, false, kernel.Fail(kernel.ErrUnauthenticated, "refreshed credential changed identity; run kc login again")
	}
	session.Principal = principal
	session.AccessToken = result.AccessToken
	if result.RefreshToken != "" {
		session.RefreshToken = result.RefreshToken
	}
	session.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	// Refreshing one service must not switch the user's default service.
	if err := jsonfile.WritePrivate(clientconfig.SessionPath(server, "session-taihu.json"), session); err != nil {
		return taihuSession{}, false, err
	}
	return session, true, nil
}

func verifyTokenPrincipal(ctx context.Context, server, token string) (string, error) {
	client, err := kcclient.New(kcclient.Config{BaseURL: server, HTTPClient: credentialHTTPClient(), Authenticator: remoteTokenAuthenticator{}})
	if err != nil {
		return "", err
	}
	if _, err := client.Login(ctx, kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "token-user"}, Authentication: kcclient.Authentication{Authorization: "Bearer " + token}}); err != nil {
		return "", err
	}
	identity, err := client.IdentityService().WhoAmI(ctx, kcclient.RequestOptions{})
	if err != nil {
		return "", err
	}
	if err := identity.Validate(); err != nil {
		return "", kernel.Fail(kernel.ErrUnauthenticated, "server did not verify a valid identity")
	}
	return identity.Principal, nil
}

// remoteTransport is shared by every authenticated session client of this
// process so concurrent preflight resolves reuse connections.
var remoteTransport = httpjson.PooledTransport(32)

func credentialHTTPClient() *http.Client {
	return &http.Client{Transport: remoteTransport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
