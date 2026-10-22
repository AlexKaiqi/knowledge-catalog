package opensearchimpl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	ports "kc/discovery/projection"
	"kc/kernel"
)

func reopenResourceEngine(t *testing.T, e *openSearchEngine, resources *namespaceResources) *openSearchEngine {
	t.Helper()
	engine, err := Open(Config{URL: e.base, Namespace: e.namespace, PrimaryShards: 1})(t.TempDir(), e.repository)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	fresh := engine.(*openSearchEngine)
	fresh.http.Transport = resources
	return fresh
}

func expireResource(t *testing.T, e *openSearchEngine, session *rebuildSession) controlDoc {
	t.Helper()
	record, version, err := e.loadControlRecord(context.Background(), session.resourceID())
	if err != nil || record.Resource == nil {
		t.Fatalf("missing resource authority: %#v %v", record, err)
	}
	record.Resource.LeaseUntil = time.Now().Add(-time.Minute).UnixMilli()
	if _, err := e.putControlRecord(session.resourceID(), record, version); err != nil {
		t.Fatal(err)
	}
	return record
}

func requireResourcePhase(t *testing.T, e *openSearchEngine, session *rebuildSession, phase string) controlDoc {
	t.Helper()
	record, _, err := e.loadControlRecord(context.Background(), session.resourceID())
	if err != nil || record.Resource == nil || record.Resource.Phase != phase {
		t.Fatalf("resource phase: want %s, got %#v %v", phase, record, err)
	}
	return record
}

func requireGenerationStatus(t *testing.T, e *openSearchEngine, name string, want int) {
	t.Helper()
	status, _, err := e.do(http.MethodHead, "/"+url.PathEscape(name), nil)
	if err != nil || status != want {
		t.Fatalf("generation %s: want HTTP %d, got %d %v", name, want, status, err)
	}
}

func TestLiveOpenSearchExpiredBuildCannotResumeAfterRecovery(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	build, err := e.BeginRebuild(ports.Meta{Basis: "abandoned"})
	if err != nil {
		t.Fatal(err)
	}
	session := build.(*rebuildSession)
	defer session.Abort(fmt.Errorf("test cleanup"))
	fresh := reopenResourceEngine(t, e, resources)
	expireResource(t, fresh, session)
	// A normal new build invokes recovery before allocating any new resource.
	next, err := fresh.BeginRebuild(ports.Meta{Basis: "replacement"})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Abort(fmt.Errorf("test cleanup"))
	requireResourcePhase(t, fresh, session, resourceDeleted)
	requireGenerationStatus(t, fresh, session.physicalIndex, http.StatusNotFound)
	if err := session.Append(publicationRecoveryDocs()); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("expired builder resumed physical writes: %v", err)
	}
	requireGenerationStatus(t, fresh, session.physicalIndex, http.StatusNotFound)
}

func TestLiveOpenSearchBuildRenewsProtectionBeforePhysicalBatch(t *testing.T) {
	e, _ := publicationRecoveryEngine(t)
	build, err := e.BeginRebuild(ports.Meta{Basis: "long-build"})
	if err != nil {
		t.Fatal(err)
	}
	session := build.(*rebuildSession)
	defer session.Abort(fmt.Errorf("test cleanup"))
	record, version, err := e.loadControlRecord(context.Background(), session.resourceID())
	if err != nil {
		t.Fatal(err)
	}
	record.Resource.LeaseUntil = time.Now().Add(time.Minute).UnixMilli()
	if _, err := e.putControlRecord(session.resourceID(), record, version); err != nil {
		t.Fatal(err)
	}
	if err := session.Append(publicationRecoveryDocs()); err != nil {
		t.Fatal(err)
	}
	renewed := requireResourcePhase(t, e, session, resourceBuilding)
	if renewed.Resource.Pending || renewed.Resource.LeaseUntil < time.Now().Add(14*time.Minute).UnixMilli() {
		t.Fatalf("physical batch did not renew and drain durable protection: %#v", renewed.Resource)
	}
}

type pausedResourceBulk struct {
	next    http.RoundTripper
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *pausedResourceBulk) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodPost && request.URL.Path == "/_bulk" {
		p.once.Do(func() { close(p.entered) })
		<-p.release
	}
	return p.next.RoundTrip(request)
}

func TestLiveOpenSearchPendingWriteProtectsExpiredResourceUntilDrained(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	build, err := e.BeginRebuild(ports.Meta{Basis: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	session := build.(*rebuildSession)
	defer session.Abort(fmt.Errorf("test cleanup"))
	pause := &pausedResourceBulk{next: resources, entered: make(chan struct{}), release: make(chan struct{})}
	e.http.Transport = pause
	done := make(chan error, 1)
	go func() { done <- session.Append(publicationRecoveryDocs()) }()
	released := false
	defer func() {
		if !released {
			close(pause.release)
			<-done
		}
	}()
	select {
	case <-pause.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("physical bulk was not admitted")
	}
	fresh := reopenResourceEngine(t, e, resources)
	record := expireResource(t, fresh, session)
	if !record.Resource.Pending {
		t.Fatal("physical request began without durable admission")
	}
	if err := fresh.reconcileCandidates(); err != nil {
		t.Fatal(err)
	}
	requireGenerationStatus(t, fresh, session.physicalIndex, http.StatusOK)
	record = requireResourcePhase(t, fresh, session, resourceBuilding)
	if !record.Resource.Pending {
		t.Fatal("expired admitted request was forgotten")
	}
	close(pause.release)
	released = true
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := fresh.reconcileCandidates(); err != nil {
		t.Fatal(err)
	}
	requireResourcePhase(t, fresh, session, resourceDeleted)
	requireGenerationStatus(t, fresh, session.physicalIndex, http.StatusNotFound)
	if err := session.Append(publicationRecoveryDocs()); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("drained old builder recreated deleted resource: %v", err)
	}
}

type lostResourceResponse struct {
	next   http.RoundTripper
	method string
	path   string
	lost   bool
	actual string
}

type rejectedResourceBulk struct {
	next      http.RoundTripper
	uncertain bool
}

func (r rejectedResourceBulk) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodPost && request.URL.Path == "/_bulk" {
		items := `{"index":{"status":400,"error":{"type":"injected_rejection"}}}`
		if r.uncertain {
			items += `,{"index":{"status":503,"error":{"type":"unknown_completion"}}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"errors":true,"items":[` + items + `]}`)), Header: http.Header{}, Request: request}, nil
	}
	return r.next.RoundTrip(request)
}

func TestLiveOpenSearchMixedBulkFailureRetainsUnknownRequest(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	build, err := e.BeginRebuild(ports.Meta{Basis: "uncertain-batch"})
	if err != nil {
		t.Fatal(err)
	}
	session := build.(*rebuildSession)
	e.http.Transport = rejectedResourceBulk{next: resources, uncertain: true}
	docs := publicationRecoveryDocs()
	second := docs[0]
	second.ObjectID = "article/two"
	failure := session.Append(append(docs, second))
	if failure == nil {
		t.Fatal("mixed failed bulk reported success")
	}
	if record := requireResourcePhase(t, e, session, resourceBuilding); !record.Resource.Pending {
		t.Fatal("early definitive rejection hid a later unknown physical result")
	}
	if err := session.Abort(failure); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("unknown batch was treated as a drained candidate: %v", err)
	}
	requireGenerationStatus(t, e, session.physicalIndex, http.StatusOK)
}

func TestLiveOpenSearchDefinitiveBulkFailureDrainsCandidate(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	build, err := e.BeginRebuild(ports.Meta{Basis: "rejected-batch"})
	if err != nil {
		t.Fatal(err)
	}
	session := build.(*rebuildSession)
	e.http.Transport = rejectedResourceBulk{next: resources}
	failure := session.Append(publicationRecoveryDocs())
	if failure == nil {
		t.Fatal("definitively rejected bulk reported success")
	}
	if record := requireResourcePhase(t, e, session, resourceBuilding); record.Resource.Pending {
		t.Fatal("known completed rejection retained an unresolved physical request")
	}
	if err := session.Abort(failure); err == nil {
		t.Fatal("abort lost the failed batch")
	}
	requireResourcePhase(t, e, session, resourceDeleted)
	requireGenerationStatus(t, e, session.physicalIndex, http.StatusNotFound)
}

func (l *lostResourceResponse) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := l.next.RoundTrip(request)
	if err == nil && !l.lost && request.Method == l.method && strings.Contains(request.URL.Path, l.path) && !strings.Contains(request.URL.Path, "/_doc/") && response.StatusCode < 300 {
		l.lost, l.actual = true, request.URL.Path
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil, fmt.Errorf("injected response loss after physical %s", request.Method)
	}
	return response, err
}

func TestLiveOpenSearchUnknownCreationIsNotReportedDeleted(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	loss := &lostResourceResponse{next: resources, method: http.MethodPut, path: e.prefix + "-g-"}
	e.http.Transport = loss
	if _, err := e.BeginRebuild(ports.Meta{Basis: "unknown"}); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || !loss.lost {
		t.Fatalf("unknown creation not retained: %v, lost=%t", err, loss.lost)
	}
	fresh := reopenResourceEngine(t, e, resources)
	name := strings.TrimPrefix(loss.actual, "/")
	session := &rebuildSession{engine: fresh, physicalIndex: name, generation: strings.TrimPrefix(name, e.prefix+"-g-")}
	expireResource(t, fresh, session)
	if err := fresh.reconcileCandidates(); err != nil {
		t.Fatal(err)
	}
	record := requireResourcePhase(t, fresh, session, resourceBuilding)
	if !record.Resource.Pending {
		t.Fatal("unknown admitted create was cleared")
	}
	requireGenerationStatus(t, fresh, name, http.StatusOK)
}

func TestLiveOpenSearchDeletionResumesAfterLostResponseAndReopen(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	build, err := e.BeginRebuild(ports.Meta{Basis: "discard"})
	if err != nil {
		t.Fatal(err)
	}
	session := build.(*rebuildSession)
	loss := &lostResourceResponse{next: resources, method: http.MethodDelete, path: session.physicalIndex}
	e.http.Transport = loss
	if err := session.Abort(fmt.Errorf("discard test")); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || !loss.lost {
		t.Fatalf("deletion uncertainty not retained: %v, lost=%t", err, loss.lost)
	}
	fresh := reopenResourceEngine(t, e, resources)
	requireResourcePhase(t, fresh, session, resourceDeleting)
	if err := fresh.reconcileCandidates(); err != nil {
		t.Fatal(err)
	}
	requireResourcePhase(t, fresh, session, resourceDeleted)
	requireGenerationStatus(t, fresh, session.physicalIndex, http.StatusNotFound)
}
