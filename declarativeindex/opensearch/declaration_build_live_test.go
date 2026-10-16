package opensearch_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"kc/declarativeindex/capability"
	"kc/declarativeindex/opensearch"
	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestOpenSearchDeclaredTemplateNormalizerAndMissingPolicies(t *testing.T) {
	endpoint := os.Getenv("KC_TEST_OPENSEARCH_URL")
	if endpoint == "" {
		t.Fatal("real construction regression requires KC_TEST_OPENSEARCH_URL")
	}
	idx := index.NewIndexEngine("", opensearch.Open(opensearch.Config{URL: endpoint, PrimaryShards: 1}))
	t.Cleanup(func() { _ = idx.Close() })
	manifest := capability.DefaultManifest()
	idx.SetDeclarationManifest(manifest.Check, manifest.Describe().Digest)
	repo := testkit.MakeRepository(t, uniqueESRepo(t, "declaration-build"))
	t.Cleanup(func() { cleanupDeclaredBuildProjection(t, endpoint, repo.ID()) })
	text := testkit.IndexMember("metric-text", "text", []string{"text"}, "schema/metric#/fields/name", "schema/metric#/fields/expression")
	body := text.Value.(map[string]any)
	body["inputs"].([]any)[1].(map[string]any)["missing"] = "skip"
	body["build"].(map[string]any)["template"] = "fixedprefix {expression} {name}"
	unit := testkit.IndexField("schema/metric", "unit", "keyword")
	unit.Value.(map[string]any)["build"].(map[string]any)["normalizer"] = "lowercase"
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/metric"}, Value: map[string]any{"entity": "Metric", "fields": map[string]any{"name": map[string]any{"type": "string"}, "expression": map[string]any{"type": "string"}, "unit": map[string]any{"type": "string"}}}},
		text, unit,
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "metric/full"}, SchemaRef: "schema/metric", Value: map[string]any{"name": "GMV", "expression": "discount", "unit": "CnY"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "metric/missing"}, SchemaRef: "schema/metric", Value: map[string]any{"name": "GMV", "unit": "CNY"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Rebuild(repo, commit); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		request retrieval.SearchRequest
		count   int
	}{
		{retrieval.SearchOf(retrieval.SearchMATCH("fixedprefix")), 2},
		{retrieval.SearchOf(retrieval.SearchMATCHMode("discount GMV", retrieval.MatchPhrase)), 1},
		{retrieval.SearchOf(retrieval.SearchEQ("unit", "CNY")), 2},
	} {
		result, err := idx.SearchAt(repo, commit, test.request)
		if err != nil || len(result.Hits) != test.count {
			t.Fatalf("declared construction query=%#v hits=%d error=%v", test.request, len(result.Hits), err)
		}
	}
	body["inputs"].([]any)[1].(map[string]any)["missing"] = "exclude"
	updated, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: commit, ExpectedTargetCommit: commit, Operations: []objectmodel.Operation{text}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Ensure(repo, updated); err != nil {
		t.Fatal(err)
	}
	result, err := idx.SearchAt(repo, updated, retrieval.SearchOf(retrieval.SearchMATCH("fixedprefix")))
	if err != nil || len(result.Hits) != 1 || result.Hits[0].Object.Address.ObjectID != "metric/full" {
		t.Fatalf("exclude did not affect only the text member: %#v %v", objectIDs(result), err)
	}
	result, err = idx.SearchAt(repo, updated, retrieval.SearchOf(retrieval.SearchEQ("unit", "cny")))
	if err != nil || len(result.Hits) != 2 {
		t.Fatalf("missing text input disabled keyword member: %#v %v", objectIDs(result), err)
	}
	if result.Hits[0].Object.Value == nil || !strings.Contains(string(result.Hits[0].Object.Address.ObjectID), "metric/") {
		t.Fatal("search did not hydrate the original object")
	}
}

func cleanupDeclaredBuildProjection(t *testing.T, endpoint string, repository kernel.RepositoryID) {
	t.Helper()
	digest := sha256.Sum256([]byte(repository))
	id := hex.EncodeToString(digest[:])
	prefix := "kc-proj-" + id[:24] + "-g-"
	response, err := http.Get(endpoint + "/_cat/indices/" + prefix + "*?format=json&h=index&expand_wildcards=all")
	if err != nil {
		t.Errorf("list owned test generations: %v", err)
		return
	}
	defer response.Body.Close()
	var indices []struct {
		Index string `json:"index"`
	}
	if err := json.NewDecoder(response.Body).Decode(&indices); err != nil {
		t.Errorf("decode owned test generations: %v", err)
		return
	}
	for _, item := range indices {
		if !strings.HasPrefix(item.Index, prefix) {
			t.Errorf("test cleanup returned an unowned index %q", item.Index)
			continue
		}
		deleteDeclaredBuildResource(t, endpoint+"/"+item.Index)
	}
	deleteDeclaredBuildResource(t, endpoint+"/kc-projection-control-v1/_doc/"+id+"?refresh=true")
}

func deleteDeclaredBuildResource(t *testing.T, resource string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodDelete, resource, nil)
	if err != nil {
		t.Errorf("owned test cleanup request: %v", err)
		return
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Errorf("owned test cleanup: %v", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode != 404 {
		t.Errorf("owned test cleanup HTTP %d", response.StatusCode)
	}
}
