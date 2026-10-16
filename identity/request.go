package identity

import (
	"context"
	"net/http"
	"strings"

	"kc/kernel"
)

// Pairing is implemented by an authenticator that names the request headers
// it consumes. Authenticators without it accept Authorization only.
type Pairing interface {
	Accepts() []string
}

// Wrapper is implemented by decorators (binding, telemetry) so capability
// queries reach the authenticator they decorate.
type Wrapper interface {
	Unwrap() HTTPAuthenticator
}

// SelfAssertion is the header a self-asserting authenticator reads. Such a
// caller names itself; only explicitly selected local pairing does this.
const SelfAssertion = "X-Kc-As"

func base(a HTTPAuthenticator) HTTPAuthenticator {
	for a != nil {
		w, ok := a.(Wrapper)
		if !ok {
			return a
		}
		a = w.Unwrap()
	}
	return nil
}

// Accepts lists the request headers that carry caller proof.
func Accepts(a HTTPAuthenticator) []string {
	if p, ok := base(a).(Pairing); ok {
		return append([]string(nil), p.Accepts()...)
	}
	return []string{"Authorization"}
}

// SelfAsserted reports whether the caller names itself through X-Kc-As.
func SelfAsserted(a HTTPAuthenticator) bool {
	if a == nil {
		return false
	}
	for _, header := range Accepts(a) {
		if header == SelfAssertion {
			return true
		}
	}
	return false
}

// Mode is the pairing name advertised to clients. Without an authenticator the
// configured label is reported, so an unconfigured server still explains itself.
func Mode(a HTTPAuthenticator, configured string) string {
	if a != nil {
		return a.Name()
	}
	if mode := strings.ToLower(strings.TrimSpace(configured)); mode != "" {
		return mode
	}
	return "unconfigured"
}

// RequestOptions configures the single request-authentication decision.
type RequestOptions struct {
	// Authenticator verifies the caller. Nil fails closed.
	Authenticator HTTPAuthenticator
	// Mode labels an unconfigured server in error messages.
	Mode string
	// Check is a caller-owned request policy (for example the browser request
	// check) that runs before a verifier sees credentials. Self-asserting
	// pairing has no browser credential and skips it.
	Check func(*http.Request) error
}

// Authenticate is the only place a request becomes an identity. Token pairing
// rejects self-reported identity and delegation; self-asserting pairing owns
// its own header rules. Every accepted identity has a principal.
func Authenticate(r *http.Request, options RequestOptions) (HTTPIdentity, error) {
	a := options.Authenticator
	if SelfAsserted(a) {
		return verified(r.Context(), a, r.Header)
	}
	mode := Mode(a, options.Mode)
	as := strings.TrimSpace(r.Header.Get(SelfAssertion))
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if as != "" && authorization == "" {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "this Server is --auth %s; send Authorization only (X-Kc-As is a pairing mismatch)", mode)
	}
	if a == nil {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "this Server is --auth %s; authentication is not configured", mode)
	}
	if as != "" {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrForbidden, "X-Kc-As is disabled when authentication is enabled")
	}
	if strings.TrimSpace(r.Header.Get("X-Kc-On-Behalf-Of")) != "" {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrForbidden, "onBehalfOf must come from the trusted authenticator")
	}
	if options.Check != nil {
		if err := options.Check(r); err != nil {
			return HTTPIdentity{}, err
		}
	}
	return verified(r.Context(), a, r.Header)
}

func verified(ctx context.Context, a HTTPAuthenticator, headers http.Header) (HTTPIdentity, error) {
	caller, err := a.Authenticate(ctx, headers)
	if err != nil {
		return HTTPIdentity{}, err
	}
	if caller.Principal == "" {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "the authenticator did not establish a principal")
	}
	return caller, nil
}
