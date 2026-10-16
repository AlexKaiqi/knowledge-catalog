package projection

import (
	"kc/discovery/space"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// DatasetMember is the read-only declaration namespace of the fixed revision.
// Creating it does not open an engine or prepare a projection.
func (idx *Index) DatasetMember(lookup reader.MemberLookup, pin reader.DatasetPin, id kernel.RepositoryID) (objectmodel.Repository, error) {
	return space.Member(lookup, pin, id)
}
