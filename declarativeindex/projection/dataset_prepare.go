package projection

import (
	"context"

	retrieval "kc/discovery"
	"kc/discovery/space"
	"kc/kernel"
	objectmodel "kc/object"
)

// An empty source can predate Object metadata. Only explicit preparation may
// prove the selected object set empty through its existing maintenance port;
// consumption keeps verifying the retained exact declaration witnesses.
func (idx *Index) prepareDatasetSpec(ctx context.Context, s datasetSpace) (retrieval.AccessSpec, []datasetValue, error) {
	at := s.pin.Repositories[s.member.ID()]
	spec, err := idx.specAtCommit(s.member, at)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		return spec, nil, err
	}
	values, nonempty, scanErr := s.valuesBySource(ctx, nil)
	if scanErr != nil {
		return retrieval.AccessSpec{}, nil, scanErr
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		repo, err := s.lookup(id)
		if err != nil || nonempty[id] {
			return repo, err
		}
		return &emptyDatasetDeclarationSource{Repository: repo, commit: s.pin.Repositories[id]}, nil
	}
	member, err := space.Member(lookup, s.pin, s.member.ID())
	if err != nil {
		return retrieval.AccessSpec{}, nil, err
	}
	spec, err = idx.specAtCommit(member, at)
	return spec, values, err
}

// This preparation-only view records facts proved by the selected maintenance
// scan, including Schema and declaration units. It is never a consumer source.
type emptyDatasetDeclarationSource struct {
	objectmodel.Repository
	commit kernel.CommitID
}

func (s *emptyDatasetDeclarationSource) SchemaObjectIDs(at kernel.CommitID) ([]objectmodel.ObjectID, error) {
	if at != s.commit {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside the proven empty Dataset source")
	}
	return nil, nil
}

func (s *emptyDatasetDeclarationSource) SchemaReferrerAddresses(_ objectmodel.ObjectID, at kernel.CommitID) ([]objectmodel.Address, error) {
	if at != s.commit {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside the proven empty Dataset source")
	}
	return nil, nil
}
