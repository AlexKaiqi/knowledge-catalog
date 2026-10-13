package service

import (
	"net/http"
	"strings"

	"kc/identity"
	"kc/kernel"
)

func identityMode(options IdentityOptions) string {
	if options.Authenticator != nil {
		return options.Authenticator.Name()
	}
	mode := strings.ToLower(strings.TrimSpace(options.Mode))
	if mode == "" {
		if options.Local {
			return "local"
		}
		return "unconfigured"
	}
	return mode
}

func localIdentity(options IdentityOptions) bool {
	return options.Local && options.Authenticator == nil && identityMode(options) == "local"
}

// AuthenticateRequest accepts one configured verifier or an explicitly enabled
// local fixture identity. Neither native roles nor self-reported proxy headers
// become authorization or trusted delegation.
func AuthenticateRequest(r *http.Request, options IdentityOptions) (identity.HTTPIdentity, error) {
	return authenticateRequest(r, options, false)
}

func authenticateRequest(r *http.Request, options IdentityOptions, nativeUpload bool) (identity.HTTPIdentity, error) {
	as := strings.TrimSpace(r.Header.Get("X-Kc-As"))
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	onBehalf := strings.TrimSpace(r.Header.Get("X-Kc-On-Behalf-Of"))
	if localIdentity(options) {
		return localRequestIdentity(as, authorization, onBehalf)
	}
	mode := identityMode(options)
	if as != "" && authorization == "" {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "this Server is --auth %s; send Authorization only (X-Kc-As is a pairing mismatch)", mode)
	}
	if options.Authenticator == nil {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "this Server is --auth %s; authentication is not configured", mode)
	}
	if as != "" {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrForbidden, "X-Kc-As is disabled when authentication is enabled")
	}
	if onBehalf != "" {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrForbidden, "onBehalfOf must come from the trusted authenticator")
	}
	if mode == "lakefs" || mode == "taihu" {
		if err := validateLakeFSConsoleRequest(r, nativeUpload); err != nil {
			return identity.HTTPIdentity{}, err
		}
	}
	caller, err := options.Authenticator.Authenticate(r.Context(), r.Header)
	if err != nil {
		return identity.HTTPIdentity{}, err
	}
	if caller.Principal == "" {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "the authenticator did not establish a principal")
	}
	return caller, nil
}

func localRequestIdentity(as, authorization, onBehalf string) (identity.HTTPIdentity, error) {
	if authorization != "" {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "this Server is --auth local; it does not accept Authorization (pairing mismatch)")
	}
	if onBehalf != "" {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrForbidden, "onBehalfOf requires a trusted authenticator")
	}
	if as == "" {
		return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "this Server is --auth local; send X-Kc-As only")
	}
	return identity.HTTPIdentity{Principal: as}, nil
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
	local := localIdentity(h.config.Identity)
	mode := identityMode(h.config.Identity)
	accepts := []string{"Authorization"}
	if local {
		accepts = []string{"X-Kc-As"}
	} else if mode == "lakefs" {
		accepts = []string{"Cookie", "Authorization"}
	}
	out := map[string]any{"mode": mode, "localAssertion": local, "accepts": accepts}
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
