package cli_test

import (
	"net/http/httptest"
	"testing"

	"kc/kernel"
	serv "kc/server"
)

func TestObjectReadBasisCLIReplaysTheOriginalSelectionAfterHeadAdvances(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	member := ordersReference(t, repo, "/columns/id")
	leaf := ordersReference(t, repo, "/columns/id/type")
	body(t, run("writer", "put", member, "--repo", repo, "--kind", "Member", "--command-id", "basis-create", "--value", `{"type":"string","owner":"trade"}`))
	first := asMap(t, body(t, run("read", leaf)))
	basis, ok := first["basis"].(string)
	if !ok || basis == "" {
		t.Fatalf("read supplied no reusable basis: %#v", first)
	}
	state := asMap(t, first["state"])
	if state["complete"] != true || state["scope"] == "" || state["digest"] == "" {
		t.Fatalf("incomplete local state: %#v", state)
	}
	body(t, run("writer", "put", member, "--repo", repo, "--command-id", "basis-change", "--value", `{"type":"number","owner":"trade"}`))
	replayed := asMap(t, body(t, run("read", leaf, "--basis", basis)))
	if replayed["value"] != "string" || replayed["commit"] != first["commit"] || replayed["basis"] != basis || asMap(t, replayed["state"])["digest"] != state["digest"] {
		t.Fatalf("read basis followed latest or changed state: first=%#v replay=%#v", first, replayed)
	}
	latest := asMap(t, body(t, run("read", leaf)))
	if latest["value"] != "number" || asMap(t, latest["state"])["digest"] == state["digest"] || asMap(t, latest["state"])["scope"] != state["scope"] {
		t.Fatalf("new request lost freshness or logical scope: %#v", latest)
	}
	expectCode(t, run("read", member, "--basis", basis), string(kernel.ErrUsageInvalid))
	expectCode(t, run("read", leaf, "--basis", basis, "--repo", repo), string(kernel.ErrUsageInvalid))
	expectCode(t, run("read", leaf, "--basis", basis, "--commit", first["commit"].(string)), string(kernel.ErrUsageInvalid))
}

func TestObjectReadBasisKeepsEmptyCandidateAndSurvivesServerReopen(t *testing.T) {
	home, url, repo, stop := objectReferenceServerLifecycle(t)
	const other = "kr://basis/empty-candidate"
	seedRepo(t, home, other)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	root := ordersReference(t, repo, "")
	body(t, run("writer", "put", ordersReference(t, repo, "/definition"), "--repo", repo, "--command-id", "basis-candidate-first", "--value", `{"name":"orders"}`))
	first := asMap(t, body(t, run("read", root)))
	basis := first["basis"].(string)
	body(t, run("writer", "put", ordersReference(t, other, "/runbook"), "--repo", other, "--command-id", "basis-candidate-second", "--value", `{"owner":"ops"}`))
	latest := asMap(t, body(t, run("read", root)))
	if asMap(t, latest["value"])["runbook"] == nil || asMap(t, latest["state"])["scope"] != asMap(t, first["state"])["scope"] || asMap(t, latest["state"])["digest"] == asMap(t, first["state"])["digest"] {
		t.Fatalf("a new unit was mistaken for a new selection scope: first=%#v latest=%#v", first, latest)
	}
	stop()
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	reopened := httptest.NewServer(handler)
	t.Cleanup(reopened.Close)
	url = reopened.URL
	old := asMap(t, body(t, kcRemote(t, reopened.URL, "operator", "read", root, "--basis", basis)))
	if _, leaked := asMap(t, old["value"])["runbook"]; leaked || old["basis"] != basis || asMap(t, old["state"])["digest"] != asMap(t, first["state"])["digest"] {
		t.Fatalf("reopened server rediscovered a newer contribution: %#v", old)
	}
	body(t, run("grant", "add", "--principal", "reader", "--repo", repo, "--action", "object.read"))
	grant := asMap(t, body(t, run("grant", "add", "--principal", "reader", "--repo", other, "--action", "object.read")))
	complete := asMap(t, body(t, kcRemote(t, url, "reader", "read", root)))
	if asMap(t, complete["state"])["complete"] != true {
		t.Fatalf("two authorized contributors were not complete: %#v", complete)
	}
	body(t, run("grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, kcRemote(t, url, "reader", "read", root, "--basis", complete["basis"].(string)), string(kernel.ErrForbidden))
	partial := asMap(t, body(t, kcRemote(t, url, "reader", "read", root)))
	if asMap(t, partial["state"])["complete"] != false {
		t.Fatalf("withheld source was presented as complete local state: %#v", partial)
	}
}

func TestDatasetReadBasisRechecksConsumptionWithoutSourceReadAuthority(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	member := ordersReference(t, repo, "/columns/id")
	body(t, admin("writer", "put", member, "--repo", repo, "--kind", "Member", "--command-id", "basis-dataset-seed", "--value", `{"type":"string"}`))
	body(t, admin("dataset", "define", "--dataset", "basis-orders", "--revision", "1", "--source", repo))
	grant := asMap(t, body(t, admin("grant", "add", "--principal", "consumer", "--dataset", "basis-orders", "--action", "file.read")))
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "consumer", args...) }
	first := asMap(t, body(t, run("read", member, "--dataset", "basis-orders")))
	basis := first["basis"].(string)
	body(t, admin("writer", "put", member, "--repo", repo, "--command-id", "basis-dataset-advance", "--value", `{"type":"number"}`))
	old := asMap(t, body(t, run("read", member, "--basis", basis)))
	if asMap(t, old["value"])["type"] != "string" || old["commit"] != first["commit"] {
		t.Fatalf("consumer basis did not preserve its accepted publication: %#v", old)
	}
	expectCode(t, run("read", member, "--repo", repo), string(kernel.ErrForbidden))
	body(t, admin("grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, run("read", member, "--basis", basis), string(kernel.ErrForbidden))
}

func TestShapedReadBasisKeepsCompleteUnitStateAndOriginalProjection(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	member, root := ordersReference(t, repo, "/columns/id"), ordersReference(t, repo, "")
	body(t, run("writer", "put", member, "--repo", repo, "--kind", "Member", "--command-id", "shape-basis-seed", "--value", `{"type":"string","owner":"trade"}`))
	shape := `{ columns(keys: ["id"]) { type } }`
	first := asMap(t, body(t, run("read", root, "--repo", repo, "--shape", shape)))
	basis := first["basis"].(string)
	body(t, run("writer", "put", member, "--repo", repo, "--command-id", "shape-basis-owner", "--value", `{"type":"string","owner":"ops"}`))
	latest := asMap(t, body(t, run("read", root, "--repo", repo, "--shape", shape)))
	if asMap(t, first["state"])["digest"] == asMap(t, latest["state"])["digest"] || asMap(t, first["state"])["scope"] != asMap(t, latest["state"])["scope"] {
		t.Fatalf("projection hid a full-unit change or changed logical scope: first=%#v latest=%#v", first, latest)
	}
	old := asMap(t, body(t, run("read", root, "--basis", basis)))
	if old["shape"] != shape || asMap(t, old["state"])["digest"] != asMap(t, first["state"])["digest"] {
		t.Fatalf("shape basis expanded or followed latest: %#v", old)
	}
	expectCode(t, run("read", root, "--basis", basis, "--shape", `{ columns }`), string(kernel.ErrUsageInvalid))
	missing := asMap(t, body(t, run("read", root, "--repo", repo, "--shape", `{ columns(keys: ["id"]) { missing } }`)))
	if asMap(t, missing["state"])["complete"] != true {
		t.Fatalf("missing projected field made persisted unit state unknown: %#v", missing)
	}
}

func TestDatasetReadBasisCannotExpandAFileSlice(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	member, root := ordersReference(t, repo, "/columns/id"), ordersReference(t, repo, "")
	body(t, admin("writer", "put", member, "--repo", repo, "--kind", "Member", "--command-id", "slice-basis-public", "--path-hint", "public/id.yaml", "--value", `{"type":"string"}`))
	body(t, admin("writer", "put", ordersReference(t, repo, "/columns/secret"), "--repo", repo, "--command-id", "slice-basis-private", "--path-hint", "private/secret.yaml", "--value", `{"type":"private"}`))
	body(t, admin("dataset", "define", "--dataset", "basis-slice", "--revision", "1", "--source", repo+"=refs/heads/main@shared@public"))
	body(t, admin("grant", "add", "--principal", "consumer", "--dataset", "basis-slice", "--action", "file.read"))
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "consumer", args...) }
	first := asMap(t, body(t, run("read", member, "--dataset", "basis-slice")))
	basis := first["basis"].(string)
	old := asMap(t, body(t, run("read", member, "--basis", basis)))
	if asMap(t, old["value"])["type"] != "string" || old["commit"] != first["commit"] {
		t.Fatalf("slice replay changed its published unit: %#v", old)
	}
	expectCode(t, run("read", root, "--basis", basis), string(kernel.ErrUsageInvalid))
	expectCode(t, run("read", ordersReference(t, repo, "/columns/secret"), "--basis", basis), string(kernel.ErrUsageInvalid))
	expectCode(t, run("read", ordersReference(t, repo, "/columns/secret"), "--dataset", "basis-slice"), string(kernel.ErrObjectRefUnresolved))
}
