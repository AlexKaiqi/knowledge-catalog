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

// readyDigestAt reports the access digest of a projection that is already READY
// at this exact basis, using metadata only.
//
// The basis commit is immutable and the access contract is a pure function of
// that commit plus this instance's declaration manifest, so a projection that
// is READY at the basis and still matches the running physical configuration
// (which includes the manifest digest and provider revision) already carries
// the contract this caller would compute. Recomputing it would resolve one
// address per referencing declaration; on a large repository that is tens of
// thousands of reads to answer a question the metadata already settled.
//
// A projection that is ahead, behind, not READY, or built against a different
// physical configuration still returns false and takes the full path.
func (idx *Index) readyDigestAt(eng ports.Engine, meta ports.Meta, commit kernel.CommitID) (kernel.Digest, bool) {
	if meta.Basis != commit || meta.AccessDigest == "" {
		return "", false
	}
	// physicalMatches rejects every state other than READY (and accepts an
	// unset state for metadata written before the field existed), so this
	// single check carries the readiness predicate as well.
	if !idx.physicalMatches(eng, meta) {
		return "", false
	}
	return meta.AccessDigest, true
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
