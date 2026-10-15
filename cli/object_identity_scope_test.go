package cli_test

import (
	"testing"

	"kc/internal/testkit"
)

// G3 product journey: one object ID is one object in the request scope. Two
// Repositories holding disjoint Aspects compose with a version vector; a
// Repository the caller may not read is withheld; the same Aspect from two
// Repositories is OBJECT_ID_CONFLICT on read and at Dataset publication.
func TestObjectIdentityComposesWithholdsAndConflictsAcrossRepositories(t *testing.T) {
	home := testkit.TempDir(t)
	a, b := "kr://identity/a", "kr://identity/b"
	body(t, kc(home, "init"))
	body(t, kc(home, "store-set", "--index", "none"))
	seedRepo(t, home, a)
	seedRepo(t, home, b)
	const ref = "urn:kc:object:table/orders"
	body(t, kc(home, "put", ref+"#/structure", "--repo", a, "--command-id", "a-structure", "--value", `{"pk":"id"}`))
	body(t, kc(home, "put", ref+"#/ownership", "--repo", b, "--command-id", "b-ownership", "--value", `{"owner":"finance"}`))

	read := asMap(t, body(t, kc(home, "read", ref)))
	value := asMap(t, read["value"])
	if asMap(t, value["structure"])["pk"] != "id" || asMap(t, value["ownership"])["owner"] != "finance" {
		t.Fatalf("disjoint Aspects from two Repositories must compose into one object: %#v", read)
	}
	sources, _ := read["sources"].([]any)
	if len(sources) != 2 || asMap(t, sources[0])["repository"] != a || asMap(t, sources[1])["repository"] != b {
		t.Fatalf("composed read must carry one source per Repository: %#v", read["sources"])
	}

	body(t, kc(home, "allow", "--principal", "alice", "--cmd", "read", "--repo", a))
	partial := asMap(t, body(t, kc(home, "read", ref, "--as", "alice")))
	withheld, _ := partial["withheld"].([]any)
	if len(withheld) != 1 || withheld[0] != b {
		t.Fatalf("an unreadable holder must be listed as withheld: %#v", partial)
	}
	if _, leaked := asMap(t, partial["value"])["ownership"]; leaked {
		t.Fatalf("withheld Repository content leaked: %#v", partial["value"])
	}

	sourcesFlags := []string{"--source", a + "=refs/heads/main@a", "--source", b + "=refs/heads/main@b"}
	body(t, kc(home, append([]string{"dataset", "define", "--dataset", "orders", "--revision", "1"}, sourcesFlags...)...))

	body(t, kc(home, "put", ref+"#/structure", "--repo", b, "--command-id", "b-structure", "--value", `{"pk":"order_id"}`))
	expectCode(t, kc(home, "read", ref), "OBJECT_ID_CONFLICT")
	expectCode(t, kc(home, append([]string{"dataset", "define", "--dataset", "orders", "--revision", "2"}, sourcesFlags...)...), "OBJECT_ID_CONFLICT")
	if got := asMap(t, body(t, kc(home, "read", ref, "--dataset", "orders"))); asMap(t, got["value"])["ownership"] == nil {
		t.Fatalf("rejected publication must keep serving the accepted version: %#v", got)
	}
}
