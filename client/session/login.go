package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	kcclient "kc/client/auth"
	"kc/internal/clientconfig"
	"kc/internal/jsonfile"
	"kc/kernel"
)

// LoginOptions contains caller selection, never deployment application secrets.
type LoginOptions struct {
	Server, Mode, Principal, Token string
	Wait                           bool
}
type LoginPhase uint8

const (
	WaitingAuthorization LoginPhase = iota
	AuthorizationPending
	AuthorizationCompleted
)

// LoginInteraction lets a host validate its browser arguments and present UI.
type LoginInteraction struct {
	BeforeBrowser     func() error
	OpenAuthorization func(string)
	Progress          func(LoginPhase)
}

// taihuAuthConfig holds the configuration for Taihu OAuth2 browser-based login.
type taihuAuthConfig struct {
	OAuth2Base string `json:"oauth2_base"`
	ClientID   string `json:"client_id"`
	Scope      string `json:"scope"`
	URL        string `json:"url"`
	Resource   string `json:"resource"`
	AppName    string `json:"app_name"`
}

// taihuPendingAuth is persisted between the start-auth and wait-auth phases.
type taihuPendingAuth struct {
	RequestURI   string `json:"request_uri"`
	CodeVerifier string `json:"code_verifier"`
	ClientID     string `json:"client_id"`
	Resource     string `json:"resource"`
	OAuth2Base   string `json:"oauth2_base"`
	Server       string `json:"server"`
}

func Login(ctx context.Context, options LoginOptions, interaction LoginInteraction) (any, error) {
	server := clientconfig.ServerURL(options.Server)
	if server == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc login requires --server or KC_SERVER_URL")
	}

	mode := strings.ToLower(strings.TrimSpace(options.Mode))
	if mode != "" && mode != "taihu" && mode != "token" && mode != "local" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--mode must be 'taihu', 'token', or 'local'")
	}
	wait := options.Wait
	if mode == "token" {
		if strings.TrimSpace(options.Token) == "" && strings.TrimSpace(os.Getenv("KC_AUTH_TOKEN")) == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc login --mode token requires --token or KC_AUTH_TOKEN")
		}
	}
	if mode == "local" {
		if strings.TrimSpace(options.Principal) == "" && strings.TrimSpace(os.Getenv("KC_AS")) == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc login --mode local requires --as or KC_AS")
		}
	}

	discovery, err := discoverServerAuth(ctx, server)
	if err != nil {
		return nil, err
	}
	if mode == "" {
		switch discovery.Mode {
		case "local":
			mode = "local"
		case "taihu":
			mode = "taihu"
		default:
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "this Server reported unknown auth mode %q", discovery.Mode)
		}
	}
	if discovery.LocalAssertion {
		if mode != "local" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid,
				"this Server is --auth local; use --mode local --as <principal> (Taihu/token login is a pairing mismatch)")
		}
		return localLogin(ctx, server, options.Principal)
	}
	if mode == "local" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"this Server is --auth %s; local assertion (--mode local / --as) is not a product login", discovery.Mode)
	}
	if mode == "token" {
		return tokenLogin(ctx, server, options.Token)
	}
	if discovery.Mode != "taihu" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"this Server is --auth %s; browser Taihu login is not available", discovery.Mode)
	}
	if discovery.BrowserLogin == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "this Server has no browser login broker; use an issued Bearer token with --mode token or contact the deployment operator")
	}
	return taihuLogin(ctx, server, wait, *discovery.BrowserLogin, interaction)
}

func Logout(ctx context.Context, server string) (any, error) {
	server = clientconfig.ServerURL(server)
	if server == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc logout requires --server or KC_SERVER_URL")
	}

	client, err := kcclient.New(kcclient.Config{BaseURL: server, Authenticator: kcclient.PassThroughAuthenticator{}, Sessions: &kcclient.MemorySessionStore{}})
	if err != nil {
		return nil, err
	}
	if err := client.Logout(ctx); err != nil {
		return nil, err
	}
	if err := clearServerSessions(server); err != nil {
		return nil, err
	}
	return map[string]any{"status": "logged out"}, nil
}

func taihuLogin(ctx context.Context, server string, wait bool, browser kcclient.BrowserLoginConfig, interaction LoginInteraction) (any, error) {
	if interaction.BeforeBrowser != nil {
		if err := interaction.BeforeBrowser(); err != nil {
			return nil, err
		}
	}
	cfg := taihuAuthConfig{
		OAuth2Base: strings.TrimRight(browser.OAuth2Base, "/"),
		ClientID:   browser.ClientID,
		Scope:      browser.Scope,
		URL:        server,
		Resource:   browser.Resource,
		AppName:    browser.AppName,
	}

	if wait {
		return taihuWaitAuth(ctx, cfg, interaction)
	}
	return taihuStartAuth(ctx, cfg, interaction)
}

func taihuStartAuth(ctx context.Context, cfg taihuAuthConfig, interaction LoginInteraction) (any, error) {
	pkce, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("PKCE generation failed: %v", err)
	}

	parBody := fmt.Sprintf(
		"client_id=%s&response_type=code&code_challenge=%s&code_challenge_method=S256&state=%s&scope=%s&resource=%s&app_name=%s",
		urlEncode(cfg.ClientID),
		urlEncode(pkce.Challenge),
		urlEncode(pkce.State),
		urlEncode(cfg.Scope),
		urlEncode(cfg.Resource),
		urlEncode(cfg.AppName),
	)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.OAuth2Base+"/oauth2/par", strings.NewReader(parBody))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	parResp, err := credentialHTTPClient().Do(request)
	if err != nil {
		return nil, fmt.Errorf("request to Taihu PAR endpoint failed: %v", err)
	}
	defer parResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(parResp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read PAR response: %v", err)
	}
	if parResp.StatusCode < 200 || parResp.StatusCode >= 300 {
		return nil, kernel.Fail(kernel.ErrUnauthenticated, "Taihu authorization start returned HTTP %d", parResp.StatusCode)
	}

	var parResult struct {
		RequestURI string `json:"request_uri"`
		ExpiresIn  int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parResult); err != nil {
		return nil, fmt.Errorf("unexpected Taihu PAR response")
	}

	if parResult.RequestURI == "" {
		return nil, fmt.Errorf("empty request_uri in Taihu PAR response")
	}

	authURL := fmt.Sprintf("%s/oauth2/authorize?client_id=%s&request_uri=%s",
		cfg.OAuth2Base, urlEncode(cfg.ClientID), urlEncode(parResult.RequestURI))

	pending := taihuPendingAuth{
		RequestURI:   parResult.RequestURI,
		CodeVerifier: pkce.Verifier,
		ClientID:     cfg.ClientID,
		Resource:     cfg.Resource,
		OAuth2Base:   cfg.OAuth2Base,
		Server:       cfg.URL,
	}
	pendingFile := clientconfig.SessionPath(cfg.URL, "pending-taihu-auth.json")
	if err := jsonfile.WritePrivate(pendingFile, pending); err != nil {
		return nil, fmt.Errorf("save pending auth: %v", err)
	}

	if interaction.OpenAuthorization != nil {
		interaction.OpenAuthorization(authURL)
	}

	return map[string]any{
		"auth_required": true,
		"auth_url":      authURL,
		"expires_in":    parResult.ExpiresIn,
		"next_step":     "kc login --wait",
	}, nil
}

func taihuWaitAuth(ctx context.Context, cfg taihuAuthConfig, interaction LoginInteraction) (any, error) {
	pendingFile := clientconfig.SessionPath(cfg.URL, "pending-taihu-auth.json")

	raw, err := os.ReadFile(pendingFile)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"no pending Taihu auth found; run 'kc login --server %s' first", cfg.URL)
	}

	var pending taihuPendingAuth
	if err := json.Unmarshal(raw, &pending); err != nil {
		return nil, fmt.Errorf("read pending auth: %v", err)
	}
	if clientconfig.NormalizeServer(pending.Server) != clientconfig.NormalizeServer(cfg.URL) || pending.OAuth2Base != cfg.OAuth2Base || pending.ClientID != cfg.ClientID || pending.Resource != cfg.Resource {
		return nil, kernel.Fail(kernel.ErrUnauthenticated, "pending login no longer matches this Server; restart kc login")
	}

	httpClient := credentialHTTPClient()
	if interaction.Progress != nil {
		interaction.Progress(WaitingAuthorization)
	}

	pollURL := fmt.Sprintf("%s/oauth2/par/poll?request_uri=%s&client_id=%s",
		pending.OAuth2Base, urlEncode(pending.RequestURI), urlEncode(pending.ClientID))

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(request)
		if err != nil {
			if interaction.Progress != nil {
				interaction.Progress(AuthorizationPending)
			}
			continue
		}

		pollBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			continue
		}

		var pollResult struct {
			Status      string `json:"status"`
			Code        string `json:"code"`
			RedirectURI string `json:"redirect_uri"`
			Error       string `json:"error"`
			ErrorDesc   string `json:"error_description"`
		}
		if err := json.Unmarshal(pollBody, &pollResult); err != nil {
			continue
		}

		switch pollResult.Status {
		case "completed":
			if interaction.Progress != nil {
				interaction.Progress(AuthorizationCompleted)
			}
			return exchangeTaihuCode(ctx, pending, pollResult.Code, pollResult.RedirectURI)
		case "error":
			_ = os.Remove(pendingFile)
			return nil, fmt.Errorf("authorization rejected by Taihu: %s", pollResult.ErrorDesc)
		case "pending":
			if interaction.Progress != nil {
				interaction.Progress(AuthorizationPending)
			}
		default:
			_ = os.Remove(pendingFile)
			return nil, fmt.Errorf("authorization request expired; restart 'kc login --server %s'", cfg.URL)
		}
	}

	return nil, fmt.Errorf("authorization timed out after 5 minutes; restart 'kc login --server %s'", cfg.URL)
}

func exchangeTaihuCode(ctx context.Context, pending taihuPendingAuth, code, redirectURI string) (any, error) {
	client, err := kcclient.New(kcclient.Config{BaseURL: pending.Server})
	if err != nil {
		return nil, err
	}
	result, err := client.IdentityService().ExchangeToken(ctx, kcclient.TokenRequest{
		GrantType: "authorization_code", Code: code, CodeVerifier: pending.CodeVerifier, RedirectURI: redirectURI,
	})
	if err != nil {
		return nil, err
	}
	principal, err := verifyTokenPrincipal(ctx, pending.Server, result.AccessToken)
	if err != nil {
		return nil, err
	}
	if err := persistTokenLogin(taihuSession{
		Server: pending.Server, Principal: principal, AccessToken: result.AccessToken,
		RefreshToken: result.RefreshToken, ExpiresAt: time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}); err != nil {
		return nil, err
	}
	_ = os.Remove(clientconfig.SessionPath(pending.Server, "pending-taihu-auth.json"))
	return map[string]any{
		"status": "authenticated", "principal": principal,
		"expires_in": result.ExpiresIn, "has_refresh": result.RefreshToken != "",
	}, nil
}

func tokenLogin(ctx context.Context, server, tokenInput string) (any, error) {
	token := strings.TrimSpace(tokenInput)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("KC_AUTH_TOKEN"))
	}
	if token == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc login --mode token requires --token or KC_AUTH_TOKEN")
	}

	if !strings.Contains(token, " ") {
		token = "Bearer " + token
	}

	_, access := bearerParts(token)
	principal, err := verifyTokenPrincipal(ctx, server, access)
	if err != nil {
		return nil, err
	}
	if err := persistTokenLogin(taihuSession{Server: server, Principal: principal, AccessToken: access}); err != nil {
		return nil, err
	}

	return map[string]any{"status": "authenticated", "principal": principal}, nil
}

// localLogin records a client-local principal for Servers that trust X-Kc-As.
// It does not create a Server session and does not mint a token.
func localLogin(ctx context.Context, server, principalInput string) (any, error) {
	principal := strings.TrimSpace(principalInput)
	if principal == "" {
		principal = strings.TrimSpace(os.Getenv("KC_AS"))
	}
	if principal == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc login --mode local requires --as or KC_AS")
	}
	if err := (kcclient.Identity{Principal: principal}).Validate(); err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "%v", err)
	}
	client, err := kcclient.New(kcclient.Config{
		BaseURL:       server,
		HTTPClient:    credentialHTTPClient(),
		Authenticator: kcclient.PassThroughAuthenticator{},
		Sessions:      &kcclient.MemorySessionStore{},
	})
	if err != nil {
		return nil, err
	}
	if _, err := client.Login(ctx, kcclient.LoginRequest{Identity: kcclient.Identity{Principal: principal}}); err != nil {
		return nil, err
	}
	verified, err := client.IdentityService().WhoAmI(ctx, kcclient.RequestOptions{})
	if err != nil {
		return nil, err
	}
	if verified.Principal != principal {
		return nil, kernel.Fail(kernel.ErrUnauthenticated, "local login whoami returned %q, want %q", verified.Principal, principal)
	}
	if err := persistLocalSession(localSession{Server: server, Principal: principal}); err != nil {
		return nil, err
	}

	return map[string]any{
		"status":    "authenticated",
		"principal": principal,
		"mode":      "local",
	}, nil
}

type pkce struct {
	Verifier  string
	Challenge string
	State     string
}

func generatePKCE() (*pkce, error) {
	verifier, err := randomBase64URL(32)
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	state, err := randomHex(8)
	if err != nil {
		return nil, err
	}

	return &pkce{
		Verifier:  verifier,
		Challenge: challenge,
		State:     state,
	}, nil
}

func randomBase64URL(n int) (string, error) {
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func randomHex(n int) (string, error) {
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func urlEncode(s string) string {
	encoded := ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			encoded += string(c)
		} else {
			encoded += fmt.Sprintf("%%%02X", c)
		}
	}
	return encoded
}

// taihuSession is the persisted client-side authentication state for a
// Taihu OAuth2 login. It lets subsequent kc commands authenticate to the
// same server without re-running the browser flow.
type taihuSession struct {
	Server       string    `json:"server"`
	Principal    string    `json:"principal,omitempty"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type localSession struct {
	Server    string `json:"server"`
	Principal string `json:"principal"`
}

func persistLocalSession(s localSession) error {
	if strings.TrimSpace(s.Server) == "" || strings.TrimSpace(s.Principal) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "local login requires a server and principal")
	}
	if err := jsonfile.WritePrivate(clientconfig.SessionPath(s.Server, "session-local.json"), s); err != nil {
		return err
	}
	if err := persistClientServer(s.Server); err != nil {
		return err
	}
	return removeMatchingSession(s.Server, "session-taihu.json")
}

func loadLocalSession(server string) (localSession, bool) {
	for _, path := range []string{clientconfig.SessionPath(server, "session-local.json"), filepath.Join(clientconfig.ConfigDir(), "session-local.json")} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s localSession
		if json.Unmarshal(raw, &s) != nil || strings.TrimSpace(s.Principal) == "" || strings.TrimSpace(s.Server) == "" {
			continue
		}
		if clientconfig.NormalizeServer(s.Server) != clientconfig.NormalizeServer(server) {
			continue
		}
		return s, true
	}
	return localSession{}, false
}
