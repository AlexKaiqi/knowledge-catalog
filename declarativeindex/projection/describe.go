package projection

import (
	ports "kc/discovery/projection"

	"kc/kernel"
	objectmodel "kc/object"
)

func (idx *Index) Describe(repo objectmodel.Repository) (ports.IndexDescriptor, error) {
	return idx.describe(repo, "")
}

// DescribeAt reports the projection at a frozen commit.
func (idx *Index) DescribeAt(repo objectmodel.Repository, commit kernel.CommitID) (ports.IndexDescriptor, error) {
	if commit == "" {
		return idx.Describe(repo)
	}
	return idx.describe(repo, commit)
}

func (idx *Index) describe(repo objectmodel.Repository, commit kernel.CommitID) (ports.IndexDescriptor, error) {
	var (
		eng     ports.Engine
		err     error
		release = func() {}
	)
	if commit == "" {
		eng, err = idx.engine(authorityEngineID(repo))
	} else {
		eng, release, err = idx.acquireEngineForCommit(authorityEngineID(repo), commit)
	}
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
	head, err := repo.Head("")
	if err != nil {
		return ports.IndexDescriptor{}, err
	}
	desc := ports.IndexDescriptor{
		BasisRepository: repo.ID(), BasisCommit: meta.Basis, ObjectCount: count,
		HeadCommit: head, LagBehindHead: meta.Basis != "" && head != meta.Basis,
		AccessDigest: meta.AccessDigest, PhysicalDigest: meta.PhysicalDigest,
		ProviderRevision: meta.ProviderRevision, Generation: meta.Generation, State: meta.State,
		Coverage: meta.Coverage, Mode: meta.Mode, Cause: meta.Cause,
	}
	if meta.Basis != "" {
		spec, err := specForProjection(repo, meta)
		if err != nil {
			return ports.IndexDescriptor{}, err
		}
		desc.Schemas = spec.Schemas
		desc.Lanes = spec.QueryLanes()
		desc.Fields = spec.Fields
	}
	return desc, nil
}
