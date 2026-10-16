package server

import (
	"context"
	"strings"

	"kc/identity"
	"kc/internal/telemetry"
	inv "kc/invocation"
)

func observeHTTPAuthenticator(base identity.HTTPAuthenticator, runtime *telemetry.Runtime) identity.HTTPAuthenticator {
	if runtime == nil {
		return base
	}
	return identity.Observe(base, func(ctx context.Context, mode string) (context.Context, func(error)) {
		ctx, span, started := runtime.StartAuthentication(ctx, mode)
		return ctx, func(err error) {
			outcome, errorType := inv.TelemetryResult(err)
			runtime.EndAuthentication(ctx, span, started, mode, outcome, errorType)
		}
	})
}

func recordHTTPIdentity(runtime *telemetry.Runtime, ctx context.Context, options HTTPServerOptions, caller identity.HTTPIdentity) {
	if runtime == nil {
		return
	}
	provider := "local"
	if options.Authenticated() {
		provider = strings.TrimSpace(caller.Provider)
		if provider == "" {
			provider = options.authenticator().Name()
		}
	}
	runtime.RecordIdentity(ctx, provider, inv.PrincipalKind(caller.Principal), caller.OnBehalfOf != "")
}
