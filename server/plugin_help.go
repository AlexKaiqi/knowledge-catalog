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
	release, admitted := f.admitRequest()
	defer release()
	run := inv.VerbPluginHelp
	var opened *Home
	if admitted {
		f.homeMu.Lock()
		opened = f.readHome
		f.homeMu.Unlock()
	}
	if opened != nil && opened.Plugins != nil {
		// Borrow the configured Entry's explanation when the serving root is
		// already assembled. This reads no source and never opens a runtime.
		if explanation, err := opened.Plugins.Help(r.PathValue("name")); err == nil {
			run = func(*inv.Invocation) (any, error) { return explanation, nil }
		}
	}
	// Run the same authorized application task without eager source mounting.
	invoke.WriteInvoke(w, inv.InvokeApplicationWithTelemetryAtHome(r.Context(), f.runtime, "object-plugin-help", "inventory.read",
		inv.Command{Stage: invoke.StageHome, Run: run}, flags, nil, nil))
}
