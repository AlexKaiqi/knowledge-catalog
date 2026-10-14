package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

func (r *Reader) PlanReference(ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Address, error) {
	repo, err := r.repoByID(ref.Repository)
	if err != nil {
		return nil, err
	}
	return planRepositoryReference(repo, ref, commit)
}

func planRepositoryReference(repo objectmodel.Repository, ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Address, error) {
	if _, err := objectmodel.FormatReference(ref); err != nil {
		return nil, err
	}
	if ref.Repository != repo.ID() || commit == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "reference planning requires the source repository and fixed commit")
	}
	planner, ok := repo.(objectmodel.ReferencePlanner)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository has no metadata-only reference planner")
	}
	addresses, err := planner.PlanReference(ref, commit)
	if err != nil {
		return nil, err
	}
	return validateReferencePlan(ref, addresses)
}

func validateReferencePlan(ref objectmodel.Reference, addresses []objectmodel.Address) ([]objectmodel.Address, error) {
	tokens, err := objectmodel.PointerTokens(ref.Locator)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(addresses))
	units := make([]objectmodel.Unit, 0, len(addresses))
	for _, address := range addresses {
		key := objectmodel.AddressKey(address)
		prefix := unitTokens(address)
		if address.ObjectID != ref.Object || objectmodel.AssertWritable(address) != nil || seen[key] || !tokenPrefix(prefix, tokens) && !tokenPrefix(tokens, prefix) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "reference planner returned invalid maintenance coordinates")
		}
		seen[key] = true
		units = append(units, objectmodel.Unit{ObjectID: ref.Object, Address: address})
	}
	if _, err := objectmodel.AssembleUnits(units); err != nil {
		return nil, err
	}
	return append([]objectmodel.Address(nil), addresses...), nil
}

func (r *treeRepository) PlanReference(ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Address, error) {
	if _, err := objectmodel.FormatReference(ref); err != nil {
		return nil, err
	}
	if ref.Repository != r.ID() || commit == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "reference planning requires the source repository and fixed commit")
	}
	tokens, err := objectmodel.PointerTokens(ref.Locator)
	if err != nil {
		return nil, err
	}
	locations, err := repofile.ReadObjectUnitLocations(r.tree, ref.Object, commit)
	if err != nil {
		return nil, err
	}
	addresses := make([]objectmodel.Address, 0, len(locations))
	for _, location := range locations {
		prefix := unitTokens(location.Address)
		if tokenPrefix(prefix, tokens) || tokenPrefix(tokens, prefix) {
			addresses = append(addresses, location.Address)
		}
	}
	return validateReferencePlan(ref, addresses)
}

func (r *nativeReadRepository) PlanReference(ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Address, error) {
	return planRepositoryReference(r.readRepository, ref, commit)
}

func (r *datasetRepository) PlanReference(ref objectmodel.Reference, commit kernel.CommitID) ([]objectmodel.Address, error) {
	if ref.Repository != r.ID() || commit != r.commit {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "reference planning is outside Dataset source basis")
	}
	// Metadata describes coordinates only. The actual read still requires all
	// selected body paths to be present in this accepted publication.
	return planRepositoryReference(r.Repository, ref, commit)
}
