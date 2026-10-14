package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

// ReadReferenceUnits returns whole selected maintenance values; a dynamic leaf
// may be absent from these fixed declarations until LiveAccess hydrates them.
func (r *Reader) ReadReferenceUnits(ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Unit, error) {
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return nil, err
	}
	return readRepositoryReferenceUnits(repo, ref, commit)
}

func readRepositoryReferenceUnits(repo objectmodel.Repository, ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Unit, error) {
	if _, err := objectmodel.FormatReference(ref); err != nil {
		return nil, err
	}
	if repo.ID() != ref.Repository {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "reference source does not match repository")
	}
	port, ok := repo.(objectmodel.ReferenceUnitsReader)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository cannot provide selected reference units")
	}
	units, err := port.ReadReferenceUnits(ref, commit)
	if err != nil {
		return nil, err
	}
	if len(units) == 0 {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "reference has no selected maintenance units")
	}
	tokens, _ := objectmodel.PointerTokens(ref.Locator)
	seen := map[string]struct{}{}
	for _, unit := range units {
		if unit.ObjectID != ref.Object || unit.Address.ObjectID != ref.Object {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "selected reference unit changed object identity")
		}
		if err := objectmodel.AssertWritable(unit.Address); err != nil {
			return nil, err
		}
		prefix := unitTokens(unit.Address)
		if !tokenPrefix(prefix, tokens) && !tokenPrefix(tokens, prefix) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "repository returned an unselected reference unit")
		}
		key := objectmodel.AddressKey(unit.Address)
		if _, ok := seen[key]; ok {
			return nil, kernel.Fail(kernel.ErrObjectIDConflict, "duplicate selected reference unit %s", key)
		}
		seen[key] = struct{}{}
		if unit.Digest != kernel.CanonicalDigest(unit.Value) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "selected reference unit %s has an invalid complete value digest", key)
		}
	}
	if _, err := objectmodel.AssembleUnits(units); err != nil {
		return nil, err
	}
	return units, nil
}

func (r *treeRepository) ReadReferenceUnits(ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Unit, error) {
	if ref.Repository != r.ID() {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "reference source does not match repository")
	}
	if _, err := objectmodel.FormatReference(ref); err != nil {
		return nil, err
	}
	if ref.Locator == "" {
		units, err := r.objectUnits(ref.Object, commit)
		if err != nil {
			return nil, err
		}
		if len(units) == 0 {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object has no maintenance content")
		}
		return repofile.ToObjectUnits(units), nil
	}
	return r.ReadReferenceUnitsAtPaths(ref, commit, nil)
}

func (r *treeRepository) ReadReferenceUnitsAtPaths(ref objectmodel.Reference, commit kernel.CommitID, paths []string) ([]objectmodel.Unit, error) {
	units, err := r.selectedReferenceUnits(ref, commit, paths)
	if err != nil {
		return nil, err
	}
	return repofile.ToObjectUnits(units), nil
}

type referenceUnitsPathsReader interface {
	ReadReferenceUnitsAtPaths(objectmodel.Reference, kernel.CommitID, []string) ([]objectmodel.Unit, error)
}

func (r *datasetRepository) ReadReferenceUnits(ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Unit, error) {
	if ref.Repository != r.ID() || commit != r.commit {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "reference is outside dataset source basis")
	}
	if ref.Locator == "" {
		if err := r.check(ref.Object, commit); err != nil {
			return nil, err
		}
		return readRepositoryReferenceUnits(r.Repository, ref, commit)
	}
	if !datasetRestrictsRepository(r.serving.pin.Items, r.ID()) && datasetHasRepository(r.serving.pin.Items, r.ID()) {
		return readRepositoryReferenceUnits(r.Repository, ref, commit)
	}
	port, ok := r.Repository.(referenceUnitsPathsReader)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository cannot provide selected units within dataset file paths")
	}
	paths, err := r.referenceAllowedPaths(ref.Object, commit)
	if err != nil {
		return nil, err
	}
	return port.ReadReferenceUnitsAtPaths(ref, commit, paths)
}

func (r *nativeReadRepository) ReadReferenceUnits(ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Unit, error) {
	return readRepositoryReferenceUnits(r.readRepository, ref, commit)
}
