package reader

import (
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

// RelationQuery is a pinned, one-hop lookup. Multi-hop traversal remains an
// upper-layer bounded operation.
type RelationQuery struct {
	Endpoint     knowledge.ObjectID `json:"endpoint"`
	RelationType string             `json:"relationType,omitempty"`
	Role         string             `json:"role,omitempty"`
}

// RelationHit always carries the Canonical Relation read from the same commit
// used to find it. Implementations may replace the reference scan with a
// projection, but the returned body remains authoritative Snapshot knowledge.
type RelationHit struct {
	KnowledgeRef knowledge.KnowledgeRef      `json:"knowledgeRef"`
	Repository   kernel.RepositoryID         `json:"repository"`
	Commit       kernel.CommitID             `json:"commit"`
	ObjectID     knowledge.ObjectID          `json:"objectId"`
	MatchedRoles []string                    `json:"matchedRoles"`
	Relation     knowledge.CanonicalRelation `json:"relation"`
}

func (r *Reader) Relations(repositoryID kernel.RepositoryID, commit kernel.CommitID, query RelationQuery) ([]RelationHit, error) {
	repo, err := r.repoByID(repositoryID)
	if err != nil {
		return nil, err
	}
	return relationsAt(repo, repositoryID, commit, query)
}

func relationsAt(repo knowledge.Repository, repositoryID kernel.RepositoryID, commit kernel.CommitID, query RelationQuery) ([]RelationHit, error) {
	if query.Endpoint == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "relation lookup requires an endpoint object_id")
	}
	if locator, ok := repo.(interface {
		LocateRelationObjectIDs(kernel.CommitID, knowledge.ObjectID, string, string) ([]knowledge.ObjectID, error)
	}); ok {
		ids, err := locator.LocateRelationObjectIDs(commit, query.Endpoint, query.RelationType, query.Role)
		if err != nil {
			return nil, err
		}
		return hydrateRelationIDs(repo, repositoryID, commit, query, ids)
	}
	out := []RelationHit{}
	err := knowledge.WalkPages(repo, commit, func(value knowledge.KnowledgeValue) error {
		address, ok := relationAddress(value)
		if !ok {
			return nil
		}
		relation, err := knowledge.DecodeRelation(address, value.Value)
		if err != nil {
			return err
		}
		if query.RelationType != "" && relation.RelationType != query.RelationType {
			return nil
		}
		roles := []string{}
		for _, endpoint := range relation.Endpoints {
			if endpoint.ObjectRef != query.Endpoint {
				continue
			}
			if query.Role != "" && endpoint.Role != query.Role {
				continue
			}
			roles = append(roles, endpoint.Role)
		}
		if len(roles) == 0 {
			return nil
		}
		out = append(out, RelationHit{
			KnowledgeRef: knowledge.KnowledgeRef{Repository: repositoryID, Object: address.ObjectID},
			Repository:   repositoryID, Commit: commit, ObjectID: address.ObjectID,
			MatchedRoles: roles, Relation: relation,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortRelationHits(out)
	return out, nil
}

func hydrateRelationIDs(repo knowledge.Repository, repositoryID kernel.RepositoryID, commit kernel.CommitID, query RelationQuery, ids []knowledge.ObjectID) ([]RelationHit, error) {
	values := map[knowledge.ObjectID]knowledge.KnowledgeValue{}
	if batch, ok := repo.(knowledge.BatchReadStore); ok {
		var err error
		values, err = batch.ReadMany(ids, commit)
		if err != nil {
			return nil, err
		}
	} else {
		for _, id := range ids {
			value, err := repo.Read(id, commit)
			if err != nil {
				return nil, err
			}
			values[id] = value
		}
	}
	out := []RelationHit{}
	for _, id := range ids {
		value, ok := values[id]
		if !ok {
			continue
		}
		address, ok := relationAddress(value)
		if !ok {
			continue
		}
		relation, err := knowledge.DecodeRelation(address, value.Value)
		if err != nil {
			return nil, err
		}
		roles := []string{}
		for _, endpoint := range relation.Endpoints {
			if endpoint.ObjectRef == query.Endpoint && (query.Role == "" || endpoint.Role == query.Role) {
				roles = append(roles, endpoint.Role)
			}
		}
		if len(roles) > 0 {
			out = append(out, RelationHit{
				KnowledgeRef: knowledge.KnowledgeRef{Repository: repositoryID, Object: id},
				Repository:   repositoryID, Commit: commit, ObjectID: id, MatchedRoles: roles, Relation: relation,
			})
		}
	}
	sortRelationHits(out)
	return out, nil
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

func sortRelationHits(hits []RelationHit) {
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			left := string(hits[i].Repository) + "\x00" + string(hits[i].ObjectID) + "\x00" + strings.Join(hits[i].MatchedRoles, "\x00")
			right := string(hits[j].Repository) + "\x00" + string(hits[j].ObjectID) + "\x00" + strings.Join(hits[j].MatchedRoles, "\x00")
			if right < left {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
}
