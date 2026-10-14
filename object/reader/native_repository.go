package reader

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// nativeReadRepository retains only knowledge read capabilities. Embedding a private read contract prevents write methods and the original
// provider from escaping through an exported field. Optional ports delegate to the provider or fail closed;
// they never reinterpret native values through the compatibility file tree.
type readRepository interface {
	objectmodel.Repository
}

type nativeReadRepository struct {
	readRepository
}

func (r *nativeReadRepository) ReadReference(ref objectmodel.Reference, at kernel.CommitID) (objectmodel.KnowledgeValue, error) {
	return readRepositoryReference(r.readRepository, ref, at)
}

func nativeReadPort[T any](repository objectmodel.Repository, name string) (T, error) {
	port, ok := repository.(T)
	if !ok {
		return port, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no %s read capability", repository.ID(), name)
	}
	return port, nil
}

func (r *nativeReadRepository) StoreDigest() kernel.Digest {
	if port, ok := r.readRepository.(objectmodel.StoreIdentified); ok {
		return port.StoreDigest()
	}
	return ""
}

func (r *nativeReadRepository) ReadMany(ids []objectmodel.ObjectID, at kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.KnowledgeValue, error) {
	port, err := nativeReadPort[objectmodel.BatchReadStore](r.readRepository, "batch")
	if err != nil {
		return nil, err
	}
	return port.ReadMany(ids, at)
}

func (r *nativeReadRepository) ObjectUnitPaths(id objectmodel.ObjectID, at kernel.CommitID) ([]string, error) {
	port, err := nativeReadPort[objectmodel.UnitLocator](r.readRepository, "unit locator")
	if err != nil {
		return nil, err
	}
	return port.ObjectUnitPaths(id, at)
}

func (r *nativeReadRepository) SchemaObjectIDs(at kernel.CommitID) ([]objectmodel.ObjectID, error) {
	port, err := nativeReadPort[objectmodel.SchemaStore](r.readRepository, "schema locator")
	if err != nil {
		return nil, err
	}
	return port.SchemaObjectIDs(at)
}

func (r *nativeReadRepository) BindingSchemaObjectIDs(at kernel.CommitID) ([]objectmodel.ObjectID, error) {
	port, err := nativeReadPort[objectmodel.BindingLocator](r.readRepository, "binding locator")
	if err != nil {
		return nil, err
	}
	return port.BindingSchemaObjectIDs(at)
}

func (r *nativeReadRepository) SchemaReferrerAddresses(schema objectmodel.ObjectID, at kernel.CommitID) ([]objectmodel.Address, error) {
	port, err := nativeReadPort[objectmodel.SchemaReferrerLocator](r.readRepository, "schema referrer locator")
	if err != nil {
		return nil, err
	}
	return port.SchemaReferrerAddresses(schema, at)
}

func (r *nativeReadRepository) ObjectIDsPage(at kernel.CommitID, limit int, continuation string) (objectmodel.ObjectIDPage, error) {
	port, err := nativeReadPort[objectmodel.SnapshotObjectPager](r.readRepository, "object identity page")
	if err != nil {
		return objectmodel.ObjectIDPage{}, err
	}
	return port.ObjectIDsPage(at, limit, continuation)
}

func (r *nativeReadRepository) FastChangedObjectIDs(from, to kernel.CommitID) ([]objectmodel.ObjectID, error) {
	port, err := nativeReadPort[objectmodel.FastChanges](r.readRepository, "changed identities")
	if err != nil {
		return nil, err
	}
	return port.FastChangedObjectIDs(from, to)
}

func (r *nativeReadRepository) FastChangedObjects(from, to kernel.CommitID) ([]objectmodel.ChangedObject, error) {
	port, err := nativeReadPort[objectmodel.FastObjectChanges](r.readRepository, "changed object paths")
	if err != nil {
		return nil, err
	}
	return port.FastChangedObjects(from, to)
}

func (r *nativeReadRepository) ReadManyAtPaths(ids []objectmodel.ObjectID, at kernel.CommitID, paths map[objectmodel.ObjectID][]string) (map[objectmodel.ObjectID]objectmodel.KnowledgeValue, error) {
	port, err := nativeReadPort[objectmodel.UnitPathsHydrator](r.readRepository, "unit path hydration")
	if err != nil {
		return nil, err
	}
	return port.ReadManyAtPaths(ids, at, paths)
}

func (r *nativeReadRepository) ReadKnowledgeFile(path string, at kernel.CommitID) ([]byte, error) {
	port, err := nativeReadPort[objectmodel.KnowledgeFileReader](r.readRepository, "knowledge file")
	if err != nil {
		return nil, err
	}
	return port.ReadKnowledgeFile(path, at)
}

func (r *nativeReadRepository) ReadAddressAtPaths(address objectmodel.Address, at kernel.CommitID, paths []string) (objectmodel.KnowledgeValue, error) {
	port, err := nativeReadPort[objectmodel.AddressPathsReader](r.readRepository, "scoped address")
	if err != nil {
		return objectmodel.KnowledgeValue{}, err
	}
	return port.ReadAddressAtPaths(address, at, paths)
}

func (r *nativeReadRepository) ResolveAddressAtPaths(address objectmodel.Address, at kernel.CommitID, paths []string) (objectmodel.Resolution, error) {
	port, err := nativeReadPort[objectmodel.AddressPathsReader](r.readRepository, "scoped address")
	if err != nil {
		return objectmodel.Resolution{}, err
	}
	return port.ResolveAddressAtPaths(address, at, paths)
}
