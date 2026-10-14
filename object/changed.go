package object

import "kc/kernel"

type FastChanges interface {
	FastChangedObjectIDs(from, to kernel.CommitID) ([]ObjectID, error)
}

// ChangedObject is one identity touched between two commits, with the unit
// paths observed on each side. Empty paths mean the object is absent there.
type ChangedObject struct {
	ObjectID  ObjectID
	FromPaths []string
	ToPaths   []string
}

// FastObjectChanges reports identities and the object files that changed.
// Tree authorities use this so a git commit that did not refresh locators is
// still indexed from the files themselves.
type FastObjectChanges interface {
	FastChangedObjects(from, to kernel.CommitID) ([]ChangedObject, error)
}

// UnitPathsHydrator reads objects using caller-supplied unit paths.
type UnitPathsHydrator interface {
	ReadManyAtPaths(objectIDs []ObjectID, commit kernel.CommitID, paths map[ObjectID][]string) (map[ObjectID]ObjectValue, error)
}

// AddressPathsReader reads one exact unit from an explicit set of candidate
// paths. It must not hydrate other paths, and must not assemble a partial object.
type AddressPathsReader interface {
	ReadAddressAtPaths(Address, kernel.CommitID, []string) (ObjectValue, error)
	ResolveAddressAtPaths(Address, kernel.CommitID, []string) (Resolution, error)
}

// ObjectFileReader is a bounded tree read for convention paths such as
// README.md and Schema files under _schemas/. It is not a Snapshot scan and
// not a Dataset service protocol type.
type ObjectFileReader interface {
	ReadObjectFile(rel string, commit kernel.CommitID) ([]byte, error)
}
