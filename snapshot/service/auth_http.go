package service

import (
	"net/http"

	"kc/identity"
	"kc/kernel"
)

// AuthenticateRequest applies the configured pairing. Neither native roles
// nor self-reported proxy headers become authorization or trusted delegation.
func AuthenticateRequest(r *http.Request, options IdentityOptions) (identity.HTTPIdentity, error) {
	return authenticateRequest(r, options, false)
}

func authenticateRequest(r *http.Request, options IdentityOptions, nativeUpload bool) (identity.HTTPIdentity, error) {
	return identity.Authenticate(r, identity.RequestOptions{Authenticator: options.Authenticator, Mode: options.Mode, Check: identity.BrowserRequestCheck(nativeUpload)})
}

func (h *handler) authenticate(w http.ResponseWriter, r *http.Request) (identity.HTTPIdentity, bool) {
	caller, err := AuthenticateRequest(r, h.config.Identity)
	if err == nil {
		return caller, true
	}
	status := http.StatusUnauthorized
	switch kernel.CodeOf(err) {
	case kernel.ErrTemporaryUnavailable, kernel.ErrPreconditionFailed:
		status = http.StatusServiceUnavailable
	case kernel.ErrForbidden:
		status = http.StatusForbidden
	default:
		w.Header().Set("WWW-Authenticate", `Bearer realm="kc"`)
	}
	writeJSON(w, status, kernel.FaultJSON(err))
	return identity.HTTPIdentity{}, false
}

func (h *handler) identityAuth(w http.ResponseWriter, _ *http.Request) {
	a := h.config.Identity.Authenticator
	out := map[string]any{"mode": identity.Mode(a, h.config.Identity.Mode), "localAssertion": identity.SelfAsserted(a), "accepts": identity.Accepts(a)}
	if h.config.Identity.ProviderURL != "" {
		out["providerURL"] = h.config.Identity.ProviderURL
	}
	if h.config.Identity.BrowserLogin != nil {
		out["browserLogin"] = h.config.Identity.BrowserLogin
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) identityWhoAmI(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	_, finish := h.observeRequest(r, caller, "whoami")
	out := map[string]any{"principal": caller.Principal, "onBehalfOf": caller.OnBehalfOf}
	if caller.Login != "" {
		out["login"] = caller.Login
	}
	if finish != nil {
		finish(out, nil)
	}
	writeJSON(w, http.StatusOK, out)
}
