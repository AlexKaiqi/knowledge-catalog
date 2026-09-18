package knowledgeapp

import (
	"context"
	"reflect"
	"testing"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
)

type operationsRepository struct {
	knowledge.Repository
	id   kernel.RepositoryID
	head kernel.CommitID
}

func (r *operationsRepository) ID() kernel.RepositoryID { return r.id }
func (r *operationsRepository) Head(string) (kernel.CommitID, error) {
	return r.head, nil
}
func (r *operationsRepository) HasCommit(commit kernel.CommitID) bool { return commit == r.head }
func (r *operationsRepository) SchemaObjectIDs(kernel.CommitID) ([]knowledge.ObjectID, error) {
	return []knowledge.ObjectID{}, nil
}

type operationsLookup struct{ repo knowledge.Repository }

func (l operationsLookup) Require(id kernel.RepositoryID, _ kernel.ErrorCode) (knowledge.Repository, error) {
	if l.repo == nil || id != l.repo.ID() {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "missing repository")
	}
	return l.repo, nil
}

type projectionPort struct {
	calls    []string
	revision string
	ready    bool
}

func (p *projectionPort) DescribeAt(knowledge.Repository, kernel.CommitID) (index.IndexDescriptor, error) {
	p.calls = append(p.calls, "describe")
	return index.IndexDescriptor{BasisCommit: "fixed"}, nil
}
func (p *projectionPort) EnsureAt(knowledge.Repository, kernel.CommitID) (index.IndexSync, error) {
	p.calls = append(p.calls, "ensure-at")
	return index.IndexSync{}, nil
}
func (p *projectionPort) Ensure(knowledge.Repository, kernel.CommitID) (index.IndexSync, error) {
	p.calls = append(p.calls, "ensure")
	return index.IndexSync{BasisCommit: "fixed"}, nil
}
func (p *projectionPort) RefreshState(context.Context, knowledge.Repository, kernel.CommitID, knowledgeserving.StateLookup, knowledgeserving.RequestContext) (index.StateSync, error) {
	p.calls = append(p.calls, "refresh-state")
	return index.StateSync{Revision: "state-1"}, nil
}
func (p *projectionPort) StateView(kernel.RepositoryID, kernel.CommitID) (string, bool) {
	p.calls = append(p.calls, "state-view")
	return p.revision, p.ready
}

type controllerPort struct {
	calls   []string
	state   knowledgeserving.StateLookup
	request knowledgeserving.RequestContext
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
func (p *controllerPort) SetStateLookup(state knowledgeserving.StateLookup) {
	p.calls = append(p.calls, "set-state")
	p.state = state
}
func (p *controllerPort) SetRequestContext(request knowledgeserving.RequestContext) {
	p.calls = append(p.calls, "set-request")
	p.request = request
}

type stateLookupPort struct{}

func (stateLookupPort) LookupState(context.Context, knowledgeserving.StateLookupRequest) (knowledgeserving.StateObservation, error) {
	return knowledgeserving.StateObservation{}, nil
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
		StateRequest: func() (knowledgeserving.RequestContext, error) {
			if !reflect.DeepEqual(projection.calls, []string{"ensure-at", "ensure"}) {
				t.Fatalf("state request resolved before snapshot publication: %v", projection.calls)
			}
			return knowledgeserving.RequestContext{RequestID: "request-1"}, nil
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
	request := knowledgeserving.RequestContext{RequestID: "request-1"}
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
	if !reflect.DeepEqual(controller.calls, []string{"set-state", "set-request", "notify", "catch-up"}) {
		t.Fatalf("controller calls = %v", controller.calls)
	}
	if controller.state == nil || controller.request.RequestID != request.RequestID {
		t.Fatalf("runtime was not configured: %#v", controller.request)
	}
	if result.Repository != repo.ID() || result.BasisCommit != "head" || result.Revision != "state-2" {
		t.Fatalf("notice result = %#v", result)
	}
}

func TestAccessDescribeExecutorPlansPinnedMembers(t *testing.T) {
	repo := &operationsRepository{id: "kr://app/operations", head: "fixed"}
	result, err := (AccessDescribeExecutor{
		Repositories: func(id kernel.RepositoryID) (knowledge.Repository, error) {
			if id != repo.ID() {
				t.Fatalf("lookup id = %s", id)
			}
			return repo, nil
		},
	}).Execute(context.Background(), AccessDescribeRequest{Pin: reader.KnowledgeSetPin{
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
