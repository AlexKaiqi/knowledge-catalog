package projection

import (
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

// SetDeclarationManifest installs this instance's capability manifest check.
// Every AccessSpec the instance compiles marks members outside the manifest
// unsupported; without a check only shape and field resolution apply.
func (idx *Index) SetDeclarationManifest(check retrieval.DeclarationCheck, digest kernel.Digest) {
	idx.check = check
	idx.manifestDigest = digest
}

func (idx *Index) specAtCommit(repo objectmodel.Repository, commit kernel.CommitID) (retrieval.AccessSpec, error) {
	return retrieval.AccessSpecForRepositoryChecked(repo, commit, idx.check)
}

func (idx *Index) specForProjection(repo objectmodel.Repository, meta ports.Meta) (retrieval.AccessSpec, error) {
	return retrieval.AccessSpecFromAdoptionsChecked(repo, meta.Basis, meta.SchemaAdoptions, idx.check)
}

// AccessSpecAt inspects declared query contracts, including independently
// verified adoption witnesses retained by explicit projection maintenance.
// It neither catches up a projection nor scans the authority for instances.
func (idx *Index) AccessSpecAt(repo objectmodel.Repository, commit kernel.CommitID) (retrieval.AccessSpec, error) {
	spec, _, err := idx.maintainedAccessSpecAt(repo, commit)
	return spec, err
}

// maintainedAccessSpecAt also reports whether a ready projection supplied the
// declaration witnesses. Without one the spec is structure only: declared
// faces are never located on the query side, so the result fails closed.
func (idx *Index) maintainedAccessSpecAt(repo objectmodel.Repository, commit kernel.CommitID) (retrieval.AccessSpec, bool, error) {
	eng, release, err := idx.acquireEngineForCommit(authorityEngineID(repo), commit)
	if err != nil {
		return retrieval.AccessSpec{}, false, err
	}
	defer release()
	meta, err := eng.LoadMeta()
	if err != nil {
		return retrieval.AccessSpec{}, false, err
	}
	if meta.Basis != commit || meta.State != ports.ProjectionStateReady || !idx.physicalMatches(eng, meta) {
		spec, err := retrieval.AccessSpecFromAdoptionsChecked(repo, commit, nil, idx.check)
		return spec, false, err
	}
	spec, err := idx.specForProjection(repo, meta)
	return spec, err == nil, err
}
