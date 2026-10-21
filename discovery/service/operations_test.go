package service

import (
	"context"
	sourcebridge "kc/liveaccess/discovery"
	"reflect"
	"testing"

	index "kc/discovery/projection"
	"kc/kernel"
	objectserving "kc/liveaccess"
	objectmodel "kc/object"
	"kc/object/reader"
)

type operationsRepository struct {
	objectmodel.Repository
	id   kernel.RepositoryID
	head kernel.CommitID
}

func (r *operationsRepository) ID() kernel.RepositoryID { return r.id }
func (r *operationsRepository) Head(string) (kernel.CommitID, error) {
	return r.head, nil
}
func (r *operationsRepository) HasCommit(commit kernel.CommitID) bool { return commit == r.head }
func (r *operationsRepository) SchemaObjectIDs(kernel.CommitID) ([]objectmodel.ObjectID, error) {
	return []objectmodel.ObjectID{}, nil
}

type operationsLookup struct{ repo objectmodel.Repository }

func (l operationsLookup) Require(id kernel.RepositoryID, _ kernel.ErrorCode) (objectmodel.Repository, error) {
	if l.repo == nil || id != l.repo.ID() {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "missing repository")
	}
	return l.repo, nil
}

type projectionPort struct {
	calls    []string
	revision string
	ready    bool
}

func (p *projectionPort) DescribeAt(objectmodel.Repository, kernel.CommitID) (index.IndexDescriptor, error) {
	p.calls = append(p.calls, "describe")
	return index.IndexDescriptor{BasisCommit: "fixed"}, nil
}
func (p *projectionPort) EnsureAt(context.Context, objectmodel.Repository, kernel.CommitID) (index.IndexSync, error) {
	p.calls = append(p.calls, "ensure-at")
	return index.IndexSync{}, nil
}
func (p *projectionPort) Ensure(context.Context, objectmodel.Repository, kernel.CommitID) (index.IndexSync, error) {
	p.calls = append(p.calls, "ensure")
	return index.IndexSync{BasisCommit: "fixed"}, nil
}
func (p *projectionPort) RefreshState(context.Context, objectmodel.Repository, kernel.CommitID, index.ObservationSource) (index.StateSync, error) {
	p.calls = append(p.calls, "refresh-state")
	return index.StateSync{Revision: "state-1"}, nil
}
func (p *projectionPort) StateView(kernel.RepositoryID, kernel.CommitID) (string, bool) {
	p.calls = append(p.calls, "state-view")
	return p.revision, p.ready
}

type controllerPort struct {
	calls   []string
	state   objectserving.StateLookup
	request objectmodel.RequestContext
}

func (p *controllerPort) Desire(kernel.RepositoryID, kernel.CommitID) error {
	p.calls = append(p.calls, "desire")
	return nil
}
func (p *controllerPort) CatchUp(context.Context) error {
	p.calls = append(p.calls, "catch-up")
	return nil
}
func (p *controllerPort) Notify(index.ChangeNotice) error {
	p.calls = append(p.calls, "notify")
	return nil
}
func (p *controllerPort) SetObservationSource(source index.ObservationSource) {
	p.calls = append(p.calls, "set-state")
	typed := source.(sourcebridge.Source)
	p.state = typed.Lookup
	p.request = typed.Request
}

type stateLookupPort struct{}

func (stateLookupPort) LookupState(context.Context, objectserving.StateLookupRequest) (objectserving.StateObservation, error) {
	return objectserving.StateObservation{}, nil
}

func TestProjectionDescribeExecutorUsesPinnedRepository(t *testing.T) {
	repo := &operationsRepository{id: "kr://app/operations", head: "fixed"}
	projection := &projectionPort{}
	result, err := (ProjectionDescribeExecutor{
		Repositories: operationsLookup{repo: repo},
		Projection:   projection,
	}).Execute(context.Background(), ProjectionDescribeRequest{Repository: repo.ID(), Commit: "fixed"})
	if err != nil {
		t.Fatal(err)
	}
	if result.BasisCommit != "fixed" || !reflect.DeepEqual(projection.calls, []string{"describe"}) {
		t.Fatalf("describe result/calls = %#v / %v", result, projection.calls)
	}
}

func TestProjectionSyncExecutorPreservesSnapshotAndDynamicSequences(t *testing.T) {
	repo := &operationsRepository{id: "kr://app/operations", head: "fixed"}
	projection := &projectionPort{}
	controller := &controllerPort{}
	executor := ProjectionSyncExecutor{
		Repositories: operationsLookup{repo: repo},
		Projection:   projection,
		Controller:   controller,
	}
	result, err := executor.Execute(context.Background(), ProjectionSyncRequest{
		Repository: repo.ID(), Commit: "fixed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != nil {
		t.Fatalf("snapshot-only sync returned state: %#v", result.State)
	}
	if !reflect.DeepEqual(projection.calls, []string{"ensure-at", "ensure"}) ||
		!reflect.DeepEqual(controller.calls, []string{"desire", "catch-up"}) {
		t.Fatalf("snapshot calls = projection %v, controller %v", projection.calls, controller.calls)
	}

	projection.calls, controller.calls = nil, nil
	result, err = executor.Execute(context.Background(), ProjectionSyncRequest{
		Repository: repo.ID(), Commit: "fixed", State: stateLookupPort{},
		StateRequest: func() (objectmodel.RequestContext, error) {
			if !reflect.DeepEqual(projection.calls, []string{"ensure-at", "ensure"}) {
				t.Fatalf("state request resolved before snapshot publication: %v", projection.calls)
			}
			return objectmodel.RequestContext{RequestID: "request-1"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State == nil || result.State.Revision != "state-1" {
		t.Fatalf("dynamic sync state = %#v", result.State)
	}
	if !reflect.DeepEqual(projection.calls, []string{"ensure-at", "ensure", "refresh-state"}) {
		t.Fatalf("dynamic projection calls = %v", projection.calls)
	}
}

func TestProjectionNoticeExecutorConfiguresRuntimeBeforeNotice(t *testing.T) {
	repo := &operationsRepository{id: "kr://app/operations", head: "head"}
	projection := &projectionPort{revision: "state-2", ready: true}
	controller := &controllerPort{}
	state := stateLookupPort{}
	request := objectmodel.RequestContext{RequestID: "request-1"}
	background := objectmodel.RequestContext{RequestID: "background"}
	controller.SetObservationSource(sourcebridge.Source{Lookup: state, Request: background})
	controller.calls = nil
	result, err := (ProjectionNoticeExecutor{
		Repositories: operationsLookup{repo: repo},
		Projection:   projection,
		Controller:   controller,
	}).Execute(context.Background(), ProjectionNoticeRequest{
		Notice: index.ChangeNotice{Repository: repo.ID()},
		State:  state, Request: request,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(controller.calls, []string{"notify", "catch-up"}) {
		t.Fatalf("controller calls = %v", controller.calls)
	}
	if controller.state == nil || controller.request.RequestID != background.RequestID {
		t.Fatalf("configured runtime was replaced by caller state: %#v", controller.request)
	}
	if result.Repository != repo.ID() || result.BasisCommit != "head" || result.Revision != "state-2" {
		t.Fatalf("notice result = %#v", result)
	}
}

func TestAccessDescribeExecutorPlansPinnedMembers(t *testing.T) {
	repo := &operationsRepository{id: "kr://app/operations", head: "fixed"}
	result, err := (AccessDescribeExecutor{
		Repositories: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			if id != repo.ID() {
				t.Fatalf("lookup id = %s", id)
			}
			return repo, nil
		},
	}).Execute(context.Background(), AccessDescribeRequest{Pin: reader.DatasetPin{
		SetID:    "workspace",
		Revision: 3,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{
			repo.ID(): "fixed",
		},
		Items: reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{repo.ID(): "fixed"}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.SetID != "workspace" || result.DefinitionRevision != 3 || len(result.Specs) != 1 {
		t.Fatalf("access plan = %#v", result)
	}
}
