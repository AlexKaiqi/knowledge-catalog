// Package discovery bridges a LiveAccess runtime to observation-backed discovery.
// Source access stays here; the projection consumes already verified values.
package discovery

import (
	"context"
	"kc/kernel"
	"kc/liveaccess"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
)

// Source binds an explicitly supplied runtime and execution identity. A declaration
// alone does not install this bridge or grant source access.
type Source struct {
	Lookup  liveaccess.StateLookup
	Request objectmodel.RequestContext
}

// ObserveDatasetValue resolves interpretation dependencies at the versions
// frozen with this Dataset revision. They do not become indexable members.
func (s Source) ObserveDatasetValue(ctx context.Context, scope *reader.Serving, repo objectmodel.Repository, value objectmodel.ObjectValue) (objectmodel.ObservedValue, error) {
	if s.Lookup == nil {
		return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dynamic Dataset projection requires a runtime")
	}
	descriptors := liveaccess.ServingDescriptors(scope)
	shared := make([]objectmodel.UnitDeclaration, 0, len(value.Declarations))
	for _, unit := range value.Declarations {
		if !unit.ValueSource.IsBinding() {
			shared = append(shared, unit)
			continue
		}
		binding, err := liveaccess.ResolveDeclaration(descriptors, value.Repository, value.Commit, unit)
		if err != nil {
			return objectmodel.ObservedValue{}, err
		}
		// A per-caller value stays out of the shared index. The rest of the
		// object is still indexed; the live unit is read on demand.
		if binding.Identity != objectmodel.IdentityShared {
			continue
		}
		shared = append(shared, unit)
	}
	value.Declarations = shared
	return liveservice.HydrateRepositoryValue(ctx, repo, value, s.Lookup, s.Request, descriptors)
}

func (s Source) ObserveRepositoryValue(ctx context.Context, repo objectmodel.Repository, value objectmodel.ObjectValue) (objectmodel.ObservedValue, error) {
	if s.Lookup == nil {
		return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dynamic State projection requires a Materialization Runtime")
	}
	// Dynamic projection resolves descriptorRef inside the projected
	// Repository commit only; the Dataset-frozen scope is debt for stage 4c.
	return liveservice.HydrateRepositoryValue(ctx, repo, value, s.Lookup, s.Request, liveaccess.RepositoryDescriptors(repo, value.Commit))
}
