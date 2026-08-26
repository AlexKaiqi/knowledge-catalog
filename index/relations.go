package index

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
)

func (idx *Index) Relations(repo knowledge.Repository, query reader.RelationQuery) ([]reader.RelationHit, error) {
	eng, err := idx.engine(repo.ID())
	if err != nil {
		return nil, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return nil, err
	}
	if meta.Basis == "" {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "projection for %s is empty; write or index-sync first", repo.ID())
	}
	return idx.relationsEngine(repo, eng, meta.Basis, query)
}

func (idx *Index) RelationsAt(repo knowledge.Repository, commit kernel.CommitID, query reader.RelationQuery) ([]reader.RelationHit, error) {
	if commit == "" {
		return idx.Relations(repo, query)
	}
	if _, err := idx.EnsureAt(repo, commit); err != nil {
		return nil, err
	}
	eng, err := idx.engineForCommit(repo.ID(), commit)
	if err != nil {
		return nil, err
	}
	return idx.relationsEngine(repo, eng, commit, query)
}

func (idx *Index) relationsEngine(repo knowledge.Repository, eng Engine, commit kernel.CommitID, query reader.RelationQuery) ([]reader.RelationHit, error) {
	if query.Endpoint == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "relation lookup requires an endpoint object_id")
	}
	retriever, ok := eng.(RelationRetriever)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection provider does not implement relation lookup")
	}
	continuation := ""
	hits := []reader.RelationHit{}
	for {
		page, err := retriever.RetrieveRelations(RelationRetrieveRequest{Query: query, Limit: 500, Continuation: continuation})
		if err != nil {
			return nil, err
		}
		for _, candidate := range page.Candidates {
			if candidate.Basis != commit {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "relation candidate basis does not match projection view")
			}
			value, err := repo.Read(candidate.ObjectID, commit)
			if err != nil {
				return nil, err
			}
			address := knowledge.Address{Kind: knowledge.KindRelation, ObjectID: candidate.ObjectID}
			relation, err := knowledge.DecodeRelation(address, value.Value)
			if err != nil {
				return nil, err
			}
			roles := relationMatchedRoles(relation, query)
			if len(roles) == 0 {
				continue
			}
			hits = append(hits, reader.RelationHit{
				KnowledgeRef: knowledge.KnowledgeRef{Repository: repo.ID(), Object: candidate.ObjectID},
				Repository:   repo.ID(), Commit: commit, ObjectID: candidate.ObjectID,
				MatchedRoles: roles, Relation: relation,
			})
		}
		if page.Exhausted || page.Continuation == "" {
			break
		}
		continuation = page.Continuation
	}
	return hits, nil
}

func relationMatchedRoles(relation knowledge.CanonicalRelation, query reader.RelationQuery) []string {
	if query.RelationType != "" && relation.RelationType != query.RelationType {
		return nil
	}
	roles := []string{}
	for _, endpoint := range relation.Endpoints {
		if endpoint.ObjectRef != query.Endpoint || (query.Role != "" && endpoint.Role != query.Role) {
			continue
		}
		roles = append(roles, endpoint.Role)
	}
	return roles
}
