package opensearchimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	ports "kc/discovery/projection"
)

type pausedResourceAdoption struct {
	next            http.RoundTripper
	arrived, resume chan struct{}
	once            sync.Once
}

func (p *pausedResourceAdoption) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodPut && request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		var record controlDoc
		decodeErr := json.NewDecoder(body).Decode(&record)
		_ = body.Close()
		if decodeErr == nil && record.Resource != nil && record.Resource.Phase == resourceAdopted {
			p.once.Do(func() { close(p.arrived) })
			select {
			case <-p.resume:
			case <-request.Context().Done():
				return nil, request.Context().Err()
			}
		}
	}
	return p.next.RoundTrip(request)
}

func TestLiveOpenSearchProtectedBuildKeepsOriginalTicketAcrossRecoveryAndReplacement(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	pause := &pausedResourceAdoption{next: resources, arrived: make(chan struct{}), resume: make(chan struct{})}
	e.http.Transport = pause
	type outcome struct {
		ticket ports.PreparedResource
		err    error
	}
	result := make(chan outcome, 1)
	completed := make(chan struct{})
	go func() {
		defer close(completed)
		build, err := e.BeginRebuild(ports.Meta{Basis: "protected-original"})
		if err != nil {
			result <- outcome{nil, err}
			return
		}
		if err := build.Append(publicationRecoveryDocs()); err != nil {
			result <- outcome{nil, build.Abort(err)}
			return
		}
		ticket, err := build.(ports.PreparedRebuildSession).CommitResource()
		result <- outcome{ticket, err}
	}()
	var resumed sync.Once
	resume := func() { resumed.Do(func() { close(pause.resume) }) }
	t.Cleanup(func() {
		resume()
		select {
		case <-completed:
			e.http.Transport = resources
		case <-time.After(10 * time.Second):
			t.Error("protected builder did not exit")
		}
	})
	select {
	case <-pause.arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("builder did not reach adoption")
	}
	root, _, err := e.loadControl()
	if err != nil || root.State != ports.ProjectionStateReady {
		t.Fatalf("ROOT not committed: %+v %v", root, err)
	}
	session := &rebuildSession{engine: e, generation: root.Generation, physicalIndex: root.ActiveIndex}
	record, _, err := e.loadControlRecord(context.Background(), session.resourceID())
	if err != nil || record.Resource.Phase != resourcePublishing || len(record.Resource.Claims) != 1 || record.Resource.Claims[0].Phase != claimPrepared {
		t.Fatalf("publication lost continuous preparation: %+v %v", record.Resource, err)
	}
	originalClaim := record.Resource.Claims[0].ID
	peer := reopenResourceEngine(t, e, resources)
	report, err := peer.RecoverResources(context.Background())
	if err != nil || len(report.Protected) != 1 {
		t.Fatalf("adoption recovery: %+v %v", report, err)
	}
	if err := peer.Rebuild(nil, ports.Meta{Basis: "replacement"}); err != nil {
		t.Fatal(err)
	}
	report, err = peer.RecoverResources(context.Background())
	if err != nil || len(report.Deleted) != 0 {
		t.Fatalf("prepared generation reclaimed before builder returned: %+v %v", report, err)
	}
	status, _, err := peer.do(http.MethodHead, "/"+root.ActiveIndex, nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("original physical generation lost: %d %v", status, err)
	}
	resume()
	var built outcome
	select {
	case built = <-result:
	case <-time.After(10 * time.Second):
		t.Fatal("builder did not return")
	}
	if built.err != nil || built.ticket == nil {
		t.Fatalf("protected build failed: %v", built.err)
	}
	ticket := built.ticket.(*preparedResource)
	if ticket.id != originalClaim || ticket.ready.Generation != root.Generation || ticket.ready.Basis != "protected-original" {
		t.Fatalf("builder returned replacement ticket: %+v", ticket.ready)
	}
	if err := ticket.Admit(context.Background(), []byte("protected-build-authority")); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Confirm(context.Background()); err != nil {
		t.Fatal(err)
	}
	report, err = peer.RecoverResources(context.Background())
	if err != nil || len(report.Deleted) != 0 {
		t.Fatalf("accepted original ticket lost: %+v %v", report, err)
	}
	if err := ticket.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLiveOpenSearchAbandonedProtectedPublicationExpiresOnlyPreparedClaim(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	build, err := e.BeginRebuild(ports.Meta{Basis: "abandoned-protected"})
	if err != nil {
		t.Fatal(err)
	}
	session := build.(*rebuildSession)
	pause := &pausedReadyPublication{next: resources, arrived: make(chan struct{}), resume: make(chan struct{})}
	e.http.Transport = pause
	finished := make(chan error, 1)
	completed := make(chan struct{})
	go func() { defer close(completed); _, err := session.CommitResource(); finished <- err }()
	var resumed sync.Once
	resume := func() { resumed.Do(func() { close(pause.resume) }) }
	t.Cleanup(func() {
		resume()
		select {
		case <-completed:
			e.http.Transport = resources
		case <-time.After(10 * time.Second):
			t.Error("protected publisher did not exit")
		}
	})
	select {
	case <-pause.arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("protected publisher did not reach ROOT")
	}
	if err := session.changeResource(func(resource *generationResource) error {
		resource.LeaseUntil = time.Now().Add(-time.Minute).UnixMilli()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	coordinator := resourceAuthorityForTest(t, e)
	report, err := coordinator.RecoverResources(context.Background())
	if err != nil || len(report.Retained) != 1 || len(report.Deleted) != 0 {
		t.Fatalf("live prepared claim did not retain candidate: %+v %v", report, err)
	}
	if err := session.changeResource(func(resource *generationResource) error {
		if len(resource.Claims) != 1 || resource.Claims[0].Phase != claimPrepared {
			t.Fatalf("missing original preparation: %+v", resource.Claims)
		}
		resource.Claims[0].Until = time.Now().Add(-time.Minute).UnixMilli()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	report, err = coordinator.RecoverResources(context.Background())
	if err != nil || len(report.Deleted) != 1 {
		t.Fatalf("abandoned preparation prevented confirmed rejection: %+v %v", report, err)
	}
	resume()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("abandoned publisher crossed rejection fence")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("abandoned publisher did not exit")
	}
	record, _, err := e.loadControlRecord(context.Background(), session.resourceID())
	if err != nil || record.Resource.Phase != resourceDeleted || len(record.Resource.Claims) != 0 {
		t.Fatalf("abandoned ticket resurrected: %+v %v", record.Resource, err)
	}
}
