package knowledgeapp

import (
	"context"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
)

type ProjectionDescriber interface {
	DescribeAt(knowledge.Repository, kernel.CommitID) (index.IndexDescriptor, error)
}

type ProjectionSynchronizer interface {
	EnsureAt(knowledge.Repository, kernel.CommitID) (index.IndexSync, error)
	Ensure(knowledge.Repository, kernel.CommitID) (index.IndexSync, error)
	RefreshState(context.Context, knowledge.Repository, kernel.CommitID, knowledgeserving.StateLookup, knowledgeserving.RequestContext) (index.StateSync, error)
}

type ProjectionController interface {
	Desire(kernel.RepositoryID, kernel.CommitID) error
	CatchUp(context.Context) error
	Notify(index.ChangeNotice) error
	SetStateLookup(knowledgeserving.StateLookup)
	SetRequestContext(knowledgeserving.RequestContext)
}

type ProjectionStateView interface {
	StateView(kernel.RepositoryID, kernel.CommitID) (string, bool)
}

type ProjectionExecutionObserver func() func()

type ProjectionStateRequest func() (knowledgeserving.RequestContext, error)

type ProjectionDescribeRequest struct {
	Repository kernel.RepositoryID
	Commit     kernel.CommitID
}

type ProjectionDescribeExecutor struct {
	Repositories RepositoryLookup
	Projection   ProjectionDescriber
}

func (e ProjectionDescribeExecutor) Execute(_ context.Context, request ProjectionDescribeRequest) (index.IndexDescriptor, error) {
	if e.Repositories == nil || e.Projection == nil {
		return index.IndexDescriptor{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"projection describe service is unavailable")
	}
	repo, err := e.Repositories.Require(request.Repository, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return index.IndexDescriptor{}, err
	}
	return e.Projection.DescribeAt(repo, request.Commit)
}

type ProjectionSyncRequest struct {
	Repository   kernel.RepositoryID
	Commit       kernel.CommitID
	State        knowledgeserving.StateLookup
	StateRequest ProjectionStateRequest
}

type ProjectionSyncResult struct {
	Snapshot index.IndexSync
	State    *index.StateSync
}

type ProjectionSyncExecutor struct {
	Repositories RepositoryLookup
	Projection   ProjectionSynchronizer
	Controller   ProjectionController
	Observe      ProjectionExecutionObserver
}

func (e ProjectionSyncExecutor) Execute(ctx context.Context, request ProjectionSyncRequest) (ProjectionSyncResult, error) {
	if e.Repositories == nil || e.Projection == nil {
		return ProjectionSyncResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"projection sync service is unavailable")
	}
	repo, err := e.Repositories.Require(request.Repository, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return ProjectionSyncResult{}, err
	}
	if e.Observe != nil {
		if done := e.Observe(); done != nil {
			defer done()
		}
	}
	if _, err := e.Projection.EnsureAt(repo, request.Commit); err != nil {
		return ProjectionSyncResult{}, err
	}
	if e.Controller != nil {
		if err := e.Controller.Desire(request.Repository, request.Commit); err != nil {
			return ProjectionSyncResult{}, err
		}
		if err := e.Controller.CatchUp(ctx); err != nil {
			return ProjectionSyncResult{}, err
		}
	}
	snapshotSync, err := e.Projection.Ensure(repo, request.Commit)
	if err != nil {
		return ProjectionSyncResult{}, err
	}
	result := ProjectionSyncResult{Snapshot: snapshotSync}
	if request.State == nil {
		return result, nil
	}
	var stateRequest knowledgeserving.RequestContext
	if request.StateRequest != nil {
		stateRequest, err = request.StateRequest()
		if err != nil {
			return ProjectionSyncResult{}, err
		}
	}
	stateSync, err := e.Projection.RefreshState(ctx, repo, request.Commit, request.State, stateRequest)
	if err != nil {
		return ProjectionSyncResult{}, err
	}
	result.State = &stateSync
	return result, nil
}

type ProjectionNoticeRequest struct {
	Notice  index.ChangeNotice
	State   knowledgeserving.StateLookup
	Request knowledgeserving.RequestContext
}

type ProjectionNoticeResult struct {
	Repository  kernel.RepositoryID `json:"repository"`
	BasisCommit kernel.CommitID     `json:"basisCommit"`
	Revision    string              `json:"revision"`
}

type ProjectionNoticeExecutor struct {
	Repositories RepositoryLookup
	Projection   ProjectionStateView
	Controller   ProjectionController
}

func (e ProjectionNoticeExecutor) Execute(ctx context.Context, request ProjectionNoticeRequest) (ProjectionNoticeResult, error) {
	if e.Controller == nil {
		return ProjectionNoticeResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"projection controller is not configured")
	}
	if e.Repositories == nil || e.Projection == nil {
		return ProjectionNoticeResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"projection notice service is unavailable")
	}
	e.Controller.SetStateLookup(request.State)
	e.Controller.SetRequestContext(request.Request)
	if err := e.Controller.Notify(request.Notice); err != nil {
		return ProjectionNoticeResult{}, err
	}
	if err := e.Controller.CatchUp(ctx); err != nil {
		return ProjectionNoticeResult{}, err
	}
	repo, err := e.Repositories.Require(request.Notice.Repository, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return ProjectionNoticeResult{}, err
	}
	head, err := repo.Head(request.Notice.Ref)
	if err != nil {
		return ProjectionNoticeResult{}, err
	}
	revision, ok := e.Projection.StateView(request.Notice.Repository, head)
	if !ok {
		return ProjectionNoticeResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"State projection is not prepared")
	}
	return ProjectionNoticeResult{
		Repository: request.Notice.Repository, BasisCommit: head, Revision: revision,
	}, nil
}

type AccessDescribeRequest struct {
	Pin reader.KnowledgeSetPin
}

type AccessDescribeExecutor struct {
	Repositories reader.MemberLookup
}

func (e AccessDescribeExecutor) Execute(_ context.Context, request AccessDescribeRequest) (retrieval.AccessPlan, error) {
	if e.Repositories == nil {
		return retrieval.AccessPlan{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"access describe service is unavailable")
	}
	return retrieval.PlanAccess(e.Repositories, request.Pin)
}
