package server

import (
	"net/http"

	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
)

func (f *httpFacade) pluginHelp(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) != 0 {
		invoke.WriteInvoke(w, inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "plugin explanation does not accept query parameters")))
		return
	}
	identity, ok := f.serviceIdentity(w, r)
	if !ok {
		return
	}
	flags := map[string]FlagValue{"home": f.home, "name": r.PathValue("name")}
	f.addIdentityFlags(flags, r, identity)
	addHTTPTraceFlags(flags, r)
	unlock := f.lockTypedInvocation("inventory.read")
	defer unlock()
	// Run the same authorized application task without eager source mounting.
	invoke.WriteInvoke(w, inv.InvokeApplicationWithTelemetryAtHome(r.Context(), f.runtime, "object-plugin-help", "inventory.read",
		inv.Command{Stage: invoke.StageHome, Run: inv.VerbPluginHelp}, flags, nil, nil, nil))
}
