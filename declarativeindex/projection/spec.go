package projection

import (
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

func specAtCommit(repo objectmodel.Repository, commit kernel.CommitID) (retrieval.AccessSpec, error) {
	return retrieval.AccessSpecForRepository(repo, commit)
}

func specForProjection(repo objectmodel.Repository, meta ports.Meta) (retrieval.AccessSpec, error) {
	return retrieval.AccessSpecFromAdoptions(repo, meta.Basis, meta.SchemaAdoptions)
}

// AccessSpecAt inspects declared query contracts, including independently
// verified adoption witnesses retained by explicit projection maintenance.
// It neither catches up a projection nor scans the authority for instances.
func (idx *Index) AccessSpecAt(repo objectmodel.Repository, commit kernel.CommitID) (retrieval.AccessSpec, error) {
	eng, release, err := idx.acquireEngineForCommit(authorityEngineID(repo), commit)
	if err != nil {
		return retrieval.AccessSpec{}, err
	}
	defer release()
	meta, err := eng.LoadMeta()
	if err != nil {
		return retrieval.AccessSpec{}, err
	}
	if meta.Basis != commit || meta.State != ports.ProjectionStateReady || !physicalMatches(eng, meta) {
		return retrieval.AccessSpecFromAdoptions(repo, commit, nil)
	}
	return specForProjection(repo, meta)
}
