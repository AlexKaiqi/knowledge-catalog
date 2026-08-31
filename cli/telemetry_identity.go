package cli

import (
	"context"
	"net/http"
	"strings"

	"kc/internal/telemetry"
)

type observedHTTPAuthenticator struct {
	base    HTTPAuthenticator
	runtime *telemetry.Runtime
}

func observeHTTPAuthenticator(base HTTPAuthenticator, runtime *telemetry.Runtime) HTTPAuthenticator {
	if base == nil || runtime == nil {
		return base
	}
	if _, wrapped := base.(*observedHTTPAuthenticator); wrapped {
		return base
	}
	return &observedHTTPAuthenticator{base: base, runtime: runtime}
}

func (a *observedHTTPAuthenticator) Name() string { return a.base.Name() }

func (a *observedHTTPAuthenticator) Authenticate(ctx context.Context, headers http.Header) (HTTPIdentity, error) {
	ctx, span, started := a.runtime.StartAuthentication(ctx, a.base.Name())
	identity, err := a.base.Authenticate(ctx, headers)
	outcome, errorType := telemetryResult(err)
	a.runtime.EndAuthentication(ctx, span, started, a.base.Name(), outcome, errorType)
	return identity, err
}

func recordHTTPIdentity(runtime *telemetry.Runtime, ctx context.Context, options HTTPServerOptions, identity HTTPIdentity) {
	if runtime == nil {
		return
	}
	provider := "local"
	if options.authenticated() {
		provider = strings.TrimSpace(identity.Provider)
		if provider == "" {
			provider = options.Authenticator.Name()
		}
	}
	runtime.RecordIdentity(ctx, provider, principalKind(identity.Principal), identity.OnBehalfOf != "")
}

func principalKind(principal string) string {
	principal = strings.ToLower(strings.TrimSpace(principal))
	switch {
	case principal == "owner" || strings.HasPrefix(principal, "owner:"):
		return "owner"
	case strings.HasPrefix(principal, "agent:"):
		return "agent"
	case strings.HasPrefix(principal, "service:") || strings.Contains(principal, ":service:"):
		return "service"
	case strings.HasPrefix(principal, "user:"), strings.HasPrefix(principal, "gitea:"), strings.HasPrefix(principal, "oidc:"), strings.HasPrefix(principal, "taihu:"):
		return "user"
	default:
		return "other"
	}
}
