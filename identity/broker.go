package identity

import (
	"context"
	"errors"
	"net/http"
)

// BrowserLogin is the public login configuration a broker advertises. It
// contains no secret.
type BrowserLogin struct {
	OAuth2Base string
	ClientID   string
	Resource   string
	AppName    string
	Scope      string
}

// TokenGrant is one token exchange or refresh request relayed for a client.
type TokenGrant struct {
	GrantType    string
	Code         string
	CodeVerifier string
	RedirectURI  string
	RefreshToken string
}

// Token is the user credential a broker obtained from its provider.
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// Authorization starts one browser authorization.
type Authorization struct {
	URL        string
	RequestURI string
	ExpiresIn  int
}

// AuthorizationStatus is one poll of a pending browser authorization.
// Status is "pending" or "completed"; a rejected or expired authorization is
// an error.
type AuthorizationStatus struct {
	Status      string
	Code        string
	RedirectURI string
}

// LoginBroker relays a deployment-owned login application for clients. It is
// stateless: it never creates a KC session or grant, and never accepts an
// upstream, application identity or secret from the caller.
type LoginBroker interface {
	BrowserLogin() BrowserLogin
	Token(context.Context, TokenGrant) (Token, error)
	Authorize(ctx context.Context, codeChallenge, state string) (Authorization, error)
	PollAuthorization(ctx context.Context, requestURI string) (AuthorizationStatus, error)
}

// BrokerProvider is implemented by an authenticator whose provider issues user
// tokens through a deployment-owned application. A nil result means the
// deployment has not configured that application.
type BrokerProvider interface {
	LoginBroker() LoginBroker
}

// ServiceAccount is implemented by an authenticator that knows the service's
// own principal for background work.
type ServiceAccount interface {
	ServicePrincipal() string
}

// BrokerOf returns the authenticator's configured login broker, if any.
func BrokerOf(a HTTPAuthenticator) LoginBroker {
	if p, ok := base(a).(BrokerProvider); ok {
		return p.LoginBroker()
	}
	return nil
}

// ServicePrincipalOf returns the service principal the authenticator declares.
func ServicePrincipalOf(a HTTPAuthenticator) string {
	if s, ok := base(a).(ServiceAccount); ok {
		return s.ServicePrincipal()
	}
	return ""
}

// BrokerFailure carries the HTTP status a relay returns for a broker error.
type BrokerFailure struct {
	Status int
	Err    error
}

func (f *BrokerFailure) Error() string { return f.Err.Error() }
func (f *BrokerFailure) Unwrap() error { return f.Err }

// BrokerStatus is the HTTP status for a broker error; unknown errors are 503.
func BrokerStatus(err error) int {
	var failure *BrokerFailure
	if errors.As(err, &failure) && failure.Status != 0 {
		return failure.Status
	}
	return http.StatusServiceUnavailable
}

// ObserveFunc starts observing one authentication and returns the context to
// use and a completion callback.
type ObserveFunc func(ctx context.Context, mode string) (context.Context, func(error))

type observed struct {
	base    HTTPAuthenticator
	observe ObserveFunc
}

// Observe decorates an authenticator with a caller-owned observation hook.
func Observe(a HTTPAuthenticator, observe ObserveFunc) HTTPAuthenticator {
	if a == nil || observe == nil {
		return a
	}
	if _, wrapped := a.(*observed); wrapped {
		return a
	}
	return &observed{base: a, observe: observe}
}

func (a *observed) Name() string              { return a.base.Name() }
func (a *observed) Unwrap() HTTPAuthenticator { return a.base }

func (a *observed) Authenticate(ctx context.Context, headers http.Header) (HTTPIdentity, error) {
	ctx, done := a.observe(ctx, a.base.Name())
	id, err := a.base.Authenticate(ctx, headers)
	done(err)
	return id, err
}
