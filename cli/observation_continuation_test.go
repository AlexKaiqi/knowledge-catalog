package cli_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
)

func TestObservationSearchContinuationAcrossRefreshReopenAndCurrentAuthorization(t *testing.T) {
	isolateClientCredentials(t)
	t.Setenv("KC_DATASET", "")
	endpoint := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if endpoint == "" {
		t.Fatal("run through the automatic OpenSearch fixture")
	}
	var revision atomic.Int32
	revision.Store(1)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/access" || r.Header.Get("X-Resource-Principal") == "" || r.Header.Get("X-Resource-Request-Id") == "" {
			t.Errorf("runtime request lacks verified context: %s", r.URL.Path)
			http.Error(w, "missing context", 400)
			return
		}
		version := revision.Load()
		status := "healthy"
		if version == 3 {
			status = "unhealthy"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"status": status}, "basis": map[string]any{"bindingGeneration": "shared-health", "consistency": "repeatable", "sourceRevision": fmt.Sprintf("r%d", version), "observedAt": fmt.Sprintf("2026-10-21T00:%02d:00Z", version)}})
	}))
	defer origin.Close()
	home := testkit.TempDir(t)
	const repository = "kr://observation/public/services"
	for _, args := range [][]string{{"local", "init"}, {"local", "store", "set", "--index", "opensearch"}, {"local", "store", "set", "--driver", "opensearch", "--url", endpoint}, {"local", "grant", "bootstrap", "--principal", "operator"}} {
		body(t, kc(home, args...))
	}
	seedRepo(t, home, repository)
	body(t, kc(home, "writer", "put", testObjectReference(t, repository, "schema/service.health", ""), "--repo", repository, "--command-id", "health-schema", "--value", `{"entity":"Service","aspect":"health","fields":{"status":{"type":"string"}}}`))
	for _, name := range []string{"first", "second", "third"} {
		id := "service/" + name
		body(t, kc(home, "writer", "put", testObjectReference(t, repository, id, "/properties"), "--repo", repository, "--command-id", name+"-definition", "--value", `{"name":"`+name+`"}`))
		descriptorID := putStateBinding(t, home, repository, id, "health", "schema/service.health", origin.URL)
		descriptor := bindingfixture.Descriptor(origin.URL)
		descriptor["identity"] = "shared"
		raw, err := json.Marshal(descriptor)
		if err != nil {
			t.Fatal(err)
		}
		body(t, kc(home, "writer", "put", testObjectReference(t, repository, descriptorID, ""), "--repo", repository, "--command-id", name+"-shared", "--value", string(raw)))
	}
	seedTextDeclaration(t, home, repository, "index/health", "schema/service.health", "status")
	body(t, kc(home, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", repository))
	grant := asMap(t, body(t, kc(home, "allow", "--principal", "agent:http-test", "--action", "file.read", "--dataset", "agent")))
	body(t, kc(home, "allow", "--principal", "agent:maintainer", "--action", "dataset.manage", "--dataset", "agent"))
	body(t, kc(home, "allow", "--principal", "agent:maintainer", "--action", "index.manage", "--dataset", "agent"))
	before := asMap(t, body(t, kc(home, "writer", "head", "--repo", repository)))["commit"]
	url, stop := serveReferenceHome(t, home)
	syncObservation := func() string {
		synced := asMap(t, postAs(t, url, "/operations/v1/indexes:prepare", "agent:maintainer", map[string]any{"dataset": "agent", "revision": 1}))
		value, ok := datasetIndexObservation(t, synced, repository)["revision"].(string)
		if !ok || value == "" {
			t.Fatalf("prepare has no fixed observation: %#v", synced)
		}
		return value
	}
	firstRevision := syncObservation()
	first := asMap(t, postAs(t, url, "/object/v1/search", "agent:http-test", map[string]any{"dataset": "agent", "revision": 1, "query": "healthy", "limit": 1}))
	firstHits := first["hits"].([]any)
	cursor, _ := first["continuation"].(string)
	if len(firstHits) != 1 || cursor == "" {
		t.Fatalf("first public page: %#v", first)
	}
	seen := map[string]bool{asMap(t, asMap(t, asMap(t, firstHits[0])["object"])["address"])["objectId"].(string): true}
	revision.Store(2)
	secondRevision := syncObservation()
	if secondRevision == firstRevision {
		t.Fatal("refresh did not replace observation generation")
	}
	current := asMap(t, postAs(t, url, "/object/v1/search", "agent:http-test", map[string]any{"dataset": "agent", "revision": 1, "query": "healthy", "limit": 1}))
	if asMap(t, asMap(t, current["searchView"])["indexRevisions"])[repository] != secondRevision {
		t.Fatalf("fresh query used old observation: %#v", current)
	}
	stop()
	url, stop = serveReferenceHome(t, home)
	defer stop()
	originalCursor := cursor
	for n := 0; cursor != ""; n++ {
		if n > 3 {
			t.Fatal("public observation continuation failed to advance")
		}
		page := asMap(t, body(t, kcRemote(t, url, "agent:http-test", "search", "--dataset", "agent", "--revision", "1", "--query", "healthy", "--limit", "1", "--continuation", cursor)))
		hits := page["hits"].([]any)
		if len(hits) != 1 || asMap(t, asMap(t, page["searchView"])["indexRevisions"])[repository] != firstRevision {
			t.Fatalf("CLI continuation changed observation: %#v", page)
		}
		hit := asMap(t, hits[0])
		id := hit["objectId"].(string)
		indexed := asMap(t, hit["indexObservations"].([]any)[0])
		read := asMap(t, asMap(t, hit["version"])["observations"].([]any)[0])
		if seen[id] || asMap(t, indexed["basis"])["sourceRevision"] != "r1" || asMap(t, read["basis"])["sourceRevision"] != "r2" {
			t.Fatalf("fixed index basis or current READ was lost: %#v", hit)
		}
		seen[id] = true
		cursor, _ = page["continuation"].(string)
	}
	if len(seen) != 3 {
		t.Fatalf("lost public candidates: %v", seen)
	}
	// The original candidate generation remains fixed; current READ predicates
	// must still eliminate values that no longer satisfy the condition.
	revision.Store(3)
	changed := asMap(t, body(t, kcRemote(t, url, "agent:http-test", "search", "--dataset", "agent", "--revision", "1", "--query", "healthy", "--limit", "1", "--continuation", originalCursor)))
	if changed["completeness"] != "complete" || len(changed["hits"].([]any)) != 0 || asMap(t, asMap(t, changed["searchView"])["indexRevisions"])[repository] != firstRevision {
		t.Fatalf("old index bypassed current predicate: %#v", changed)
	}
	body(t, kcRemote(t, url, "operator", "grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, kcRemote(t, url, "agent:http-test", "search", "--dataset", "agent", "--revision", "1", "--query", "healthy", "--continuation", originalCursor), string(kernel.ErrForbidden))
	body(t, kcRemote(t, url, "operator", "grant", "add", "--principal", "agent:http-test", "--dataset", "agent", "--action", "file.read"))
	body(t, kcRemote(t, url, "operator", "dataset", "retire", "--dataset", "agent"))
	expectCode(t, kcRemote(t, url, "agent:http-test", "search", "--dataset", "agent", "--revision", "1", "--query", "healthy", "--continuation", originalCursor), string(kernel.ErrDatasetInvalid))
	if after := asMap(t, body(t, kcRemote(t, url, "operator", "writer", "head", "--repo", repository)))["commit"]; after != before {
		t.Fatalf("observation refresh changed Snapshot: before=%v after=%v", before, after)
	}
}
