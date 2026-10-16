package service

import (
	"context"
	sourcebridge "kc/liveaccess/discovery"

	retrieval "kc/discovery"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/kernel"
	objectserving "kc/liveaccess"
	objectmodel "kc/object"
	"kc/object/reader"
)

type ProjectionDescriber = consumerplugin.ProjectionDescriber
type ProjectionSynchronizer = consumerplugin.ProjectionSynchronizer
type ProjectionController = consumerplugin.ProjectionController
type ProjectionStateView = consumerplugin.ProjectionStateView

type ProjectionExecutionObserver func() func()

type ProjectionStateRequest func() (objectmodel.RequestContext, error)

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
	repo, err := e.Repositories.Require(request.Repository, kernel.ErrObjectRefUnresolved)
	if err != nil {
		return index.IndexDescriptor{}, err
	}
	return e.Projection.DescribeAt(repo, request.Commit)
}

type ProjectionSyncRequest struct {
	Repository   kernel.RepositoryID
	Commit       kernel.CommitID
	State        objectserving.StateLookup
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
	repo, err := e.Repositories.Require(request.Repository, kernel.ErrObjectRefUnresolved)
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
		if err := catchUpRepository(ctx, e.Controller, request.Repository); err != nil {
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
	var stateRequest objectmodel.RequestContext
	if request.StateRequest != nil {
		stateRequest, err = request.StateRequest()
		if err != nil {
			return ProjectionSyncResult{}, err
		}
	}
	stateSync, err := e.Projection.RefreshState(ctx, repo, request.Commit, sourcebridge.Source{Lookup: request.State, Request: stateRequest})
	if err != nil {
		return ProjectionSyncResult{}, err
	}
	result.State = &stateSync
	return result, nil
}

type ProjectionNoticeRequest struct {
	Notice  index.ChangeNotice
	State   objectserving.StateLookup
	Request objectmodel.RequestContext
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

type projectionNoticeApplier interface {
	ApplyNotice(context.Context, index.ChangeNotice, index.ObservationSource) (index.StateSync, error)
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
	if request.State != nil {
		if controller, ok := e.Controller.(projectionNoticeApplier); ok {
			syncResult, err := controller.ApplyNotice(ctx, request.Notice, sourcebridge.Source{Lookup: request.State, Request: request.Request})
			if err != nil {
				return ProjectionNoticeResult{}, err
			}
			return ProjectionNoticeResult{Repository: syncResult.Repository, BasisCommit: syncResult.BasisCommit, Revision: syncResult.Revision}, nil
		}
	}
	// A notice invalidates the configured background projection. Caller runtime
	// and identity must not replace the worker's retained observation source.
	if err := e.Controller.Notify(request.Notice); err != nil {
		return ProjectionNoticeResult{}, err
	}
	if err := catchUpRepository(ctx, e.Controller, request.Notice.Repository); err != nil {
		return ProjectionNoticeResult{}, err
	}
	repo, err := e.Repositories.Require(request.Notice.Repository, kernel.ErrObjectRefUnresolved)
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

func catchUpRepository(ctx context.Context, controller ProjectionController, repo kernel.RepositoryID) error {
	if selected, ok := controller.(interface {
		CatchUpRepository(context.Context, kernel.RepositoryID) error
	}); ok {
		return selected.CatchUpRepository(ctx, repo)
	}
	return controller.CatchUp(ctx)
}

type AccessDescribeRequest struct {
	Pin reader.DatasetPin
}

type AccessDescribeExecutor struct {
	Repositories reader.MemberLookup
	Plugins      *consumerplugin.IndexPlugins
}

type AccessOverview struct {
	retrieval.AccessPlan
	Plugins []consumerplugin.SourcePlugin `json:"plugins"`
}

func (e AccessDescribeExecutor) Execute(_ context.Context, request AccessDescribeRequest) (AccessOverview, error) {
	if e.Repositories == nil {
		return AccessOverview{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"access describe service is unavailable")
	}
	// Describe compiles the declarations each member selects with the same
	// instance manifest check search admits by, without touching an engine;
	// without an installed plugin the plan stays structure-only.
	var contracts retrieval.AccessContractProvider
	if e.Plugins != nil {
		contracts = func(repo objectmodel.Repository, commit kernel.CommitID) (retrieval.AccessSpec, error) {
			if e.Plugins.Configured() {
				var err error
				repo, err = e.Plugins.DatasetMember(e.Repositories, request.Pin, repo.ID())
				if err != nil {
					return retrieval.AccessSpec{}, err
				}
			}
			spec, err := e.Plugins.DeclaredAccessSpecAt(repo, commit)
			if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
				return spec, err
			}
			// No instance interprets this member: report structure only.
			report, err := reader.DescribeRepoSchema(repo, commit, "")
			if err != nil {
				return retrieval.AccessSpec{}, err
			}
			return retrieval.AccessSpecFromReport(report), nil
		}
	}
	plan, err := retrieval.PlanAccess(e.Repositories, request.Pin, contracts)
	if err != nil {
		return AccessOverview{}, err
	}
	plugins, err := e.Plugins.Overview(request.Pin)
	if err != nil {
		return AccessOverview{}, err
	}
	return AccessOverview{AccessPlan: plan, Plugins: plugins}, nil
}
