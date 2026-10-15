package service

import (
	"context"
	retrieval "kc/discovery"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"sort"
)

type RelationProjection = consumerplugin.RelationProjection

type DatasetRelationsExecutor struct {
	Serving      *reader.Serving
	Repositories RepositoryLookup
	Projection   RelationProjection
}

func (e DatasetRelationsExecutor) Execute(ctx context.Context, request retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	if e.Serving == nil || e.Repositories == nil || e.Projection == nil {
		return retrieval.RelationPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset relation services are incomplete")
	}
	if objectmodel.ValidateObjectID(request.Query.Endpoint) != nil || request.Limit < 0 || request.Limit > 1000 {
		return retrieval.RelationPage{}, kernel.Fail(kernel.ErrUsageInvalid, "relations require an endpoint object ID and a limit between 1 and 1000")
	}
	// The endpoint is an object ID; it need not be held in the Dataset. The
	// scope applies to the stored relations, checked per candidate below.
	pin := e.Serving.Pin()
	ctx, cancel := index.WithSearchBudget(ctx, index.SearchBudget{})
	defer cancel()
	ctx = index.WithCandidateScope(ctx, kernel.CanonicalDigest(pin.Items), func(id kernel.RepositoryID, at kernel.CommitID, object objectmodel.ObjectID) (bool, error) {
		if pin.Repositories[id] != at {
			return false, kernel.Fail(kernel.ErrPreconditionFailed, "relation basis is outside dataset")
		}
		return e.Serving.Contains(id, object)
	})
	out := retrieval.RelationPage{SearchView: retrieval.SearchView{Snapshots: pin.Repositories}, Hits: []retrieval.RelationHit{}}
	view := projectionViewDigest([]any{pin.Repositories, pin.Items}, e.Projection, pin.Repositories)
	query := retrieval.RelationQueryDigest(request.Query)
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for id := range pin.Repositories {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	members := make([]retrieval.MemberContinuation, len(ids))
	for i, id := range ids {
		members[i].Repository = id
	}
	if request.Continuation != "" {
		saved, err := retrieval.DecodeContinuation(request.Continuation)
		if err != nil || saved.Scope != "dataset-relations" || saved.Query != query || saved.SearchView != view || len(saved.Members) != len(members) {
			return retrieval.RelationPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "relation continuation does not match dataset")
		}
		for i, member := range saved.Members {
			if member.Repository != ids[i] {
				return retrieval.RelationPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "relation continuation membership changed")
			}
		}
		members = saved.Members
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 100
	}
	for i, id := range ids {
		if members[i].Exhausted {
			continue
		}
		if len(out.Hits) >= limit {
			break
		}
		repo, err := e.Repositories.Require(id, kernel.ErrCapabilityUnsatisfied)
		if err != nil {
			return retrieval.RelationPage{}, err
		}
		pageRequest := request
		pageRequest.Limit = limit - len(out.Hits)
		pageRequest.Continuation = members[i].Position
		page, err := e.Projection.RelationsAtContext(ctx, repo, pin.Repositories[id], pageRequest)
		if err != nil {
			return retrieval.RelationPage{}, err
		}
		out.Hits = append(out.Hits, page.Hits...)
		out.Claims = append(out.Claims, page.Claims...)
		members[i].Position = page.Continuation
		members[i].Exhausted = page.Exhausted
		if !page.Exhausted {
			break
		}
	}
	out.Exhausted = true
	for _, member := range members {
		if !member.Exhausted {
			out.Exhausted = false
		}
	}
	if !out.Exhausted {
		out.Continuation = retrieval.EncodeContinuation(retrieval.ContinuationState{Scope: "dataset-relations", Query: query, SearchView: view, Members: members})
	}
	return out, nil
}
