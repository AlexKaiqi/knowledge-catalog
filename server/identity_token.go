package server

import (
	"net/http"

	kcclient "kc/client"
	"kc/identity"
	"kc/invoke"
	"kc/kernel"
)

func (f *httpFacade) browserLoginConfig() *kcclient.BrowserLoginConfig {
	if f.options.LoginBroker == nil {
		return nil
	}
	login := f.options.LoginBroker.BrowserLogin()
	return &kcclient.BrowserLoginConfig{OAuth2Base: login.OAuth2Base, ClientID: login.ClientID, Resource: login.Resource, AppName: login.AppName, Scope: login.Scope}
}

func writeBrokerError(w http.ResponseWriter, err error) {
	invoke.WriteJSON(w, identity.BrokerStatus(err), kernel.FaultJSON(err))
}

// identityToken relays a token exchange to the deployment's login broker. The
// authorization server validates the code/PKCE proof or refresh credential; no
// KC session or grant is created here.
func (f *httpFacade) identityToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	broker := f.options.LoginBroker
	if broker == nil {
		invoke.WriteJSON(w, http.StatusServiceUnavailable, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "browser login is not configured for this deployment")))
		return
	}
	if r.Header.Get("Authorization") != "" || r.Header.Get(identity.SelfAssertion) != "" {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "token exchange accepts authorization proof in its request body only")))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input kcclient.TokenRequest
	if !decodeServiceRequest(w, r, &input) {
		return
	}
	token, err := broker.Token(r.Context(), identity.TokenGrant{GrantType: input.GrantType, Code: input.Code, CodeVerifier: input.CodeVerifier, RedirectURI: input.RedirectURI, RefreshToken: input.RefreshToken})
	if err != nil {
		writeBrokerError(w, err)
		return
	}
	invoke.WriteJSON(w, http.StatusOK, kcclient.TokenResponse{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, ExpiresIn: token.ExpiresIn})
}
