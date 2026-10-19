package service

import (
	"context"

	"kc/kernel"
	objectmodel "kc/object"
)

// A Dataset may publish some complete units without publishing a Canonical
// whole object. Plan those coordinates inside its Member boundary, then use
// the same logical unit reads as explicit references. Reader's whole-object
// reads and caches retain their complete-object precondition.
func (s *Service) readPartialReference(ctx context.Context, ref objectmodel.Reference, rawReference string) ([]objectmodel.ObservedValue, error) {
	member, err := s.declarations.Member(ref.Repository)
	if err != nil {
		return nil, err
	}
	planner, ok := member.(objectmodel.ReferencePlanner)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset source cannot plan selected object units")
	}
	commit := s.declarations.Pin().Repositories[ref.Repository]
	addresses, err := planner.PlanReference(ref, commit)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "reference has no selected object units")
	}
	units := make([]objectmodel.Unit, 0, len(addresses))
	result := objectmodel.ObservedValue{Observations: []objectmodel.UnitObservation{}}
	for _, address := range addresses {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ref.Locator = objectmodel.FormatPointer(unitPointerTokens(address))
		if ref.Locator == "" {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "partial object plan contains a whole-object unit")
		}
		selected, err := s.ReadReference(ctx, ref)
		if err != nil {
			return nil, err
		}
		if len(selected) != 1 || len(selected[0].Declarations) != 1 || selected[0].Address != address || selected[0].Declarations[0].Address != address {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "logical reference returned different selected object units")
		}
		observed := selected[0]
		declaration := observed.Declarations[0]
		units = append(units, objectmodel.Unit{ObjectID: ref.Object, Address: address, SchemaRef: declaration.SchemaRef, ValueSource: declaration.ValueSource, Provenance: declaration.Provenance, Value: observed.Value, Digest: declaration.Digest})
		result.Declarations = append(result.Declarations, declaration)
		result.Observations = append(result.Observations, observed.Observations...)
		result.Failures = append(result.Failures, observed.Failures...)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, err := objectmodel.AssembleObjectValue(ref.Repository, ref.Object, commit, units)
	if err != nil {
		return nil, err
	}
	// The root is a collection of selected units, not a Canonical maintenance
	// value. Keep each original unit declaration rather than inventing a root
	// digest or replacing an adopted binding declaration with a static one.
	result.FederatedValue = objectmodel.FederatedValue{Reference: rawReference, ObjectRef: value.ObjectRef, Repository: value.Repository, Commit: value.Commit, ObjectID: ref.Object, Address: value.Address, Value: value.Value, Provenance: value.Provenance, Units: value.Units, Declarations: result.Declarations}
	return []objectmodel.ObservedValue{result}, nil
}
