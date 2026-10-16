package server

import (
	"kc/identity"
	"kc/identity/local"
	"kc/invoke"

	"net/http"
	"strings"

	retrieval "kc/discovery"
	"kc/kernel"
	objectserving "kc/liveaccess"
	liveservice "kc/liveaccess/service"
)

// HTTPServerOptions enables production-style authentication while keeping
// HTTPHandler(home) available as a test seam equivalent to --auth local.
type HTTPServerOptions struct {
	// AuthMode is the pairing advertised by GET /identity/v1/auth.
	// Empty means local when Authenticator is also nil (test seam).
	AuthMode string
	// NativeAPIOrigin is a fixed lakeFS origin, never a client-selected proxy.
	NativeAPIOrigin string
	// Authenticator is the selected pairing. Nil with an empty or local
	// AuthMode is the local test seam; any other nil fails closed.
	Authenticator   identity.HTTPAuthenticator
	AdminPrincipals []string
	// LoginBroker relays the deployment-owned login application for clients;
	// nil means browser and CLI login are not offered by this Server.
	LoginBroker identity.LoginBroker
	// ServicePrincipal is the service's own principal for background
	// observation; empty installs no worker identity.
	ServicePrincipal string
	// StateLookup may be supplied by an embedding application. Standalone
	// kc serve installs a generic HTTP adapter; each request uses the origin
	// declared on that Binding's Domain Schema. Nil fails bound READs instead
	// of returning a misleading null value.
	StateLookup objectserving.StateLookup
	// LiveAccessPlugins supplies compiled entries; Repo metadata remains the
	// adoption authority. The standard entry may use StateLookup as its adapter.
	LiveAccessPlugins map[string]liveservice.PluginEntry
	// LiveAccessCredentialOrigins is the deployment's trusted proof audience
	// set. Descriptor origins and redirects cannot extend this set.
	LiveAccessCredentialOrigins []string
	// LiveAccessReadLimits bounds each external unit and the time all external
	// units of one request share. Zero values leave a limit unset.
	LiveAccessReadLimits objectserving.ReadLimits
	// Embedder is an optional request-time query-vector provider for semantic
	// recall. Nil keeps semantic recall fail-closed (CAPABILITY_UNSATISFIED).
	Embedder retrieval.Embedder
	// Reranker is an optional wall-out semantic provider. Nil keeps ordinary
	// SEARCH available and makes only the explicit rerank operation fail closed.
	Reranker retrieval.Reranker
}

// authenticator is the effective pairing. The zero-value options used by
// HTTPHandler(home) and a bare --auth local resolve to the local
// authenticator, so every request goes through the same identity decision.
func (o HTTPServerOptions) authenticator() identity.HTTPAuthenticator {
	if o.Authenticator == nil {
		if mode := strings.TrimSpace(o.AuthMode); mode == "" || strings.EqualFold(mode, local.Name) {
			return local.New()
		}
	}
	return o.Authenticator
}

// Authenticated reports whether callers must prove identity. A
// self-asserting pairing (local) only names the caller.
func (o HTTPServerOptions) Authenticated() bool {
	a := o.authenticator()
	return a != nil && !identity.SelfAsserted(a)
}

func (o HTTPServerOptions) AuthModeLabel() string {
	return identity.Mode(o.authenticator(), o.AuthMode)
}

func (o HTTPServerOptions) localAssertion() bool { return identity.SelfAsserted(o.authenticator()) }

func (o HTTPServerOptions) isAdmin(id identity.HTTPIdentity) bool {
	if id.Admin {
		return true
	}
	for _, principal := range o.AdminPrincipals {
		if strings.TrimSpace(principal) == id.Principal {
			return true
		}
	}
	return false
}

func authenticateHTTPRequest(w http.ResponseWriter, r *http.Request, options HTTPServerOptions) (identity.HTTPIdentity, bool) {
	if !options.Authenticated() {
		return identity.HTTPIdentity{}, true
	}
	id, err := identity.Authenticate(r, identity.RequestOptions{Authenticator: options.authenticator(), Mode: options.AuthMode, Check: identity.BrowserRequestCheck(false)})
	if err != nil {
		status := http.StatusUnauthorized
		if kernel.CodeOf(err) == kernel.ErrTemporaryUnavailable {
			status = http.StatusServiceUnavailable
		} else if kernel.CodeOf(err) == kernel.ErrForbidden {
			status = http.StatusForbidden
		} else if kernel.CodeOf(err) == kernel.ErrPreconditionFailed {
			status = http.StatusServiceUnavailable
		} else {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kc"`)
		}
		invoke.WriteJSON(w, status, kernel.FaultJSON(err))
		return identity.HTTPIdentity{}, false
	}
	return id, true
}

func writeHTTPForbidden(w http.ResponseWriter, format string, args ...any) {
	invoke.WriteJSON(w, http.StatusForbidden, kernel.FaultJSON(kernel.Fail(kernel.ErrForbidden, format, args...)))
}
