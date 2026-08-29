package index

import (
	"sort"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

// RequireRelationReadyAt is deliberately read-only. Consumer requests never
// build, catch up, or otherwise mutate a projection.
func (idx *Index) RequireRelationReadyAt(repository kernel.RepositoryID, commit kernel.CommitID) (retrieval.RelationRetriever, Meta, error) {
	engine, err := idx.engineForCommit(repository, commit)
	if err != nil {
		return nil, Meta{}, err
	}
	return requireRelationReady(engine, repository, commit)
}

func requireRelationReady(engine Engine, repository kernel.RepositoryID, commit kernel.CommitID) (retrieval.RelationRetriever, Meta, error) {
	meta, err := engine.LoadMeta()
	if err != nil {
		return nil, Meta{}, err
	}
	if meta.State == ProjectionStateBuilding || meta.State == ProjectionStateUpdating {
		return nil, Meta{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "relation projection for %s is being built", repository)
	}
	if meta.State != ProjectionStateReady {
		return nil, Meta{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "relation projection for %s is not available", repository)
	}
	if meta.Basis != commit {
		return nil, Meta{}, kernel.Fail(kernel.ErrPreconditionFailed,
			"relation projection basis %s does not match fixed commit %s", meta.Basis, commit)
	}
	if identity, ok := engine.(ProviderIdentity); ok {
		if meta.ProviderRevision != identity.ProviderRevision() || meta.PhysicalDigest != identity.PhysicalDigest() {
			return nil, Meta{}, kernel.Fail(kernel.ErrPreconditionFailed,
				"relation projection physical identity does not match its provider")
		}
	}
	retriever, ok := engine.(retrieval.RelationRetriever)
	if !ok {
		return nil, Meta{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection provider does not implement relation retrieval")
	}
	return retriever, meta, nil
}

// RelationsAt is the sole relation execution lane: exact-basis candidates are
// returned by layer ③, then complete relation objects are read from authority
// at that same commit and rechecked before exposure.
func (idx *Index) RelationsAt(repo knowledge.Repository, commit kernel.CommitID, request retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	if request.Query.Endpoint.Repository == "" || request.Query.Endpoint.Object == "" {
		return retrieval.RelationPage{}, kernel.Fail(kernel.ErrUsageInvalid, "relation lookup requires a repository-qualified endpoint")
	}
	if request.Query.Endpoint.Repository != repo.ID() {
		return retrieval.RelationPage{}, kernel.Fail(kernel.ErrUsageInvalid, "relation endpoints may only reference their own repository")
	}
	limit, err := relationLimit(request.Limit)
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	engine, release, err := idx.acquireEngineForCommit(repo.ID(), commit)
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	defer release()
	retriever, meta, err := requireRelationReady(engine, repo.ID(), commit)
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	out := retrieval.RelationPage{Hits: []retrieval.RelationHit{}, Generation: relationGeneration(meta)}
	continuation := ""
	if request.Continuation != "" {
		continuation, err = decodeRelationContinuation(request.Continuation, repo.ID(), commit, request.Query, out.Generation)
		if err != nil {
			return retrieval.RelationPage{}, err
		}
	}
	seen := map[knowledge.ObjectID]struct{}{}
	for len(out.Hits) < limit {
		page, retrieveErr := retriever.RetrieveRelations(retrieval.RelationRetrieveRequest{
			Repository: repo.ID(), Basis: commit, Query: request.Query,
			Limit: limit - len(out.Hits), Continuation: continuation,
		})
		if retrieveErr != nil {
			return retrieval.RelationPage{}, retrieveErr
		}
		if !page.Exhausted && (page.Continuation == "" || page.Continuation == continuation) {
			return retrieval.RelationPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "relation retriever returned a non-advancing continuation")
		}
		ids := make([]knowledge.ObjectID, 0, len(page.Candidates))
		candidates := make(map[knowledge.ObjectID]retrieval.RelationCandidate, len(page.Candidates))
		for _, candidate := range page.Candidates {
			if candidate.Repository != repo.ID() || candidate.Basis != commit {
				return retrieval.RelationPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "relation candidate does not match repository and fixed basis")
			}
			if _, duplicate := seen[candidate.ObjectID]; duplicate {
				continue
			}
			seen[candidate.ObjectID] = struct{}{}
			ids = append(ids, candidate.ObjectID)
			candidates[candidate.ObjectID] = candidate
		}
		if len(ids) > 0 {
			values, readErr := hydrateMany(repo, commit, ids)
			if readErr != nil {
				return retrieval.RelationPage{}, readErr
			}
			for _, id := range ids {
				value, exists := values[id]
				if !exists {
					return retrieval.RelationPage{}, kernel.Fail(kernel.ErrPreconditionFailed,
						"relation projection candidate %s is missing from canonical basis %s", id, commit)
				}
				hit, matched, checkErr := relationHitAt(repo.ID(), commit, request.Query, value, candidates[id].Evidence)
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
		continuation = page.Continuation
		if page.Exhausted || len(out.Hits) >= limit {
			out.Exhausted = page.Exhausted
			if !page.Exhausted && page.Continuation != "" {
				out.Continuation = encodeRelationContinuation(relationContinuation{
					Repository: repo.ID(), Basis: commit, Query: retrieval.RelationQueryDigest(request.Query),
					Generation: out.Generation, Position: page.Continuation,
				})
			}
			break
		}
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

func relationGeneration(meta Meta) string {
	if meta.Generation != "" {
		return meta.Generation
	}
	return string(kernel.CanonicalDigest(map[string]any{
		"basis": meta.Basis, "providerRevision": meta.ProviderRevision, "physicalDigest": meta.PhysicalDigest,
	}))
}

func relationHitAt(repository kernel.RepositoryID, commit kernel.CommitID, query retrieval.RelationQuery, value knowledge.KnowledgeValue, evidence []retrieval.LaneEvidence) (retrieval.RelationHit, bool, error) {
	address, ok := relationAddress(value)
	if !ok {
		return retrieval.RelationHit{}, false, nil
	}
	relation, err := knowledge.DecodeRelation(address, value.Value)
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
		if endpoint.ObjectRef == query.Endpoint && (query.Role == "" || endpoint.Role == query.Role) {
			roles = append(roles, endpoint.Role)
		}
	}
	if len(roles) == 0 {
		return retrieval.RelationHit{}, false, nil
	}
	sort.Strings(roles)
	return retrieval.RelationHit{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: repository, Object: address.ObjectID},
		Repository:   repository, Commit: commit, ObjectID: address.ObjectID,
		MatchedRoles: roles, Relation: relation, Evidence: evidence,
	}, true, nil
}

func relationAddress(value knowledge.KnowledgeValue) (knowledge.Address, bool) {
	if value.Address.Kind == knowledge.KindRelation {
		return value.Address, true
	}
	for _, declaration := range value.Declarations {
		if declaration.Address.Kind == knowledge.KindRelation {
			return declaration.Address, true
		}
	}
	return knowledge.Address{}, false
}

func rolesKey(roles []string) string {
	var out string
	for _, role := range roles {
		out += "\x00" + role
	}
	return out
}
