package cli_test

import (
	"fmt"
	"strings"
	"testing"

	"kc/internal/testkit"
)

// Publication rejects a candidate that takes one Aspect (or whole value) of one
// object from two source Repositories (doc/design/dataset.md 组合规则). The
// world has no index plugin, so the check cannot lean on search preparation.
func TestDatasetPublicationRejectsSchemaHeldByTwoSources(t *testing.T) {
	h := testkit.TempDir(t)
	one, two := "kr://acme/conflict/one", "kr://acme/conflict/two"
	body(t, kc(h, "init"))
	for i, repo := range []string{one, two} {
		seedRepo(t, h, repo)
		body(t, kc(h, "put", testObjectReference(t, repo, "schema/item.structure", ""), "--repo", repo, "--command-id", fmt.Sprintf("schema-%d", i),
			"--value", `{"entity":"Item","pattern":"record","fields":{"name":{"type":"string"}}}`))
	}
	// An accepted revision from one source keeps serving after a failed candidate.
	body(t, kc(h, "dataset", "define", "--dataset", "dup", "--revision", "1", "--source", one+"=refs/heads/main"))
	result := kc(h, "dataset", "define", "--dataset", "dup", "--revision", "2", "--source", one+"=refs/heads/main", "--source", two+"=refs/heads/main")
	expectCode(t, result, "OBJECT_ID_CONFLICT")
	for _, want := range []string{"schema/item.structure", one, two} {
		if !strings.Contains(result.Stdout, want) {
			t.Fatalf("conflict must name object and both sources (%s): %s", want, result.Stdout)
		}
	}
	page := asMap(t, body(t, kc(h, "schema", "list", "--dataset", "dup")))
	if page["revision"] != float64(1) {
		t.Fatalf("failed candidate replaced the accepted revision: %#v", page)
	}
}

// Disjoint objects from two sources, and one source using a Schema held only
// by the other, still publish.
func TestDatasetPublicationComposesDisjointSources(t *testing.T) {
	h := testkit.TempDir(t)
	one, two := "kr://acme/compose/one", "kr://acme/compose/two"
	body(t, kc(h, "init"))
	seedRepo(t, h, one)
	seedRepo(t, h, two)
	body(t, kc(h, "put", testObjectReference(t, one, "schema/item.structure", ""), "--repo", one, "--command-id", "schema",
		"--value", `{"entity":"Item","pattern":"record","fields":{"name":{"type":"string"}}}`))
	for i, repo := range []string{one, two} {
		body(t, kc(h, "put", testObjectReference(t, repo, fmt.Sprintf("item/%d", i), ""), "--repo", repo, "--command-id", fmt.Sprintf("item-%d", i), "--value", `{"name":"a"}`))
	}
	body(t, kc(h, "dataset", "define", "--dataset", "joined", "--revision", "1", "--source", one+"=refs/heads/main", "--source", two+"=refs/heads/main"))
	got := asMap(t, body(t, kc(h, "read", "urn:kc:object:item/1", "--dataset", "joined")))
	if got["objectId"] != "item/1" {
		t.Fatalf("disjoint source object must read through the Dataset: %#v", got)
	}
}
