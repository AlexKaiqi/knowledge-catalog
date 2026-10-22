package projection

import (
	"context"

	ports "kc/discovery/projection"
	"kc/kernel"
	"kc/object/reader"
)

// RepairDatasetPublication is maintenance only. It rebuilds from the complete
// fixed reader scope and never relies on whichever corpus pointer is active.
// accept must recheck the published revision before allowing the binding CAS.
func (idx *Index) RepairDatasetPublication(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, accept func(func() error) error) error {
	s, err := idx.openDatasetSpace(lookup, pin)
	if err != nil {
		return err
	}
	engine, err := idx.engineAt(s.id, s.basis)
	if err != nil {
		return err
	}
	repairer, managed := engine.(ports.PublicationRepairer)
	if !managed {
		return nil
	}
	idx.mu.Lock()
	evidenceFor := idx.publicationEvidence
	idx.mu.Unlock()
	if evidenceFor == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset repair requires its accepted publication authority")
	}
	evidence, err := evidenceFor(ctx, pin.SetID, pin.Revision)
	if err != nil {
		return err
	}
	if len(evidence) == 0 {
		return nil
	} // Legacy preparation adoption is a separate migration.
	check, _, err := idx.datasetCheck(s)
	if err != nil {
		return err
	}
	spec, _, err := idx.prepareDatasetSpec(ctx, s, check)
	if err != nil {
		return err
	}
	desired := idx.projectionMeta(engine, s.basis, spec, ports.IndexModeRebuild, "repair")
	return repairer.RepairPublication(ctx, evidence, desired, func(ctx context.Context) (ports.Meta, error) {
		result, err := idx.prepareDataset(ctx, lookup, pin, true)
		if err != nil {
			return ports.Meta{}, err
		}
		meta, err := loadMetaContext(ctx, engine)
		if err != nil {
			return ports.Meta{}, err
		}
		if !idx.projectionMatches(engine, meta, s.basis, result.AccessDigest) {
			return ports.Meta{}, kernel.Fail(kernel.ErrPreconditionFailed, "repaired Dataset corpus changed before resource protection")
		}
		return meta, nil
	}, accept)
}
