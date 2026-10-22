package opensearchimpl

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	"kc/object/reader"
)

type pausedRootSearch struct {
	path    string
	next    http.RoundTripper
	arrived chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (p *pausedRootSearch) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodPost && ((p.path == "" && request.URL.Path == "/_search") || (p.path != "" && request.URL.Path == p.path)) {
		p.once.Do(func() { close(p.arrived) })
		select {
		case <-p.resume:
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	}
	return p.next.RoundTrip(request)
}

func TestLiveOpenSearchHistoricalGCWaitsForOrdinaryRootQueryThenDeletesAcrossReopen(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "fixed"}); err != nil {
		t.Fatal(err)
	}
	original, version, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	old := &rebuildSession{engine: e, generation: original.Generation, physicalIndex: original.ActiveIndex}
	pause := &pausedRootSearch{next: resources, arrived: make(chan struct{}), resume: make(chan struct{})}
	e.http.Transport = pause
	finished := make(chan error, 1)
	released, joined := false, false
	t.Cleanup(func() {
		if !released {
			close(pause.resume)
		}
		if !joined {
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				t.Error("ROOT query did not exit")
			}
		}
		e.http.Transport = resources
	})
	go func() {
		page, err := e.RetrieveContext(context.Background(), ports.RetrieveRequest{Search: retrieval.SearchOf(retrieval.SearchMATCH("runbook")), Spec: retrieval.AccessSpec{Repository: "logical", Commit: "fixed", Fields: []retrieval.AccessField{{FieldRef: retrieval.FieldRef{Schema: "schema/article", Path: "body"}, Type: "string", Access: []reader.AccessHint{reader.HintText}}}}})
		if err == nil && (len(page.Candidates) != 1 || page.Candidates[0].ObjectID != "article/one") {
			err = errors.New("original query lost its fixed candidate")
		}
		finished <- err
	}()
	select {
	case <-pause.arrived:
	case err := <-finished:
		joined = true
		t.Fatalf("query did not reach search: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("query admission timeout")
	}
	peer := reopenResourceEngine(t, e, resources)
	if err := peer.Rebuild(nil, ports.Meta{Basis: "new"}); err != nil {
		t.Fatal(err)
	}
	manager := resourceAuthorityForTest(t, peer)
	report, err := manager.RecoverResources(context.Background())
	if err != nil || !slices.Contains(report.Protected, original.ActiveIndex) || slices.Contains(report.Deleted, original.ActiveIndex) {
		t.Fatalf("in-flight ROOT query was unprotected: %#v %v", report, err)
	}
	requireGenerationStatus(t, peer, original.ActiveIndex, http.StatusOK)
	close(pause.resume)
	released = true
	queryErr := <-finished
	joined = true
	if queryErr != nil {
		t.Fatal(queryErr)
	}
	e.http.Transport = resources
	// An old admitted update that loses the ROOT CAS made no physical request;
	// its write exclusion must drain instead of pinning the obsolete generation.
	err = e.updateResource(original, func(guard func(func() error) error) error {
		return e.applyGeneration(original, version, publicationRecoveryDocs(), nil, ports.Meta{Basis: "late"}, guard)
	})
	if kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("old writer was not fenced: %v", err)
	}
	record, _, err := e.loadControlRecord(context.Background(), old.resourceID())
	if err != nil || record.Resource.Pending {
		t.Fatalf("definite pre-write rejection left unknown IO: %#v %v", record.Resource, err)
	}
	reopened := reopenResourceEngine(t, peer, resources)
	report, err = reopened.RecoverResources(context.Background())
	if err != nil || !slices.Contains(report.Deleted, original.ActiveIndex) {
		t.Fatalf("unprotected superseded generation was not deleted: %#v %v", report, err)
	}
	requireGenerationStatus(t, reopened, original.ActiveIndex, http.StatusNotFound)
	record, _, err = reopened.loadControlRecord(context.Background(), old.resourceID())
	if err != nil || record.Resource.Phase != resourceDeleted || record.Resource.SupersededBy == "" || record.Resource.QueryLeases != 1 {
		t.Fatalf("deletion lost its durable proof: %#v %v", record.Resource, err)
	}
	if err := old.resourceIO(func() error { return e.createGeneration(original.ActiveIndex) }); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("old builder recreated a deleted generation: %v", err)
	}
	requireGenerationStatus(t, reopened, original.ActiveIndex, http.StatusNotFound)
}

func TestLiveOpenSearchHistoricalGCRespectsPublicationAndOrdinaryContinuationLeases(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	docs := publicationRecoveryDocs()
	second := docs[0]
	second.ObjectID = "article/two"
	docs = append(docs, second)
	if err := e.Rebuild(docs, ports.Meta{Basis: "fixed"}); err != nil {
		t.Fatal(err)
	}
	meta, err := e.LoadMeta()
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := e.PrepareResource(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}
	proof := []byte("retained-dataset")
	if err := ticket.Admit(context.Background(), proof); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Confirm(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := ports.RetrieveRequest{Search: retrieval.SearchOf(retrieval.SearchMATCH("runbook")), Spec: retrieval.AccessSpec{Repository: "logical", Commit: "fixed", Fields: []retrieval.AccessField{{FieldRef: retrieval.FieldRef{Schema: "schema/article", Path: "body"}, Type: "string", Access: []reader.AccessHint{reader.HintText}}}}}
	request.Search.Limit = 1
	page, err := e.RetrieveContext(context.Background(), request)
	if err != nil || page.Continuation == "" {
		t.Fatalf("ordinary ROOT query has no continuation: %#v %v", page, err)
	}
	state, err := decodePITContinuation(page.Continuation)
	if err != nil || state.Lease == "" {
		t.Fatalf("ordinary continuation lacks durable protection: %#v %v", state, err)
	}
	original, _, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.BindPublicationContinuation(context.Background(), rootQueryEvidence(original), page.Continuation); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("ordinary ROOT continuation became an immutable publication view: %v", err)
	}
	if err := e.Rebuild(nil, ports.Meta{Basis: "replacement"}); err != nil {
		t.Fatal(err)
	}
	peer := reopenResourceEngine(t, e, resources)
	report, err := peer.RecoverResources(context.Background())
	if err != nil || !slices.Contains(report.Protected, original.ActiveIndex) {
		t.Fatalf("publication was not retained: %#v %v", report, err)
	}
	released, err := peer.ReconcilePublicationReferences(context.Background(), func(context.Context, []byte) (bool, error) { return false, nil })
	if err != nil || len(released.Released) != 1 {
		t.Fatalf("publication release: %#v %v", released, err)
	}
	report, err = peer.RecoverResources(context.Background())
	if err != nil || !slices.Contains(report.Protected, original.ActiveIndex) {
		t.Fatalf("ROOT continuation was discarded with publication: %#v %v", report, err)
	}
	session := ticket.(*preparedResource).session
	if err := session.changeResource(func(resource *generationResource) error {
		for i := range resource.Claims {
			if resource.Claims[i].ID == state.Lease {
				if resource.Claims[i].Phase != claimRootContinuation {
					t.Fatal("ordinary continuation borrowed publication lane")
				}
				resource.Claims[i].Until = time.Now().Add(-time.Second).UnixMilli()
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	report, err = peer.RecoverResources(context.Background())
	if err != nil || !slices.Contains(report.Deleted, original.ActiveIndex) {
		t.Fatalf("last bounded protection did not permit deletion: %#v %v", report, err)
	}
	requireGenerationStatus(t, peer, original.ActiveIndex, http.StatusNotFound)
}

type uncertainHistoricalDelete struct {
	next       http.RoundTripper
	physical   string
	rootPath   string
	failDelete bool
	hideRoot   bool
}

func (p *uncertainHistoricalDelete) RoundTrip(request *http.Request) (*http.Response, error) {
	if p.hideRoot && request.Method == http.MethodGet && request.URL.Path == p.rootPath {
		return nil, errors.New("ROOT unavailable")
	}
	response, err := p.next.RoundTrip(request)
	if p.failDelete && request.Method == http.MethodDelete && request.URL.Path == "/"+p.physical && err == nil && response.StatusCode < 300 {
		p.failDelete = false
		_ = response.Body.Close()
		return nil, errors.New("lost deletion acknowledgement")
	}
	return response, err
}

func TestLiveOpenSearchHistoricalDeletionResumesOnlyWithKnownRootAndRetainsTombstone(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "before"}); err != nil {
		t.Fatal(err)
	}
	original, _, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Rebuild(nil, ports.Meta{Basis: "after"}); err != nil {
		t.Fatal(err)
	}
	transport := &uncertainHistoricalDelete{next: resources, physical: original.ActiveIndex, rootPath: "/" + e.controlIndexPath() + "/_doc/" + e.controlID, failDelete: true}
	e.http.Transport = transport
	transport.hideRoot = true
	unknown, err := e.RecoverResources(context.Background())
	if err != nil || !slices.Contains(unknown.Retained, original.ActiveIndex) || slices.Contains(unknown.Deleted, original.ActiveIndex) {
		t.Fatalf("unknown ROOT permitted deletion admission: %#v %v", unknown, err)
	}
	retainedSession := &rebuildSession{engine: e, generation: original.Generation, physicalIndex: original.ActiveIndex}
	unchanged, _, err := e.loadControlRecord(context.Background(), retainedSession.resourceID())
	if err != nil || unchanged.Resource.Phase != resourceAdopted || unchanged.Resource.SupersededBy != "" {
		t.Fatalf("unknown ROOT changed resource authority: %#v %v", unchanged.Resource, err)
	}
	requireGenerationStatus(t, e, original.ActiveIndex, http.StatusOK)
	transport.hideRoot = false
	if _, err := e.RecoverResources(context.Background()); err == nil {
		t.Fatal("lost delete response claimed completion")
	}
	session := &rebuildSession{engine: e, generation: original.Generation, physicalIndex: original.ActiveIndex}
	record, _, err := e.loadControlRecord(context.Background(), session.resourceID())
	if err != nil || record.Resource.Phase != resourceDeleting || record.Resource.SupersededBy == "" {
		t.Fatalf("lost deletion authority: %#v %v", record.Resource, err)
	}
	transport.hideRoot = true
	report, err := e.RecoverResources(context.Background())
	if err != nil || !slices.Contains(report.Retained, original.ActiveIndex) || slices.Contains(report.Deleted, original.ActiveIndex) {
		t.Fatalf("unknown ROOT converted lost response to completion: %#v %v", report, err)
	}
	reopened := reopenResourceEngine(t, e, resources)
	report, err = reopened.RecoverResources(context.Background())
	if err != nil || !slices.Contains(report.Deleted, original.ActiveIndex) {
		t.Fatalf("restart did not finish known deletion: %#v %v", report, err)
	}
	requireGenerationStatus(t, reopened, original.ActiveIndex, http.StatusNotFound)
	record, _, err = reopened.loadControlRecord(context.Background(), session.resourceID())
	if err != nil || record.Resource.Phase != resourceDeleted || record.Resource.SupersededBy == "" {
		t.Fatalf("restart lost deletion fence: %#v %v", record.Resource, err)
	}
}

func TestLiveOpenSearchHistoricalGCCannotUpgradeLegacyReaderCoverageByTouchingLedger(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "legacy"}); err != nil {
		t.Fatal(err)
	}
	old, _, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	session := &rebuildSession{engine: e, generation: old.Generation, physicalIndex: old.ActiveIndex}
	record, version, err := e.loadControlRecord(context.Background(), session.resourceID())
	if err != nil {
		t.Fatal(err)
	}
	record.Resource.Version = 4
	record.Resource.QueryLeases = 0
	if _, err := e.putControlRecord(session.resourceID(), record, version); err != nil {
		t.Fatal(err)
	}
	if err := session.changeResource(func(*generationResource) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := e.Rebuild(nil, ports.Meta{Basis: "current"}); err != nil {
		t.Fatal(err)
	}
	peer := reopenResourceEngine(t, e, resources)
	report, err := peer.RecoverResources(context.Background())
	if err != nil || !slices.Contains(report.Retained, old.ActiveIndex) || slices.Contains(report.Deleted, old.ActiveIndex) {
		t.Fatalf("codec promotion fabricated query coverage: %#v %v", report, err)
	}
	record, _, err = peer.loadControlRecord(context.Background(), session.resourceID())
	if err != nil || record.Resource.Version != resourceRecordVersion || record.Resource.QueryLeases != 0 || record.Resource.Phase != resourceAdopted {
		t.Fatalf("legacy ownership was rewritten: %#v %v", record.Resource, err)
	}
	requireGenerationStatus(t, peer, old.ActiveIndex, http.StatusOK)
}

func TestLiveOpenSearchRootQueryContextCannotBorrowAnotherEngineProtection(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "fixed"}); err != nil {
		t.Fatal(err)
	}
	root, version, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	ctx, closeQuery, err := e.protectRootQuery(context.Background(), root, version, "")
	if err != nil {
		t.Fatal(err)
	}
	closeQuery()
	peer := reopenResourceEngine(t, e, resources)
	page, err := peer.RetrieveContext(ctx, ports.RetrieveRequest{Search: retrieval.SearchOf(retrieval.SearchMATCH("runbook")), Spec: retrieval.AccessSpec{Repository: "logical", Commit: "fixed", Fields: []retrieval.AccessField{{FieldRef: retrieval.FieldRef{Schema: "schema/article", Path: "body"}, Type: "string", Access: []reader.AccessHint{reader.HintText}}}}})
	if err != nil || len(page.Candidates) != 1 {
		t.Fatalf("peer borrowed a closed lease from another concrete owner: %#v %v", page, err)
	}
	record, _, err := peer.loadControlRecord(context.Background(), (&rebuildSession{engine: peer, generation: root.Generation}).resourceID())
	if err != nil || len(record.Resource.Claims) != 0 {
		t.Fatalf("peer failed to release its own request protection: %#v %v", record.Resource, err)
	}
}

func TestLiveOpenSearchHistoricalDeletionIsIdempotentAcrossConcurrentCollectors(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "before"}); err != nil {
		t.Fatal(err)
	}
	original, _, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Rebuild(nil, ports.Meta{Basis: "after"}); err != nil {
		t.Fatal(err)
	}
	peer := reopenResourceEngine(t, e, resources)
	results := make(chan error, 2)
	for _, owner := range []*openSearchEngine{e, peer} {
		go func(owner *openSearchEngine) {
			session := &rebuildSession{engine: owner, generation: original.Generation, physicalIndex: original.ActiveIndex}
			results <- session.discardHistoricalResource()
		}(owner)
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent deletion was not idempotent: %v", err)
		}
	}
	requireGenerationStatus(t, peer, original.ActiveIndex, http.StatusNotFound)
	record, _, err := peer.loadControlRecord(context.Background(), (&rebuildSession{engine: peer, generation: original.Generation}).resourceID())
	if err != nil || record.Resource.Phase != resourceDeleted || record.Resource.SupersededBy == "" {
		t.Fatalf("concurrent collectors lost deletion authority: %#v %v", record.Resource, err)
	}
}

func TestLiveOpenSearchResidualAnalysisKeepsRootResourceUntilCompletion(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	if err := e.Rebuild(publicationRecoveryDocs(), ports.Meta{Basis: "original"}); err != nil {
		t.Fatal(err)
	}
	root, _, err := e.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	pause := &pausedRootSearch{next: resources, path: "/" + root.ActiveIndex + "/_analyze", arrived: make(chan struct{}), resume: make(chan struct{})}
	e.http.Transport = pause
	done := make(chan error, 1)
	resumed, joined := false, false
	t.Cleanup(func() {
		if !resumed {
			close(pause.resume)
		}
		if !joined {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("analysis did not exit")
			}
		}
		e.http.Transport = resources
	})
	go func() {
		matched, err := e.VerifyResidualMatchContext(context.Background(), "runbook guide", "runbook", retrieval.MatchAllTerms)
		if err == nil && !matched {
			err = errors.New("published analyzer lost supplied-text match")
		}
		done <- err
	}()
	select {
	case <-pause.arrived:
	case err := <-done:
		joined = true
		t.Fatalf("analysis did not reach physical read: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("analysis admission timeout")
	}
	peer := reopenResourceEngine(t, e, resources)
	if err := peer.Rebuild(nil, ports.Meta{Basis: "replacement"}); err != nil {
		t.Fatal(err)
	}
	report, err := peer.RecoverResources(context.Background())
	if err != nil || !slices.Contains(report.Protected, root.ActiveIndex) || slices.Contains(report.Deleted, root.ActiveIndex) {
		t.Fatalf("analysis was not a physical consumer: %#v %v", report, err)
	}
	requireGenerationStatus(t, peer, root.ActiveIndex, http.StatusOK)
	close(pause.resume)
	resumed = true
	analysisErr := <-done
	joined = true
	if analysisErr != nil {
		t.Fatal(analysisErr)
	}
	report, err = peer.RecoverResources(context.Background())
	if err != nil || !slices.Contains(report.Deleted, root.ActiveIndex) {
		t.Fatalf("analysis protection leaked after completion: %#v %v", report, err)
	}
	requireGenerationStatus(t, peer, root.ActiveIndex, http.StatusNotFound)
}
