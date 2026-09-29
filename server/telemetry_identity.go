package server

import (
	"context"
	inv "kc/invocation"
	"net/http"
	"strings"

	"kc/internal/telemetry"
)

type observedHTTPAuthenticator struct {
	base    inv.HTTPAuthenticator
	runtime *telemetry.Runtime
}

func observeHTTPAuthenticator(base inv.HTTPAuthenticator, runtime *telemetry.Runtime) inv.HTTPAuthenticator {
	if base == nil || runtime == nil {
		return base
	}
	if _, wrapped := base.(*observedHTTPAuthenticator); wrapped {
		return base
	}
	return &observedHTTPAuthenticator{base: base, runtime: runtime}
}

func (a *observedHTTPAuthenticator) Name() string { return a.base.Name() }

func (a *observedHTTPAuthenticator) Authenticate(ctx context.Context, headers http.Header) (inv.HTTPIdentity, error) {
	ctx, span, started := a.runtime.StartAuthentication(ctx, a.base.Name())
	identity, err := a.base.Authenticate(ctx, headers)
	outcome, errorType := inv.TelemetryResult(err)
	a.runtime.EndAuthentication(ctx, span, started, a.base.Name(), outcome, errorType)
	return identity, err
}

func recordHTTPIdentity(runtime *telemetry.Runtime, ctx context.Context, options HTTPServerOptions, identity inv.HTTPIdentity) {
	if runtime == nil {
		return
	}
	provider := "local"
	if options.Authenticated() {
		provider = strings.TrimSpace(identity.Provider)
		if provider == "" {
			provider = options.Authenticator.Name()
		}
	}
	runtime.RecordIdentity(ctx, provider, inv.PrincipalKind(identity.Principal), identity.OnBehalfOf != "")
}
