package projection

import (
	"context"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/object/reader"
)

// DatasetAccessSpec returns the declarations retained by the prepared space.
// Like search it verifies exact member addresses, rather than discovering any.
func (idx *Index) DatasetAccessSpec(lookup reader.MemberLookup, pin reader.DatasetPin) (retrieval.AccessSpec, error) {
	s, _, release, err := idx.preparedDataset(context.Background(), lookup, pin)
	if err != nil {
		return retrieval.AccessSpec{}, err
	}
	defer release()
	return s.spec, nil
}

// CheckDatasetProjection observes the same readiness gate search uses.
func (idx *Index) CheckDatasetProjection(lookup reader.MemberLookup, pin reader.DatasetPin) (ports.Meta, error) {
	_, eng, release, err := idx.preparedDataset(context.Background(), lookup, pin)
	if err != nil {
		return ports.Meta{}, err
	}
	defer release()
	return eng.LoadMeta()
}

// DescribeDataset reports only the frozen retrieval space, without HEAD lag.
func (idx *Index) DescribeDataset(lookup reader.MemberLookup, pin reader.DatasetPin) (ports.IndexDescriptor, error) {
	s, eng, release, err := idx.preparedDataset(context.Background(), lookup, pin)
	if err != nil {
		return ports.IndexDescriptor{}, err
	}
	defer release()
	meta, err := eng.LoadMeta()
	if err != nil {
		return ports.IndexDescriptor{}, err
	}
	count, err := eng.Count()
	if err != nil {
		return ports.IndexDescriptor{}, err
	}
	return ports.IndexDescriptor{BasisRepository: s.id, BasisCommit: meta.Basis, ObjectCount: count, AccessDigest: meta.AccessDigest, PhysicalDigest: meta.PhysicalDigest, ProviderRevision: meta.ProviderRevision, Generation: meta.Generation, State: meta.State, Coverage: meta.Coverage, Mode: meta.Mode, Cause: meta.Cause, Schemas: s.spec.Schemas, Lanes: s.spec.QueryLanes(), Fields: s.spec.Fields}, nil
}
