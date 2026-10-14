package control

import (
	"context"
	"encoding/json"
	fixtures "kc/declarativeindex/internal/testfixture"
	query "kc/declarativeindex/projection"
	ports "kc/discovery/projection"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
	"kc/snapshot"
)

type noticeRequestLookup struct {
	mu       sync.Mutex
	name     string
	requests []liveaccess.StateLookupRequest
	err      error
}

func (l *noticeRequestLookup) LookupState(_ context.Context, req liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests = append(l.requests, req)
	if l.err != nil {
		return liveaccess.StateObservation{}, l.err
	}
	return liveaccess.StateObservation{Value: map[string]any{"status": l.name}, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: l.name, ObservedAt: "2026-10-13T00:00:00Z"}}, nil
}

func noticeRequestFixture(t *testing.T, origins ...string) (*Controller, *query.Index, []objectmodel.Repository, map[kernel.RepositoryID]kernel.CommitID, objectmodel.Address) {
	t.Helper()
	origin := "https://runtime.example"
	if len(origins) > 0 {
		origin = origins[0]
	}
	repos := []objectmodel.Repository{}
	commits := map[kernel.RepositoryID]kernel.CommitID{}
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "Job:request-notice", AspectName: "runtime"}
	for _, id := range []string{"kr://notice/request-a", "kr://notice/request-b"} {
		s := testkit.NewSetup(t, id)
		at, err := s.Repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{"entity": "Job", "aspect": "runtime", "origin": origin, "fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"text"}}}}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}, Value: map[string]any{"owner": "fixed"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		repos = append(repos, s.Repo)
		commits[s.RepositoryID] = at
	}
	engines := map[kernel.RepositoryID]*fixtures.StateEngine{}
	idx := query.NewIndexEngine("", func(_ string, id kernel.RepositoryID) (ports.Engine, error) {
		if engines[id] == nil {
			engines[id] = &fixtures.StateEngine{Docs: map[objectmodel.ObjectID]ports.CompiledDoc{}}
		}
		return engines[id], nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	controller, err := NewController(idx, NewTargetStore(filepath.Join(t.TempDir(), "controller.db")), func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		for _, repo := range repos {
			if repo.ID() == id {
				return repo, nil
			}
		}
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "unknown repository")
	})
	if err != nil {
		t.Fatal(err)
	}
	controller.SetInventory(func() ([]kernel.RepositoryID, error) { return []kernel.RepositoryID{repos[0].ID(), repos[1].ID()}, nil })
	t.Cleanup(controller.Close)
	return controller, idx, repos, commits, address
}

func TestProjectionBackgroundNoticeDoesNotForwardCallerCredentialContext(t *testing.T) {
	var headers []http.Header
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Clone())
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"status": "observed"}, "basis": objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "one", ObservedAt: "2026-10-13T00:00:00Z"}})
	}))
	defer runtime.Close()
	c, _, repos, _, address := noticeRequestFixture(t, runtime.URL)
	lookup, err := liveaccess.NewHTTPStateLookupWithOptions(liveaccess.HTTPStateLookupOptions{
		Client: runtime.Client(), CredentialOrigins: []string{runtime.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := liveaccess.ContextWithCallerCredentials(context.Background(), liveaccess.CallerCredentialsFromHeader(http.Header{"Authorization": []string{"Bearer request-caller"}, "X-Tai-Identity": []string{"request-proof"}}))
	port, ok := any(c).(requestNoticeApplier)
	if !ok {
		t.Fatal("request-local notice port is unavailable")
	}
	if _, err := port.ApplyNotice(ctx, ports.ChangeNotice{Repository: repos[0].ID(), Address: &address}, sourcebridge.Source{Lookup: lookup, Request: objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "caller"}}}); err != nil {
		t.Fatal(err)
	}
	if len(headers) != 1 || headers[0].Get("Authorization") != "Bearer request-caller" || headers[0].Get("X-Tai-Identity") != "request-proof" || headers[0].Get("X-Resource-Principal") != "caller" {
		t.Fatalf("explicit request source lost caller proof: %#v", headers)
	}
	c.SetObservationSource(sourcebridge.Source{Lookup: lookup, Request: objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "worker"}, RequestID: "worker-request"}})
	if _, err := port.ApplyNotice(ctx, ports.ChangeNotice{Repository: repos[0].ID(), Address: &address}, sourcebridge.Source{Lookup: lookup, Request: objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "caller"}}}); err != nil {
		t.Fatal(err)
	}
	if len(headers) != 2 || headers[1].Get("Authorization") != "" || headers[1].Get("X-Tai-Identity") != "" || headers[1].Get("X-Resource-Principal") != "worker" || headers[1].Get("X-Resource-Request-Id") != "worker-request" {
		t.Fatalf("configured worker borrowed request credential context: %#v", headers)
	}
	if err := c.Notify(ports.ChangeNotice{Repository: repos[1].ID(), Address: &address}); err != nil {
		t.Fatal(err)
	}
	if err := c.CatchUp(ctx); err != nil {
		t.Fatal(err)
	}
	for _, header := range headers[2:] {
		if header.Get("Authorization") != "" || header.Get("X-Tai-Identity") != "" || header.Get("X-Resource-Principal") != "worker" {
			t.Fatalf("background catch-up borrowed caller proof: %#v", header)
		}
	}
}

type requestNoticeApplier interface {
	ApplyNotice(context.Context, ports.ChangeNotice, ports.ObservationSource) (ports.StateSync, error)
}

func applyRequestNotice(t *testing.T, c *Controller, notice ports.ChangeNotice, source ports.ObservationSource) (ports.StateSync, error) {
	t.Helper()
	port, ok := any(c).(requestNoticeApplier)
	if !ok {
		t.Fatal("Controller cannot apply a request-local notice without replacing background configuration")
	}
	return port.ApplyNotice(context.Background(), notice, source)
}

func TestProjectionRequestNoticeDoesNotRetainCallerOrRefreshUnrelatedRepository(t *testing.T) {
	c, idx, repos, commits, address := noticeRequestFixture(t)
	first := &noticeRequestLookup{name: "caller-one"}
	request := objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "caller-one"}, RequestID: "request-one"}
	result, err := applyRequestNotice(t, c, ports.ChangeNotice{Repository: repos[0].ID(), Address: &address}, sourcebridge.Source{Lookup: first, Request: request})
	if err != nil || result.Repository != repos[0].ID() || result.BasisCommit != commits[repos[0].ID()] || result.Revision == "" {
		t.Fatalf("request-local notice result %#v %v", result, err)
	}
	if len(first.requests) != 1 || first.requests[0].Binding.Repository != repos[0].ID() || first.requests[0].Identity != request.Identity || first.requests[0].RequestID != request.RequestID {
		t.Fatalf("request source lost local scope/context: %#v", first.requests)
	}
	if c.stateRuntime() != nil {
		t.Fatal("caller source became background configuration")
	}
	if _, ready := idx.StateView(repos[1].ID(), commits[repos[1].ID()]); ready {
		t.Fatal("request notice refreshed unrelated repository")
	}
	if err := c.Notify(ports.ChangeNotice{Repository: repos[1].ID(), Address: &address}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("later unrelated notice borrowed caller source: %v", err)
	}
	if err := c.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(first.requests) != 1 {
		t.Fatalf("background CatchUp reused caller: %#v", first.requests)
	}
	second := &noticeRequestLookup{name: "caller-two"}
	next, err := applyRequestNotice(t, c, ports.ChangeNotice{Repository: repos[0].ID(), Address: &address, SourceRevision: "two"}, sourcebridge.Source{Lookup: second, Request: objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "caller-two"}, RequestID: "request-two"}})
	if err != nil || next.Revision == result.Revision || next.BasisCommit != result.BasisCommit || len(second.requests) != 1 || len(first.requests) != 1 || second.requests[0].Identity.Principal != "caller-two" {
		t.Fatalf("next caller/basis/revision %#v %#v %v", next, second.requests, err)
	}
	failed := &noticeRequestLookup{name: "failed-caller", err: kernel.Fail(kernel.ErrTemporaryUnavailable, "runtime failed")}
	_, err = applyRequestNotice(t, c, ports.ChangeNotice{Repository: repos[0].ID(), Address: &address}, sourcebridge.Source{Lookup: failed, Request: request})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || c.stateRuntime() != nil {
		t.Fatalf("failed request retained source or hid error: %v", err)
	}
	if revision, ready := idx.StateView(repos[0].ID(), result.BasisCommit); !ready || revision != next.Revision {
		t.Fatalf("failed notice replaced serving revision: %s %v", revision, ready)
	}
	for _, repo := range repos {
		head, err := repo.Head(snapshot.DefaultRef)
		if err != nil || head != commits[repo.ID()] {
			t.Fatalf("notice changed Snapshot %s %s %v", repo.ID(), head, err)
		}
	}
}

func TestProjectionRequestNoticePreservesConfiguredSourceAndPendingWork(t *testing.T) {
	c, idx, repos, commits, address := noticeRequestFixture(t)
	background := &noticeRequestLookup{name: "background"}
	request := objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "worker"}, RequestID: "worker-request"}
	c.SetObservationSource(sourcebridge.Source{Lookup: background, Request: request})
	pending := ports.ChangeNotice{Repository: repos[1].ID(), Address: &address, SourceRevision: "pending"}
	if err := c.Notify(pending); err != nil {
		t.Fatal(err)
	}
	caller := &noticeRequestLookup{name: "caller"}
	_, err := applyRequestNotice(t, c, ports.ChangeNotice{Repository: repos[0].ID(), Address: &address}, sourcebridge.Source{Lookup: caller, Request: objectmodel.RequestContext{Identity: kernel.IdentityContext{Principal: "caller"}}})
	if err != nil || len(caller.requests) != 0 || len(background.requests) != 1 || background.requests[0].Binding.Repository != repos[0].ID() || background.requests[0].Identity != request.Identity || background.requests[0].RequestID != request.RequestID {
		t.Fatalf("configured source replaced: caller=%#v background=%#v error=%v", caller.requests, background.requests, err)
	}
	records, err := c.store.pendingNotices()
	notices := make([]ports.ChangeNotice, 0, len(records))
	for _, record := range records {
		notices = append(notices, record.ChangeNotice)
	}
	if err != nil || !reflect.DeepEqual(notices, []ports.ChangeNotice{pending}) {
		t.Fatalf("explicit notice drained unrelated pending work: %#v %v", notices, err)
	}
	if _, ready := idx.StateView(repos[1].ID(), commits[repos[1].ID()]); ready {
		t.Fatal("unrelated pending notice used request execution")
	}
	if _, err := applyRequestNotice(t, c, ports.ChangeNotice{}, sourcebridge.Source{Lookup: caller}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("invalid notice accepted: %v", err)
	}
	if err := c.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(caller.requests) != 0 || len(background.requests) < 2 {
		t.Fatalf("background retained caller after notice: %#v %#v", caller.requests, background.requests)
	}
	for _, req := range background.requests {
		if req.Identity != request.Identity || req.RequestID != request.RequestID {
			t.Fatalf("background worker adopted caller context: %#v", req)
		}
	}
	background.err = kernel.Fail(kernel.ErrTemporaryUnavailable, "configured worker runtime failed")
	_, err = applyRequestNotice(t, c, ports.ChangeNotice{Repository: repos[0].ID(), Address: &address}, sourcebridge.Source{Lookup: caller})
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || len(caller.requests) != 0 || c.stateRuntime() == nil {
		t.Fatalf("configured runtime failure fell back to caller: %#v %v", caller.requests, err)
	}
}

func TestProjectionRequestNoticeRejectsLookupSourceSubstitution(t *testing.T) {
	c, idx, repos, commits, address := noticeRequestFixture(t)
	c.lookup = func(kernel.RepositoryID) (objectmodel.Repository, error) { return repos[1], nil }
	caller := &noticeRequestLookup{name: "caller"}
	_, err := applyRequestNotice(t, c, ports.ChangeNotice{Repository: repos[0].ID(), Address: &address}, sourcebridge.Source{Lookup: caller})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || len(caller.requests) != 0 {
		t.Fatalf("request source was used for another repository: %#v %v", caller.requests, err)
	}
	if _, ready := idx.StateView(repos[1].ID(), commits[repos[1].ID()]); ready {
		t.Fatal("source substitution published an unrelated repository")
	}
}
