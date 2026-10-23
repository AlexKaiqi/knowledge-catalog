package projection

import (
	"context"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/discovery/space"
	"kc/kernel"
	objectmodel "kc/object"
)

// An empty source can predate Object metadata. Only explicit preparation may
// prove the selected object set empty through its existing maintenance port;
// consumption keeps verifying the retained exact declaration witnesses.
func (idx *Index) prepareDatasetSpec(ctx context.Context, s datasetSpace, check retrieval.DeclarationCheck) (retrieval.AccessSpec, []datasetValue, int, error) {
	at := s.pin.Repositories[s.member.ID()]
	spec, err := retrieval.AccessSpecForRepositoryChecked(s.member, at, check)
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		return spec, nil, 0, err
	}
	values, nonempty, skipped, scanErr := s.valuesBySource(ctx, nil)
	if scanErr != nil {
		return retrieval.AccessSpec{}, nil, 0, scanErr
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
		return retrieval.AccessSpec{}, nil, 0, err
	}
	spec, err = retrieval.AccessSpecForRepositoryChecked(member, at, check)
	return spec, values, skipped, err
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

// Publication protection and query consumption validate the same fixed
// declaration witnesses; neither borrows a READY flag as interpretation proof.
func (idx *Index) validateDatasetInterpretation(s datasetSpace, eng ports.Engine, meta ports.Meta) (datasetSpace, error) {
	check, units, err := idx.datasetCheck(s)
	if err != nil {
		return s, err
	}
	s.unobserved = units
	s.spec, err = retrieval.AccessSpecFromAdoptionsChecked(s.member, s.pin.Repositories[s.member.ID()], meta.SchemaAdoptions, check)
	if err != nil {
		return s, err
	}
	if meta.AccessDigest != s.spec.AccessDigest {
		return s, kernel.Fail(kernel.ErrPreconditionFailed, "Dataset search interpretation changed (declaration basis); prepare the revision again")
	}
	if !idx.physicalMatches(eng, meta) {
		return s, kernel.Fail(kernel.ErrPreconditionFailed, "Dataset search interpretation changed (physical configuration); prepare the revision again")
	}
	return s, nil
}
