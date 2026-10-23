package projection

import (
	"context"

	ports "kc/discovery/projection"
	"kc/kernel"
	"kc/object/reader"
)

// DatasetReadiness answers whether one accepted Dataset revision is searchable
// without building anything. It reads the same metadata a search would check,
// so the answer and the query path cannot disagree.
func (idx *Index) DatasetReadiness(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) (ports.IndexReadiness, error) {
	result := ports.IndexReadiness{Dataset: pin.SetID, Revision: pin.Revision}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	s, err := idx.openDatasetSpace(lookup, pin)
	if err != nil {
		return result, err
	}
	eng, err := idx.engineAt(s.id, s.basis)
	if err != nil {
		return result, err
	}
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		return result, err
	}
	digest, ready := idx.readyDigestAt(eng, meta, s.basis)
	if !ready || digest == "" {
		return result, nil
	}
	count, err := eng.Count()
	if err != nil {
		return result, err
	}
	result.Ready, result.ObjectCount = true, count
	return result, nil
}

// DatasetDiagnosis explains a readiness result. It walks the selected members,
// which is why it is a separate call from DatasetReadiness.
func (idx *Index) DatasetDiagnosis(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) (ports.IndexDiagnosis, error) {
	result := ports.IndexDiagnosis{Dataset: pin.SetID, Revision: pin.Revision}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	s, err := idx.openDatasetSpace(lookup, pin)
	if err != nil {
		return result, err
	}
	eng, err := idx.engineAt(s.id, s.basis)
	if err != nil {
		return result, err
	}
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		return result, err
	}
	digest, ready := idx.readyDigestAt(eng, meta, s.basis)
	result.Ready = ready && digest != ""
	if result.Ready {
		// describe is the call that may walk the selected members, so the
		// declaration contract is compiled here rather than in readiness. Without
		// it the basis would report provider facts but no lanes, schemas or
		// fields, and a caller could not tell which query surface the ready
		// corpus actually answers (doc/design/affordance.md U7).
		check, _, err := idx.datasetCheck(s)
		if err != nil {
			return result, err
		}
		spec, _, _, err := idx.prepareDatasetSpec(ctx, s, check)
		if err != nil {
			return result, err
		}
		s.spec = spec
	}
	defaultStage := ports.IndexStageNotBuilt
	if result.Ready {
		defaultStage = ports.IndexStageReady
	} else {
		// meta.State is the provider's own word. Only the distinctions a caller
		// can act on survive: still working, stopped, or never started.
		switch meta.State {
		case ports.ProjectionStateBuilding, ports.ProjectionStateUpdating:
			defaultStage = ports.IndexStageBuilding
		case ports.ProjectionStateFailed:
			defaultStage = ports.IndexStageFailed
		case ports.ProjectionStateRetired:
			defaultStage = ports.IndexStageNotBuilt
		}
	}
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for id := range pin.Repositories {
		ids = append(ids, id)
	}
	sortRepositoryIDs(ids)
	count, err := eng.Count()
	if err != nil {
		return result, err
	}
	// A revision carries one corpus, so its document count is the whole
	// revision's and is not attributed to a single selected member.
	result.Basis = &ports.IndexBasisFacts{
		BasisCommit: s.basis, ObjectCount: count,
		AccessDigest:   meta.AccessDigest,
		PhysicalDigest: meta.PhysicalDigest, ProviderRevision: meta.ProviderRevision,
		Generation: meta.Generation, Coverage: meta.Coverage, Mode: meta.Mode,
		Cause: meta.Cause, Schemas: s.spec.Schemas, Lanes: s.spec.QueryLanes(),
		Fields: s.spec.Fields,
	}
	for _, id := range ids {
		result.Members = append(result.Members, ports.IndexMemberState{
			Repository: id, BasisCommit: pin.Repositories[id], Stage: defaultStage,
		})
	}
	return result, nil
}

func sortRepositoryIDs(ids []kernel.RepositoryID) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}
