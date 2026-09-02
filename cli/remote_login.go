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
	return taihuLogin(cx, server, wait)
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
	clearPersistedSessions()
	return map[string]any{"status": "logged out"}, nil
}

func taihuLogin(cx *invocation, server string, wait bool) (any, error) {
	oauth2Base := strings.TrimSpace(FlagString(cx.Flags, "oauth2-base"))
	if oauth2Base == "" {
		oauth2Base = "http://iam.it.woa.com"
	}
	clientID := strings.TrimSpace(FlagString(cx.Flags, "client-id"))
	if clientID == "" {
		clientID = "knowledge-catalog"
	}
	resource := strings.TrimSpace(FlagString(cx.Flags, "resource"))
	if resource == "" {
		resource = server
	}
	appName := strings.TrimSpace(FlagString(cx.Flags, "app-name"))
	if appName == "" {
		appName = "knowledge-catalog"
	}

	cfg := taihuAuthConfig{
		OAuth2Base: oauth2Base,
		ClientID:   clientID,
		Scope:      "*",
		URL:        server,
		Resource:   resource,
		AppName:    appName,
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

	parResp, err := http.Post(cfg.OAuth2Base+"/oauth2/par", "application/x-www-form-urlencoded", strings.NewReader(parBody))
	if err != nil {
		return nil, fmt.Errorf("request to Taihu PAR endpoint failed: %v", err)
	}
	defer parResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(parResp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read PAR response: %v", err)
	}

	var parResult struct {
		RequestURI string `json:"request_uri"`
		ExpiresIn  int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parResult); err != nil {
		return nil, fmt.Errorf("unexpected Taihu PAR response: %s", string(body))
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
	pendingDir := configDir()
	_ = os.MkdirAll(pendingDir, 0700)
	pendingFile := pendingDir + "/pending-taihu-auth.json"
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
	pendingDir := configDir()
	pendingFile := pendingDir + "/pending-taihu-auth.json"

	raw, err := os.ReadFile(pendingFile)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"no pending Taihu auth found; run 'kc login --server %s' first", cfg.URL)
	}

	var pending taihuPendingAuth
	if err := json.Unmarshal(raw, &pending); err != nil {
		return nil, fmt.Errorf("read pending auth: %v", err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	fmt.Fprintf(os.Stderr, "  Waiting for authorization... (timeout: 5 min)\n")

	pollURL := fmt.Sprintf("%s/oauth2/par/poll?request_uri=%s&client_id=%s",
		pending.OAuth2Base, urlEncode(pending.RequestURI), urlEncode(pending.ClientID))

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)

		resp, err := httpClient.Get(pollURL)
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
	tokenResult, err := exchangeTaihuAccessToken(pending, code, redirectURI, os.Getenv("KC_SERVICE_CLIENT_SECRET"), nil)
	if err != nil {
		return nil, err
	}

	client, err := kcclient.New(kcclient.Config{
		BaseURL:       pending.Server,
		Authenticator: &taihuClientAuthenticator{},
		Sessions:      &kcclient.MemorySessionStore{},
	})
	if err != nil {
		return nil, err
	}

	identity := kcclient.Identity{Principal: "taihu:user"}
	auth := kcclient.Authentication{Authorization: "Bearer " + tokenResult.AccessToken}
	if _, err := client.Login(context.Background(), kcclient.LoginRequest{Identity: identity, Authentication: auth}); err != nil {
		return nil, err
	}

	// Ask the server for the verified principal (e.g. "taihu:alice"). The
	// server derives it from the introspection result; the client must use the
	// same value so allow.json matches and audit logs are consistent.
	principal := identity.Principal
	if verified, err := client.IdentityService().WhoAmI(context.Background(), kcclient.RequestOptions{}); err == nil && verified.Principal != "" {
		principal = verified.Principal
	}

	// Persist the session token so subsequent kc commands (without
	// KC_AUTH_TOKEN) can reuse this authenticated identity. The refresh
	// token enables silent rotation before the access token expires.
	pendingDir := configDir()
	_ = os.MkdirAll(pendingDir, 0700)
	if err := persistTaihuSession(pendingDir+"/session-taihu.json", taihuSession{
		Server:       pending.Server,
		Principal:    principal,
		AccessToken:  tokenResult.AccessToken,
		RefreshToken: tokenResult.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResult.ExpiresIn) * time.Second),
	}); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not persist session: %v\n", err)
	} else {
		_ = os.Remove(localSessionPath())
	}
	_ = os.Remove(pendingDir + "/pending-taihu-auth.json")

	fmt.Fprintf(os.Stderr, "  Token expires in: %d seconds\n", tokenResult.ExpiresIn)
	if tokenResult.RefreshToken != "" {
		fmt.Fprintf(os.Stderr, "  Refresh token available\n")
	}

	return map[string]any{
		"status":      "authenticated",
		"server":      pending.Server,
		"principal":   principal,
		"expires_in":  tokenResult.ExpiresIn,
		"has_refresh": tokenResult.RefreshToken != "",
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

	client, err := kcclient.New(kcclient.Config{
		BaseURL:       server,
		Authenticator: remoteTokenAuthenticator{},
		Sessions:      &kcclient.MemorySessionStore{},
	})
	if err != nil {
		return nil, err
	}

	header, access := bearerParts(token)
	identity := kcclient.Identity{Principal: "token-user"}
	auth := kcclient.Authentication{Authorization: header}
	if _, err := client.Login(context.Background(), kcclient.LoginRequest{Identity: identity, Authentication: auth}); err != nil {
		return nil, err
	}
	principal := identity.Principal
	verified, err := client.IdentityService().WhoAmI(context.Background(), kcclient.RequestOptions{})
	if err != nil {
		return nil, err
	}
	if verified.Principal != "" {
		principal = verified.Principal
	}
	if err := persistTaihuSession(configDir()+"/session-taihu.json", taihuSession{
		Server:      server,
		Principal:   principal,
		AccessToken: access,
	}); err != nil {
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
	if err := persistLocalSession(localSession{Server: server, Principal: principal}); err != nil {
		return nil, err
	}
	client, err := kcclient.New(kcclient.Config{
		BaseURL:       server,
		Authenticator: kcclient.PassThroughAuthenticator{},
		Sessions:      &kcclient.MemorySessionStore{},
	})
	if err != nil {
		_ = os.Remove(localSessionPath())
		return nil, err
	}
	if _, err := client.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: principal}}); err != nil {
		_ = os.Remove(localSessionPath())
		return nil, err
	}
	verified, err := client.IdentityService().WhoAmI(context.Background(), kcclient.RequestOptions{})
	if err != nil {
		_ = os.Remove(localSessionPath())
		return nil, err
	}
	if verified.Principal != principal {
		_ = os.Remove(localSessionPath())
		return nil, kernel.Fail(kernel.ErrUnauthenticated, "local login whoami returned %q, want %q", verified.Principal, principal)
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
	if path == configDir()+"/session-taihu.json" {
		if err := os.MkdirAll(configDir(), 0o700); err != nil {
			return err
		}
		_ = os.Remove(localSessionPath())
	}
	return writeJSONFile(path, s)
}

type localSession struct {
	Server    string `json:"server"`
	Principal string `json:"principal"`
}

func localSessionPath() string {
	return configDir() + "/session-local.json"
}

func persistLocalSession(s localSession) error {
	if strings.TrimSpace(s.Server) == "" || strings.TrimSpace(s.Principal) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "local login requires a server and principal")
	}
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	_ = os.Remove(configDir() + "/session-taihu.json")
	return writeJSONFile(localSessionPath(), s)
}

func loadLocalSession(server string) (localSession, bool) {
	raw, err := os.ReadFile(localSessionPath())
	if err != nil {
		return localSession{}, false
	}
	var s localSession
	if err := json.Unmarshal(raw, &s); err != nil {
		return localSession{}, false
	}
	if strings.TrimSpace(s.Principal) == "" {
		return localSession{}, false
	}
	if s.Server != "" && normalizeLoginServer(s.Server) != normalizeLoginServer(server) {
		return localSession{}, false
	}
	return s, true
}

func clearPersistedSessions() {
	_ = os.Remove(configDir() + "/session-taihu.json")
	_ = os.Remove(localSessionPath())
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
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir() + "/.kc"
	}
	return home + "/.config/kc"
}

func writeJSONFile(path string, data any) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0600)
}
