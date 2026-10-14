package service

import (
	"context"
	"reflect"
	"testing"

	index "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	sourcebridge "kc/liveaccess/discovery"
	objectmodel "kc/object"
	"kc/snapshot"
)

type noticeIdentityLookup struct {
	name     string
	requests []liveaccess.StateLookupRequest
}

func (l *noticeIdentityLookup) LookupState(_ context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	l.requests = append(l.requests, request)
	return liveaccess.StateObservation{
		Value: map[string]any{"status": l.name},
		Basis: objectmodel.ObservationBasis{
			BindingGeneration: "generation-1", Consistency: objectmodel.ObservationRepeatable,
			SourceRevision: "revision-1", ObservedAt: "2026-10-13T00:00:00Z",
		},
	}, nil
}

type rejectingNoticeController struct {
	*controllerPort
	rejection error
	notified  bool
}

func (c *rejectingNoticeController) Notify(notice index.ChangeNotice) error {
	if err := index.ValidateChangeNotice(notice); err != nil {
		return err
	}
	c.notified = true
	return c.rejection
}

func noticeBackgroundFixture(t *testing.T) (objectmodel.Repository, kernel.CommitID) {
	t.Helper()
	setup := testkit.NewSetup(t, "kr://acme/public/notice-source")
	commit, err := setup.Repo.ApplyKnowledgeCommit(objectmodel.ChangeSet{
		TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: setup.RootCommitID, ExpectedTargetCommit: setup.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "Job:notice"}, Value: map[string]any{"name": "notice"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{
				"entity": "Job", "aspect": "runtime", "origin": "https://scheduler.example",
				"fields": map[string]any{"status": map[string]any{"type": "string", "access": []any{"filter"}}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return setup.Repo, commit
}

func noticeRequestContext(principal string) objectmodel.RequestContext {
	return objectmodel.RequestContext{
		Identity:  kernel.IdentityContext{Principal: principal},
		Trace:     kernel.TraceContext{TraceID: principal + "-trace"},
		RequestID: principal + "-request",
	}
}

func assertNoticeBackgroundSource(t *testing.T, checkpoint string, controller *controllerPort, background *noticeIdentityLookup, request objectmodel.RequestContext, repo objectmodel.Repository, commit kernel.CommitID) {
	t.Helper()
	// Exercise the retained source through the real observation bridge, so this
	// verifies the runtime and execution identity used by later background work.
	value, err := repo.Read("Job:notice", commit)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := (sourcebridge.Source{Lookup: controller.state, Request: controller.request}).ObserveRepositoryValue(context.Background(), repo, value)
	if err != nil {
		t.Fatalf("background observation %s: %v", checkpoint, err)
	}
	active, ok := controller.state.(*noticeIdentityLookup)
	if !ok || len(active.requests) == 0 {
		t.Fatalf("background observation %s did not reach the retained source", checkpoint)
	}
	actual := active.requests[len(active.requests)-1]
	actualRequest := objectmodel.RequestContext{Identity: actual.Identity, Trace: actual.Trace, RequestID: actual.RequestID}
	if active != background || !reflect.DeepEqual(actualRequest, request) {
		t.Errorf("background Source changed %s: runtime=%q request=%#v; want runtime=%q request=%#v", checkpoint, active.name, actualRequest, background.name, request)
	}
	valueWithRuntime, ok := observed.Value.(map[string]any)
	if !ok {
		t.Fatalf("background observation %s lost its object shape: %#v", checkpoint, observed.Value)
	}
	if runtime, ok := valueWithRuntime["runtime"].(map[string]any); !ok || runtime["status"] != background.name {
		t.Errorf("background observation %s used caller source: %#v", checkpoint, observed.Value)
	}
}

func TestProjectionNoticeExecutorPreservesBackgroundSourceAcrossCallers(t *testing.T) {
	repo, commit := noticeBackgroundFixture(t)
	background := &noticeIdentityLookup{name: "background"}
	backgroundRequest := noticeRequestContext("projection-worker")
	controller := &controllerPort{state: background, request: backgroundRequest}
	executor := ProjectionNoticeExecutor{
		Repositories: operationsLookup{repo: repo},
		Projection:   &projectionPort{revision: "state-ready", ready: true},
		Controller:   controller,
	}
	assertNoticeBackgroundSource(t, "before notice", controller, background, backgroundRequest, repo, commit)
	for _, caller := range []string{"caller-a", "caller-b"} {
		result, err := executor.Execute(context.Background(), ProjectionNoticeRequest{
			Notice: index.ChangeNotice{Repository: repo.ID(), Ref: snapshot.DefaultRef},
			State:  &noticeIdentityLookup{name: caller}, Request: noticeRequestContext(caller),
		})
		if err != nil {
			t.Fatalf("valid %s notice failed before background isolation check: %v", caller, err)
		}
		if result.Repository != repo.ID() || result.BasisCommit != commit || result.Revision != "state-ready" {
			t.Fatalf("valid %s notice did not return its ready projection: %#v", caller, result)
		}
		assertNoticeBackgroundSource(t, "after "+caller+" notice", controller, background, backgroundRequest, repo, commit)
	}
}

func TestProjectionNoticeExecutorPreservesBackgroundSourceWhenNoticeRejected(t *testing.T) {
	repo, commit := noticeBackgroundFixture(t)
	background := &noticeIdentityLookup{name: "background"}
	backgroundRequest := noticeRequestContext("projection-worker")
	controller := &controllerPort{state: background, request: backgroundRequest}
	rejection := kernel.Fail(kernel.ErrForbidden, "notice rejected by control policy")
	rejecting := &rejectingNoticeController{controllerPort: controller, rejection: rejection}
	executor := ProjectionNoticeExecutor{
		Repositories: operationsLookup{repo: repo},
		Projection:   &projectionPort{revision: "state-ready", ready: true},
		Controller:   rejecting,
	}
	assertNoticeBackgroundSource(t, "before rejected notice", controller, background, backgroundRequest, repo, commit)
	_, err := executor.Execute(context.Background(), ProjectionNoticeRequest{
		Notice: index.ChangeNotice{Repository: repo.ID(), Ref: snapshot.DefaultRef},
		State:  &noticeIdentityLookup{name: "rejected-caller"}, Request: noticeRequestContext("rejected-caller"),
	})
	if !rejecting.notified || kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("notice did not reach its intended control rejection: %v", err)
	}
	assertNoticeBackgroundSource(t, "after rejected notice", controller, background, backgroundRequest, repo, commit)
}
