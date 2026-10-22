package opensearchimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ports "kc/discovery/projection"
)

func TestLiveOpenSearchPublishingRecoveryRequiresCompleteReadyEvidence(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching", true: "different-count"}[mismatch], func(t *testing.T) {
			e, _ := publicationRecoveryEngine(t)
			_, ticket := preparePublicationResource(t, e)
			session := ticket.session
			if err := session.changeResourceRecord(func(record *controlDoc) error {
				record.Resource.Phase = resourcePublishing
				if mismatch {
					record.ObjectCount++
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			report, err := resourceAuthorityForTest(t, e).RecoverResources(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			record, _, err := e.loadControlRecord(context.Background(), session.resourceID())
			if err != nil {
				t.Fatal(err)
			}
			if mismatch {
				if record.Resource.Phase != resourcePublishing || len(report.Retained) != 1 || len(report.Deleted) != 0 {
					t.Fatalf("unproven publication changed: %+v %+v", record.Resource, report)
				}
				return
			}
			if record.Resource.Phase != resourceAdopted || len(report.Protected) != 1 {
				t.Fatalf("publication not recovered: %+v %+v", record.Resource, report)
			}
			if len(record.Resource.Claims) != 1 {
				t.Fatal("recovery lost independent protection")
			}
			if err := session.resourcePhase(resourceAdopted); err != nil {
				t.Fatalf("late publisher acknowledgement: %v", err)
			}
			meta, err := e.LoadMeta()
			if err != nil || meta.State != ports.ProjectionStateReady {
				t.Fatalf("recovered projection: %+v %v", meta, err)
			}
		})
	}
}

// Fail the final durable acknowledgement before it reaches OpenSearch, after
// the real ROOT CAS has succeeded. Recovery uses a separate client.
type rejectAdoptionAcknowledgement struct {
	next     http.RoundTripper
	rejected atomic.Bool
}

func (r *rejectAdoptionAcknowledgement) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodPut && request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		var record controlDoc
		decodeErr := json.NewDecoder(body).Decode(&record)
		_ = body.Close()
		if decodeErr == nil && record.Resource != nil && record.Resource.Phase == resourceAdopted {
			r.rejected.Store(true)
			return nil, fmt.Errorf("injected final adoption acknowledgement failure")
		}
	}
	return r.next.RoundTrip(request)
}
func TestLiveOpenSearchPublishingRecoveryAfterActualRootCommit(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	failure := &rejectAdoptionAcknowledgement{next: resources}
	e.http.Transport = failure
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "committed-before-ack"}); err == nil || !failure.rejected.Load() {
		t.Fatalf("fault did not interrupt acknowledgement: %v", err)
	}
	e.http.Transport = resources
	root, _, err := e.loadControl()
	if err != nil || root.State != ports.ProjectionStateReady {
		t.Fatalf("actual ROOT did not commit: %+v %v", root, err)
	}
	session := &rebuildSession{engine: e, generation: root.Generation, physicalIndex: root.ActiveIndex}
	before, _, err := e.loadControlRecord(context.Background(), session.resourceID())
	if err != nil || before.Resource == nil || before.Resource.Phase != resourcePublishing {
		t.Fatalf("missing interrupted publication: %+v %v", before.Resource, err)
	}
	report, err := resourceAuthorityForTest(t, e).RecoverResources(context.Background())
	if err != nil || len(report.Protected) != 1 || len(report.Deleted) != 0 {
		t.Fatalf("restart recovery: %+v %v", report, err)
	}
	after, _, err := e.loadControlRecord(context.Background(), session.resourceID())
	if err != nil || after.Resource.Phase != resourceAdopted {
		t.Fatalf("durable adoption was not recovered: %+v %v", after.Resource, err)
	}
	rootAfter, _, err := e.loadControl()
	if err != nil || rootAfter.Generation != root.Generation || rootAfter.Basis != "committed-before-ack" {
		t.Fatalf("recovery rewrote ROOT: %+v %v", rootAfter, err)
	}
}

type pausedReadyPublication struct {
	next            http.RoundTripper
	arrived, resume chan struct{}
	once            sync.Once
}

func (p *pausedReadyPublication) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodPut && request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, err
		}
		var record controlDoc
		decodeErr := json.NewDecoder(body).Decode(&record)
		_ = body.Close()
		if decodeErr == nil && record.State == ports.ProjectionStateReady {
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
func TestLiveOpenSearchExpiredPublishingFenceExcludesDelayedRootCommit(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	build, err := e.BeginRebuild(ports.Meta{Basis: "delayed-publication"})
	if err != nil {
		t.Fatal(err)
	}
	session := build.(*rebuildSession)
	if err := build.Append(publicationRecoveryDocs()); err != nil {
		t.Fatal(build.Abort(err))
	}
	pause := &pausedReadyPublication{next: resources, arrived: make(chan struct{}), resume: make(chan struct{})}
	e.http.Transport = pause
	finished := make(chan error, 1)
	completed := make(chan struct{})
	go func() { defer close(completed); finished <- build.Commit() }()
	var resumed sync.Once
	resume := func() { resumed.Do(func() { close(pause.resume) }) }
	t.Cleanup(func() {
		resume()
		select {
		case <-completed:
			e.http.Transport = resources
		case <-time.After(10 * time.Second):
			t.Error("publisher did not exit during cleanup")
		}
	})
	select {
	case <-pause.arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("publisher did not reach ROOT CAS")
	}
	_, before, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	coordinator := resourceAuthorityForTest(t, e)
	report, err := coordinator.RecoverResources(context.Background())
	if err != nil || len(report.Protected) != 1 || len(report.Deleted) != 0 {
		t.Fatalf("live publisher was fenced: %+v %v", report, err)
	}
	_, unchanged, err := e.loadControl()
	if err != nil || *unchanged != *before {
		t.Fatalf("active publication changed ROOT: %+v %+v %v", before, unchanged, err)
	}
	if err := session.changeResource(func(resource *generationResource) error {
		resource.LeaseUntil = time.Now().Add(-time.Minute).UnixMilli()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	report, err = coordinator.RecoverResources(context.Background())
	if err != nil || len(report.Deleted) != 1 {
		t.Fatalf("expired publication not fenced and deleted: %+v %v", report, err)
	}
	resume()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("delayed publisher crossed ROOT fence")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("publisher did not finish")
	}
	e.http.Transport = resources
	root, after, err := e.loadControl()
	if err != nil || root.State == ports.ProjectionStateReady || *after == *before {
		t.Fatalf("delayed publisher changed ROOT after fence: %+v %v", root, err)
	}
	record, _, err := e.loadControlRecord(context.Background(), session.resourceID())
	if err != nil || record.Resource.Phase != resourceDeleted {
		t.Fatalf("deletion not durable: %+v %v", record.Resource, err)
	}
	status, _, err := e.do(http.MethodHead, "/"+session.physicalIndex, nil)
	if err != nil || status != http.StatusNotFound {
		t.Fatalf("delayed publisher resurrected resource: %d %v", status, err)
	}
}

type losePublicationFence struct {
	next http.RoundTripper
	path string
	lost atomic.Bool
}

func (l *losePublicationFence) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := l.next.RoundTrip(request)
	if err == nil && request.Method == http.MethodPut && request.URL.Path == l.path && response.StatusCode < 300 && l.lost.CompareAndSwap(false, true) {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return nil, fmt.Errorf("injected rejection fence confirmation loss")
	}
	return response, err
}
func TestLiveOpenSearchPublishingRejectionRetainsUnknownFenceAndMissingRoot(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "lost-fence-confirmation", true: "missing-root"}[missing], func(t *testing.T) {
			e, resources := publicationRecoveryEngine(t)
			build, err := e.BeginRebuild(ports.Meta{Basis: "unconfirmed"})
			if err != nil {
				t.Fatal(err)
			}
			session := build.(*rebuildSession)
			defer func() { _ = build.Abort(fmt.Errorf("test abandoned publisher")) }()
			if err := session.resourcePhase(resourcePublishing); err != nil {
				t.Fatal(err)
			}
			if err := session.changeResource(func(resource *generationResource) error {
				resource.LeaseUntil = time.Now().Add(-time.Minute).UnixMilli()
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			peer := reopenResourceEngine(t, e, resources)
			path := "/" + e.controlIndexPath() + "/_doc/" + url.PathEscape(e.controlID)
			loss := &losePublicationFence{next: resources, path: path}
			if missing {
				status, _, err := e.do(http.MethodDelete, path, nil)
				if err != nil || status >= 300 {
					t.Fatalf("remove ROOT: %d %v", status, err)
				}
			} else {
				peer.http.Transport = loss
			}
			report, err := peer.RecoverResources(context.Background())
			if err != nil || len(report.Retained) != 1 || len(report.Deleted) != 0 {
				t.Fatalf("unknown rejection was treated as deletion: %+v %v", report, err)
			}
			record, _, err := e.loadControlRecord(context.Background(), session.resourceID())
			if err != nil || record.Resource.Phase != resourcePublishing {
				t.Fatalf("unknown fence closed candidate: %+v %v", record.Resource, err)
			}
			status, _, err := e.do(http.MethodHead, "/"+session.physicalIndex, nil)
			if err != nil || status != http.StatusOK {
				t.Fatalf("unknown fence deleted physical candidate: %d %v", status, err)
			}
			if missing {
				_, version, err := e.loadControl()
				if err != nil || version != nil {
					t.Fatalf("recovery initialized missing ROOT: %+v %v", version, err)
				}
				return
			}
			if !loss.lost.Load() {
				t.Fatal("fence failure was not exercised")
			}
			peer.http.Transport = resources
			report, err = peer.RecoverResources(context.Background())
			if err != nil || len(report.Deleted) != 1 {
				t.Fatalf("confirmed retry did not finish deletion: %+v %v", report, err)
			}
		})
	}
}
