//go:build !short

package serving_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"kc/index"
	"kc/index/opensearch"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/serving"
	"kc/observability"
	"kc/retrieval"
	"kc/snapshot"
)

func TestLiveHTTPStateRuntimeContainer(t *testing.T) {
	origin := os.Getenv("KC_TEST_STATE_RUNTIME_URL")
	if origin == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_STATE_RUNTIME_URL is required")
		}
		t.Skip("set KC_TEST_STATE_RUNTIME_URL or run make test-state-runtime-e2e")
	}
	lookup := serving.NewHTTPStateLookup(nil)
	result, err := lookup.LookupState(context.Background(), serving.StateLookupRequest{
		Binding: reader.ResolvedBinding{
			Repository: "kr://acme/core", DeclarationCommit: "commit-live-1",
			Address:           knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"},
			DeclarationDigest: "decl-live-1", Mode: knowledge.BindingState,
			Runtime: "health", Protocol: "resource-access/v1",
			Operations: map[string]knowledge.BindingOperation{"lookup": {Call: "health.lookup"}},
			Origin:     origin,
		},
		Origin: origin, Identity: observability.IdentityContext{Principal: "agent:docker-test"}, RequestID: "docker-request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	value := result.Value.(map[string]any)
	if value["runtime"] != "docker" || result.Basis.BindingGeneration != "docker-runtime-v1" || result.Basis.SourceRevision != "docker-health-1" {
		t.Fatalf("container runtime result: %#v", result)
	}
}

func TestLiveHTTPRuntimeBuildsOpenSearchStateProjection(t *testing.T) {
	origin := strings.TrimSpace(os.Getenv("KC_TEST_STATE_RUNTIME_URL"))
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if origin == "" || opensearchURL == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_STATE_RUNTIME_URL and KC_TEST_OPENSEARCH_URL are required")
		}
		t.Skip("run make test-state-runtime-e2e")
	}
	lookup := serving.NewHTTPStateLookup(nil)
	setup := testkit.NewSetup(t, "")
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "Service:orders", AspectName: "health"}
	commit, err := setup.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: setup.RootCommitID, ExpectedTargetCommit: setup.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/service.health"}, Value: map[string]any{
				"entity": "Service", "aspect": "health", "origin": origin, "fields": map[string]any{
					"status": map[string]any{"type": "string", "access": []any{"text", "filter"}},
				},
			}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: address.ObjectID, AspectName: "properties"}, Value: map[string]any{"name": "orders"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	idx := index.NewIndexEngine("", opensearch.Open(opensearch.Config{URL: opensearchURL, PrimaryShards: 1}))
	t.Cleanup(func() { _ = idx.Close() })
	sync, err := idx.RefreshState(context.Background(), setup.Repo, commit, lookup, serving.RequestContext{
		Identity: observability.IdentityContext{Principal: "agent:docker-test"}, RequestID: "docker-projection-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := idx.SearchStateAt(setup.Repo, commit, retrieval.SearchOf(retrieval.SearchMATCH("healthy")))
	if err != nil || len(result.Hits) != 1 || len(result.Hits[0].Version.Observations) != 1 {
		t.Fatalf("cross-container State search: %#v %v", result, err)
	}
	if result.SearchView.ProjectionRevisions[setup.RepositoryID] != sync.Revision {
		t.Fatalf("SearchView revision: %#v", result.SearchView)
	}
	raw, err := setup.Repo.Read(address.ObjectID, commit)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := raw.Value.(map[string]any); body["health"] != nil {
		t.Fatalf("runtime observation leaked into Snapshot: %#v", raw)
	}
	if _, err := setup.Repo.ReadAddress(address, commit); kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
		t.Fatalf("Bound State must not occupy a Snapshot unit: %#v %v", address, err)
	}
	if head, err := setup.Repo.Head(snapshot.DefaultRef); err != nil || head != commit {
		t.Fatalf("dynamic refresh moved Repository HEAD: %s %v", head, err)
	}
}
