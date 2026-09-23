package cli

import (
	"net/http"
	"strings"

	"kc/identity"
	"kc/kernel"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
)

// HTTPIdentity is the authenticated caller of one HTTP request. Principal is
// the only field used by allow.json. OnBehalfOf, when present, must be a
// delegation claim verified by the authenticator; request headers cannot set
// it in authenticated mode. The remaining fields make whoami useful without
// turning provider claims into authorization rules.
type HTTPIdentity struct {
	Principal  string
	OnBehalfOf string
	Provider   string
	Subject    string
	Login      string
	Admin      bool
	// User contains the verifier's human identity, including the immutable
	// external subject and issuer. It is bound durably before authorization and
	// never exposed as a replacement for the username principal.
	User *identity.VerifiedUser
}

// HTTPServerOptions enables production-style authentication while keeping
// HTTPHandler(home) available as a test seam equivalent to --auth local.
type HTTPServerOptions struct {
	// AuthMode is the pairing advertised by GET /identity/v1/auth.
	// Empty means local when Authenticator is also nil (test seam).
	AuthMode        string
	Authenticator   HTTPAuthenticator
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
	// Embedder is an optional request-time query-vector provider for semantic
	// recall. Nil keeps semantic recall fail-closed (CAPABILITY_UNSATISFIED).
	Embedder retrieval.Embedder
	// Reranker is an optional wall-out semantic provider. Nil keeps ordinary
	// SEARCH available and makes only the explicit rerank operation fail closed.
	Reranker retrieval.Reranker
}

func (o HTTPServerOptions) authenticated() bool { return o.Authenticator != nil }

func (o HTTPServerOptions) authMode() string {
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
	return o.Authenticator == nil && o.authMode() == "local"
}

func (o HTTPServerOptions) isAdmin(id HTTPIdentity) bool {
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

func authenticateHTTPRequest(w http.ResponseWriter, r *http.Request, options HTTPServerOptions) (HTTPIdentity, bool) {
	if !options.authenticated() {
		return HTTPIdentity{}, true
	}
	id, err := options.Authenticator.Authenticate(r.Context(), r.Header)
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
		writeJSON(w, status, kernel.FaultJSON(err))
		return HTTPIdentity{}, false
	}
	return id, true
}

func writeHTTPForbidden(w http.ResponseWriter, format string, args ...any) {
	writeJSON(w, http.StatusForbidden, kernel.FaultJSON(kernel.Fail(kernel.ErrForbidden, format, args...)))
}
