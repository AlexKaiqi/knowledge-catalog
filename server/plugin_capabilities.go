package server

import (
	"net/http"

	retrieval "kc/discovery"
	apphome "kc/home"
	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
)

func rejectPluginQuery(w http.ResponseWriter, r *http.Request) bool {
	// RawQuery, not the parsed map: malformed escapes are dropped by Query().
	if r.URL.RawQuery == "" && !r.URL.ForceQuery {
		return false
	}
	invoke.WriteInvoke(w, inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "plugin capability tasks do not accept query parameters")))
	return true
}

func (f *httpFacade) pluginCapabilities(w http.ResponseWriter, r *http.Request) {
	if rejectPluginQuery(w, r) {
		return
	}
	f.executePluginRead(w, r, "object-plugin-capabilities", "inventory.read", inv.VerbPluginCapabilities, map[string]FlagValue{"name": r.PathValue("name")})
}

func (f *httpFacade) trialDeclarations(w http.ResponseWriter, r *http.Request) {
	if rejectPluginQuery(w, r) {
		return
	}
	var request retrieval.DeclarationTrialRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := compactFlags(map[string]FlagValue{"name": r.PathValue("name"), "repo": string(request.Repository), "commit": string(request.Commit), "ref": request.Ref})
	flags["_declaration-trial"] = request
	f.executePluginRead(w, r, "object-plugin-trial", "object.read", inv.VerbTrialDeclarations, flags)
}

// Capability explanation and validation must not activate maintenance merely
// because they are the first request to a cold service.
func (f *httpFacade) pluginReadHome() (*Home, error) {
	f.homeMu.Lock()
	defer f.homeMu.Unlock()
	if f.closed {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "HTTP runtime has closed")
	}
	if f.deployment != nil {
		if err := apphome.ValidateDeploymentState(*f.deployment); err != nil {
			return nil, err
		}
	}
	if f.readHome == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "plugin capabilities require an assembled service runtime; use the configured deployment or start the legacy handler explicitly")
	}
	return f.readHome, nil
}

func (f *httpFacade) executePluginRead(w http.ResponseWriter, r *http.Request, name, action string, run inv.Handler, flags map[string]FlagValue) {
	identity, ok := f.serviceIdentity(w, r)
	if !ok {
		return
	}
	flags["home"] = f.home
	f.addIdentityFlags(flags, r, identity)
	addHTTPTraceFlags(flags, r)
	unlock := f.lockTypedInvocation(action)
	defer unlock()
	opened, err := f.pluginReadHome()
	if err != nil {
		invoke.WriteInvoke(w, inv.ErrorResult(err))
		return
	}
	invoke.WriteInvoke(w, inv.InvokeApplicationWithTelemetryAtHome(r.Context(), f.runtime, name, action, inv.Command{Stage: invoke.StageGoverned, Run: run}, flags, nil, opened))
}
