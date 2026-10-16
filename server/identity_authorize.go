package server

import (
	"net/http"

	"kc/identity"
	"kc/invoke"
	"kc/kernel"
)

// Browser authorization relays the broker's stateless PAR/PKCE flow. Neither
// endpoint establishes a KC session nor accepts application secrets.
func (f *httpFacade) identityAuthorize(w http.ResponseWriter, r *http.Request) {
	broker, ok := f.browserAuthorizationBroker(w)
	if !ok {
		return
	}
	var input struct {
		CodeChallenge string `json:"codeChallenge"`
		State         string `json:"state"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if !decodeServiceRequest(w, r, &input) || !rejectCallerIdentity(w, r) {
		return
	}
	result, err := broker.Authorize(r.Context(), input.CodeChallenge, input.State)
	if err != nil {
		writeBrokerError(w, err)
		return
	}
	invoke.WriteJSON(w, http.StatusOK, map[string]any{"authorizationURL": result.URL, "requestURI": result.RequestURI, "expiresIn": result.ExpiresIn})
}

func (f *httpFacade) identityAuthorizePoll(w http.ResponseWriter, r *http.Request) {
	broker, ok := f.browserAuthorizationBroker(w)
	if !ok {
		return
	}
	var input struct {
		RequestURI string `json:"requestURI"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if !decodeServiceRequest(w, r, &input) || !rejectCallerIdentity(w, r) {
		return
	}
	result, err := broker.PollAuthorization(r.Context(), input.RequestURI)
	if err != nil {
		writeBrokerError(w, err)
		return
	}
	out := map[string]string{"status": result.Status}
	if result.Status == "completed" {
		out["code"], out["redirectURI"] = result.Code, result.RedirectURI
	}
	invoke.WriteJSON(w, http.StatusOK, out)
}

func (f *httpFacade) browserAuthorizationBroker(w http.ResponseWriter) (identity.LoginBroker, bool) {
	if f.options.LoginBroker == nil {
		invoke.WriteJSON(w, http.StatusServiceUnavailable, kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "browser login is not configured")))
		return nil, false
	}
	return f.options.LoginBroker, true
}

func rejectCallerIdentity(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if r.Header.Get("Authorization") != "" || r.Header.Get(identity.SelfAssertion) != "" {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "browser authorization does not accept caller identity headers")))
		return false
	}
	return true
}
