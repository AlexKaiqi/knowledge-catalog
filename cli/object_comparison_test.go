package cli_test

import (
	"context"
	"encoding/json"
	"testing"

	"kc/client"
	"kc/kernel"
	objectmodel "kc/object"
	objectclient "kc/object/client"
)

func comparisonClient(t *testing.T, url, principal string) objectclient.Client {
	t.Helper()
	remote, err := client.New(client.Config{BaseURL: url})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: principal}}); err != nil {
		t.Fatal(err)
	}
	return remote.ObjectService()
}

func TestObjectComparisonHTTPUsesFixedViewsAndFullLocalUnits(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	member := ordersReference(t, repo, "/columns/id")
	leaf := ordersReference(t, repo, "/columns/id/type")
	body(t, run("writer", "put", member, "--repo", repo, "--kind", "Member", "--command-id", "compare-first", "--value", `{"type":"string","owner":"trade"}`))
	first := asMap(t, body(t, run("read", leaf, "--repo", repo)))
	body(t, run("writer", "put", member, "--repo", repo, "--command-id", "compare-second", "--value", `{"type":"string","owner":"ops"}`))
	second := asMap(t, body(t, run("read", leaf, "--repo", repo)))
	remote := comparisonClient(t, url, "operator")
	request := objectclient.ComparisonRequest{Reference: leaf, From: objectmodel.ComparisonView{Basis: first["basis"].(string)}, To: objectmodel.ComparisonView{Repository: kernel.RepositoryID(repo), Commit: kernel.CommitID(second["commit"].(string))}}
	var out objectmodel.ObjectComparison
	if err := remote.Compare(context.Background(), request, objectclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	if out.ScopeChanged || out.Equal == nil || *out.Equal || len(out.Changes) != 1 || !out.Changes[0].ValueChanged || out.Changes[0].Before.Value.(map[string]any)["owner"] != "trade" || out.Changes[0].After.Value.(map[string]any)["owner"] != "ops" {
		t.Fatalf("unreturned field change lost or coordinates followed latest: %#v", out)
	}
	fromJSON, _ := json.Marshal(request.From)
	toJSON, _ := json.Marshal(request.To)
	cliResult := asMap(t, body(t, run("diff", leaf, "--from", string(fromJSON), "--to", string(toJSON))))
	if cliResult["equal"] != false || len(cliResult["changes"].([]any)) != 1 {
		t.Fatalf("CLI lost fixed object comparison: %#v", cliResult)
	}
	expectCode(t, run("diff", leaf, "--from", string(fromJSON), "--to", string(toJSON), "--dir", t.TempDir()), string(kernel.ErrUsageInvalid))
	expectCode(t, run("diff", "--repo", repo, "--dir", t.TempDir(), "--from", string(fromJSON)), string(kernel.ErrUsageInvalid))
	// The explicit old view remains old after another write.
	body(t, run("writer", "put", member, "--repo", repo, "--command-id", "compare-third", "--value", `{"type":"boolean","owner":"later"}`))
	if err := remote.Compare(context.Background(), request, objectclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	if out.To.Basis.Candidates[0].Commit != request.To.Commit || out.Changes[0].After.Value.(map[string]any)["owner"] != "ops" {
		t.Fatalf("fixed comparison followed head: %#v", out)
	}
	request.To.Commit = ""
	if err := remote.Compare(context.Background(), request, objectclient.RequestOptions{}, &out); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("implicit head accepted: %v", err)
	}
}

func TestObjectComparisonDatasetConsumesBothPublicationsWithoutHistoryAuthority(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	ref := ordersReference(t, repo, "/definition")
	body(t, run("writer", "put", ref, "--repo", repo, "--kind", "Aspect", "--command-id", "compare-dataset-one", "--value", `{"name":"one"}`))
	body(t, run("dataset", "define", "--dataset", "comparison-one", "--revision", "1", "--source", repo))
	body(t, run("writer", "put", ref, "--repo", repo, "--command-id", "compare-dataset-two", "--value", `{"name":"two"}`))
	body(t, run("dataset", "define", "--dataset", "comparison-two", "--revision", "1", "--source", repo))
	body(t, run("grant", "add", "--principal", "comparison-consumer", "--dataset", "comparison-one", "--action", "file.read"))
	grant := asMap(t, body(t, run("grant", "add", "--principal", "comparison-consumer", "--dataset", "comparison-two", "--action", "file.read")))
	remote := comparisonClient(t, url, "comparison-consumer")
	request := objectclient.ComparisonRequest{Reference: ref, From: objectmodel.ComparisonView{Dataset: "comparison-one", Revision: 1}, To: objectmodel.ComparisonView{Dataset: "comparison-two", Revision: 1}}
	var out objectmodel.ObjectComparison
	if err := remote.Compare(context.Background(), request, objectclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Equal == nil || *out.Equal || len(out.Changes) != 1 || out.From.Basis.Dataset != "comparison-one" || out.To.Basis.Dataset != "comparison-two" {
		t.Fatalf("published content comparison: %#v", out)
	}
	// Consumption of both releases never grants source content/history access.
	expectCode(t, kcRemote(t, url, "comparison-consumer", "read", ref, "--repo", repo), string(kernel.ErrForbidden))
	expectCode(t, kcRemote(t, url, "comparison-consumer", "log", "--repo", repo, "--object", "table/orders"), string(kernel.ErrForbidden))
	body(t, run("grant", "remove", "--id", grant["id"].(string)))
	if err := remote.Compare(context.Background(), request, objectclient.RequestOptions{}, &out); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("second-side revocation ignored: %v", err)
	}
}

func TestObjectComparisonFileSlicesDoNotInventDeletionOrGlobalEquality(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	body(t, run("writer", "put", ordersReference(t, repo, "/columns/public"), "--repo", repo, "--kind", "Member", "--command-id", "comparison-public", "--path-hint", "public/id.yaml", "--value", `{"type":"public"}`))
	body(t, run("writer", "put", ordersReference(t, repo, "/columns/private"), "--repo", repo, "--command-id", "comparison-private", "--path-hint", "private/id.yaml", "--value", `{"type":"private"}`))
	body(t, run("dataset", "define", "--dataset", "comparison-public-slice", "--revision", "1", "--source", repo+"=refs/heads/main@shared@public"))
	body(t, run("dataset", "define", "--dataset", "comparison-private-slice", "--revision", "1", "--source", repo+"=refs/heads/main@shared@private"))
	for _, name := range []string{"comparison-public-slice", "comparison-private-slice"} {
		body(t, run("grant", "add", "--principal", "slice-comparer", "--dataset", name, "--action", "file.read"))
	}
	remote := comparisonClient(t, url, "slice-comparer")
	request := objectclient.ComparisonRequest{Reference: ordersReference(t, repo, ""), From: objectmodel.ComparisonView{Dataset: "comparison-public-slice", Revision: 1}, To: objectmodel.ComparisonView{Dataset: "comparison-private-slice", Revision: 1}}
	var out objectmodel.ObjectComparison
	if err := remote.Compare(context.Background(), request, objectclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	if !out.ScopeChanged || out.Equal != nil || len(out.Changes) != 2 {
		t.Fatalf("slices became full equality: %#v", out)
	}
	seen := map[objectmodel.UnitPresenceChange]bool{}
	for _, change := range out.Changes {
		seen[change.Presence] = true
	}
	if !seen[objectmodel.UnitLeft] || !seen[objectmodel.UnitEntered] || len(seen) != 2 {
		t.Fatalf("scope exit invented deletion: %#v", out.Changes)
	}
	if out.From.Basis.Candidates[0].Commit != out.To.Basis.Candidates[0].Commit {
		t.Fatal("fixture did not compare same-commit publications")
	}
	first := asMap(t, body(t, kcRemote(t, url, "slice-comparer", "read", request.Reference, "--dataset", "comparison-public-slice")))
	request.From = objectmodel.ComparisonView{Basis: first["basis"].(string)}
	if err := remote.Compare(context.Background(), request, objectclient.RequestOptions{}, &out); err != nil {
		t.Fatalf("accepted slice basis did not reopen: %v", err)
	}
	request.Reference = ordersReference(t, repo, "/columns/private")
	if err := remote.Compare(context.Background(), request, objectclient.RequestOptions{}, &out); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("basis changed its object selection: %v", err)
	}
}

func TestObjectComparisonExplicitViewsProveEmptyStateWithoutHistory(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	before := asMap(t, body(t, run("writer", "head", "--repo", repo)))["commit"].(string)
	ref := ordersReference(t, repo, "/definition")
	body(t, run("writer", "put", ref, "--repo", repo, "--kind", "Aspect", "--command-id", "comparison-create-empty", "--value", `{"name":"new"}`))
	after := asMap(t, body(t, run("writer", "head", "--repo", repo)))["commit"].(string)
	remote := comparisonClient(t, url, "operator")
	request := objectclient.ComparisonRequest{Reference: ref, From: objectmodel.ComparisonView{Repository: kernel.RepositoryID(repo), Commit: kernel.CommitID(before)}, To: objectmodel.ComparisonView{Repository: kernel.RepositoryID(repo), Commit: kernel.CommitID(after)}}
	var out objectmodel.ObjectComparison
	if err := remote.Compare(context.Background(), request, objectclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Equal == nil || *out.Equal || !out.From.State.Complete || len(out.Changes) != 1 || out.Changes[0].Presence != objectmodel.UnitAdded {
		t.Fatalf("proved empty treated as failure or partial: %#v", out)
	}
}
