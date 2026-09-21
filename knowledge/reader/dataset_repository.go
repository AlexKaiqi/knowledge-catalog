package reader

import (
	"kc/kernel"
	"kc/knowledge"
)

// datasetRepository keeps secondary reads (Schema, Binding, Descriptor) under
// the same scope as the primary object. Embedding must not expose a raw tree.
type datasetRepository struct {
	knowledge.Repository
	serving *Serving
	commit  kernel.CommitID
}

func (r *datasetRepository) check(id knowledge.ObjectID, at kernel.CommitID) error {
	if at != r.commit {
		return kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "commit is outside dataset release")
	}
	ok, err := r.serving.objectInDataset(r.ID(), at, r.Repository, id)
	if err != nil {
		return err
	}
	if !ok {
		return kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "object is outside dataset file scope")
	}
	return nil
}

func (r *datasetRepository) Read(id knowledge.ObjectID, at kernel.CommitID) (knowledge.KnowledgeValue, error) {
	if err := r.check(id, at); err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	return r.Repository.Read(id, at)
}
func (r *datasetRepository) ReadAddress(addr knowledge.Address, at kernel.CommitID) (knowledge.KnowledgeValue, error) {
	if err := r.check(addr.ObjectID, at); err != nil {
		if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
			return knowledge.KnowledgeValue{}, err
		}
		paths, scoped, scopeErr := r.addressPaths(addr.ObjectID, at)
		if scopeErr != nil {
			return knowledge.KnowledgeValue{}, scopeErr
		}
		return scoped.ReadAddressAtPaths(addr, at, paths)
	}
	return r.Repository.ReadAddress(addr, at)
}
func (r *datasetRepository) Resolve(id knowledge.ObjectID, at kernel.CommitID) (knowledge.Resolution, error) {
	if err := r.check(id, at); err != nil {
		return knowledge.Resolution{}, err
	}
	return r.Repository.Resolve(id, at)
}
func (r *datasetRepository) ResolveAddress(addr knowledge.Address, at kernel.CommitID) (knowledge.Resolution, error) {
	if err := r.check(addr.ObjectID, at); err != nil {
		if kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved {
			return knowledge.Resolution{}, err
		}
		paths, scoped, scopeErr := r.addressPaths(addr.ObjectID, at)
		if scopeErr != nil {
			return knowledge.Resolution{}, scopeErr
		}
		return scoped.ResolveAddressAtPaths(addr, at, paths)
	}
	return r.Repository.ResolveAddress(addr, at)
}

func (r *datasetRepository) addressPaths(id knowledge.ObjectID, at kernel.CommitID) ([]string, knowledge.AddressPathsReader, error) {
	if at != r.commit {
		return nil, nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "commit is outside dataset release")
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
		return nil, nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "address is outside dataset file scope")
	}
	scoped, ok := r.Repository.(knowledge.AddressPathsReader)
	if !ok {
		return nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository cannot read an exact unit within selected paths")
	}
	return allowed, scoped, nil
}
func (r *datasetRepository) GetProvenance(id knowledge.ObjectID, at kernel.CommitID) (knowledge.ProvenanceTrace, error) {
	if err := r.check(id, at); err != nil {
		return knowledge.ProvenanceTrace{}, err
	}
	return r.Repository.GetProvenance(id, at)
}
func (r *datasetRepository) Log(id knowledge.ObjectID, at kernel.CommitID, q knowledge.ObjectLogQuery) ([]knowledge.ObjectRevision, error) {
	if err := r.check(id, at); err != nil {
		return nil, err
	}
	if datasetRestrictsRepository(r.serving.pin.Items, r.ID()) {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset file scope does not publish repository history")
	}
	return r.Repository.Log(id, at, q)
}
func (r *datasetRepository) Diff(id knowledge.ObjectID, from, to kernel.CommitID) (knowledge.ObjectDiff, error) {
	if err := r.check(id, from); err != nil {
		return knowledge.ObjectDiff{}, err
	}
	if err := r.check(id, to); err != nil {
		return knowledge.ObjectDiff{}, err
	}
	return r.Repository.Diff(id, from, to)
}

func (r *datasetRepository) ObjectUnitPaths(id knowledge.ObjectID, at kernel.CommitID) ([]string, error) {
	locator, ok := r.Repository.(knowledge.UnitLocator)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository has no unit locator")
	}
	return locator.ObjectUnitPaths(id, at)
}

func (r *datasetRepository) filterIDs(ids []knowledge.ObjectID, at kernel.CommitID) ([]knowledge.ObjectID, error) {
	if at != r.commit {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "commit is outside dataset release")
	}
	out := make([]knowledge.ObjectID, 0, len(ids))
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
func (r *datasetRepository) SchemaObjectIDs(at kernel.CommitID) ([]knowledge.ObjectID, error) {
	locator, ok := r.Repository.(knowledge.SchemaStore)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository has no schema locator")
	}
	ids, err := locator.SchemaObjectIDs(at)
	if err != nil {
		return nil, err
	}
	return r.filterIDs(ids, at)
}
func (r *datasetRepository) BindingSchemaObjectIDs(at kernel.CommitID) ([]knowledge.ObjectID, error) {
	// Some native binding locators classify schemas by reading their bodies.
	// Discover from the scoped namespace instead of filtering that raw result.
	schemas, err := listBoundSchemas(r, at)
	if err != nil {
		return nil, err
	}
	ids := make([]knowledge.ObjectID, 0, len(schemas))
	for _, schema := range schemas {
		ids = append(ids, schema.ObjectID)
	}
	return ids, nil
}
func (r *datasetRepository) ReadMany(ids []knowledge.ObjectID, at kernel.CommitID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	allowed, err := r.filterIDs(ids, at)
	if err != nil {
		return nil, err
	}
	if batch, ok := r.Repository.(knowledge.BatchReadStore); ok {
		return batch.ReadMany(allowed, at)
	}
	out := map[knowledge.ObjectID]knowledge.KnowledgeValue{}
	for _, id := range allowed {
		value, err := r.Repository.Read(id, at)
		if err != nil {
			return nil, err
		}
		out[id] = value
	}
	return out, nil
}

func (r *datasetRepository) CreateRef(string, kernel.CommitID) error {
	return kernel.Fail(kernel.ErrForbidden, "dataset is read-only")
}
func (r *datasetRepository) Merge(string, kernel.CommitID, kernel.CommitID) (kernel.CommitID, error) {
	return "", kernel.Fail(kernel.ErrForbidden, "dataset is read-only")
}
func (r *datasetRepository) Archive() error {
	return kernel.Fail(kernel.ErrForbidden, "dataset is read-only")
}
