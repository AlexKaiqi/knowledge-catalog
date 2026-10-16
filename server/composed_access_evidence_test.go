package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"go.opentelemetry.io/otel/trace"

	kcclient "kc/client"
	apphome "kc/home"
	"kc/internal/telemetry"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/observability"
	"kc/snapshot"
)

const composedAuditReference = "urn:kc:object:service/orders"

func TestHTTPComposedReadAuditKeepsEachAdoptedSource(t *testing.T) {
	c, dir, repos, commits := composedAuditFixture(t)
	span := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}})
	ctx := trace.ContextWithSpanContext(context.Background(), span)
	var value objectmodel.ObjectValue
	if err := c.Objects().Read(ctx, kcclient.ObjectReadRequest{Reference: composedAuditReference}, kcclient.RequestOptions{RequestID: "composed-root"}, &value); err != nil {
		t.Fatal(err)
	}
	if len(value.Sources) != 2 || len(value.Withheld) != 1 || value.Withheld[0] != repos[2].ID() {
		t.Fatalf("read did not compose the two authorized sources: %#v", value)
	}
	byRequest := composedAuditQuery(t, c, kcclient.AuditQueryRequest{RequestID: "composed-root"})
	byTrace := composedAuditQuery(t, c, kcclient.AuditQueryRequest{TraceID: span.TraceID().String()})
	if !reflect.DeepEqual(byRequest, byTrace) {
		t.Fatalf("request and trace queries disagree: %#v / %#v", byRequest, byTrace)
	}
	if len(byRequest.Object) != 2 {
		t.Fatalf("composed audit must name both adopted sources, got %#v", byRequest.Object)
	}
	want := map[kernel.RepositoryID]string{repos[0].ID(): "definition", repos[1].ID(): "health"}
	var auditedHealth objectmodel.PinnedObjectRef
	for _, target := range byRequest.Object {
		aspect, ok := want[target.ObjectRef.Repository]
		if !ok || target.ObjectRef.Object != "service/orders" || target.ObjectRef.Commit != commits[target.ObjectRef.Repository] ||
			target.Reference != composedAuditReference || target.Locator != "" || target.Address == nil ||
			*target.Address != (objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: aspect}) {
			t.Fatalf("wrong source, version, or selected unit in audit: %#v", target)
		}
		if aspect == "health" {
			auditedHealth = target.ObjectRef
		}
		delete(want, target.ObjectRef.Repository)
	}
	if len(want) != 0 {
		t.Fatalf("missing source evidence: %#v", want)
	}

	// A source advances after the access. Its old audit and fixed content remain usable.
	advanceComposedAuditAspect(t, repos[1], "health", map[string]any{"status": "degraded"})
	if got := composedAuditQuery(t, c, kcclient.AuditQueryRequest{RequestID: "composed-root"}); !reflect.DeepEqual(got, byRequest) {
		t.Fatalf("source advancement rewrote old access evidence: %#v", got)
	}
	var old objectmodel.ObjectValue
	if err := c.Objects().Read(context.Background(), kcclient.ObjectReadRequest{Reference: composedAuditReference + "#/health", Repository: string(auditedHealth.Repository), Commit: string(auditedHealth.Commit)}, kcclient.RequestOptions{RequestID: "old-health"}, &old); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(old.Value, map[string]any{"status": "healthy"}) {
		t.Fatalf("old access basis returned changed content: %#v", old.Value)
	}

	// Selecting a leaf adopts only the holder of that Aspect, and retains the exact locator.
	var leaf objectmodel.ObjectValue
	if err := c.Objects().Read(context.Background(), kcclient.ObjectReadRequest{Reference: composedAuditReference + "#/health/status"}, kcclient.RequestOptions{RequestID: "health-leaf"}, &leaf); err != nil {
		t.Fatal(err)
	}
	selected := composedAuditQuery(t, c, kcclient.AuditQueryRequest{RequestID: "health-leaf"})
	if leaf.Value != "degraded" || len(selected.Object) != 1 || selected.Object[0].ObjectRef.Repository != repos[1].ID() ||
		selected.Object[0].ObjectRef.Commit != testkit.MustHead(t, repos[1], snapshot.DefaultRef) || selected.Object[0].Locator != "/health/status" {
		t.Fatalf("leaf audit included another source or lost the selection: %#v / %#v", leaf.Value, selected.Object)
	}

	// Revocation removes that holder from successful adoption even though its metadata remains withheld.
	if err := inv.WriteAllow(dir, inv.AllowFile{Rules: composedAuditRules(repos[:1])}); err != nil {
		t.Fatal(err)
	}
	var partial objectmodel.ObjectValue
	if err := c.Objects().Read(context.Background(), kcclient.ObjectReadRequest{Reference: composedAuditReference}, kcclient.RequestOptions{RequestID: "partial-root"}, &partial); err != nil {
		t.Fatal(err)
	}
	partialAudit := composedAuditQuery(t, c, kcclient.AuditQueryRequest{RequestID: "partial-root"})
	if len(partial.Withheld) != 2 || len(partialAudit.Object) != 1 || partialAudit.Object[0].ObjectRef.Repository != repos[0].ID() {
		t.Fatalf("unreadable source became adopted evidence: %#v / %#v", partial.Withheld, partialAudit.Object)
	}
}

func composedAuditFixture(t *testing.T) (*kcclient.Client, string, []*testkit.ObjectRepository, map[kernel.RepositoryID]kernel.CommitID) {
	t.Helper()
	registry := snapshot.NewRegistry()
	repos := []*testkit.ObjectRepository{}
	commits := map[kernel.RepositoryID]kernel.CommitID{}
	for _, seed := range []struct {
		repo, aspect string
		value        any
	}{
		{"kr://audit/definition", "definition", map[string]any{"repository": "kr://payload/forged", "commit": "forged", "objectId": "forged/object", "sources": []any{map[string]any{"repository": "kr://payload/source", "commit": "forged", "aspects": []string{"fake"}}}}},
		{"kr://audit/health", "health", map[string]any{"status": "healthy"}},
		{"kr://audit/secret", "secret", map[string]any{"key": "must remain withheld"}},
	} {
		repo := testkit.MakeRepository(t, seed.repo)
		commits[repo.ID()] = advanceComposedAuditAspect(t, repo, seed.aspect, seed.value)
		if err := registry.Add(repo); err != nil {
			t.Fatal(err)
		}
		repos = append(repos, repo)
	}
	dir := t.TempDir()
	if err := inv.WriteAllow(dir, inv.AllowFile{Rules: composedAuditRules(repos[:2])}); err != nil {
		t.Fatal(err)
	}
	ws := &Home{Dir: dir, Store: registry, Reader: reader.NewReader(registry), File: apphome.HomeFile{Registry: apphome.HomeRegistry{ID: "kr://audit/registry"}}}
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-composed-audit-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	facade := &httpFacade{home: dir, readHome: ws, options: HTTPServerOptions{AuthMode: "local"}, runtime: runtime}
	t.Cleanup(func() { _ = facade.closeReadHome() })
	mux := http.NewServeMux()
	facade.registerServiceRoutes(mux)
	srv := httptest.NewServer(observedHTTPHandler(runtime, mux))
	t.Cleanup(srv.Close)
	c, err := kcclient.New(kcclient.Config{BaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "reader"}}); err != nil {
		t.Fatal(err)
	}
	return c, dir, repos, commits
}

func advanceComposedAuditAspect(t *testing.T, repo *testkit.ObjectRepository, aspect string, value any) kernel.CommitID {
	t.Helper()
	base := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: aspect}, Value: value}}})
	if err != nil {
		t.Fatal(err)
	}
	return commit
}

func composedAuditRules(repos []*testkit.ObjectRepository) []inv.AllowRule {
	rules := []inv.AllowRule{{ID: "audit", Principal: "reader", Actions: []string{"audit.read"}}}
	for _, repo := range repos {
		rules = append(rules, inv.AllowRule{ID: string(repo.ID()), Principal: "reader", Repo: string(repo.ID()), Actions: []string{"object.read"}})
	}
	return rules
}

func composedAuditQuery(t *testing.T, c *kcclient.Client, query kcclient.AuditQueryRequest) observability.AccessEvent {
	t.Helper()
	var page observability.AccessPage
	if err := c.OperationsService().AccessLog(context.Background(), query, kcclient.RequestOptions{}, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Decision != "ALLOW" || page.Entries[0].Result != "RESOLVED" {
		t.Fatalf("expected one successful read access, got %#v", page)
	}
	return page.Entries[0]
}
