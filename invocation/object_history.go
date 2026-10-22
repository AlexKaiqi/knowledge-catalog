package invocation

import (
	"context"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	objectservice "kc/object/service"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

// VerbLog scans fixed source history with durable, reauthorized continuations.
func VerbLog(cx *Invocation) (any, error) {
	if cx.Flag("aspect") != "" || cx.Flag("member") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "log is object history; do not pass --aspect or --member")
	}
	raw, err := cx.Require("object")
	if err != nil {
		return nil, err
	}
	id := objectmodel.ObjectID(raw)
	if err := objectmodel.ValidateObjectID(id); err != nil {
		return nil, err
	}
	limit, err := PageLimit(cx.Flags, DefaultHistoryLimit, MaxHistoryPageSize)
	if err != nil {
		return nil, err
	}
	parent := cx.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, objectmodel.HistoryScanRequestTimeout)
	defer cancel()
	invocation := *cx
	invocation.Context = ctx
	cx = &invocation
	if ServingWorkspace(cx.Flags) {
		if err := bindHistoryReadContext(cx); err != nil {
			return nil, err
		}
		serving, _, err := OpenCompleteServing(cx, string(id))
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pinned := reader.Open(func(repository kernel.RepositoryID) (objectmodel.Repository, error) {
			repo, err := serving.Member(repository)
			if err != nil {
				return nil, err
			}
			return objectmodel.BindContext(ctx, repo), nil
		}, serving.Pin())
		basis, err := pinned.LocalReadBasis(objectmodel.Reference{Object: id}, "")
		if err != nil {
			return nil, err
		}
		return executeFixedObjectHistory(cx, id, limit, basis, "")
	}
	repository, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	source, err := cx.WS.Reader.Require(kernel.RepositoryID(repository), kernel.ErrObjectRefUnresolved)
	if err != nil {
		return nil, err
	}
	source = objectmodel.BindContext(ctx, source)
	commit := kernel.CommitID(cx.Flag("commit"))
	if commit == "" {
		commit, err = source.Head(snapshot.RefOrDefault(cx.Flag("ref")))
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cx.Flags[ResolvedCommitFlag] = string(commit)
	pin := map[kernel.RepositoryID]kernel.CommitID{source.ID(): commit}
	serving := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return source, nil }, reader.DatasetPin{Repositories: pin, Items: reader.WholeRepositoryItems(pin)})
	basis, err := serving.LocalReadBasis(objectmodel.Reference{Object: id}, "")
	if err != nil {
		return nil, err
	}
	return executeFixedObjectHistory(cx, id, limit, basis, snapshot.RefOrDefault(cx.Flag("ref")))
}

func executeFixedObjectHistory(cx *Invocation, id objectmodel.ObjectID, limit int, basis objectmodel.ReadBasis, ref string) (any, error) {
	// Interpretation versions do not participate in local source history. The
	// accepted publication's source coordinates and file selection still do.
	basis.Dependencies = nil
	basis.UnresolvedDependencies = nil
	if previous := cx.historyCheckpoint; previous != nil {
		if kernel.CanonicalDigest(previous.Basis) != kernel.CanonicalDigest(basis) || previous.Ref != ref {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "history source binding or accepted publication scope changed")
		}
		basis = previous.Basis
	}
	for _, source := range basis.Candidates {
		if err := cx.Context.Err(); err != nil {
			return nil, err
		}
		if err := authorizeRepositoryHistory(cx, source.Repository, id); err != nil {
			return nil, err
		}
	}
	serving, err := reader.OpenHistoryBasis(func(repository kernel.RepositoryID) (objectmodel.Repository, error) {
		repo, err := cx.WS.Reader.Require(repository, kernel.ErrObjectRefUnresolved)
		if err != nil {
			return nil, err
		}
		return objectmodel.BindContext(cx.Context, repo), nil
	}, basis)
	if err != nil {
		return nil, err
	}
	executor := objectservice.ObjectLogExecutor{Checkpoints: cx.WS.HistoryCheckpoints, ReadScan: func(ctx context.Context, repository kernel.RepositoryID, commit kernel.CommitID, query objectmodel.HistoryScanQuery) (objectmodel.HistoryScanPage, error) {
		repo, err := serving.Member(repository)
		if err != nil {
			return objectmodel.HistoryScanPage{}, err
		}
		scanner, ok := repo.(objectmodel.HistoryScanner)
		if !ok {
			return objectmodel.HistoryScanPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no bounded object history", repository)
		}
		return scanner.ScanHistory(ctx, id, commit, query)
	}}
	return executor.Execute(cx.Context, objectservice.ObjectLogRequest{Object: id, Basis: basis, Ref: ref, Limit: limit, Continuation: cx.Flag("continuation")})
}

func bindHistoryReadContext(cx *Invocation) error {
	if cx.historyContextBound || !ServingWorkspace(cx.Flags) {
		return nil
	}
	bound, ok := cx.WS.Datasets.(dataset.ReadContextBound)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset service has no bounded read context")
	}
	datasets, err := bound.ReadContext(cx.Context)
	if err != nil {
		return err
	}
	view := cx.WS.ReadView(cx.WS.Journal)
	view.Datasets = datasets
	cx.WS = view
	cx.historyContextBound = true
	return nil
}
