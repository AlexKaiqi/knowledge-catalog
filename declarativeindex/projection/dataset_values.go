package projection

import (
	"context"
	"sort"

	ports "kc/discovery/projection"
	"kc/discovery/space"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/maintenance"
	"kc/object/reader"
)

type datasetValue struct {
	value        objectmodel.ObjectValue
	observations []objectmodel.UnitObservation
}

func (s datasetSpace) values(ctx context.Context, source ports.ObservationSource) ([]datasetValue, error) {
	values, _, err := s.valuesBySource(ctx, source)
	return values, err
}

func (s datasetSpace) valuesBySource(ctx context.Context, source ports.ObservationSource) ([]datasetValue, map[kernel.RepositoryID]bool, error) {
	parts := map[objectmodel.ObjectID][]objectmodel.ObjectValue{}
	observations := map[objectmodel.ObjectID][]objectmodel.UnitObservation{}
	nonempty := map[kernel.RepositoryID]bool{}
	for id, at := range s.pin.Repositories {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		member, err := space.Member(s.lookup, s.pin, id)
		if err != nil {
			return nil, nil, err
		}
		member = objectmodel.BindContext(ctx, member)
		if err := maintenance.WalkRepository(member, at, func(raw objectmodel.ObjectValue) error {
			nonempty[id] = true
			if objectmodel.IsSchemaObject(raw.Address.ObjectID) {
				return nil
			}
			value := raw
			if source != nil {
				var observed objectmodel.ObservedValue
				var err error
				stripped, changed := s.withoutUnobserved(id, raw)
				if datasetSource, ok := source.(interface {
					ObserveDatasetValue(context.Context, *reader.Serving, objectmodel.Repository, objectmodel.ObjectValue) (objectmodel.ObservedValue, error)
				}); ok {
					observed, err = datasetSource.ObserveDatasetValue(ctx, s.serving, member, stripped)
				} else {
					observed, err = source.ObserveRepositoryValue(ctx, member, stripped)
				}
				if err != nil {
					return err
				}
				if changed {
					observed.Declarations = restoreDeclarations(raw.Declarations, observed.Declarations)
				}
				value = objectmodel.ObjectValue{ObjectRef: observed.ObjectRef, Repository: observed.Repository, Commit: observed.Commit, Address: observed.Address, Value: observed.Value, Provenance: observed.Provenance, Units: observed.Units, Declarations: observed.Declarations}
				if err := objectmodel.ValidateHydratedObject(id, at, raw.Address.ObjectID, value); err != nil {
					return err
				}
				observations[raw.Address.ObjectID] = append(observations[raw.Address.ObjectID], observed.Observations...)
			}
			parts[raw.Address.ObjectID] = append(parts[raw.Address.ObjectID], value)
			return ctx.Err()
		}); err != nil {
			return nil, nil, err
		}
	}
	ids := make([]objectmodel.ObjectID, 0, len(parts))
	for id := range parts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]datasetValue, 0, len(ids))
	for _, id := range ids {
		values := parts[id]
		sort.Slice(values, func(i, j int) bool { return values[i].Repository < values[j].Repository })
		value, err := objectmodel.ComposeHoldingValues(string(id), values)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, datasetValue{value: value, observations: observations[id]})
	}
	return out, nonempty, nil
}
