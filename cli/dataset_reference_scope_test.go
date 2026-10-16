package cli_test

import (
	"testing"

	"kc/internal/testkit"
)

func TestDatasetReferenceIgnoresUnpublishedHoldingsAndKeepsSelectedConflicts(t *testing.T) {
	home := testkit.TempDir(t)
	body(t, kc(home, "init"))
	body(t, kc(home, "store-set", "--index", "none"))
	a, b, hidden := "kr://slice/a", "kr://slice/b", "kr://slice/hidden"
	for _, repository := range []string{a, b, hidden} {
		seedRepo(t, home, repository)
	}
	const reference = "urn:kc:object:table/orders"
	body(t, kc(home, "put", reference+"#/structure", "--repo", a, "--command-id", "a-structure", "--path-hint", "public/structure.yaml", "--value", `{"pk":"id"}`))
	body(t, kc(home, "put", reference+"#/ownership", "--repo", b, "--command-id", "b-ownership", "--path-hint", "public/ownership.yaml", "--value", `{"owner":"finance"}`))
	body(t, kc(home, "put", reference+"#/structure", "--repo", hidden, "--command-id", "hidden-structure", "--path-hint", "private/structure.yaml", "--value", `{"pk":"secret"}`))
	body(t, kc(home, "put", "urn:kc:object:note/visible", "--repo", hidden, "--command-id", "hidden-public", "--path-hint", "public/note.yaml", "--value", `{"purpose":"selected unrelated content"}`))
	const partial = "urn:kc:object:note/partial"
	body(t, kc(home, "put", partial+"#/summary", "--repo", a, "--command-id", "partial-public", "--path-hint", "public/summary.yaml", "--value", `{"text":"public"}`))
	body(t, kc(home, "put", partial+"#/internal", "--repo", a, "--command-id", "partial-private", "--path-hint", "private/internal.yaml", "--value", `{"text":"private"}`))
	sources := []string{"--source", a + "@a@public", "--source", b + "@b@public", "--source", hidden + "@hidden@public"}
	body(t, kc(home, append([]string{"dataset", "define", "sliced", "--revision", "1"}, sources...)...))
	body(t, kc(home, "grant", "add", "--principal", "agent:consumer", "--action", "dataset.resolve,file.read", "--dataset", "sliced"))
	consume := func(ref string) kcRunResult {
		return kc(home, "read", ref, "--dataset", "sliced", "--as", "agent:consumer")
	}
	read := asMap(t, body(t, consume(reference)))
	value := asMap(t, read["value"])
	if asMap(t, value["structure"])["pk"] != "id" || asMap(t, value["ownership"])["owner"] != "finance" {
		t.Fatalf("selected disjoint Aspects must compose without an unpublished holding: %#v", read)
	}
	if sources, _ := read["sources"].([]any); len(sources) != 2 {
		t.Fatalf("only the two published holdings may supply the object: %#v", read)
	}
	selected := asMap(t, body(t, consume(partial+"#/summary")))
	if asMap(t, selected["value"])["text"] != "public" {
		t.Fatalf("a selected Aspect must stay readable: %#v", selected)
	}
	expectCode(t, consume(partial+"#/internal"), "OBJECT_REF_UNRESOLVED")
	partialRoot := asMap(t, body(t, consume(partial)))
	partialValue := asMap(t, partialRoot["value"])
	if len(partialValue) != 1 || asMap(t, partialValue["summary"])["text"] != "public" || partialRoot["reference"] != partial || partialRoot["repository"] != a || partialRoot["commit"] == "" {
		t.Fatalf("logical root must assemble only the published Aspect: %#v", partialRoot)
	}
	expectCode(t, kc(home, "read", reference, "--repo", hidden, "--as", "agent:consumer"), "FORBIDDEN")
	conflicting := append(append([]string(nil), sources...), "--source", hidden+"@conflict@private")
	expectCode(t, kc(home, append([]string{"dataset", "define", "sliced", "--revision", "2"}, conflicting...)...), "OBJECT_ID_CONFLICT")
	body(t, consume(reference))
	body(t, kc(home, "put", partial+"#/summary", "--repo", a, "--command-id", "partial-public-update", "--value", `{"text":"new publication"}`))
	body(t, kc(home, append([]string{"dataset", "define", "sliced", "--revision", "2"}, sources...)...))
	current := asMap(t, body(t, consume(partial)))
	if value := asMap(t, current["value"]); len(value) != 1 || asMap(t, value["summary"])["text"] != "new publication" || current["commit"] == partialRoot["commit"] {
		t.Fatalf("new publication did not adopt the changed selected unit: %#v", current)
	}
	fixed := asMap(t, body(t, kc(home, "read", partial, "--dataset", "sliced", "--revision", "1", "--as", "agent:consumer")))
	if value := asMap(t, fixed["value"]); len(value) != 1 || asMap(t, value["summary"])["text"] != "public" || fixed["commit"] != partialRoot["commit"] {
		t.Fatalf("partial root silently followed the newer Dataset revision: old=%#v fixed=%#v", partialRoot, fixed)
	}
	expectCode(t, consume(partial+"#/internal"), "OBJECT_REF_UNRESOLVED")
}
