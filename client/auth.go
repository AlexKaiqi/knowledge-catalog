// Package client provides the caller-side KC service boundary.
//
// Authentication is deliberately a client concern: protocol packages consume
// an established identity, while a Client obtains and refreshes credentials
// and attaches them to every remote request. A Login is local client state; it
// does not create a server-side Workspace session.
package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"unicode"

	"kc/observability"
)

// Identity is shared by authorization and access evidence. Principal is the
// actor performing the request; OnBehalfOf is an optional delegated user.
type Identity = observability.IdentityContext

// Authentication is an opaque HTTP authorization value. KC does not interpret
// its scheme or secret on the client side. A future OIDC, Gitea, or deployment
// authenticator can replace it or refresh it per audience.
//
// Authentication values are secrets. They must not be logged, serialized into
// a Workspace pin, or added to trace baggage.
type Authentication struct {
	Authorization string `json:"-" yaml:"-"`
}

func (Authentication) String() string   { return "<redacted authentication>" }
func (Authentication) GoString() string { return "client.Authentication{<redacted>}" }

func (a Authentication) validate() error {
	if a.Authorization == "" {
		return nil
	}
	if strings.TrimSpace(a.Authorization) != a.Authorization {
		return fmt.Errorf("authorization must be trimmed")
	}
	for _, r := range a.Authorization {
		if unicode.IsControl(r) {
			return fmt.Errorf("authorization must not contain control characters")
		}
	}
	return nil
}

// LoginRequest is the provider-neutral input to Client.Login. Pass-through
// mode accepts these values as-is; a real Authenticator may exchange them for
// a short-lived access token and a verified Identity.
type LoginRequest struct {
	Identity       Identity
	Authentication Authentication
}

// Session is the current client-local login result. It is re-read for every
// request so credential refresh or logout never changes a pinned Workspace.
type Session struct {
	Identity       Identity
	Authentication Authentication
}

func (s Session) validate() error {
	if err := s.Identity.Validate(); err != nil {
		return err
	}
	return s.Authentication.validate()
}

// Authenticator owns the login/logout lifecycle and request authentication.
// audience identifies the target service so a future implementation can mint
// scoped credentials rather than forwarding one bearer token everywhere.
type Authenticator interface {
	Login(context.Context, LoginRequest) (Session, error)
	Logout(context.Context, Session) error
	AuthenticateRequest(context.Context, Session, string, *http.Request) error
}

// PassThroughAuthenticator is the current development implementation. It
// proves nothing: Login validates shape, and AuthenticateRequest forwards both
// the opaque Authorization value and the asserted identity. Production
// deployments must replace it with a verifier/token provider and must not
// trust X-Kc-As.
type PassThroughAuthenticator struct{}

func (PassThroughAuthenticator) Login(_ context.Context, request LoginRequest) (Session, error) {
	session := Session{Identity: request.Identity, Authentication: request.Authentication}
	if err := session.validate(); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (PassThroughAuthenticator) Logout(context.Context, Session) error { return nil }

func (PassThroughAuthenticator) AuthenticateRequest(_ context.Context, session Session, _ string, request *http.Request) error {
	if request == nil {
		return fmt.Errorf("request is required")
	}
	if err := session.validate(); err != nil {
		return err
	}
	if session.Authentication.Authorization != "" {
		request.Header.Set("Authorization", session.Authentication.Authorization)
	}
	request.Header.Set("X-Kc-As", session.Identity.Principal)
	if session.Identity.OnBehalfOf != "" {
		request.Header.Set("X-Kc-On-Behalf-Of", session.Identity.OnBehalfOf)
	}
	return nil
}

// SessionStore keeps login state on the client. It is not a KC protocol store
// and must never be implemented inside a Catalog or Repository.
type SessionStore interface {
	Load(context.Context) (Session, bool, error)
	Save(context.Context, Session) error
	Delete(context.Context) error
}

// MemorySessionStore is the default until an application supplies an OS
// keychain or Agent credential store. It intentionally does not persist login
// state across processes.
type MemorySessionStore struct {
	mu      sync.RWMutex
	session Session
	present bool
}

func (s *MemorySessionStore) Load(context.Context) (Session, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session, s.present, nil
}

func (s *MemorySessionStore) Save(_ context.Context, session Session) error {
	if err := session.validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = session
	s.present = true
	return nil
}

func (s *MemorySessionStore) Delete(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.session = Session{}
	s.present = false
	return nil
}
