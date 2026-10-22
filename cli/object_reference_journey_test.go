package cli_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"kc/client"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	objectclient "kc/object/client"
	"kc/object/writer"
	serv "kc/server"
)

func objectReferenceServer(t *testing.T) (string, string, string) {
	home, url, repo, _ := objectReferenceServerLifecycle(t)
	return home, url, repo
}

func objectReferenceServerLifecycle(t *testing.T) (string, string, string, func()) {
	t.Helper()
	isolateClientCredentials(t)
	t.Setenv("KC_DATASET", "")
	home := testkit.TempDir(t)
	const repo = "kr://reference/orders"
	body(t, kc(home, "local", "init"))
	seedRepo(t, home, repo)
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", "operator"))
	url, stop := serveReferenceHome(t, home)
	return home, url, repo, stop
}

func serveReferenceHome(t *testing.T, home string) (string, func()) {
	t.Helper()
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	server := httptest.NewServer(handler)
	var once sync.Once
	stop := func() {
		once.Do(func() {
			server.Close()
			if closer, ok := handler.(interface{ Close() error }); ok {
				if err := closer.Close(); err != nil {
					t.Errorf("close reference server: %v", err)
				}
			}
		})
	}
	t.Cleanup(stop)
	return server.URL, stop
}

func ordersReference(t *testing.T, repo, locator string) string {
	t.Helper()
	raw, err := objectmodel.FormatReference(objectmodel.Reference{Repository: kernel.RepositoryID(repo), Object: "table/orders", Locator: locator})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestObjectReferenceCLIUsesOneTargetAcrossReadPutRemove(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	member := ordersReference(t, repo, "/columns/id")
	leaf := ordersReference(t, repo, "/columns/id/constraints/nullable")
	body(t, run("writer", "put", member, "--repo", repo, "--kind", "Member", "--command-id", "id-create", "--if-absent", "--value", `{"type":"string","constraints":{"nullable":false},"old":"must disappear"}`))
	body(t, run("writer", "put", ordersReference(t, repo, "/columns/status"), "--repo", repo, "--command-id", "status-create", "--value", `{"type":"integer"}`))
	selected := asMap(t, body(t, run("read", leaf)))
	readMember := asMap(t, body(t, run("read", member)))
	if selected["value"] != false || selected["reference"] != leaf || selected["locator"] != "/columns/id/constraints/nullable" || selected["digest"] == "" || selected["digest"] != readMember["digest"] || selected["commit"] != readMember["commit"] || asMap(t, selected["address"])["memberKey"] != "id" {
		t.Fatalf("fragment or full maintenance basis was lost across CLI/HTTP: selected=%#v member=%#v", selected, readMember)
	}
	oldCommit, oldDigest := selected["commit"].(string), selected["digest"].(string)
	body(t, run("writer", "put", member, "--repo", repo, "--command-id", "id-replace", "--base", oldCommit, "--if-digest", oldDigest, "--value", `{"type":"number","constraints":{"nullable":false}}`))
	updated := asMap(t, body(t, run("read", member)))
	if updated["commit"] == oldCommit || updated["digest"] == oldDigest || asMap(t, updated["value"])["type"] != "number" {
		t.Fatalf("complete Member replacement was not published: %#v", updated)
	}
	if _, exists := asMap(t, updated["value"])["old"]; exists {
		t.Fatalf("PUT merged an omitted field: %#v", updated)
	}
	unchangedLeaf := asMap(t, body(t, run("read", leaf)))
	if unchangedLeaf["value"] != false || unchangedLeaf["digest"] != updated["digest"] {
		t.Fatalf("same leaf must retain its changed complete-unit digest: %#v", unchangedLeaf)
	}
	expectCode(t, run("writer", "put", member, "--repo", repo, "--command-id", "stale-digest", "--base", updated["commit"].(string), "--if-digest", oldDigest, "--value", `{"type":"integer"}`), string(kernel.ErrPreconditionFailed))
	if after := asMap(t, body(t, run("read", member))); after["commit"] != updated["commit"] || after["digest"] != updated["digest"] {
		t.Fatalf("rejected stale digest mutated the object: %#v", after)
	}
	old := asMap(t, body(t, run("read", member, "--repo", repo, "--commit", oldCommit)))
	if asMap(t, old["value"])["old"] != "must disappear" || old["digest"] != oldDigest {
		t.Fatalf("fixed old version drifted: %#v", old)
	}
	body(t, run("writer", "remove", member, "--repo", repo, "--command-id", "id-remove", "--if-digest", updated["digest"].(string)))
	expectCode(t, run("read", member), string(kernel.ErrObjectRefUnresolved))
	status := asMap(t, body(t, run("read", ordersReference(t, repo, "/columns/status"))))
	if asMap(t, status["value"])["type"] != "integer" {
		t.Fatalf("Member REMOVE changed its sibling: %#v", status)
	}
	root := asMap(t, body(t, run("read", ordersReference(t, repo, ""))))
	body(t, run("writer", "remove", ordersReference(t, repo, ""), "--repo", repo, "--command-id", "orders-remove", "--if-digest", root["digest"].(string)))
	expectCode(t, run("read", ordersReference(t, repo, "")), string(kernel.ErrObjectRefUnresolved))
	body(t, run("read", member, "--repo", repo, "--commit", oldCommit))
}

func TestDatasetReferenceKeepsAcceptedSourceVersionAndCurrentPermissions(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	member := ordersReference(t, repo, "/columns/id")
	body(t, admin("writer", "put", member, "--repo", repo, "--kind", "Member", "--command-id", "dataset-seed", "--value", `{"type":"string"}`))
	before := asMap(t, body(t, admin("read", member)))
	body(t, admin("dataset", "define", "--dataset", "published-orders", "--revision", "1", "--source", repo))
	grant := asMap(t, body(t, admin("grant", "add", "--principal", "consumer", "--dataset", "published-orders", "--action", "dataset.resolve,file.read")))
	body(t, admin("writer", "put", member, "--repo", repo, "--command-id", "dataset-after", "--value", `{"type":"number"}`))
	latest := asMap(t, body(t, admin("read", member)))
	read := func(commit string) kcRunResult {
		args := []string{"read", member, "--dataset", "published-orders"}
		if commit != "" {
			args = append(args, "--commit", commit)
		}
		return kcRemote(t, url, "consumer", args...)
	}
	fixed := asMap(t, body(t, read(before["commit"].(string))))
	if fixed["reference"] != member {
		t.Fatalf("source-qualified reference did not return the selected source: %#v", fixed)
	}

	if fixed["repository"] != repo || fixed["commit"] != before["commit"] || fixed["digest"] != before["digest"] || asMap(t, fixed["value"])["type"] != "string" {
		t.Fatalf("Dataset read followed source HEAD: %#v", fixed)
	}
	expectCode(t, read(latest["commit"].(string)), string(kernel.ErrUsageInvalid))
	expectCode(t, kcRemote(t, url, "consumer", "writer", "remove", member, "--repo", repo, "--command-id", "consumer-write"), string(kernel.ErrForbidden))
	if current := asMap(t, body(t, admin("read", member))); current["commit"] != latest["commit"] || current["digest"] != latest["digest"] {
		t.Fatalf("Dataset consumption grant mutated source: %#v", current)
	}
	body(t, admin("grant", "remove", "--id", grant["id"].(string)))
	expectCode(t, read(before["commit"].(string)), string(kernel.ErrForbidden))
}

type loseAcceptedObjectResponse struct {
	mu    sync.Mutex
	posts int
}

func (l *loseAcceptedObjectResponse) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil || request.URL.Path != "/object/v1/objects:put" {
		return response, err
	}
	l.mu.Lock()
	l.posts++
	l.mu.Unlock()
	if response.StatusCode != http.StatusOK {
		return response, nil
	}
	_ = response.Body.Close()
	return nil, context.DeadlineExceeded
}

func TestObjectMaintenanceRecoversRealAcceptedWriterAfterLostResponse(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	loss := &loseAcceptedObjectResponse{}
	remote, err := client.New(client.Config{BaseURL: url, HTTPClient: &http.Client{Transport: loss}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "operator"}}); err != nil {
		t.Fatal(err)
	}
	ref := ordersReference(t, repo, "")
	output, err := objectclient.MaintainWithReceiptRecovery(context.Background(), remote.ObjectService(), false, objectclient.MaintenanceRequest{
		Reference: ref, Repository: repo, CommandID: "real-response-lost", Value: map[string]any{"body": "accepted exactly once"},
	}, objectclient.RequestOptions{}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := output.(writer.CommitReceipt)
	if !ok || receipt.CommandID != "real-response-lost" || receipt.Result.NewCommit == "" {
		t.Fatalf("lost transport result was not recovered from Writer: %#v", output)
	}
	loss.mu.Lock()
	posts := loss.posts
	loss.mu.Unlock()
	if posts != 1 {
		t.Fatalf("uncertain accepted mutation was posted %d times", posts)
	}
	actual := asMap(t, body(t, kcRemote(t, url, "operator", "read", ref)))
	if actual["commit"] != string(receipt.Result.NewCommit) || asMap(t, actual["value"])["body"] != "accepted exactly once" {
		t.Fatalf("recovered receipt did not describe actual authority content: receipt=%#v value=%#v", receipt, actual)
	}
}

func TestObjectReferencePreservesActualAspectAuthorization(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	member := ordersReference(t, repo, "/columns/id")
	body(t, admin("writer", "put", member, "--repo", repo, "--kind", "Member", "--command-id", "scope-seed", "--value", `{"type":"string"}`))
	body(t, admin("grant", "add", "--principal", "column-editor", "--repo", repo, "--object", "table/orders", "--aspect", "columns", "--action", "object.read,writer.commit"))
	run := func(args ...string) kcRunResult { return kcRemote(t, url, "column-editor", args...) }
	body(t, run("read", testObjectReference(t, repo, "table/orders", "/columns/id"), "--repo", repo))
	leaf := asMap(t, body(t, run("read", ordersReference(t, repo, "/columns/id/type"))))
	if leaf["value"] != "string" {
		t.Fatalf("URN lost existing scoped read affordance: %#v", leaf)
	}
	body(t, run("writer", "put", member, "--repo", repo, "--command-id", "scope-replace", "--value", `{"type":"number"}`))
	body(t, run("writer", "put", ordersReference(t, repo, "/columns/added"), "--repo", repo, "--command-id", "scope-add", "--value", `{"type":"boolean"}`))
	expectCode(t, run("read", ordersReference(t, repo, "")), string(kernel.ErrForbidden))
	expectCode(t, run("writer", "remove", ordersReference(t, repo, ""), "--repo", repo, "--command-id", "scope-root-remove"), string(kernel.ErrForbidden))
	body(t, run("writer", "remove", member, "--repo", repo, "--command-id", "scope-remove"))
	// A JSON field called columns inside a whole Entity is not an Aspect grant.
	body(t, admin("writer", "remove", ordersReference(t, repo, ""), "--repo", repo, "--command-id", "scope-clear"))
	body(t, admin("writer", "put", ordersReference(t, repo, ""), "--repo", repo, "--command-id", "scope-blob", "--value", `{"columns":{"id":{"type":"secret"}}}`))
	expectCode(t, run("read", ordersReference(t, repo, "/columns/id/type")), string(kernel.ErrForbidden))
}
