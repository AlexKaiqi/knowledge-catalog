package cli

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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	kcclient "kc/client"
	"kc/kernel"
)

// loginVerbs returns the login/logout command group. These are stage-local
// commands that manage client-side authentication state before connecting to
// a KC server.
func loginVerbs() map[string]command {
	return map[string]command{
		"login":  {stage: stageHome, run: verbLogin},
		"logout": {stage: stageHome, run: verbLogout},
	}
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

func verbLogin(cx *invocation) (any, error) {
	server := remoteServerURL(cx.Flags)
	if server == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc login requires --server or KC_SERVER_URL")
	}

	mode := strings.ToLower(strings.TrimSpace(FlagString(cx.Flags, "mode")))
	if mode != "" && mode != "taihu" && mode != "token" && mode != "local" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--mode must be 'taihu', 'token', or 'local'")
	}
	wait := FlagBool(cx.Flags, "wait")
	if mode == "token" {
		if strings.TrimSpace(FlagString(cx.Flags, "token")) == "" && strings.TrimSpace(os.Getenv("KC_AUTH_TOKEN")) == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc login --mode token requires --token or KC_AUTH_TOKEN")
		}
	}
	if mode == "local" {
		if strings.TrimSpace(FlagString(cx.Flags, "as")) == "" && strings.TrimSpace(os.Getenv("KC_AS")) == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc login --mode local requires --as or KC_AS")
		}
	}

	discovery, err := discoverServerAuth(cx.Context, server)
	if err != nil {
		return nil, err
	}
	if mode == "" {
		switch discovery.Mode {
		case "local":
			mode = "local"
		case "taihu":
			mode = "taihu"
		case "gitea":
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "this Server is --auth gitea; use --mode token with a Bearer token")
		default:
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "this Server reported unknown auth mode %q", discovery.Mode)
		}
	}
	if discovery.LocalAssertion {
		if mode != "local" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid,
				"this Server is --auth local; use --mode local --as <principal> (Taihu/token login is a pairing mismatch)")
		}
		return localLogin(cx, server)
	}
	if mode == "local" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"this Server is --auth %s; local assertion (--mode local / --as) is not a product login", discovery.Mode)
	}
	if mode == "token" {
		return tokenLogin(cx, server)
	}
	if discovery.Mode != "taihu" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"this Server is --auth %s; browser Taihu login is not available", discovery.Mode)
	}
	if discovery.BrowserLogin == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "this Server has no browser login broker; use an issued Bearer token with --mode token or contact the deployment operator")
	}
	return taihuLogin(cx, server, wait, *discovery.BrowserLogin)
}

func verbLogout(cx *invocation) (any, error) {
	server := remoteServerURL(cx.Flags)
	if server == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "kc logout requires --server or KC_SERVER_URL")
	}

	client, err := newClientWithSession(server, cx.Flags)
	if err != nil {
		return nil, err
	}
	if err := client.Logout(context.Background()); err != nil {
		return nil, err
	}
	if err := clearServerSessions(server); err != nil {
		return nil, err
	}
	return map[string]any{"status": "logged out", "server": server}, nil
}

func taihuLogin(cx *invocation, server string, wait bool, browser kcclient.BrowserLoginConfig) (any, error) {
	// Login endpoints and application coordinates belong to this deployment.
	for _, name := range []string{"oauth2-base", "client-id", "resource", "app-name"} {
		if FlagString(cx.Flags, name) != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "--%s is deployment-owned; browser login uses Server discovery", name)
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
		return taihuWaitAuth(cx, cfg)
	}
	return taihuStartAuth(cx, cfg)
}

func taihuStartAuth(cx *invocation, cfg taihuAuthConfig) (any, error) {
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

	request, err := http.NewRequestWithContext(cx.Context, http.MethodPost, cfg.OAuth2Base+"/oauth2/par", strings.NewReader(parBody))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	parResp, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
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
	pendingFile := serverSessionPath(cfg.URL, "pending-taihu-auth.json")
	if err := writeJSONFile(pendingFile, pending); err != nil {
		return nil, fmt.Errorf("save pending auth: %v", err)
	}

	fmt.Fprintf(os.Stderr, "\n  Opening browser for Taihu authentication...\n")
	fmt.Fprintf(os.Stderr, "  If the browser does not open, visit:\n  %s\n\n", authURL)
	openBrowser(authURL)

	return map[string]any{
		"auth_required": true,
		"auth_url":      authURL,
		"request_uri":   parResult.RequestURI,
		"expires_in":    parResult.ExpiresIn,
		"next_step":     fmt.Sprintf("kc login --wait --server %s", cfg.URL),
	}, nil
}

func taihuWaitAuth(cx *invocation, cfg taihuAuthConfig) (any, error) {
	pendingFile := serverSessionPath(cfg.URL, "pending-taihu-auth.json")

	raw, err := os.ReadFile(pendingFile)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"no pending Taihu auth found; run 'kc login --server %s' first", cfg.URL)
	}

	var pending taihuPendingAuth
	if err := json.Unmarshal(raw, &pending); err != nil {
		return nil, fmt.Errorf("read pending auth: %v", err)
	}
	if normalizeLoginServer(pending.Server) != normalizeLoginServer(cfg.URL) || pending.OAuth2Base != cfg.OAuth2Base || pending.ClientID != cfg.ClientID || pending.Resource != cfg.Resource {
		return nil, kernel.Fail(kernel.ErrUnauthenticated, "pending login no longer matches this Server; restart kc login")
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	fmt.Fprintf(os.Stderr, "  Waiting for authorization... (timeout: 5 min)\n")

	pollURL := fmt.Sprintf("%s/oauth2/par/poll?request_uri=%s&client_id=%s",
		pending.OAuth2Base, urlEncode(pending.RequestURI), urlEncode(pending.ClientID))

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-cx.Context.Done():
			return nil, cx.Context.Err()
		case <-time.After(3 * time.Second):
		}
		request, err := http.NewRequestWithContext(cx.Context, http.MethodGet, pollURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(request)
		if err != nil {
			fmt.Fprintf(os.Stderr, ".")
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
			fmt.Fprintf(os.Stderr, "\n  Authorization completed!\n")
			return exchangeTaihuCode(cx, pending, pollResult.Code, pollResult.RedirectURI)
		case "error":
			_ = os.Remove(pendingFile)
			return nil, fmt.Errorf("authorization rejected by Taihu: %s", pollResult.ErrorDesc)
		case "pending":
			fmt.Fprintf(os.Stderr, ".")
		default:
			_ = os.Remove(pendingFile)
			return nil, fmt.Errorf("authorization request expired; restart 'kc login --server %s'", cfg.URL)
		}
	}

	return nil, fmt.Errorf("authorization timed out after 5 minutes; restart 'kc login --server %s'", cfg.URL)
}

type taihuTokenResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func exchangeTaihuAccessToken(pending taihuPendingAuth, code, redirectURI, clientSecret string, httpClient *http.Client) (taihuTokenResult, error) {
	var zero taihuTokenResult
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
	var tokenResult taihuTokenResult
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

func exchangeTaihuCode(cx *invocation, pending taihuPendingAuth, code, redirectURI string) (any, error) {
	client, err := kcclient.New(kcclient.Config{BaseURL: pending.Server})
	if err != nil {
		return nil, err
	}
	result, err := client.IdentityService().ExchangeToken(cx.Context, kcclient.TokenRequest{
		GrantType: "authorization_code", Code: code, CodeVerifier: pending.CodeVerifier, RedirectURI: redirectURI,
	})
	if err != nil {
		return nil, err
	}
	principal, err := verifyTokenPrincipal(cx.Context, pending.Server, result.AccessToken)
	if err != nil {
		return nil, err
	}
	if err := persistTokenLogin(taihuSession{
		Server: pending.Server, Principal: principal, AccessToken: result.AccessToken,
		RefreshToken: result.RefreshToken, ExpiresAt: time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}); err != nil {
		return nil, err
	}
	_ = os.Remove(serverSessionPath(pending.Server, "pending-taihu-auth.json"))
	return map[string]any{
		"status": "authenticated", "server": pending.Server, "principal": principal,
		"expires_in": result.ExpiresIn, "has_refresh": result.RefreshToken != "",
	}, nil
}

func tokenLogin(cx *invocation, server string) (any, error) {
	token := strings.TrimSpace(FlagString(cx.Flags, "token"))
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
	principal, err := verifyTokenPrincipal(cx.Context, server, access)
	if err != nil {
		return nil, err
	}
	if err := persistTokenLogin(taihuSession{Server: server, Principal: principal, AccessToken: access}); err != nil {
		return nil, err
	}

	return map[string]any{"status": "authenticated", "server": server, "principal": principal}, nil
}

// localLogin records a client-local principal for Servers that trust X-Kc-As.
// It does not create a Server session and does not mint a token.
func localLogin(cx *invocation, server string) (any, error) {
	principal := strings.TrimSpace(FlagString(cx.Flags, "as"))
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
		Authenticator: kcclient.PassThroughAuthenticator{},
		Sessions:      &kcclient.MemorySessionStore{},
	})
	if err != nil {
		return nil, err
	}
	if _, err := client.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: principal}}); err != nil {
		return nil, err
	}
	verified, err := client.IdentityService().WhoAmI(context.Background(), kcclient.RequestOptions{})
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
		"server":    server,
		"principal": principal,
		"mode":      "local",
	}, nil
}

func newClientWithSession(server string, flags map[string]FlagValue) (*kcclient.Client, error) {
	client, err := kcclient.New(kcclient.Config{
		BaseURL:       server,
		Authenticator: kcclient.PassThroughAuthenticator{},
		Sessions:      &kcclient.MemorySessionStore{},
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}

type taihuClientAuthenticator struct{}

func (taihuClientAuthenticator) Login(_ context.Context, request kcclient.LoginRequest) (kcclient.Session, error) {
	return kcclient.Session(request), nil
}

func (taihuClientAuthenticator) Logout(context.Context, kcclient.Session) error { return nil }

func (taihuClientAuthenticator) AuthenticateRequest(_ context.Context, session kcclient.Session, _ string, request *http.Request) error {
	if session.Authentication.Authorization != "" {
		request.Header.Set("Authorization", session.Authentication.Authorization)
	}
	return nil
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

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Please open this URL manually: %s\n", url)
	}
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

func persistTaihuSession(path string, s taihuSession) error {
	return writeJSONFile(path, s)
}

type localSession struct {
	Server    string `json:"server"`
	Principal string `json:"principal"`
}

func localSessionPath() string {
	if server := savedClientServer(); server != "" {
		return serverSessionPath(server, "session-local.json")
	}
	return filepath.Join(configDir(), "session-local.json")
}

func persistLocalSession(s localSession) error {
	if strings.TrimSpace(s.Server) == "" || strings.TrimSpace(s.Principal) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "local login requires a server and principal")
	}
	if err := writeJSONFile(serverSessionPath(s.Server, "session-local.json"), s); err != nil {
		return err
	}
	if err := persistClientServer(s.Server); err != nil {
		return err
	}
	return removeMatchingSession(s.Server, "session-taihu.json")
}

func loadLocalSession(server string) (localSession, bool) {
	for _, path := range []string{serverSessionPath(server, "session-local.json"), filepath.Join(configDir(), "session-local.json")} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s localSession
		if json.Unmarshal(raw, &s) != nil || strings.TrimSpace(s.Principal) == "" || strings.TrimSpace(s.Server) == "" {
			continue
		}
		if normalizeLoginServer(s.Server) != normalizeLoginServer(server) {
			continue
		}
		return s, true
	}
	return localSession{}, false
}

func normalizeLoginServer(server string) string {
	return strings.TrimRight(strings.TrimSpace(server), "/")
}

// loadTaihuSession reads a persisted Taihu session. Returns ok=false when
// no valid (non-expired) session exists.
func loadTaihuSession(path string) (taihuSession, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return taihuSession{}, false
	}
	var s taihuSession
	if err := json.Unmarshal(raw, &s); err != nil {
		return taihuSession{}, false
	}
	if s.AccessToken == "" {
		return taihuSession{}, false
	}
	if !s.ExpiresAt.IsZero() && time.Now().After(s.ExpiresAt) {
		return taihuSession{}, false
	}
	return s, true
}

func configDir() string {
	if dir := strings.TrimSpace(os.Getenv("KC_CONFIG_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".kc")
	}
	return filepath.Join(home, ".config", "kc")
}

func writeJSONFile(path string, data any) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".kc-session-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
