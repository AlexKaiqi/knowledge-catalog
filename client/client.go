package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/propagation"

	"kc/kernel"
)

const maxResponseBytes = 8 << 20

// Config assembles a KC Client without selecting a concrete authentication
// mechanism. BaseURL is the KC server origin (optionally with a path prefix).
type Config struct {
	BaseURL       string
	HTTPClient    *http.Client
	Authenticator Authenticator
	Sessions      SessionStore
}

// Client carries the current identity, authentication, request correlation,
// and W3C trace context across remote calls.
type Client struct {
	baseURL       string
	httpClient    *http.Client
	authenticator Authenticator
	sessions      SessionStore
	propagator    propagation.TextMapPropagator
}

func New(config Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("client base URL must be an http(s) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("client base URL must not contain credentials, query, or fragment")
	}
	authenticator := config.Authenticator
	if authenticator == nil {
		authenticator = PassThroughAuthenticator{}
	}
	sessions := config.Sessions
	if sessions == nil {
		sessions = &MemorySessionStore{}
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:       baseURL,
		httpClient:    httpClient,
		authenticator: authenticator,
		sessions:      sessions,
		propagator:    propagation.TraceContext{},
	}, nil
}

// Login establishes client-local identity and authentication state. The
// default pass-through authenticator performs no remote verification.
func (c *Client) Login(ctx context.Context, request LoginRequest) (Identity, error) {
	session, err := c.authenticator.Login(ctx, request)
	if err != nil {
		return Identity{}, err
	}
	if err := session.validate(); err != nil {
		return Identity{}, err
	}
	if err := c.sessions.Save(ctx, session); err != nil {
		return Identity{}, err
	}
	return session.Identity, nil
}

// Logout clears local credentials even if a future provider-side logout or
// revocation attempt fails. This prevents subsequent calls from accidentally
// continuing under the old identity.
func (c *Client) Logout(ctx context.Context) error {
	session, ok, loadErr := c.sessions.Load(ctx)
	var logoutErr error
	if loadErr == nil && ok {
		logoutErr = c.authenticator.Logout(ctx, session)
	}
	deleteErr := c.sessions.Delete(ctx)
	return errors.Join(loadErr, logoutErr, deleteErr)
}

// Identity returns the current local identity without contacting a server.
// Use the server's whoami operation when the identity must be proven remotely.
func (c *Client) Identity(ctx context.Context) (Identity, bool, error) {
	session, ok, err := c.sessions.Load(ctx)
	if err != nil || !ok {
		return Identity{}, ok, err
	}
	if err := session.validate(); err != nil {
		return Identity{}, false, err
	}
	return session.Identity, true, nil
}

// Do authenticates and sends an HTTP request to KC or another target system.
// audience is passed to the Authenticator for future token exchange/scoping;
// when empty, the request origin is used. Request headers are cloned before
// identity and credentials are added, so the caller's headers are not mutated.
func (c *Client) Do(ctx context.Context, audience string, request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, fmt.Errorf("request URL is required")
	}
	session, ok, err := c.sessions.Load(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, kernel.Fail(kernel.ErrUnauthenticated, "kc client is logged out")
	}
	if err := session.validate(); err != nil {
		return nil, err
	}
	prepared := request.Clone(ctx)
	prepared.Header = request.Header.Clone()
	if strings.TrimSpace(audience) == "" {
		audience = prepared.URL.Scheme + "://" + prepared.URL.Host
	}
	if err := c.authenticator.AuthenticateRequest(ctx, session, audience, prepared); err != nil {
		return nil, err
	}
	// Identity and credentials are intentionally not propagated through baggage.
	c.propagator.Inject(ctx, propagation.HeaderCarrier(prepared.Header))
	return c.httpClient.Do(prepared)
}

// RequestOptions are transport metadata shared by typed service clients.
// RequestID must be reused when retrying the same logical request.
type RequestOptions struct {
	RequestID string
}

// IdentityService is the typed client for /identity/v1.
type IdentityService struct{ client *Client }

func (c *Client) IdentityService() IdentityService { return IdentityService{client: c} }

func (s IdentityService) WhoAmI(ctx context.Context, options RequestOptions) (Identity, error) {
	var identity Identity
	err := s.client.doJSON(ctx, http.MethodGet, "/identity/v1/whoami", nil, options, &identity)
	return identity, err
}

// doJSON is transport plumbing for typed namespace clients. Endpoint paths
// are compile-time constants owned by those clients; callers cannot provide a
// CLI verb, flag bag, or arbitrary KC route.
func (c *Client) doJSON(ctx context.Context, method, path string, input any, options RequestOptions, output any) error {
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
	}
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	requestID := strings.TrimSpace(options.RequestID)
	if requestID == "" {
		requestID = newRequestID()
	}
	request.Header.Set("X-Kc-Request-Id", requestID)
	response, err := c.Do(ctx, c.baseURL, request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxResponseBytes {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "kc server response exceeds %d bytes", maxResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var envelope struct {
			Error *kernel.IngressError `json:"error"`
		}
		if json.Unmarshal(raw, &envelope) == nil && envelope.Error != nil {
			return envelope.Error
		}
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "kc server returned HTTP %d", response.StatusCode)
	}
	if output == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "decode kc server response: %v", err)
	}
	return nil
}

func newRequestID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("kc-client-%d", time.Now().UnixNano())
	}
	return "kc-client-" + hex.EncodeToString(raw)
}
