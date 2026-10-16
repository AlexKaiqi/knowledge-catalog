package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

// datasetRepository keeps secondary reads (Schema, Binding, Descriptor) under
// the same scope as the primary object. Embedding must not expose a raw tree.
type datasetRepository struct {
	objectmodel.Repository
	serving *Serving
	commit  kernel.CommitID
}

// StoreDigest forwards the optional authority-instance identity: interface
// embedding does not forward type assertions to new optional capabilities, so
// without this the dataset-scoped view would look unidentified and its
// retrieval projections would be keyed differently from the same repository's
// primary view.
func (r *datasetRepository) StoreDigest() kernel.Digest {
	if identified, ok := r.Repository.(objectmodel.StoreIdentified); ok {
		return identified.StoreDigest()
	}
	return ""
}

func (r *datasetRepository) check(id objectmodel.ObjectID, at kernel.CommitID) error {
	if at != r.commit {
		return kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside dataset release")
	}
	ok, err := r.serving.objectInDataset(r.ID(), at, r.Repository, id)
	if err != nil {
		return err
	}
	if !ok {
		return kernel.Fail(kernel.ErrObjectRefUnresolved, "object is outside dataset file scope")
	}
	return nil
}

func (r *datasetRepository) Read(id objectmodel.ObjectID, at kernel.CommitID) (objectmodel.ObjectValue, error) {
	if err := r.check(id, at); err != nil {
		return objectmodel.ObjectValue{}, err
	}
	return r.Repository.Read(id, at)
}
func (r *datasetRepository) ReadAddress(addr objectmodel.Address, at kernel.CommitID) (objectmodel.ObjectValue, error) {
	if err := r.check(addr.ObjectID, at); err != nil {
		if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			return objectmodel.ObjectValue{}, err
		}
		paths, scoped, scopeErr := r.addressPaths(addr.ObjectID, at)
		if scopeErr != nil {
			return objectmodel.ObjectValue{}, scopeErr
		}
		return scoped.ReadAddressAtPaths(addr, at, paths)
	}
	return r.Repository.ReadAddress(addr, at)
}
func (r *datasetRepository) Resolve(id objectmodel.ObjectID, at kernel.CommitID) (objectmodel.Resolution, error) {
	if err := r.check(id, at); err != nil {
		return objectmodel.Resolution{}, err
	}
	return r.Repository.Resolve(id, at)
}
func (r *datasetRepository) ResolveAddress(addr objectmodel.Address, at kernel.CommitID) (objectmodel.Resolution, error) {
	if err := r.check(addr.ObjectID, at); err != nil {
		if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			return objectmodel.Resolution{}, err
		}
		paths, scoped, scopeErr := r.addressPaths(addr.ObjectID, at)
		if scopeErr != nil {
			return objectmodel.Resolution{}, scopeErr
		}
		return scoped.ResolveAddressAtPaths(addr, at, paths)
	}
	return r.Repository.ResolveAddress(addr, at)
}

func (r *datasetRepository) addressPaths(id objectmodel.ObjectID, at kernel.CommitID) ([]string, objectmodel.AddressPathsReader, error) {
	if at != r.commit {
		return nil, nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside dataset release")
	}
	paths, err := r.ObjectUnitPaths(id, at)
	if err != nil {
		return nil, nil, err
	}
	allowed := make([]string, 0, len(paths))
	for _, path := range paths {
		if datasetPathAllowed(r.serving.pin.Items, r.ID(), path) {
			allowed = append(allowed, path)
		}
	}
	if len(allowed) == 0 {
		return nil, nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "address is outside dataset file scope")
	}
	scoped, ok := r.Repository.(objectmodel.AddressPathsReader)
	if !ok {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository cannot read an exact unit within selected paths")
	}
	return allowed, scoped, nil
}
func (r *datasetRepository) GetProvenance(id objectmodel.ObjectID, at kernel.CommitID) (objectmodel.ProvenanceTrace, error) {
	if err := r.check(id, at); err != nil {
		return objectmodel.ProvenanceTrace{}, err
	}
	return r.Repository.GetProvenance(id, at)
}
func (r *datasetRepository) Log(id objectmodel.ObjectID, at kernel.CommitID, q objectmodel.ObjectLogQuery) ([]objectmodel.ObjectRevision, error) {
	if err := r.check(id, at); err != nil {
		return nil, err
	}
	if datasetRestrictsRepository(r.serving.pin.Items, r.ID()) {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset file scope does not publish repository history")
	}
	return r.Repository.Log(id, at, q)
}
func (r *datasetRepository) Diff(id objectmodel.ObjectID, from, to kernel.CommitID) (objectmodel.ObjectDiff, error) {
	if err := r.check(id, from); err != nil {
		return objectmodel.ObjectDiff{}, err
	}
	if err := r.check(id, to); err != nil {
		return objectmodel.ObjectDiff{}, err
	}
	return r.Repository.Diff(id, from, to)
}

func (r *datasetRepository) ObjectUnitPaths(id objectmodel.ObjectID, at kernel.CommitID) ([]string, error) {
	locator, ok := r.Repository.(objectmodel.UnitLocator)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository has no unit locator")
	}
	return locator.ObjectUnitPaths(id, at)
}

func (r *datasetRepository) filterIDs(ids []objectmodel.ObjectID, at kernel.CommitID) ([]objectmodel.ObjectID, error) {
	if at != r.commit {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside dataset release")
	}
	out := make([]objectmodel.ObjectID, 0, len(ids))
	for _, id := range ids {
		ok, err := r.serving.objectInDataset(r.ID(), at, r.Repository, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, id)
		}
	}
	return out, nil
}
func (r *datasetRepository) SchemaObjectIDs(at kernel.CommitID) ([]objectmodel.ObjectID, error) {
	locator, ok := r.Repository.(objectmodel.SchemaStore)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository has no schema locator")
	}
	ids, err := locator.SchemaObjectIDs(at)
	if err != nil {
		return nil, err
	}
	return r.filterIDs(ids, at)
}

func (r *datasetRepository) BindingSchemaObjectIDs(at kernel.CommitID) ([]objectmodel.ObjectID, error) {
	// Only intersect fixed namespace coordinates. Classification and access
	// declaration interpretation belong to the consuming capability.
	allowed, err := r.SchemaObjectIDs(at)
	if err != nil || len(allowed) == 0 {
		return allowed, err
	}
	coordinates, err := r.selectedBindingCoordinates(at)
	if err != nil {
		return nil, err
	}
	ids := repofile.BindingSchemas(coordinates)
	scoped := make(map[objectmodel.ObjectID]bool, len(allowed))
	for _, id := range allowed {
		scoped[id] = true
	}
	out := make([]objectmodel.ObjectID, 0, len(ids))
	for _, id := range ids {
		if scoped[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// This private port marks Reader-owned metadata access. An arbitrary native
// BindingLocator may classify bodies, so it cannot safely serve a file slice.
type bindingCoordinatesReader interface {
	bindingUnitCoordinates(kernel.CommitID) ([]repofile.BindingCoordinate, error)
}

// selectedBindingCoordinates intersects the binding namespace with the exact
// units selected by the fixed Dataset. Planning uses maintenance metadata;
// sibling bodies and external descriptors are never read to classify a unit.
func (r *datasetRepository) selectedBindingCoordinates(at kernel.CommitID) ([]repofile.BindingCoordinate, error) {
	if at != r.commit {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "binding coordinates are outside Dataset source basis")
	}
	locator, ok := r.Repository.(bindingCoordinatesReader)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no metadata-only binding namespace coordinates", r.ID())
	}
	coordinates, err := locator.bindingUnitCoordinates(at)
	if err != nil {
		return nil, err
	}
	selected := map[objectmodel.ObjectID]map[string]bool{}
	out := make([]repofile.BindingCoordinate, 0, len(coordinates))
	for _, coordinate := range coordinates {
		units, planned := selected[coordinate.Address.ObjectID]
		if !planned {
			units, err = r.selectedBindingUnits(coordinate.Address.ObjectID, at)
			if err != nil {
				return nil, err
			}
			selected[coordinate.Address.ObjectID] = units
		}
		if units == nil || units[objectmodel.AddressKey(coordinate.Address)] {
			out = append(out, coordinate)
		}
	}
	return out, nil
}

// Nil means the complete object's paths are selected. That proof does not
// require newer per-unit locations; partial objects must provide those locations.
func (r *datasetRepository) selectedBindingUnits(id objectmodel.ObjectID, at kernel.CommitID) (map[string]bool, error) {
	if err := r.check(id, at); err == nil {
		return nil, nil
	} else if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		return nil, err
	}
	addresses, err := r.PlanReference(objectmodel.Reference{Repository: r.ID(), Object: id}, at)
	if err != nil {
		return nil, err
	}
	units := make(map[string]bool, len(addresses))
	for _, address := range addresses {
		units[objectmodel.AddressKey(address)] = true
	}
	return units, nil
}

// bindingReferences lists the selected binding units and their descriptorRefs.
func (r *datasetRepository) bindingReferences() ([]objectmodel.BindingReference, error) {
	coordinates, err := r.selectedBindingCoordinates(r.commit)
	if err != nil {
		return nil, err
	}
	return repofile.BindingReferences(coordinates)
}

func (r *datasetRepository) ReadMany(ids []objectmodel.ObjectID, at kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
	allowed, err := r.filterIDs(ids, at)
	if err != nil {
		return nil, err
	}
	if batch, ok := r.Repository.(objectmodel.BatchReadStore); ok {
		return batch.ReadMany(allowed, at)
	}
	out := map[objectmodel.ObjectID]objectmodel.ObjectValue{}
	for _, id := range allowed {
		value, err := r.Repository.Read(id, at)
		if err != nil {
			return nil, err
		}
		out[id] = value
	}
	return out, nil
}
