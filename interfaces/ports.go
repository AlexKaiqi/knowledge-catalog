package interfaces

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// DeclarationReader grants fixed, scope-preserving point reads. A handler
// cannot retrieve a raw repository, inspect HEAD, move refs or acquire writes.
type DeclarationReader interface {
	ReadObject(kernel.RepositoryID, objectmodel.ObjectID) (objectmodel.ObjectValue, error)
	ReadAddress(kernel.RepositoryID, objectmodel.Address) (objectmodel.ObjectValue, error)
}
