package identity

import (
	"context"
	"net/http"

	"kc/kernel"
)

type boundAuthenticator struct {
	base       HTTPAuthenticator
	path       string
	startupErr error
}

// WrapAuthenticator binds verified human provider claims to the initialized
// identity authority before authorization. Fixtures may initialize their separate state;
// declared deployments must have initialized it explicitly.
func WrapAuthenticator(base HTTPAuthenticator, bindingPath string, fixture bool) HTTPAuthenticator {
	if base == nil {
		return nil
	}
	a := &boundAuthenticator{base: base, path: bindingPath}
	if fixture {
		a.startupErr = Initialize(a.path)
	}
	return a
}

func (a *boundAuthenticator) Name() string              { return a.base.Name() }
func (a *boundAuthenticator) Unwrap() HTTPAuthenticator { return a.base }

func (a *boundAuthenticator) Authenticate(ctx context.Context, headers http.Header) (HTTPIdentity, error) {
	id, err := a.base.Authenticate(ctx, headers)
	if err != nil {
		return HTTPIdentity{}, err
	}
	if id.User == nil {
		return id, nil
	} // Explicit embedding authenticators own non-user identities.
	if id.Principal != id.User.Username && id.OnBehalfOf != id.User.Username {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "verified username does not match the request identity")
	}
	if a.startupErr != nil {
		return HTTPIdentity{}, a.startupErr
	}
	if err := Bind(a.path, *id.User); err != nil {
		return HTTPIdentity{}, err
	}
	return id, nil
}
