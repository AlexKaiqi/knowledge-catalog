// Package local is the no-verification pairing selected by `--auth local`.
// The caller names itself with X-Kc-As; authorization still comes from
// allow.json. It is meant for single-user machines and fixtures only.
package local

import (
	"context"
	"net/http"
	"strings"

	"kc/identity"
	"kc/kernel"
)

// Name is the `--auth` value of this pairing.
const Name = "local"

type authenticator struct{}

// New returns the local self-asserting authenticator.
func New() identity.HTTPAuthenticator { return authenticator{} }

func (authenticator) Name() string      { return Name }
func (authenticator) Accepts() []string { return []string{identity.SelfAssertion} }

func (authenticator) Authenticate(_ context.Context, headers http.Header) (identity.HTTPIdentity, error) {
	if strings.TrimSpace(headers.Get("Authorization")) != "" {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "this Server is --auth local; it does not accept Authorization (pairing mismatch)")
	}
	if strings.TrimSpace(headers.Get("X-Kc-On-Behalf-Of")) != "" {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrForbidden, "onBehalfOf requires a trusted authenticator")
	}
	as := strings.TrimSpace(headers.Get(identity.SelfAssertion))
	if as == "" {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "this Server is --auth local; send X-Kc-As only")
	}
	return identity.HTTPIdentity{Principal: as}, nil
}

func factory(settings identity.Settings) (identity.HTTPAuthenticator, error) {
	if settings.String("auth-url") != "" || len(settings.Strings("auth-admin")) > 0 || settings.String("auth-hmac-secret") != "" {
		return nil, identity.SettingsError("--auth local does not accept --auth-url, --auth-admin, or --auth-hmac-secret")
	}
	return New(), nil
}

func init() { identity.Register(Name, factory) }
