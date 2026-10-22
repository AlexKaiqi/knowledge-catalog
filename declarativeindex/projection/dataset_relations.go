package projection

import (
	"context"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// RelationsDataset uses the same prepared corpus as Dataset search. Its
// continuation fixes that corpus generation; hits retain the relation's source.
func (idx *Index) RelationsDataset(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, request retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	if err := ctx.Err(); err != nil {
		return retrieval.RelationPage{}, err
	}
	if err := objectmodel.ValidateObjectID(request.Query.Endpoint); err != nil {
		return retrieval.RelationPage{}, kernel.Fail(kernel.ErrUsageInvalid, "relation lookup endpoint: %v", err)
	}
	if _, err := relationLimit(request.Limit); err != nil {
		return retrieval.RelationPage{}, err
	}
	ctx, cancel := ports.WithSearchBudget(ctx, ports.SearchBudget{})
	defer cancel()
	s, engine, release, err := idx.preparedDataset(ctx, lookup, pin, datasetContinuation{relationToken: request.Continuation})
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	defer release()
	retriever, meta, err := idx.requireRelationReadyContext(ctx, engine, s.id, s.basis)
	if err != nil {
		return retrieval.RelationPage{}, err
	}
	return relationsOnEngine(ctx, s.id, s.basis, request, retriever, meta, relationExecutionScope{
		snapshots: pin.Repositories,
		allow: func(_ context.Context, candidate retrieval.RelationCandidate) (bool, error) {
			_, err := s.filterCandidates(ports.CandidatePage{Candidates: []ports.CandidateRef{{Repository: candidate.Repository, Basis: candidate.Basis, ObjectID: candidate.ObjectID}}})
			return err == nil, err
		},
		hydrate: func(ids []objectmodel.ObjectID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
			values := make(map[objectmodel.ObjectID]objectmodel.ObjectValue, len(ids))
			for _, id := range ids {
				value, err := s.read(id)
				if err != nil {
					return nil, kernel.Fail(kernel.ErrPreconditionFailed, "Dataset relation candidate %s cannot be read at its fixed revision: %v", id, err)
				}
				values[id] = value
			}
			return values, nil
		},
		hit: func(query retrieval.RelationQuery, value objectmodel.ObjectValue, evidence []retrieval.LaneEvidence) (retrieval.RelationHit, bool, error) {
			if value.Repository == "" || pin.Repositories[value.Repository] != value.Commit || value.Commit == "" {
				return retrieval.RelationHit{}, false, kernel.Fail(kernel.ErrPreconditionFailed, "relation has no exact source coordinate in the Dataset revision")
			}
			return relationHitAt(value.Repository, value.Commit, query, value, evidence)
		},
	})
}
