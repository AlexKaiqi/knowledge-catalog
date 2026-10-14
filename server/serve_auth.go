package server

import (
	"kc/interfaces"
	inv "kc/invocation"
	"kc/invoke"

	"net/http"
	"strings"

	retrieval "kc/discovery"
	"kc/kernel"
	knowledgeserving "kc/liveaccess"
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
	Authenticator   inv.HTTPAuthenticator
	AdminPrincipals []string
	// ServiceIdentity identifies the KC service to Taihu for token introspection
	// and client-side login flows. When non-nil, the service has a registered
	// Taihu application identity.
	ServiceIdentity *ServiceIdentity
	// StateLookup may be supplied by an embedding application. Standalone
	// kc serve installs a generic HTTP adapter; each request uses the origin
	// declared on that Binding's Domain Schema. Nil fails bound READs instead
	// of returning a misleading null value.
	StateLookup knowledgeserving.StateLookup
	// LiveAccessPlugins supplies compiled entries; Repo metadata remains the
	// adoption authority. The standard entry may use StateLookup as its adapter.
	LiveAccessPlugins map[string]liveservice.PluginEntry
	// LiveAccessCredentialOrigins is the deployment's trusted proof audience
	// set. Object Schema origin and redirects cannot extend this set.
	LiveAccessCredentialOrigins []string
	// ObjectInterfaces installs independent object capability handlers. Nil
	// enables only the verified, published LiveAccess reference implementation.
	ObjectInterfaces *interfaces.InterfaceRegistry
	// Embedder is an optional request-time query-vector provider for semantic
	// recall. Nil keeps semantic recall fail-closed (CAPABILITY_UNSATISFIED).
	Embedder retrieval.Embedder
	// Reranker is an optional wall-out semantic provider. Nil keeps ordinary
	// SEARCH available and makes only the explicit rerank operation fail closed.
	Reranker retrieval.Reranker
}

func (o HTTPServerOptions) Authenticated() bool { return o.Authenticator != nil }

func (o HTTPServerOptions) AuthModeLabel() string {
	if o.Authenticator != nil {
		return o.Authenticator.Name()
	}
	mode := strings.ToLower(strings.TrimSpace(o.AuthMode))
	if mode == "" {
		return "local"
	}
	return mode
}

func (o HTTPServerOptions) localAssertion() bool {
	return o.Authenticator == nil && o.AuthModeLabel() == "local"
}

func (o HTTPServerOptions) isAdmin(id inv.HTTPIdentity) bool {
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

func authenticateHTTPRequest(w http.ResponseWriter, r *http.Request, options HTTPServerOptions) (inv.HTTPIdentity, bool) {
	if !options.Authenticated() {
		return inv.HTTPIdentity{}, true
	}
	var id inv.HTTPIdentity
	var err error
	if options.AuthModeLabel() == "lakefs" || options.AuthModeLabel() == "taihu" {
		err = validateLakeFSConsoleRequest(r)
	}
	if err == nil {
		id, err = options.Authenticator.Authenticate(r.Context(), r.Header)
	}
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
		return inv.HTTPIdentity{}, false
	}
	return id, true
}

func writeHTTPForbidden(w http.ResponseWriter, format string, args ...any) {
	invoke.WriteJSON(w, http.StatusForbidden, kernel.FaultJSON(kernel.Fail(kernel.ErrForbidden, format, args...)))
}
