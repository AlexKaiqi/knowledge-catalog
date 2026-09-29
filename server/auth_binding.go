package server

import (
	"context"
	inv "kc/invocation"
	"net/http"
	"path/filepath"

	"kc/identity"
	"kc/kernel"
)

// boundHTTPAuthenticator turns verified provider claims into a durable KC
// identity before authorization. Fixtures may initialize their separate state;
// declared deployments must have initialized it explicitly.
type boundHTTPAuthenticator struct {
	base       inv.HTTPAuthenticator
	path       string
	startupErr error
}

func bindHTTPAuthenticator(base inv.HTTPAuthenticator, home string, fixture bool) inv.HTTPAuthenticator {
	if base == nil {
		return nil
	}
	a := &boundHTTPAuthenticator{base: base, path: filepath.Join(home, identity.Filename)}
	if fixture {
		a.startupErr = identity.Initialize(a.path)
	}
	return a
}

func (a *boundHTTPAuthenticator) Name() string { return a.base.Name() }

func (a *boundHTTPAuthenticator) Authenticate(ctx context.Context, headers http.Header) (inv.HTTPIdentity, error) {
	id, err := a.base.Authenticate(ctx, headers)
	if err != nil {
		return inv.HTTPIdentity{}, err
	}
	if id.User == nil {
		return id, nil
	} // Explicit embedding authenticators own non-user identities.
	if id.Principal != id.User.Username && id.OnBehalfOf != id.User.Username {
		return inv.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "verified username does not match the request identity")
	}
	if a.startupErr != nil {
		return inv.HTTPIdentity{}, a.startupErr
	}
	if err := identity.Bind(a.path, *id.User); err != nil {
		return inv.HTTPIdentity{}, err
	}
	return id, nil
}
