// Package identitytest holds the shared authenticator double for tests.
//
// Tests that need a verified caller use this package instead of declaring
// their own Authenticate method; production code never imports it.
package identitytest

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"kc/identity"
	"kc/kernel"
)

// Authenticator is an identity.HTTPAuthenticator whose verdict is Verify.
type Authenticator struct {
	Mode   string
	Verify func(http.Header) (identity.HTTPIdentity, error)
}

// Name reports Mode, or "test" when Mode is empty.
func (a Authenticator) Name() string {
	if strings.TrimSpace(a.Mode) == "" {
		return "test"
	}
	return a.Mode
}

// Authenticate delegates to Verify; a nil Verify rejects every request.
func (a Authenticator) Authenticate(_ context.Context, headers http.Header) (identity.HTTPIdentity, error) {
	if a.Verify == nil {
		return identity.HTTPIdentity{}, Reject("test authenticator has no verifier")
	}
	return a.Verify(headers)
}

// Reject is the unauthenticated failure a double returns for a bad proof.
func Reject(message string) error {
	return kernel.Fail(kernel.ErrUnauthenticated, message)
}

// Bearer accepts exactly the listed Authorization header values.
func Bearer(mode string, proofs map[string]identity.HTTPIdentity) Authenticator {
	return Authenticator{Mode: mode, Verify: func(headers http.Header) (identity.HTTPIdentity, error) {
		if caller, ok := proofs[headers.Get("Authorization")]; ok {
			return caller, nil
		}
		return identity.HTTPIdentity{}, Reject("test authenticator rejected proof")
	}}
}

// Static returns caller for every request, regardless of headers.
func Static(mode string, caller identity.HTTPIdentity) Authenticator {
	return Authenticator{Mode: mode, Verify: func(http.Header) (identity.HTTPIdentity, error) { return caller, nil }}
}

// RequireHeaders returns caller only when every named header is non-empty.
func RequireHeaders(mode string, caller identity.HTTPIdentity, names ...string) Authenticator {
	return Authenticator{Mode: mode, Verify: func(headers http.Header) (identity.HTTPIdentity, error) {
		for _, name := range names {
			if strings.TrimSpace(headers.Get(name)) == "" {
				return identity.HTTPIdentity{}, Reject("missing " + name)
			}
		}
		return caller, nil
	}}
}

var registered sync.Map

// Register adds authenticator to the identity registry under name, exactly as
// a provider subpackage would. Repeated calls with the same name in one test
// process are no-ops, so the first registration wins.
func Register(name string, authenticator identity.HTTPAuthenticator) {
	if _, loaded := registered.LoadOrStore(name, true); loaded {
		return
	}
	identity.Register(name, func(identity.Settings) (identity.HTTPAuthenticator, error) { return authenticator, nil })
}

// Recorder returns Caller and Err and counts Authenticate calls, so a test can
// prove a request was rejected before verification ran. It is not safe for
// concurrent requests.
type Recorder struct {
	Mode   string
	Caller identity.HTTPIdentity
	Err    error
	Calls  int
}

// Name reports Mode, or "test" when Mode is empty.
func (r *Recorder) Name() string { return Authenticator{Mode: r.Mode}.Name() }

// Authenticate counts the call and returns the configured verdict.
func (r *Recorder) Authenticate(context.Context, http.Header) (identity.HTTPIdentity, error) {
	r.Calls++
	return r.Caller, r.Err
}
