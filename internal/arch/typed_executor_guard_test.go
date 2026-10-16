package arch_test

import (
	"go/ast"
	"maps"
	"strings"
	"testing"
)

func TestTypedExecutorGuardFollowsDelegationAndRejectsDisconnectedEvidence(t *testing.T) {
	sources := map[string]string{
		"invocation/verbs.go": `package invocation
import access "kc/liveaccess/service"
import discovery "kc/discovery/service"
func VerbRead() { OnTarget(func() { readRepositoryObserved() }) }
func OnTarget(read func()) { read() }
func VerbSearch() { searchDataset() }`,
		"invocation/repository_read.go": `package invocation
import liveservice "kc/liveaccess/service"
func readRepositoryObserved() { liveservice.ReadRepository() }`,
		"invocation/dataset_search.go": `package invocation
import discovery "kc/discovery/service"
func searchDataset() { (discovery.DatasetSearchExecutor{}).Execute() }`,
		"cli/commands_test.go": `package cli
import inv "kc/invocation"
func readVerbs() map[string]command { return map[string]command{"object-read": {Run: inv.VerbRead}} }
func indexVerbs() map[string]command { return map[string]command{"object-search": {Run: inv.VerbSearch}} }`,
		"server/routes.go": `package server
import inv "kc/invocation"
func (f *httpFacade) registerServiceRoutes(mux Mux) {
 mux.HandleFunc("POST /object/v1/objects:read", f.objectRead)
 mux.HandleFunc("POST /object/v1/search", f.objectSearch)
}
func (f *httpFacade) objectRead() { f.executeTyped(w,r,"object-read","object.read",inv.Command{Run: inv.VerbRead},flags) }
func (f *httpFacade) objectSearch() { f.executeTyped(w,r,"object-search","file.read",inv.Command{Run: inv.VerbSearch},flags) }
func (f *httpFacade) executeTyped() { inv.InvokeApplicationWithTelemetryAtHome() }`,
	}
	parse := func(sources map[string]string) map[string]*ast.File {
		files := map[string]*ast.File{}
		for path, source := range sources {
			files[path] = declarativeIndexGuardFixture(t, path, source)
		}
		return files
	}
	if signals := typedExecutorWiringSignals(parse(sources)); len(signals) != 0 {
		t.Fatalf("real delegated helper across files and an invoked callback rejected: %v", signals)
	}
	for _, fixture := range []struct{ name, path, before, after, signal string }{
		{"direct executor", "invocation/verbs.go", "OnTarget(func() { readRepositoryObserved() })", "access.ReadRepository()", ""},
		{"named callback", "invocation/verbs.go", "OnTarget(func() { readRepositoryObserved() })", "OnTarget(readRepositoryObserved)", ""},
		{"uncalled helper", "invocation/verbs.go", "OnTarget(func() { readRepositoryObserved() })", "unrelated()", "VerbRead does not call"},
		{"comment cannot call helper", "invocation/verbs.go", "OnTarget(func() { readRepositoryObserved() })", "/* OnTarget(func() { readRepositoryObserved() }) */", "VerbRead does not call"},
		{"unused callback", "invocation/verbs.go", "OnTarget(func() { readRepositoryObserved() })", "_ = func() { readRepositoryObserved() }", "VerbRead does not call"},
		{"callee does not invoke callback", "invocation/verbs.go", "func OnTarget(read func()) { read() }", "func OnTarget(read func()) { _ = read }", "VerbRead does not call"},
		{"foreign executor alias", "invocation/repository_read.go", `"kc/liveaccess/service"`, `"kc/unrelated/service"`, "VerbRead does not call"},
		{"shadowed executor alias", "invocation/repository_read.go", "liveservice.ReadRepository()", "liveservice := other; liveservice.ReadRepository()", "VerbRead does not call"},
		{"direct Dataset search executor", "invocation/verbs.go", "searchDataset() }", "(discovery.DatasetSearchExecutor{}).Execute() }", ""},
		{"uncalled search helper", "invocation/verbs.go", "searchDataset() }", "unrelated() }", "VerbSearch does not call"},
		{"constructed but not executed search", "invocation/dataset_search.go", "(discovery.DatasetSearchExecutor{}).Execute()", "_ = discovery.DatasetSearchExecutor{}", "VerbSearch does not call"},
		{"retired repository search executor", "invocation/dataset_search.go", "discovery.DatasetSearchExecutor", "discovery.SearchExecutor", "VerbSearch does not call"},
		{"foreign search executor alias", "invocation/dataset_search.go", `"kc/discovery/service"`, `"kc/unrelated/service"`, "VerbSearch does not call"},
		{"shadowed search executor alias", "invocation/dataset_search.go", "(discovery.DatasetSearchExecutor{}).Execute()", "discovery := other; (discovery.DatasetSearchExecutor{}).Execute()", "VerbSearch does not call"},
		{"missing CLI route", "cli/commands_test.go", `"object-read": {Run: inv.VerbRead}`, `"unrelated": {Run: inv.VerbRead}`, "route is not bound"},
		{"missing HTTP registration", "server/routes.go", `mux.HandleFunc("POST /object/v1/objects:read", f.objectRead)`, "", "route is not bound"},
		{"comment cannot bind Run", "server/routes.go", "Run: inv.VerbRead", "Run: nil /* Run: inv.VerbRead */", "route is not bound"},
		{"foreign invocation alias", "server/routes.go", `"kc/invocation"`, `"kc/unrelated/invocation"`, "route is not bound"},
		{"handler uses another facade", "server/routes.go", "f.executeTyped(w,r,\"object-read\"", "other.executeTyped(w,r,\"object-read\"", "route is not bound"},
		{"called helper detours around executor", "invocation/repository_read.go", "liveservice.ReadRepository()", "unrelated()", "VerbRead does not call"},
		{"recursive detour", "invocation/repository_read.go", "liveservice.ReadRepository()", "readRepositoryObserved()", "VerbRead does not call"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			modified := maps.Clone(sources)
			if !strings.Contains(modified[fixture.path], fixture.before) {
				t.Fatal("counterexample no longer changes its selected wiring")
			}
			modified[fixture.path] = strings.Replace(modified[fixture.path], fixture.before, fixture.after, 1)
			signals := typedExecutorWiringSignals(parse(modified))
			if fixture.signal == "" {
				if len(signals) != 0 {
					t.Fatalf("valid wiring rejected: %v", signals)
				}
			} else if !strings.Contains(strings.Join(signals, "\n"), fixture.signal) {
				t.Fatalf("disconnected wiring was accepted: want %q in %v", fixture.signal, signals)
			}
		})
	}
}
