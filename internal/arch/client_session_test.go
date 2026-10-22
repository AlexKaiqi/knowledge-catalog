package arch_test

import "testing"

func TestClientSessionBoundaryIncludesIndirectProvidersAndApplicationHelpers(t *testing.T) {
	for _, denied := range []string{"cli", "invocation", "invoke", "home", "server", "identity/taihu", "object/writer/internal/ledger", "snapshot/internal/lakefsimpl", "declarativeindex/opensearch/internal/opensearchimpl"} {
		t.Run(denied, func(t *testing.T) {
			graph := depGraph{"client/session": {"internal/session-helper"}, "internal/session-helper": {denied}}
			if _, ok := reachDenied(graph.reachable("client/session"), denied); !ok {
				t.Fatalf("indirect session dependency escaped guard: %s", denied)
			}
		})
	}
	graph := depGraph{"client/session": {"client/auth", "kernel", "internal/jsonfile"}, "client/auth": {"kernel", "internal/httpjson"}}
	for _, denied := range []string{"cli", "invocation", "home", "server", "snapshot/internal"} {
		if path, ok := reachDenied(graph.reachable("client/session"), denied); ok {
			t.Fatalf("SDK-only caller boundary rejected: %v", path)
		}
	}
}
