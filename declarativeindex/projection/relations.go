package projection

import (
	"context"
	ports "kc/discovery/projection"
	"sort"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// RequireRelationReadyAt is deliberately read-only. Consumer requests never
// build, catch up, or otherwise mutate a projection.
func (idx *Index) RequireRelationReadyAt(repository kernel.RepositoryID, commit kernel.CommitID) (retrieval.RelationRetriever, ports.Meta, error) {
	engine, err := idx.engineForCommit(repository, commit)
	if err != nil {
		return nil, ports.Meta{}, err
	}
	return idx.requireRelationReady(engine, repository, commit)
}

func (idx *Index) requireRelationReady(engine ports.Engine, repository kernel.RepositoryID, commit kernel.CommitID) (retrieval.RelationRetriever, ports.Meta, error) {
	return idx.requireRelationReadyContext(context.Background(), engine, repository, commit)
}

func (idx *Index) requireRelationReadyContext(ctx context.Context, engine ports.Engine, repository kernel.RepositoryID, commit kernel.CommitID) (retrieval.RelationRetriever, ports.Meta, error) {
	meta, err := loadMetaContext(ctx, engine)
	if err != nil {
		return nil, ports.Meta{}, err
	}
	if meta.State == ports.ProjectionStateBuilding || meta.State == ports.ProjectionStateUpdating {
		return nil, ports.Meta{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "relation projection for %s is being built", repository)
	}
	if meta.State != ports.ProjectionStateReady {
		return nil, ports.Meta{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "relation projection for %s is not available", repository)
	}
	if meta.Basis != commit {
		return nil, ports.Meta{}, kernel.Fail(kernel.ErrPreconditionFailed,
			"relation projection basis %s does not match fixed commit %s", meta.Basis, commit)
	}
	if !idx.physicalMatches(engine, meta) {
		return nil, ports.Meta{}, kernel.Fail(kernel.ErrPreconditionFailed, "relation projection physical identity does not match its provider and manifest")
	}
	retriever, ok := engine.(retrieval.RelationRetriever)
	if !ok {
		return nil, ports.Meta{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection provider does not implement relation retrieval")
	}
	return retriever, meta, nil
}

// RelationsAt is the sole relation execution lane: exact-basis candidates are
// returned by layer ③, then complete relation objects are read from authority
// at that same commit and rechecked before exposure.
func (idx *Index) RelationsAt(repo objectmodel.Repository, commit kernel.CommitID, request retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	return idx.RelationsAtContext(context.Background(), repo, commit, request)
}

func (idx *Index) RelationsAtContext(ctx context.Context, repo objectmodel.Repository, commit kernel.CommitID, request retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	if scoped, ok := repo.(interface {
		DatasetScope() (reader.MemberLookup, reader.DatasetPin)
	}); ok {
		lookup, pin := scoped.DatasetScope()
		return idx.RelationsDataset(ctx, lookup, pin, request)
	}
	ctx, cancel := ports.WithSearchBudget(ctx, ports.SearchBudget{})
	defer cancel()
	if err := objectmodel.ValidateObjectID(request.Query.Endpoint); err != nil {
		return retrieval.RelationPage{}, kernel.Fail(kernel.ErrUsageInvalid, "relation lookup endpoint: %v", err)
	}
	if _, err := relationLimit(request.Limit); err != nil {
		return retrieval.RelationPage{}, err
	}
	engine, release, err := idx.acquireEngineForCommitContext(ctx, authorityEngineID(repo), commit)
	if err != nil {
		return retrieval.RelationPage{}, searchPreparationError(ctx, err)
	}
	defer release()
	retriever, meta, err := idx.requireRelationReadyContext(ctx, engine, repo.ID(), commit)
	if err != nil {
		return retrieval.RelationPage{}, searchPreparationError(ctx, err)
	}
	return relationsOnEngine(ctx, repo.ID(), commit, request, retriever, meta, relationExecutionScope{
		snapshots: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit},
		allow: func(ctx context.Context, candidate retrieval.RelationCandidate) (bool, error) {
			if candidate.Repository != repo.ID() || candidate.Basis != commit {
				return false, kernel.Fail(kernel.ErrPreconditionFailed, "relation candidate does not match repository and fixed basis")
			}
			return ports.CandidateScopeContains(ctx, repo.ID(), commit, candidate.ObjectID)
		},
		hydrate: func(ids []objectmodel.ObjectID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
			return idx.hydrateMany(repo, commit, ids)
		},
		hit: func(query retrieval.RelationQuery, value objectmodel.ObjectValue, evidence []retrieval.LaneEvidence) (retrieval.RelationHit, bool, error) {
			return relationHitAt(repo.ID(), commit, query, value, evidence)
		},
	})
}

type relationExecutionScope struct {
	snapshots map[kernel.RepositoryID]kernel.CommitID
	allow     func(context.Context, retrieval.RelationCandidate) (bool, error)
	hydrate   func([]objectmodel.ObjectID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error)
	hit       func(retrieval.RelationQuery, objectmodel.ObjectValue, []retrieval.LaneEvidence) (retrieval.RelationHit, bool, error)
}

func relationsOnEngine(ctx context.Context, repository kernel.RepositoryID, commit kernel.CommitID, request retrieval.RelationPageRequest, retriever retrieval.RelationRetriever, meta ports.Meta, scope relationExecutionScope) (retrieval.RelationPage, error) {
	limit, err := relationLimit(request.Limit)
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	out := retrieval.RelationPage{
		SearchView: retrieval.SearchView{Snapshots: scope.snapshots},
		Hits:       []retrieval.RelationHit{}, Generation: relationGeneration(meta),
	}
	sealer, err := relationSealer(ctx, retriever, request.Continuation != "")
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	nextPosition := ""
	emitContinuation := false
	continuation := ""
	if request.Continuation != "" {
		continuation, err = decodeRelationContinuation(sealer, request.Continuation, repository, commit, request.Query, out.Generation)
		if err != nil {
			return retrieval.RelationPage{}, err
		}
	}
	seen := map[objectmodel.ObjectID]struct{}{}
	budgetStop := func(reason string) {
		out.Exhausted = false
		out.Claims = append(out.Claims, "relation execution budget exhausted: "+reason)
		nextPosition, emitContinuation = continuation, true
	}
	for len(out.Hits) < limit {
		reserved, finish, reason, err := ports.ReserveSearchPage(ctx, limit-len(out.Hits))
		if err != nil {
			return retrieval.RelationPage{}, err
		}
		if reason != "" {
			budgetStop(reason)
			break
		}
		providerReq := retrieval.RelationRetrieveRequest{
			Repository: repository, Basis: commit, Query: request.Query,
			Limit: reserved, Continuation: continuation,
		}
		var page retrieval.RelationCandidatePage
		var retrieveErr error
		if cancellable, ok := retriever.(retrieval.ContextRelationRetriever); ok {
			page, retrieveErr = cancellable.RetrieveRelationsContext(ctx, providerReq)
		} else {
			page, retrieveErr = retriever.RetrieveRelations(providerReq)
		}
		if retrieveErr != nil {
			finish(0)
		} else {
			finish(len(page.Candidates))
		}
		if reason, canceled := ports.SearchContextStatus(ctx); reason != "" {
			budgetStop(reason)
			break
		} else if canceled != nil {
			return retrieval.RelationPage{}, canceled
		}
		if len(page.Candidates) > reserved {
			return retrieval.RelationPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "relation provider exceeded candidate page limit")
		}
		hitsBeforePage := len(out.Hits)
		if retrieveErr != nil {
			return retrieval.RelationPage{}, retrieveErr
		}
		if !page.Exhausted && (page.Continuation == "" || page.Continuation == continuation) {
			return retrieval.RelationPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "relation retriever returned a non-advancing continuation")
		}
		ids := make([]objectmodel.ObjectID, 0, len(page.Candidates))
		candidates := make(map[objectmodel.ObjectID]retrieval.RelationCandidate, len(page.Candidates))
		for _, candidate := range page.Candidates {
			allowed, err := scope.allow(ctx, candidate)
			if err != nil {
				return retrieval.RelationPage{}, err
			}
			if !allowed {
				continue
			}
			if _, duplicate := seen[candidate.ObjectID]; duplicate {
				continue
			}
			seen[candidate.ObjectID] = struct{}{}
			ids = append(ids, candidate.ObjectID)
			candidates[candidate.ObjectID] = candidate
		}
		if len(ids) > 0 {
			values, readErr := scope.hydrate(ids)
			if readErr != nil {
				return retrieval.RelationPage{}, readErr
			}
			for _, id := range ids {
				value, exists := values[id]
				if !exists {
					return retrieval.RelationPage{}, kernel.Fail(kernel.ErrPreconditionFailed,
						"relation projection candidate %s is missing from canonical basis %s", id, commit)
				}
				hit, matched, checkErr := scope.hit(request.Query, value, candidates[id].Evidence)
				if checkErr != nil {
					return retrieval.RelationPage{}, checkErr
				}
				if matched {
					out.Hits = append(out.Hits, hit)
				} else {
					out.Claims = append(out.Claims,
						"projection consistency: candidate "+string(id)+" failed canonical relation predicate")
				}
			}
		}
		if reason, canceled := ports.SearchContextStatus(ctx); reason != "" {
			out.Hits = out.Hits[:hitsBeforePage]
			budgetStop(reason)
			break
		} else if canceled != nil {
			return retrieval.RelationPage{}, canceled
		}
		continuation = page.Continuation
		if page.Exhausted || len(out.Hits) >= limit {
			out.Exhausted = page.Exhausted
			if !page.Exhausted && page.Continuation != "" {
				nextPosition, emitContinuation = page.Continuation, true
			}
			break
		}
	}
	if emitContinuation {
		out.Continuation = encodeRelationContinuation(sealer, relationContinuation{Repository: repository, Basis: commit, Query: retrieval.RelationQueryDigest(request.Query), Generation: out.Generation, Position: nextPosition})
	}
	sort.Slice(out.Hits, func(i, j int) bool {
		left, right := out.Hits[i], out.Hits[j]
		if left.Repository != right.Repository {
			return left.Repository < right.Repository
		}
		if left.ObjectID != right.ObjectID {
			return left.ObjectID < right.ObjectID
		}
		return rolesKey(left.MatchedRoles) < rolesKey(right.MatchedRoles)
	})
	return out, nil
}

func relationLimit(value int) (int, error) {
	if value < 0 || value > 1000 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "relation limit must be between 1 and 1000")
	}
	if value == 0 {
		return 100, nil
	}
	return value, nil
}

func relationGeneration(meta ports.Meta) string {
	if meta.Generation != "" {
		return meta.Generation
	}
	return string(kernel.CanonicalDigest(map[string]any{
		"basis": meta.Basis, "providerRevision": meta.ProviderRevision, "physicalDigest": meta.PhysicalDigest,
	}))
}

func relationHitAt(repository kernel.RepositoryID, commit kernel.CommitID, query retrieval.RelationQuery, value objectmodel.ObjectValue, evidence []retrieval.LaneEvidence) (retrieval.RelationHit, bool, error) {
	address, ok := relationAddress(value)
	if !ok {
		return retrieval.RelationHit{}, false, nil
	}
	relation, err := objectmodel.DecodeRelation(address, value.Value)
	if err != nil {
		return retrieval.RelationHit{}, false, err
	}
	if query.RelationType != "" && relation.RelationType != query.RelationType {
		return retrieval.RelationHit{}, false, nil
	}
	if query.Direction != "" && relation.Direction != query.Direction {
		return retrieval.RelationHit{}, false, nil
	}
	roles := make([]string, 0, len(relation.Endpoints))
	for _, endpoint := range relation.Endpoints {
		if endpoint.Object == query.Endpoint && (query.Role == "" || endpoint.Role == query.Role) {
			roles = append(roles, endpoint.Role)
		}
	}
	if len(roles) == 0 {
		return retrieval.RelationHit{}, false, nil
	}
	sort.Strings(roles)
	return retrieval.RelationHit{
		ObjectRef:  objectmodel.ObjectRef{Repository: repository, Object: address.ObjectID},
		Repository: repository, Commit: commit, ObjectID: address.ObjectID,
		MatchedRoles: roles, Relation: relation, Evidence: evidence,
	}, true, nil
}

func relationAddress(value objectmodel.ObjectValue) (objectmodel.Address, bool) {
	if value.Address.Kind == objectmodel.KindRelation {
		return value.Address, true
	}
	for _, declaration := range value.Declarations {
		if declaration.Address.Kind == objectmodel.KindRelation {
			return declaration.Address, true
		}
	}
	return objectmodel.Address{}, false
}

func rolesKey(roles []string) string {
	var out string
	for _, role := range roles {
		out += "\x00" + role
	}
	return out
}
